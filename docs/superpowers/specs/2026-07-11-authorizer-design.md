# Spec — video-processor-authorizer

**Data:** 2026-07-11 (revisado 2026-07-16 — ADR-011; revisado 2026-07-19 — implementação Go+Terraform, ver nota abaixo)
**Status:** Draft — pronto para virar plano de implementação
**Repo antigo de referência:** `tech-challenge-user-authorizer`
**Spec guarda-chuva:** `docs/superpowers/specs/2026-07-11-video-processor-auth-infra-migration-design.md` (workspace raiz), atualizada em 2026-07-16

> Este documento copia na íntegra `service-authorizer.md` (fonte: `Video Processing/specs/specs/`) e adiciona, na seção 9, o que é específico deste repositório (porta do código antigo, Terraform local, dependências).

> **Nota da revisão 2026-07-16 (ADR-011):** as RFCs de `authentication`/`users-api` (signup público, verificação de email, admin bootstrap via seed) **não exigem nenhuma mudança neste serviço**. `authorizer` continua só validando assinatura/expiração do JWT e devolvendo `{userId, role}` no `context` — nenhuma lógica de autorização por rota/role é adicionada aqui. A checagem de `role administrator` nas rotas administrativas de `users-api` continua inteiramente no handler daquele serviço (defesa em profundidade, mesmo padrão já documentado na seção 4, item 5, abaixo). `video-processor-users-api`, por rodar atrás de uma integração `HTTP_PROXY`/ALB (não Lambda proxy), passou a validar o JWT por conta própria em vez de depender do `context` deste authorizer para obter `userId`/`role` — ver `video-processor-users-api`, seção 5.1. Esse authorizer continua sendo usado normalmente na borda do API Gateway para essas rotas, só não é mais a única fonte de `userId`/`role` para o serviço que roda em EKS.

> **Nota da revisão 2026-07-19 (brainstorming de implementação, sem mudança de comportamento):** decisões tomadas pra sair do Draft rumo ao plano de implementação, todas documentadas em detalhe na seção 9 revisada:
> 1. **Escopo:** este plano cobre Go (handler) **e** Terraform juntos — diferente de `video-processor-authentication-api`, que deferiu Terraform pra uma "Plan 2" separada.
> 2. **Empacotamento — Image via ECR, não Zip** (decisão explícita do time, verificado contra `tech-challenge-user-authorizer/terraform/` e `iac-tech-challenge-infra/aws/ecr.tf`): mantém o padrão do repo antigo — `aws_lambda_function` com `package_type = "Image"`, ECR repo próprio (`video-processor-authorizer-${var.environment}`, criado em `iac-video-processor-infra`, mesmo padrão do `ecr_users_api` já existente lá). A referência geral de AWS Serverless deste projeto recomendaria `.zip` pra um binário Go simples como este (ver seção 9.2, nota); optou-se por Image mesmo assim, por consistência com o restante do parque de Lambdas/ECR do projeto.
> 3. **IAM diverge por ambiente:** `prod` usa `data.aws_iam_role.lab_role` (LabRole fixa — AWS Academy sandbox não permite IAM role/policy customizada; mesmo padrão já usado em `iac-video-processor-infra/prod/eks.tf`); `dev`/LocalStack usa role própria com policy scoped só em `secretsmanager:GetSecretValue` no ARN do secret (least-privilege real da seção 7, testável sem a restrição do sandbox).
> 4. **Terraform não builda nem publica a imagem** — referencia uma imagem já existente no ECR via `data.aws_ecr_repository` + `var.image_tag` (mesmo padrão do `terraform/main.tf` antigo). **Ponto em aberto:** como a imagem chega no ECR (build/push manual vs pipeline CI futuro) fica pra o time decidir depois — fora deste plano.
> 5. **Fora de escopo, aceito nesta fase:** CI/CD (pipeline `.github/workflows`) e provisioned concurrency (item 2 da seção original permanece só recomendação, não implementada agora).

---

## 1. Responsabilidade

Validar o JWT em toda requisição antes de chegar às Lambdas de negócio. É o único ponto de decisão de autenticação da borda — nenhum serviço downstream reprocessa o token (apenas reconferem *ownership* quando necessário).

## 2. Trigger e integração

- API Gateway HTTP API → **Lambda authorizer do tipo `REQUEST`** (não `TOKEN`), porque precisamos devolver `userId` e `role` no `context`, não só um allow/deny binário.
- Config: `memory 128MB`, `timeout 3s` (crítico — está no caminho de *toda* requisição autenticada), `arch arm64`, **provisioned concurrency recomendado** junto com `authentication`.
- Cache de autorização do API Gateway: `authorizerResultTtlInSeconds = 300`, chave de cache = header `Authorization` (evita reinvocar a Lambda a cada request do mesmo token).

