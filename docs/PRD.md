# PRD — AWSX

## 1. Visão geral

**Nome do produto:** AWSX
**Tipo:** CLI interativa para AWS
**Linguagem:** Go
**Licença sugerida:** MIT
**Distribuição:** Repositório público no GitHub
**Plataformas iniciais:** macOS e Linux

O AWSX será uma ferramenta de terminal para simplificar o login na AWS e o acesso a instâncias EC2 por meio do AWS Systems Manager Session Manager.

O usuário executará apenas:

```bash
awsx
```

A ferramenta deverá verificar as dependências locais, apresentar um menu de login semelhante ao Granted, autenticar o usuário na AWS e permitir o acesso interativo às instâncias EC2 disponíveis via SSM.

## 2. Problema

O fluxo atual para acessar uma instância EC2 via SSM exige vários passos manuais:

1. Identificar o perfil AWS correto.
2. Verificar se a sessão está válida.
3. Executar o login via AWS CLI.
4. Encontrar o ID da instância EC2.
5. Verificar se a instância está disponível no SSM.
6. Executar manualmente `aws ssm start-session --target i-xxxxxxxxxxxxxxxxx`.

Esse processo exige conhecimento de perfis, comandos, IDs de instâncias e detalhes da configuração da AWS.

## 3. Objetivo

Criar uma experiência única e interativa para:

1. Verificar se a AWS CLI está instalada.
2. Solicitar a instalação da AWS CLI quando necessário.
3. Listar os perfis AWS configurados localmente.
4. Permitir que o usuário selecione um perfil.
5. Realizar o login na AWS.
6. Exibir um menu com as opções: Acessar EC2 / Sair.
7. Listar as instâncias EC2 disponíveis via SSM.
8. Abrir o terminal da instância selecionada.

O usuário não deverá precisar digitar IDs de instâncias nem comandos adicionais.

## 4. Público-alvo

Desenvolvedores, Tech Leads, DevOps, SREs, Cloud Engineers, equipes com múltiplas contas AWS, empresas que usam AWS IAM Identity Center e EC2 com SSM Session Manager.

## 5. Fluxo principal

```text
awsx
  -> Verificar dependências (AWS CLI ausente -> instalar; presente -> continuar)
  -> Listar perfis AWS -> Selecionar perfil
  -> Verificar sessão (válida -> continuar; inválida -> aws sso login -> browser -> confirmar)
  -> Menu principal
       -> Acessar EC2 -> listar EC2 via SSM -> selecionar -> abrir terminal SSM
       -> Sair -> encerrar
```

## 6. Escopo da versão 1

### 6.1 Inicialização
`awsx` inicia a interface interativa. Sem subcomandos na V1.

### 6.2 Verificação da AWS CLI
Verifica se `aws` está disponível (`aws --version`). Se ausente, informa e oferece instalação.

### 6.3 Instalação da AWS CLI
Detecta SO, arquitetura e gerenciadores de pacote. Plataformas: macOS Intel, macOS Apple Silicon, Linux amd64, Linux arm64. No macOS com Homebrew, sugere `brew install awscli`. No Linux, usa o instalador oficial da AWS CLI v2 ou instruções compatíveis. Instalação exige confirmação explícita. Não executa `sudo` sem informar claramente. Se automático não for possível, mostra instruções e encerra.

### 6.4 Verificação do Session Manager Plugin
Verifica se `session-manager-plugin` está instalado. Se ausente, oferece instalação ou instruções.

### 6.5 Leitura dos perfis AWS
Lê `~/.aws/config` e `~/.aws/credentials`. O menu prioriza perfis de AWS IAM Identity Center. O usuário pesquisa digitando.

### 6.6 Login
Após selecionar o perfil, valida sessão com `aws sts get-caller-identity --profile <perfil>`. Se válida, continua. Se expirada/inválida, executa `aws sso login --profile <perfil>` (pode abrir o browser). Aguarda a conclusão e continua automaticamente.

### 6.7 Menu após login
Mostra perfil, conta, identidade/role, região e as opções Acessar EC2 / Sair. Sair apenas encerra; login permanece no cache da AWS CLI (sem logout).

### 6.8 Listagem das EC2
Exibe apenas instâncias `running`, disponíveis no SSM e acessíveis pelo perfil. Informações: Nome, ID, Região, Estado, Tipo, IP privado, Status do SSM. O nome vem da tag `Name`; fallback = ID da instância.

### 6.9 Descoberta das instâncias
Usa o perfil selecionado. `aws ec2 describe-instances` + `aws ssm describe-instance-information`, cruzando os resultados para exibir só instâncias que aceitam sessão SSM. Pode usar o AWS SDK for Go v2 diretamente.

### 6.10 Busca interativa
Busca incremental por nome, ID, IP privado, tags relevantes e tipo.

### 6.11 Abertura da sessão SSM
`aws ssm start-session --profile <perfil> --target <instance-id>`. Assume o controle do terminal. Ao encerrar, retorna ao menu: Voltar para as instâncias / Voltar para o menu principal / Sair.

## 7. Interface

TUI com Bubble Tea, Bubbles, Lip Gloss. Navegação por setas, Enter confirma, busca digitando, Esc volta, `q`/`Ctrl+C` sai. Indicador de carregamento, mensagens de erro claras, confirmações antes de instalações, sem mouse.

## 9. Tratamento de erros

- AWS CLI ausente → Instalar / Ver instruções / Sair.
- Session Manager Plugin ausente → Instalar / Ver instruções / Sair.
- Nenhum perfil encontrado → executar `aws configure sso` / Sair.
- Login cancelado → Tentar novamente / Escolher outro perfil / Sair.
- Perfil sem permissão EC2 → mostrar ação negada (ex. `ec2:DescribeInstances`).
- Sem permissão SSM → `ssm:DescribeInstanceInformation`.
- Sem instâncias disponíveis → listar possíveis motivos.
- Erro ao abrir sessão → mostrar nome, ID, perfil, região, mensagem original da AWS.

