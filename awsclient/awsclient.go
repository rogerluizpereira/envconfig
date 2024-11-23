// Este package abstrai a comunicação com a AWS, além de aspectos como:
// - Evitar consultas duplicadas ao mesmo secret
// - Evitar novas tentativas de utilização credenciais incorretas.
// - Evitar novas tentativas de utilização de uma sessão que não pode ser estabelecida
package awsclient

import (
	"fmt"
	"sync"

	"regexp"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
)

// Estrutura genérica para representar um item de cache
type CacheItem[T any] struct {
	item T
	err  error
}

// Estrutura responsável pela interação com a AWS.
type AWSClient struct {
	sessions      map[string]CacheItem[*session.Session]
	services      map[string]CacheItem[*secretsmanager.SecretsManager]
	secrets       map[string]CacheItem[string]
	sessionsMutex sync.Mutex
	servicesMutex sync.Mutex
	secretsMutex  sync.Mutex
	profile				string
	defaultRegion string
}

// Expressões regulares usadas para avaliar identificadores de
// secrets e outros itens necessários no contexto da AWS
var (
	arnRegex             = regexp.MustCompile(`^(arn:aws:secretsmanager:[a-z]{2}-[a-z]+-\d{1,3}:\d{12}:secret:[\w/_+=.@-]+)$`)
	nameRegex            = regexp.MustCompile(`^([\w/+=.@-]{1,512})$`)
	nameRestrictionRegex = regexp.MustCompile(`-[A-Za-z0-9]{6}$`)
	regionRegex          = regexp.MustCompile(`(?:secretsmanager:)([a-z]{2}-[a-z]+-\d{1,3})`)
)

// Cria uma nova instância de AWSClient.
func NewAWSClient(profile, defaultRegion string) *AWSClient {
	return &AWSClient{
		sessions: make(map[string]CacheItem[*session.Session]),
		services: make(map[string]CacheItem[*secretsmanager.SecretsManager]),
		secrets:  make(map[string]CacheItem[string]),
		profile: profile,
		defaultRegion: defaultRegion,
	}
}

// Retorna uma sessão com a AWS.
// Se a sessão foi criada anteriormente, retorna a referência existente, caso
// contrário cria uma nova sessão, armazena sua referência a retorna ao chamador.
func (client *AWSClient) getSession(region string) (*session.Session, error) {
	client.sessionsMutex.Lock()
	defer client.sessionsMutex.Unlock()

	if cacheItem, exists := client.sessions[region]; exists {
		return cacheItem.item, cacheItem.err
	}

	sessionOptions := session.Options{}
	if region == "" {
		sessionOptions = session.Options{
			Profile: client.profile,
			SharedConfigState: session.SharedConfigEnable,
		}	
	} else {
		sessionOptions = session.Options{
			Profile: client.profile,
			Config: aws.Config{
				Region: aws.String(region),
			},
		}
	}

	item, err := session.NewSessionWithOptions(sessionOptions)

	cacheItem := CacheItem[*session.Session]{
		item: item,
		err:  err,
	}

	if cacheItem.err != nil {
		cacheItem.err = fmt.Errorf("não foi possível criar sessão para a região %s. %w", region, err)
	}

	if _, err := item.Config.Credentials.Get(); err != nil {
		cacheItem.err = fmt.Errorf("perfil AWS '%s' não possui credenciais válidas, ou não está configurado corretamente. %w", client.profile, err)
	}

	client.sessions[region] = cacheItem
	return cacheItem.item, cacheItem.err
}

// Retorna um client para o serviço AWS.
// Se o client foi criado anteriormente, retorna a referência existente, caso
// contrário cria um novo client, armazena sua referência a retorna ao chamador.
func (client *AWSClient) getService(region string) (*secretsmanager.SecretsManager, error) {
	client.servicesMutex.Lock()
	defer client.servicesMutex.Unlock()

	if cacheItem, exists := client.services[region]; exists {
		return cacheItem.item, cacheItem.err
	}

	cacheItem := CacheItem[*secretsmanager.SecretsManager]{
		item: nil,
		err:  nil,
	}

	session, err := client.getSession(region)
	if err != nil {
		cacheItem.err = fmt.Errorf("não foi possível obter o client para o secrect manager. %w", err)
	} else {
		cacheItem.item = secretsmanager.New(session)
	}

	client.services[region] = cacheItem
	return cacheItem.item, cacheItem.err
}

// Verifica se é um identificador válido para uma secret.
func (client *AWSClient) isValidSecretIdentifier(identifier string) bool {
	if identifier == "" {
		return false
	}
	return arnRegex.MatchString(identifier) || (nameRegex.MatchString(identifier) && !nameRestrictionRegex.MatchString(identifier))
}

// Obtém a região a partir do ARN da secrect
func (client *AWSClient) getRegionFromIdentifier(identifier string) string {
	matches := regionRegex.FindStringSubmatch(identifier)
	if len(matches) > 1 {
		return matches[1]
	}
	return client.defaultRegion
}

// Retorna o valor de um secret.
// Se o secret foi carregado anteriormente, retorna a referência existente, caso
// contrário carrega o secret, armazena sua referência a retorna ao chamador.
func (client *AWSClient) GetSecret(identifier string) (string, error) {

	if !client.isValidSecretIdentifier(identifier) {
		return "", fmt.Errorf("identificador '%s' não é válido", identifier)
	}

	client.secretsMutex.Lock()
	defer client.secretsMutex.Unlock()

	if cacheItem, exists := client.secrets[identifier]; exists {
		return cacheItem.item, cacheItem.err
	}

	cacheItem := CacheItem[string]{
		item: "",
		err:  nil,
	}

	secretRegion := client.getRegionFromIdentifier(identifier)
	service, err := client.getService(secretRegion)
	if err != nil {
		cacheItem.err = fmt.Errorf("não foi possível obter o segredo: %w", err)
	} else {
		result, err := service.GetSecretValue(
			&secretsmanager.GetSecretValueInput{
				SecretId: aws.String(identifier),
			})

		if result.SecretString != nil {
			cacheItem.item = *result.SecretString
		} else {
			if err == nil {
				cacheItem.err = fmt.Errorf("o segredo '%s' não contém um texto", identifier)
			} else {
				cacheItem.err = fmt.Errorf("não foi possível obter valor do segredo '%s'. %v", identifier, err)
			}
		}
	}

	client.secrets[identifier] = cacheItem
	return cacheItem.item, cacheItem.err
}