## 3. Contrato de entrada/saída

**Entrada** (evento `REQUEST` do API Gateway v2 com payload format 2.0):
```json
{ "headers": { "authorization": "Bearer eyJhbGciOi..." }, "routeArn": "arn:aws:execute-api:..." }
```

**Saída — sucesso:**
```json
{
  "isAuthorized": true,
  "context": { "userId": "abc", "role": "user" }
}
```

**Saída — negado:** `{ "isAuthorized": false }` → API Gateway responde `403` automaticamente.

## 4. Regras de negócio

1. Extrai o token do header `Authorization: Bearer <jwt>`; ausência ou formato inválido → `isAuthorized: false` imediato (sem tentar parsear).
2. Valida assinatura com a chave carregada do Secrets Manager **uma única vez fora do handler** (cacheada em variável de pacote — só recarrega se a versão do segredo mudar).
3. Valida `exp` (expiração) e `iat`; token expirado → nega.
4. Não consulta banco de dados em nenhuma hipótese — decisão 100% baseada nos claims assinados.
5. Propaga `userId` e `role` no `context` para os handlers de negócio lerem via evento da API Gateway (não confiar em nenhum outro header do cliente para isso).

## 5. Casos de teste (tabela)

| Caso | Entrada | Resultado esperado |
|---|---|---|
| Token válido, não expirado | JWT assinado corretamente | `isAuthorized: true` + context |
| Token expirado | `exp` no passado | `isAuthorized: false` |
| Assinatura inválida | JWT adulterado | `isAuthorized: false` |
| Header ausente | sem `Authorization` | `isAuthorized: false` |
| Claims faltando (`sub`/`role`) | JWT malformado | `isAuthorized: false` |

## 6. Dependências

- Secrets Manager: `jwt-signing-key-${var.environment}` (mesmo segredo usado por `authentication` para assinar; nome atualizado — ver ADR-013, `docs/superpowers/specs/2026-07-18-jwt-secret-gateway-routes-design.md`).
- Bibliotecas: `github.com/golang-jwt/jwt/v5`, SDK v2 (`secretsmanager` client).

## 7. IAM

- `secretsmanager:GetSecretValue` apenas no ARN do segredo JWT.
- Nenhum acesso a RDS, DynamoDB, S3, SQS ou SNS — este serviço não deveria conseguir tocar em nenhum dado de negócio mesmo que comprometido.

## 8. Observabilidade

- Métrica de taxa de `isAuthorized: false` — pico anômalo pode indicar ataque de token forjado ou bug de expiração no client.
- Log sem o token completo (logar só os primeiros/últimos caracteres para correlação, nunca o JWT inteiro).

---

## 9. Contexto de migração/repositório (específico deste repo)

### 9.1 Porta do repo antigo (`tech-challenge-user-authorizer`)

| Antigo | Novo | Observação |
|---|---|---|
| `internal/auth/jwt.go` | mesmo caminho | portável quase 1:1 — validação de assinatura/exp. **Remove** a checagem de `issuer` (`JWTIssuer`): a spec nova não define esse claim e `video-processor-authentication-api` não o emite. |
| `internal/config/config.go` | mesmo caminho | portável, bem mais enxuto — só `JWT_SIGNING_KEY_SECRET_NAME` (nome do secret no Secrets Manager) e `AWS_REGION`. **Remove** `DynamoDBTableName`/`DynamoDBEndpoint` (sem sessão). |
| `cmd/authorizer/main.go` | mesmo caminho | **muda o contrato**: antigo respondia a `TOKEN` authorizer com header `X-AUTH-TOKEN`; novo responde a `REQUEST` authorizer (evento `APIGatewayV2CustomAuthorizerV2Request`, `enable_simple_responses`) com `Authorization: Bearer` e devolve `context` com `userId`/`role` (claims `sub`/`role` do JWT). **Remove** a dependência do Datadog (`ddlambda`/`dd-trace-go`) — sem observabilidade de terceiros nesta fase. |
| `pkg/utils/logger.go` | mesmo caminho | portável sem mudança |
| `internal/session/store.go` | **não portado** | conceito de sessão armazenada não existe na arquitetura nova (JWT stateless, sem consulta a banco no authorizer) |

