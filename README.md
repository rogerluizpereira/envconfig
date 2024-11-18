# envconfig
## _Uma opção viável_

Imagine um cenário de aplicações legadas, de dificil manutenção, que precisam de secrets aws para execução.
Considere também o agravante de que para desenvolver o time precisa de acesso à chaves válidas no ambiente de desenvolvimento e consequentemnete a rotação periódica destas chaves impacta a produtividade do time. 

A utilização de secrets neste cenário naturalmente apresenta risco alto de vazamento, que é agravado por períodos maiores entre as rotações periódicas.

Certamente encontraremos outras formas de mitigar o problema, em especial a médio e longo prazo investindo certo esforço e tempo em algumas refatorações, configurações e testes, porém é preciso uma ação viável a curto prazo que gere menor impacto ao ambiente e ao menos permita reduzir a exposição das chaves e aumentar a frequêcia de rotacionamento.

A proposta é que ao invés do conteúdo ( ID e Secret ) de uma credencial, os arquivos de configuração, e consequentemente o desenvolvedor, precisariam apenas do ```ARN``` ou ```Nome da Credencial``` armazenada no secret manager, e somente em tempo de inicialização da aplicação, os arquivos tenham o valor destas chaves substituidos pelo conteúdo real.

Desta forma, as chaves poderiam ser rotacionadas a qualquer tempo, pois na proxima execução o novo valor seria obtido sem nenhuma fricção para o desenvolvedor. 
Os segredos também não estariam mais expostos nas definições do container ou nas suas variáveis de ambiente facilmente inspecionáveis pelo docker.

>Claro que a proposta não impede integralmente o vazamento, pois quem puder logar no container em execução ou na imagem em cache contendo o arquivo de configuração atualizado, terá acesso às chaves, mas a proposta é reduzir a exposição e proporcionar rotações
mais frequentes, até que as aplicações possan refatoradas utilizando técnicas mais adequadas. 

É possivel reduzir ainda pouco mais a exposição, caso a aplicação leia todas as configurações do arquivo em sua inicialização e não precisar mais dele durante sua execução, pois podemos preencher as configurações no arquivo usando um ponto de montagem em memória, e remover o arquivo logo após a inicialização. Desta forma os segredos estariam disponiveis apenas em memória na aplicação

Para isso, este utilitário foi desenvolvido para buscar os segredos e substituir as entradas no arquivo especificado
- Em uma execução local, ele utilizará o profile AWS do desenvolvedor 
- Em uma execução no sevidor, ele utilizará uma role designada.

Como bônus visando a simplicidade, ele também faz a substituição de entradas por variáveis de ambiente correspondentes.

## Modo de uso
1. Para buildar o utilitário:
    ```bash
    go build -ldflags "-X main.version=<versão>" -o envconfig 

    ```

2. Formato do placeholder de chaves 
    ``` {arn:aws:secretsmanager:<region>:<account>:secret:<nome-da-secret>[atributo]} ```
    ```
    {arn:aws:secretsmanager:<region>:<account>:secret:<nome-da-secret>[AWS_ACCESS_KEY_ID]}
    {arn:aws:secretsmanager:<region>:<account>:secret:<nome-da-secret>[AWS_ACCESS_KEY_ID]}
    ```

3. Formato do placeholder de variáveis de ambiente
    ```
    ${NOME_VARIAVEL}
    ```

4. Exemplo de utilização em um ```entrypoint.sh``` de uma aplicação que suporta exclusão do arquivo de configuração
    ``` bash
    # Obtem segredos do cofre, evitando a exposição em variáveis de ambiente
    # atualizando o arquivo de configuração com os segredos que serão usados pela aplicação
    envconfig $AWS_PROFILE $CONFIG_FILE $TEMP_CONFIG_FILE
    ...
    # Agenda a exclusão do arquivo para 5 segundos, para que o arquivo permaneça
    # o menor tempo possível em disco, evitado exposição de credenciais.
    nohup sh -c "sleep 5 && rm -f $TEMP_CONFIG_FILE" &
    ```

A variável ```$AWS_PROFILE```, se preenchida com o conteúdo ```-profile NomeProfile``` possibilita a execução local, uma vez que neste ambiente não há como associar uma role. Já em uma execução no servidor, a inexistência desta variável provoca a execução do utilitário sem designar um profile e desta forma a role associada ao container será utilizada para recuperar as chaves.  
