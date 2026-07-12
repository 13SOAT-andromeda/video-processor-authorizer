# Spec — video-processor-authorizer

**Data:** 2026-07-11
**Status:** Draft — pronto para virar plano de implementação
**Repo antigo de referência:** `tech-challenge-user-authorizer`
**Spec guarda-chuva:** `docs/superpowers/specs/2026-07-11-video-processor-auth-infra-migration-design.md` (workspace raiz)

> Este documento copia na íntegra `service-authorizer.md` (fonte: `Video Processing/specs/specs/`) e adiciona, na seção 9, o que é específico deste repositório (porta do código antigo, Terraform local, dependências).

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

- Secrets Manager: `jwt-signing-key` (mesmo segredo usado por `authentication` para assinar).
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
| `internal/auth/jwt.go` | mesmo caminho | portável quase 1:1 — validação de assinatura/exp |
| `internal/config/config.go` | mesmo caminho | portável — leitura de env/Secrets Manager |
| `cmd/authorizer/main.go` | mesmo caminho | **muda o contrato**: antigo respondia a `TOKEN` authorizer com header `X-AUTH-TOKEN`; novo responde a `REQUEST` authorizer com `Authorization: Bearer` e devolve `context` com `userId`/`role` |
| `pkg/utils/logger.go` | mesmo caminho | portável sem mudança |
| `internal/session/store.go` | **não portado** | conceito de sessão armazenada não existe na arquitetura nova (JWT stateless, sem consulta a banco no authorizer) |

### 9.2 Terraform local (`terraform/` neste repo)

- `aws_lambda_function` — instanciando o módulo `terraform-aws-modules/lambda/aws` (fonte: `iac-video-processor-infra`), **nome da função deve ser exatamente `video-processor-authorizer`** (contrato consumido por `iac-video-processor-gateway` via `data.aws_lambda_function`).
- IAM policy: `secretsmanager:GetSecretValue` restrita ao ARN de `jwt-signing-key` — nenhuma outra permissão.
- Config: `memory 128MB`, `timeout 3s`, `arch arm64`.

### 9.3 Dependências

- **Nenhuma dependência de dado** (não lê RDS nem DynamoDB) — pode ser implementado em paralelo a qualquer outro repo desta fase.
- Depende apenas de `iac-video-processor-infra` existir (módulo Lambda) e do secret `jwt-signing-key` estar criado no Secrets Manager (mesmo segredo usado por `video-processor-authentication-api`).