Testes reescritos do zero (TDD), cobrindo a tabela de casos da seção 5 (token válido, expirado, assinatura inválida, header ausente, claims faltando).

### 9.2 Terraform local (`terraform/{dev,prod}/` neste repo)

Estrutura em duas pastas (mesmo padrão dos repos `iac-video-processor-*`): `dev/` aponta pro LocalStack (endpoints `http://localhost:4566`, backend S3 fake), `prod/` aponta pra AWS real. Motivo: a estratégia de IAM diverge por ambiente (abaixo), então separar evita condicionais misturando as duas lógicas num único `main.tf`.

**Empacotamento — `package_type = "Image"` via ECR** (não `.zip`):

- Decisão explícita do time (2026-07-19), verificada contra o repo antigo (`tech-challenge-user-authorizer/terraform/main.tf` — `aws_lambda_function` com `package_type = "Image"`, `data.aws_ecr_repository`) e `iac-tech-challenge-infra/aws/ecr.tf` (um `module "ecr"` por serviço, incluindo `tech-challenge-user-authorizer-repo`).
- **Nota:** pra um binário Go simples e estático como este, a referência geral de AWS Serverless deste projeto recomendaria `.zip` (árvore de decisão: "simples, poucas deps → .zip"; Image é indicado só acima de 250MB ou build nativo complexo). Optou-se por Image mesmo assim, por consistência com o ECR já usado no restante do projeto (`ecr_users_api`) e com o padrão já validado no repo antigo. Revisitar se o overhead de manter Dockerfile/ECR só pra este serviço não compensar.
- `video-processor-authorizer/Dockerfile`: builda o binário Go (`GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o bootstrap ./cmd/authorizer`) sobre uma base `public.ecr.aws/lambda/provided:al2023-arm64`, `CMD ["bootstrap"]`.
- `module "authorizer_lambda"` = `terraform-aws-modules/lambda/aws ~> 8.8` (versão confirmada via registry em 2026-07-19), com:
  - `package_type = "Image"`, `create_package = false`, `image_uri = "${data.aws_ecr_repository.this.repository_url}:${var.image_tag}"`.
  - `var.image_tag`: input obrigatório, sem default — quem aplica passa a tag da imagem já publicada.
  - **Terraform não builda nem publica a imagem** — só referencia uma já existente no ECR (mesmo padrão do `terraform/main.tf` antigo, que também só referenciava via `data.aws_ecr_repository` + `var.image_tag`, com o build feito por CI). **Ponto em aberto:** como a imagem chega no ECR (comando manual `docker build && docker push` documentado no README vs pipeline CI futuro) — decisão do time, fora deste plano.
  - `architectures = ["arm64"]`, `memory_size = 128`, `timeout = 3`.
  - `function_name = "video-processor-authorizer"` — **nome fixo, exato** (contrato consumido por `iac-video-processor-gateway` via `data.aws_lambda_function`).
  - `environment_variables`: `JWT_SIGNING_KEY_SECRET_NAME`, `AWS_REGION`.
- **IAM diverge por ambiente:**
  - `prod/`: `lambda_role = data.aws_iam_role.lab_role.arn`, `create_role = false` — AWS Academy sandbox não permite criar IAM role/policy customizada em prod (mesma restrição e mesmo padrão já documentados em `iac-video-processor-infra/prod/eks.tf`).
  - `dev/`: `create_role = true`, `attach_policy_statements = true`, `policy_statements` restrito a `secretsmanager:GetSecretValue` só no ARN de `jwt-signing-key-${var.environment}` — least-privilege real da seção 7, testável no LocalStack sem a restrição do sandbox.
- Providers: `aws ~> 6.55` (versão atual confirmada via registry em 2026-07-19; compatível com o `~> 6.54` já usado nos outros repos `video-processor-*`).
- Fora de escopo nesta fase: CI/CD e provisioned concurrency (aceito, ver nota de revisão 2026-07-19 no topo do documento).

### 9.3 Dependências

- Depende de `iac-video-processor-infra` ganhar um `module "ecr_authorizer"` novo (mesmo padrão do `ecr_users_api` já existente — repositório ECR com lifecycle policy de 10 imagens), em `dev/` e `prod/`.
- Depende do secret `jwt-signing-key-${var.environment}` já existir no Secrets Manager (mesmo segredo usado por `video-processor-authentication-api` — já provisionado, ADR-013).
- **Nenhuma dependência de dado** (não lê RDS nem DynamoDB) — pode ser implementado em paralelo a qualquer outro repo desta fase, exceto pela pequena dependência acima em `iac-video-processor-infra` (ECR).