## 10. Segurança

Não armazena permanentemente `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_SESSION_TOKEN`, senha, MFA ou token SSO. Usa o cache oficial da AWS CLI. Lê `~/.aws/config`, `~/.aws/credentials`, `~/.aws/sso/cache`. Não imprime credenciais nem registra tokens em logs. Debug oculta dados sensíveis.

## 11. Dependências

Runtime obrigatório: AWS CLI v2, Session Manager Plugin. Go sugeridas: bubbletea, bubbles, lipgloss, aws-sdk-go-v2, cobra (opcional na V1, recomendado).

## 12. Estrutura sugerida do projeto

```text
awsx/
├── cmd/awsx/main.go
├── internal/{app,auth,awscli,config,dependencies,ec2,installer,profiles,session,ssm,tui}
├── scripts/install.sh
├── .github/workflows/{ci.yml,release.yml}
├── .goreleaser.yaml
├── go.mod / go.sum
├── LICENSE / README.md / CONTRIBUTING.md
```

## 13. Comandos futuros

V1 só `awsx`. Arquitetura deve permitir depois: `awsx login`, `awsx ec2`, `awsx whoami`, `awsx profiles`, `awsx doctor`, `awsx version`, `awsx ec2 <nome>`, `awsx --profile <p>`.

## 14. Instalação

macOS Homebrew: `brew install <org>/tap/awsx`. Script: `curl -fsSL .../scripts/install.sh | sh`. O instalador detecta SO/arch, acha a release mais recente, baixa o binário, valida checksum, instala no PATH (`/usr/local/bin/awsx` ou `~/.local/bin/awsx`) e reporta.

## 15. Builds

Alvos: darwin-amd64, darwin-arm64, linux-amd64, linux-arm64. Artefatos: `awsx_Darwin_x86_64.tar.gz`, `awsx_Darwin_arm64.tar.gz`, `awsx_Linux_x86_64.tar.gz`, `awsx_Linux_arm64.tar.gz`, `checksums.txt`.

## 16. Releases

Via GitHub Actions + GoReleaser, acionado por tag `vX.Y.Z`. Pipeline: testes, lint, compilar, checksums, GitHub Release, atualizar Homebrew Tap.

## 17. Repositório público

Nome `awsx`, `github.com/<org>/awsx`. Arquivos obrigatórios: README.md, LICENSE, CONTRIBUTING.md, CODE_OF_CONDUCT.md, SECURITY.md. Licença MIT.

## 18. README mínimo

Deve explicar: o que é, como instalar, dependências, como configurar AWS SSO, como executar, permissões IAM necessárias, como usar EC2 via SSM, como contribuir, como publicar release.

## 19. Permissões IAM necessárias

`ec2:DescribeInstances`, `ssm:DescribeInstanceInformation`, `ssm:StartSession`, `ssm:TerminateSession`, `ssm:DescribeSessions`, `ssm:GetConnectionStatus`. Possível recurso: `arn:aws:ssm:*:*:document/SSM-SessionManagerRunShell`.

## 20. Requisitos não funcionais

**Performance:** init em até 2s (fora chamadas externas); menu responsivo com centenas de perfis; busca responsiva com milhares de instâncias; chamadas AWS com timeout. **Compatibilidade:** macOS ARM/Intel, Linux amd64/arm64, terminais ANSI. **Confiabilidade:** não corromper arquivos AWS, não modificar `~/.aws/config` na V1, não remover sessões, não armazenar credenciais próprias. **Observabilidade:** `AWSX_DEBUG=true awsx`; logs sem credenciais/tokens.

## 21. Fora do escopo da V1

SSH tradicional, PEM, ECS Exec, EKS, RDS, S3, Lambda, CloudWatch, Secrets Manager, port forwarding, transferência de arquivos, Windows, múltiplas regiões simultâneas, favoritos, histórico de conexões, logout remoto, gerenciamento de IAM, criação automática de perfis, implementação própria de SSO/OIDC.

## 22. Roadmap

- **V1:** verificar deps, listar perfis, login SSO, menu, listar EC2, filtrar SSM, abrir terminal SSM, instalador macOS/Linux, GitHub Releases, Homebrew Tap.
- **V1.1:** favoritos, histórico, último perfil/instância, seleção de região.
- **V1.2:** conexão direta pelo nome, `awsx ec2`, `awsx whoami`, `awsx doctor`.
- **V2:** ECS Exec, port forwarding, bancos privados, CloudWatch Logs, Secrets Manager.

## 23. Critérios de aceitação

1. Instalar via terminal no macOS. 2. Instalar via terminal no Linux. 3. `awsx` no PATH. 4. Detectar AWS CLI. 5. Detectar Session Manager Plugin. 6. Orientar/solicitar instalação de deps ausentes. 7. Listar perfis locais. 8. Selecionar perfil pelo menu. 9. Detectar sessão válida. 10. Login SSO quando necessário. 11. Mostrar Acessar EC2 / Sair. 12. Sair encerra sem remover sessão. 13. Acessar EC2 lista instâncias running. 14. Só instâncias disponíveis via SSM. 15. Pesquisar por nome. 16. Selecionar sem conhecer ID. 17. Abrir sessão SSM funcional. 18. Ao encerrar, retornar ao menu. 19. Nenhuma credencial permanente criada. 20. Nenhum token nos logs.

## 24. Definição da experiência final

O usuário completa todo o processo sem digitar ID de instância, nome de role, comando de login, comando do SSM ou AWS Account ID. A única entrada obrigatória é `awsx`.
