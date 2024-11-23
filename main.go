package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"

	"os"
	"path/filepath"
	"regexp"

	"envconfig/awsclient"
)

// Versão será injetada no build.
// Ex. go build -ldflags "-X main.Version=<versão>" -o <nomeExecutavel>
var Version = "indefinida"

// Expressões regulares para a validação de contexto
var (
	commentRegex     = regexp.MustCompile(`^\s*[#\/*]`)
	envVarRegex      = regexp.MustCompile(`{?\${?([A-Za-z_{][A-Za-z0-9_]*)}?`)
	placeholderRegex = regexp.MustCompile(`\{\{?([\w/:+_=.@-]+)(\[([\w]+)\])?\}?\}`)
)

func processTemplateFile(inputFilePath string, outputFilePath string, profile string, region string) error {

	if _, err := os.Stat(inputFilePath); os.IsNotExist(err) {
		return fmt.Errorf("arquivo de entrada '%s' não encontrado", inputFilePath)
	}

	inputFile, err := os.Open(inputFilePath)
	if err != nil {
		return fmt.Errorf("erro ao abrir o arquivo '%s'. %w", inputFilePath, err)
	}
	defer inputFile.Close()

	outputFile, err := os.Create(outputFilePath)
	if err != nil {
		return fmt.Errorf("erro ao criar o arquivo '%s'. %w", outputFilePath, err)
	}
	defer outputFile.Close()

	scanner := bufio.NewScanner(inputFile)
	writer := bufio.NewWriter(outputFile)
	client := awsclient.NewAWSClient(profile, region)
	failed := 0 

	for scanner.Scan() {
		line := scanner.Text()

		//Ignora comentários
		if commentRegex.MatchString(line) {
			fmt.Fprintln(writer, line)
			continue
		}

		//Processa primeiro as substituições por variáveis de ambiente
		//Substitui os placeholders pelo valor da variável com o mesmo nome
		//Se não for encontrado uma variável para o placeholder, o mantém
		//para a possibilidade de substituir posteriormente com algum segredo.
		processedLine := envVarRegex.ReplaceAllStringFunc(line, func(placeholder string) string {
			matches := envVarRegex.FindStringSubmatch(placeholder)
			envVarValue := os.Getenv(matches[1])
			if envVarValue != "" {
				return envVarValue
			}
			return matches[0]
		})

		//Processa em seguida as substituições de secrets
		//Se não for identificado o valor do segredo para o placeholder, o mantém como na origem
		processedLine = placeholderRegex.ReplaceAllStringFunc(processedLine, func(placeholder string) string {
			matches := placeholderRegex.FindStringSubmatch(placeholder)
			secretValue, err := client.GetSecret(matches[1])

			if err != nil {
				failed ++
				log.Println(err)
			} else if secretValue != "" {
				if matches[3] == "" {
					return secretValue
				} else {
					var secretMap map[string]string
					if err := json.Unmarshal([]byte(secretValue), &secretMap); err == nil {
						if value, exists := secretMap[matches[3]]; exists {
							return value
						} else {
							failed ++
							log.Printf("chave '%s' não encontrada", matches[3])
						}
					} else {
						failed ++
						log.Println("erro ao parsear JSON: ", err)
					}
				}
			}
			return matches[0]
		})

		fmt.Fprintln(writer, processedLine)
	}

	if failed > 0 {
		return fmt.Errorf("%v itens não puderam ser substituídos. Verifique o log para mais detalhes.", failed)
	}

	if err := writer.Flush(); err != nil {
		return fmt.Errorf("erro ao escrever no arquivo de saída: %w", err)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("erro ao ler o arquivo de entrada: %w", err)
	}

	return nil
}

func main() {
	var profile string
	var region string

	versionFlag := flag.Bool("version", false, "Exibe a versão do software")
	flag.StringVar(&profile, "profile", "default", "Perfil AWS a ser utilizado")
	flag.StringVar(&region, "region", "", "Região da AWS a ser utilizada")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("Versão: %s\n", Version)
		os.Exit(0)
	}

	posArgs := flag.Args()

	if len(posArgs) != 2 {
		fmt.Println("Uso: " + filepath.Base(os.Args[0]) + " [-profile <profile>] [-region <region>] <inputFilePath> <outputFilePath>")
		os.Exit(1)
	}

	if err := processTemplateFile(posArgs[0], posArgs[1], profile, region); err != nil {
		fmt.Fprintf(os.Stderr, "Erro: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Arquivo de saída gerado com sucesso:", posArgs[1])
}
