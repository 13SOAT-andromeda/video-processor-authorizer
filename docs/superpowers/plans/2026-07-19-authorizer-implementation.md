# video-processor-authorizer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the `video-processor-authorizer` Lambda `REQUEST` authorizer (Go handler + Terraform) that validates the JWT session token issued by `video-processor-authentication-api` and returns `{userId, role}` in the API Gateway authorizer context.

**Architecture:** Single Go binary (`cmd/authorizer/main.go`) built into a container image (AWS `provided:al2023-arm64` base) and deployed as an `Image`-type Lambda function via `terraform-aws-modules/lambda/aws`. No database, no session store — decision is 100% based on the signed JWT claims (`sub`, `role`, `typ`). Two Terraform environments (`dev/` against LocalStack, `prod/` against real AWS) because IAM strategy diverges: `prod` reuses the fixed AWS Academy `LabRole` (sandbox forbids custom IAM), `dev` creates a real least-privilege role scoped to `secretsmanager:GetSecretValue`.

**Tech Stack:** Go 1.25, `github.com/golang-jwt/jwt/v5`, `github.com/aws/aws-lambda-go`, `github.com/aws/aws-sdk-go-v2` (`config`, `secretsmanager`), `github.com/stretchr/testify`, Terraform `~> 1.7`, `hashicorp/aws ~> 6.55`, `terraform-aws-modules/lambda/aws ~> 8.8`, `terraform-aws-modules/ecr/aws ~> 3.2`.

## Global Constraints

- Go module: `github.com/13SOAT-andromeda/video-processor-authorizer`, Go `1.25.0` (matches `video-processor-authentication-api`).
- Lambda function name **must be exactly** `video-processor-authorizer` (contract with `iac-video-processor-gateway`'s `data.aws_lambda_function.authorizer`).
- Lambda config: `architectures = ["arm64"]`, `memory_size = 128`, `timeout = 3`.
- Secret name pattern: `jwt-signing-key-${var.environment}` (already provisioned in Secrets Manager by `iac-video-processor-infra`, ADR-013 — do not create it here).
- ECR repo name pattern: `video-processor-authorizer-${var.environment}` (created in `iac-video-processor-infra`, not in this repo).
- Package type: `Image`, **not** `.zip`. Terraform does **not** build or push the image — it only references `var.image_tag`, an already-published tag, via `data.aws_ecr_repository`.
- IAM diverges by environment: `prod` → `data.aws_iam_role.lab_role` (`name = "LabRole"`, `create_role = false`); `dev` → `create_role = true` + `attach_policy_statements = true` scoped to `secretsmanager:GetSecretValue` on the JWT secret ARN only.
- JWT claims contract (from `video-processor-authentication-api/internal/core/domain/token.go`): `sub` (userID), `role`, `typ` (must equal `"session"` — reject `"verification"`), `exp`/`iat` (standard, validated by the JWT library).
- No CI/CD, no provisioned concurrency, no session store, no Datadog — all explicitly out of scope (see spec revision notes, 2026-07-19).
- Test style: external test packages (`package x_test`), `github.com/stretchr/testify` (`assert`/`require`) — matches `video-processor-authentication-api` convention.

---

## File Structure

```
video-processor-authorizer/
├── go.mod, go.sum
├── Dockerfile
├── README.md
├── cmd/authorizer/
│   ├── main.go          # wiring (Secrets Manager load, lambda.Start) + handleRequest (testable, pure)
│   └── main_test.go
├── internal/auth/
│   ├── jwt.go            # ExtractBearerToken, ValidateToken, Claims
│   └── jwt_test.go
├── internal/config/
│   ├── config.go         # Load() reads JWT_SIGNING_KEY_SECRET_NAME / AWS_REGION
│   └── config_test.go
├── pkg/utils/
│   ├── logger.go         # InfoLogger / ErrorLogger (ported from tech-challenge-user-authorizer)
│   └── logger_test.go
└── terraform/
    ├── dev/{main.tf,variables.tf,data.tf,lambda.tf,outputs.tf}
    └── prod/{main.tf,variables.tf,data.tf,lambda.tf,outputs.tf}
```

Also modifies (different repo): `iac-video-processor-infra/{dev,prod}/ecr.tf` — adds `module "ecr_authorizer"`.

---

### Task 1: Go module scaffold + logger

**Files:**
- Create: `video-processor-hackathon/video-processor-authorizer/go.mod`
- Create: `video-processor-hackathon/video-processor-authorizer/pkg/utils/logger.go`
- Test: `video-processor-hackathon/video-processor-authorizer/pkg/utils/logger_test.go`

**Interfaces:**
- Produces: `utils.InfoLogger *log.Logger`, `utils.ErrorLogger *log.Logger` — consumed by Task 4 (`cmd/authorizer/main.go`).

- [ ] **Step 1: Initialize the Go module**

```bash
cd /home/juliovaz/workspaces/video-processor-hackathon/video-processor-authorizer
go mod init github.com/13SOAT-andromeda/video-processor-authorizer
```

Expected: creates `go.mod` with `module github.com/13SOAT-andromeda/video-processor-authorizer` and a `go` directive.

- [ ] **Step 2: Write the failing test for the logger**

Create `pkg/utils/logger_test.go`:

```go
package utils_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/13SOAT-andromeda/video-processor-authorizer/pkg/utils"
)

func TestLoggersInitialized(t *testing.T) {
	assert.NotNil(t, utils.InfoLogger)
	assert.NotNil(t, utils.ErrorLogger)
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd /home/juliovaz/workspaces/video-processor-hackathon/video-processor-authorizer && go test ./pkg/utils/... -v`
Expected: FAIL — `package utils is not in std` / `no required module provides package .../pkg/utils` (package doesn't exist yet), and also missing `testify` dependency.

- [ ] **Step 4: Write the logger implementation**

Create `pkg/utils/logger.go`:

```go
package utils

import (
	"log"
	"os"
)

var (
	InfoLogger  *log.Logger
	ErrorLogger *log.Logger
)

func init() {
	InfoLogger = log.New(os.Stdout, "INFO: ", log.Ldate|log.Ltime|log.Lshortfile)
	ErrorLogger = log.New(os.Stderr, "ERROR: ", log.Ldate|log.Ltime|log.Lshortfile)
}
```

- [ ] **Step 5: Add testify and tidy the module**

```bash
go get github.com/stretchr/testify@v1.11.1
go mod tidy
```

Expected: `go.sum` created/updated, no errors.

- [ ] **Step 6: Run test to verify it passes**

Run: `go test ./pkg/utils/... -v`
Expected: PASS — `--- PASS: TestLoggersInitialized`

- [ ] **Step 7: Commit**

```bash
git add go.mod go.sum pkg/utils/
git commit -m "feat: scaffold Go module and port logger from tech-challenge-user-authorizer"
```

---

### Task 2: `internal/config` package

**Files:**
- Create: `video-processor-hackathon/video-processor-authorizer/internal/config/config.go`
- Test: `video-processor-hackathon/video-processor-authorizer/internal/config/config_test.go`

**Interfaces:**
- Consumes: nothing (reads only `os.Getenv`).
- Produces: `config.Config{JWTSigningKeySecretName string, AWSRegion string}`, `config.Load() Config` — consumed by Task 4 (`cmd/authorizer/main.go`, to read `JWT_SIGNING_KEY_SECRET_NAME`).

- [ ] **Step 1: Write the failing tests**

Create `internal/config/config_test.go`:

```go
package config_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/13SOAT-andromeda/video-processor-authorizer/internal/config"
)

func TestLoad(t *testing.T) {
	t.Setenv("JWT_SIGNING_KEY_SECRET_NAME", "jwt-signing-key-test")
	t.Setenv("AWS_REGION", "sa-east-1")

	cfg := config.Load()

	assert.Equal(t, "jwt-signing-key-test", cfg.JWTSigningKeySecretName)
	assert.Equal(t, "sa-east-1", cfg.AWSRegion)
}

func TestLoad_DefaultRegion(t *testing.T) {
	t.Setenv("JWT_SIGNING_KEY_SECRET_NAME", "jwt-signing-key-test")
	os.Unsetenv("AWS_REGION")

	cfg := config.Load()

	assert.Equal(t, "us-east-1", cfg.AWSRegion)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/... -v`
Expected: FAIL — `no required module provides package .../internal/config`

- [ ] **Step 3: Write the implementation**

Create `internal/config/config.go`:

```go
package config

import "os"

type Config struct {
	JWTSigningKeySecretName string
	AWSRegion               string
}

func Load() Config {
	return Config{
		JWTSigningKeySecretName: os.Getenv("JWT_SIGNING_KEY_SECRET_NAME"),
		AWSRegion:               getEnv("AWS_REGION", "us-east-1"),
	}
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/config/... -v`
Expected: PASS — both `TestLoad` and `TestLoad_DefaultRegion`

- [ ] **Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat: add config package (JWT secret name, AWS region)"
```

---

### Task 3: `internal/auth` package — JWT validation

**Files:**
- Create: `video-processor-hackathon/video-processor-authorizer/internal/auth/jwt.go`
- Test: `video-processor-hackathon/video-processor-authorizer/internal/auth/jwt_test.go`

**Interfaces:**
- Consumes: nothing beyond `github.com/golang-jwt/jwt/v5`.
- Produces: `auth.Claims{UserID string, Role string}`, `auth.ExtractBearerToken(authHeader string) (string, error)`, `auth.ValidateToken(tokenString string, secret []byte) (Claims, error)`, sentinel errors `auth.ErrMissingAuthHeader`, `auth.ErrInvalidAuthHeader`, `auth.ErrInvalidToken`, `auth.ErrTokenNotSession` — all consumed by Task 4 (`cmd/authorizer/main.go`).

- [ ] **Step 1: Write the failing tests**

Create `internal/auth/jwt_test.go`:

```go
package auth_test

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/13SOAT-andromeda/video-processor-authorizer/internal/auth"
)

const testSecret = "test-secret"

func signSessionToken(t *testing.T, secret string, overrides map[string]interface{}) string {
	t.Helper()

	claims := jwt.MapClaims{
		"sub":  "user-123",
		"role": "user",
		"typ":  "session",
		"exp":  time.Now().Add(time.Hour).Unix(),
		"iat":  time.Now().Unix(),
	}
	for k, v := range overrides {
		if v == nil {
			delete(claims, k)
			continue
		}
		claims[k] = v
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	require.NoError(t, err)
	return signed
}

func TestExtractBearerToken(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		token, err := auth.ExtractBearerToken("Bearer abc.def.ghi")
		require.NoError(t, err)
		assert.Equal(t, "abc.def.ghi", token)
	})

	t.Run("missing header", func(t *testing.T) {
		_, err := auth.ExtractBearerToken("")
		assert.ErrorIs(t, err, auth.ErrMissingAuthHeader)
	})

	t.Run("missing bearer prefix", func(t *testing.T) {
		_, err := auth.ExtractBearerToken("abc.def.ghi")
		assert.ErrorIs(t, err, auth.ErrInvalidAuthHeader)
	})

	t.Run("malformed - only Bearer keyword", func(t *testing.T) {
		_, err := auth.ExtractBearerToken("Bearer ")
		assert.ErrorIs(t, err, auth.ErrInvalidAuthHeader)
	})
}

func TestValidateToken(t *testing.T) {
	t.Run("valid session token", func(t *testing.T) {
		tokenString := signSessionToken(t, testSecret, nil)

		claims, err := auth.ValidateToken(tokenString, []byte(testSecret))
		require.NoError(t, err)
		assert.Equal(t, "user-123", claims.UserID)
		assert.Equal(t, "user", claims.Role)
	})

	t.Run("expired token", func(t *testing.T) {
		tokenString := signSessionToken(t, testSecret, map[string]interface{}{
			"exp": time.Now().Add(-time.Hour).Unix(),
		})

		_, err := auth.ValidateToken(tokenString, []byte(testSecret))
		assert.ErrorIs(t, err, auth.ErrInvalidToken)
	})

	t.Run("wrong secret", func(t *testing.T) {
		tokenString := signSessionToken(t, "wrong-secret", nil)

		_, err := auth.ValidateToken(tokenString, []byte(testSecret))
		assert.ErrorIs(t, err, auth.ErrInvalidToken)
	})

	t.Run("wrong signing method (alg none)", func(t *testing.T) {
		claims := jwt.MapClaims{
			"sub": "user-123", "role": "user", "typ": "session",
			"exp": time.Now().Add(time.Hour).Unix(),
		}
		token := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
		tokenString, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
		require.NoError(t, err)

		_, err = auth.ValidateToken(tokenString, []byte(testSecret))
		assert.ErrorIs(t, err, auth.ErrInvalidToken)
	})

	t.Run("missing sub claim", func(t *testing.T) {
		tokenString := signSessionToken(t, testSecret, map[string]interface{}{"sub": nil})

		_, err := auth.ValidateToken(tokenString, []byte(testSecret))
		assert.ErrorIs(t, err, auth.ErrInvalidToken)
	})

	t.Run("missing role claim", func(t *testing.T) {
		tokenString := signSessionToken(t, testSecret, map[string]interface{}{"role": nil})

		_, err := auth.ValidateToken(tokenString, []byte(testSecret))
		assert.ErrorIs(t, err, auth.ErrInvalidToken)
	})

	t.Run("verification token rejected", func(t *testing.T) {
		tokenString := signSessionToken(t, testSecret, map[string]interface{}{"typ": "verification"})

		_, err := auth.ValidateToken(tokenString, []byte(testSecret))
		assert.ErrorIs(t, err, auth.ErrTokenNotSession)
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/auth/... -v`
Expected: FAIL — `no required module provides package .../internal/auth`

- [ ] **Step 3: Write the implementation**

Create `internal/auth/jwt.go`:

```go
package auth

import (
	"errors"
	"fmt"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrMissingAuthHeader = errors.New("authorization header is missing")
	ErrInvalidAuthHeader = errors.New("invalid authorization header format")
	ErrInvalidToken      = errors.New("invalid token")
	ErrTokenNotSession   = errors.New("token is not a session token")
)

// Claims is the subset of JWT claims the authorizer propagates to
// downstream handlers via the API Gateway authorizer context.
type Claims struct {
	UserID string
	Role   string
}

// ExtractBearerToken extracts the token from an "Authorization: Bearer <jwt>"
// header value.
func ExtractBearerToken(authHeader string) (string, error) {
	if authHeader == "" {
		return "", ErrMissingAuthHeader
	}

	const prefix = "Bearer "
	if !strings.HasPrefix(authHeader, prefix) || len(authHeader) <= len(prefix) {
		return "", ErrInvalidAuthHeader
	}

	return authHeader[len(prefix):], nil
}

// ValidateToken parses and validates a JWT signed with a symmetric secret,
// and requires the "typ" claim to be "session" — video-processor-authentication-api
// also issues "verification" tokens with the same signature/format, and those
// must never be accepted here.
func ValidateToken(tokenString string, secret []byte) (Claims, error) {
	token, err := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return secret, nil
	})
	if err != nil {
		return Claims{}, ErrInvalidToken
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return Claims{}, ErrInvalidToken
	}

	typ, _ := claims["typ"].(string)
	if typ != "session" {
		return Claims{}, ErrTokenNotSession
	}

	userID, ok := claims["sub"].(string)
	if !ok || userID == "" {
		return Claims{}, ErrInvalidToken
	}

	role, ok := claims["role"].(string)
	if !ok || role == "" {
		return Claims{}, ErrInvalidToken
	}

	return Claims{UserID: userID, Role: role}, nil
}
```

- [ ] **Step 4: Add golang-jwt and tidy**

```bash
go get github.com/golang-jwt/jwt/v5@v5.3.1
go mod tidy
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/auth/... -v`
Expected: PASS — all subtests of `TestExtractBearerToken` and `TestValidateToken`

- [ ] **Step 6: Commit**

```bash
git add internal/auth/ go.mod go.sum
git commit -m "feat: add JWT validation (session claim, sub/role, typ check)"
```

---

### Task 4: `cmd/authorizer` — REQUEST authorizer handler

**Files:**
- Create: `video-processor-hackathon/video-processor-authorizer/cmd/authorizer/main.go`
- Test: `video-processor-hackathon/video-processor-authorizer/cmd/authorizer/main_test.go`

**Interfaces:**
- Consumes: `auth.ExtractBearerToken`, `auth.ValidateToken`, `auth.Claims` (Task 3); `utils.ErrorLogger` (Task 1); `config.Load() config.Config` (Task 2, gives `JWTSigningKeySecretName`) — `main()` uses this for the secret *name*, then calls Secrets Manager itself to fetch the secret *value* (the name alone isn't the secret).
- Produces: `handleRequest(request events.APIGatewayV2CustomAuthorizerV2Request, jwtSecret []byte) events.APIGatewayV2CustomAuthorizerSimpleResponse` (unexported, package `main`, pure function — no AWS calls, fully unit-testable) and the `main()` Lambda entrypoint.

- [ ] **Step 1: Write the failing tests**

Create `cmd/authorizer/main_test.go`:

```go
package main

import (
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testSecret = "test-secret"

func signToken(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(testSecret))
	require.NoError(t, err)
	return signed
}

func TestHandleRequest(t *testing.T) {
	t.Run("missing authorization header", func(t *testing.T) {
		request := events.APIGatewayV2CustomAuthorizerV2Request{Headers: map[string]string{}}

		response := handleRequest(request, []byte(testSecret))

		assert.False(t, response.IsAuthorized)
	})

	t.Run("invalid bearer format", func(t *testing.T) {
		request := events.APIGatewayV2CustomAuthorizerV2Request{
			Headers: map[string]string{"authorization": "not-bearer"},
		}

		response := handleRequest(request, []byte(testSecret))

		assert.False(t, response.IsAuthorized)
	})

	t.Run("valid session token", func(t *testing.T) {
		claims := jwt.MapClaims{
			"sub": "user-123", "role": "user", "typ": "session",
			"exp": time.Now().Add(time.Hour).Unix(),
		}
		tokenString := signToken(t, claims)
		request := events.APIGatewayV2CustomAuthorizerV2Request{
			Headers: map[string]string{"authorization": "Bearer " + tokenString},
		}

		response := handleRequest(request, []byte(testSecret))

		require.True(t, response.IsAuthorized)
		assert.Equal(t, "user-123", response.Context["userId"])
		assert.Equal(t, "user", response.Context["role"])
	})

	t.Run("expired token", func(t *testing.T) {
		claims := jwt.MapClaims{
			"sub": "user-123", "role": "user", "typ": "session",
			"exp": time.Now().Add(-time.Hour).Unix(),
		}
		tokenString := signToken(t, claims)
		request := events.APIGatewayV2CustomAuthorizerV2Request{
			Headers: map[string]string{"authorization": "Bearer " + tokenString},
		}

		response := handleRequest(request, []byte(testSecret))

		assert.False(t, response.IsAuthorized)
	})

	t.Run("verification token rejected", func(t *testing.T) {
		claims := jwt.MapClaims{
			"sub": "user-123", "role": "user", "typ": "verification",
			"exp": time.Now().Add(time.Hour).Unix(),
		}
		tokenString := signToken(t, claims)
		request := events.APIGatewayV2CustomAuthorizerV2Request{
			Headers: map[string]string{"authorization": "Bearer " + tokenString},
		}

		response := handleRequest(request, []byte(testSecret))

		assert.False(t, response.IsAuthorized)
	})

	t.Run("missing role claim", func(t *testing.T) {
		claims := jwt.MapClaims{
			"sub": "user-123", "typ": "session",
			"exp": time.Now().Add(time.Hour).Unix(),
		}
		tokenString := signToken(t, claims)
		request := events.APIGatewayV2CustomAuthorizerV2Request{
			Headers: map[string]string{"authorization": "Bearer " + tokenString},
		}

		response := handleRequest(request, []byte(testSecret))

		assert.False(t, response.IsAuthorized)
	})
}

func TestRedactToken(t *testing.T) {
	t.Run("long token truncated", func(t *testing.T) {
		result := redactToken("abcdefghijklmnopqrstuvwxyz")
		assert.Equal(t, "abcdef...uvwxyz", result)
	})

	t.Run("short token fully redacted", func(t *testing.T) {
		result := redactToken("short")
		assert.Equal(t, "***", result)
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/authorizer/... -v`
Expected: FAIL — `undefined: handleRequest` / `undefined: redactToken` (package `main` has no code yet besides nothing)

- [ ] **Step 3: Write the implementation**

Create `cmd/authorizer/main.go`:

```go
package main

import (
	"context"
	"fmt"
	"log"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"

	"github.com/13SOAT-andromeda/video-processor-authorizer/internal/auth"
	"github.com/13SOAT-andromeda/video-processor-authorizer/internal/config"
	"github.com/13SOAT-andromeda/video-processor-authorizer/pkg/utils"
)

func main() {
	ctx := context.Background()

	appConfig := config.Load()

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(appConfig.AWSRegion))
	if err != nil {
		log.Fatalf("failed to load AWS config: %v", err)
	}

	// JWT signing key is fetched once here, before lambda.Start — never
	// inside the per-request handler — so it's loaded once per cold start
	// and reused across every warm invocation (same pattern as
	// video-processor-authentication-api/cmd/api/main.go).
	secretsClient := secretsmanager.NewFromConfig(awsCfg)
	secretValue, err := secretsClient.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: &appConfig.JWTSigningKeySecretName,
	})
	if err != nil {
		log.Fatalf("failed to load jwt signing key: %v", err)
	}
	jwtSecret := []byte(*secretValue.SecretString)

	lambda.Start(func(_ context.Context, request events.APIGatewayV2CustomAuthorizerV2Request) (events.APIGatewayV2CustomAuthorizerSimpleResponse, error) {
		return handleRequest(request, jwtSecret), nil
	})
}

func handleRequest(request events.APIGatewayV2CustomAuthorizerV2Request, jwtSecret []byte) events.APIGatewayV2CustomAuthorizerSimpleResponse {
	// API Gateway v2 always lowercases header names.
	authHeader := request.Headers["authorization"]

	tokenString, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		utils.ErrorLogger.Printf("rejected request: %v", err)
		return deny()
	}

	claims, err := auth.ValidateToken(tokenString, jwtSecret)
	if err != nil {
		utils.ErrorLogger.Printf("rejected token %s: %v", redactToken(tokenString), err)
		return deny()
	}

	return events.APIGatewayV2CustomAuthorizerSimpleResponse{
		IsAuthorized: true,
		Context: map[string]interface{}{
			"userId": claims.UserID,
			"role":   claims.Role,
		},
	}
}

func deny() events.APIGatewayV2CustomAuthorizerSimpleResponse {
	return events.APIGatewayV2CustomAuthorizerSimpleResponse{IsAuthorized: false}
}

// redactToken keeps only enough of a JWT for log correlation, never the
// full token (spec section 8).
func redactToken(token string) string {
	const visible = 6
	if len(token) <= visible*2 {
		return "***"
	}
	return fmt.Sprintf("%s...%s", token[:visible], token[len(token)-visible:])
}
```

- [ ] **Step 4: Add remaining AWS SDK dependencies and tidy**

```bash
go get github.com/aws/aws-lambda-go@v1.54.0
go get github.com/aws/aws-sdk-go-v2/config@v1.32.30
go get github.com/aws/aws-sdk-go-v2/service/secretsmanager@v1.43.1
go mod tidy
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./cmd/authorizer/... -v`
Expected: PASS — all subtests of `TestHandleRequest` and `TestRedactToken`

- [ ] **Step 6: Run the full test suite, vet, and build**

```bash
go build ./...
go vet ./...
go test ./... -v
```

Expected: build succeeds, vet clean, all tests PASS across `cmd/authorizer`, `internal/auth`, `internal/config`, `pkg/utils`.

- [ ] **Step 7: Commit**

```bash
git add cmd/authorizer/ go.mod go.sum
git commit -m "feat: wire REQUEST authorizer handler (cmd/authorizer/main.go)"
```

---

### Task 5: Dockerfile

**Files:**
- Create: `video-processor-hackathon/video-processor-authorizer/Dockerfile`

**Interfaces:**
- Consumes: the Go module built in Tasks 1-4 (`./cmd/authorizer`).
- Produces: a container image with entrypoint `bootstrap`, consumed by Task 7/8's `image_uri` (once pushed to ECR — pushing itself is out of scope, see Task 9's README).

- [ ] **Step 1: Write the Dockerfile**

Create `Dockerfile`:

```dockerfile
# syntax=docker/dockerfile:1

FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o /bootstrap ./cmd/authorizer

FROM public.ecr.aws/lambda/provided:al2023-arm64
COPY --from=build /bootstrap ${LAMBDA_TASK_ROOT}/bootstrap
CMD ["bootstrap"]
```

- [ ] **Step 2: Build the image locally to verify the Dockerfile is correct**

```bash
cd /home/juliovaz/workspaces/video-processor-hackathon/video-processor-authorizer
docker build -t video-processor-authorizer:local .
```

Expected: `Successfully tagged video-processor-authorizer:local` (or BuildKit's equivalent final `naming to docker.io/library/video-processor-authorizer:local done`). If Docker isn't available in this environment, skip this step and note it in the task's completion — this is a local sanity check, not a substitute for the terraform apply flow (which needs the image pushed to ECR, out of scope per Task 9's open point).

- [ ] **Step 3: Commit**

```bash
git add Dockerfile
git commit -m "feat: add Dockerfile (Go arm64 binary on Lambda provided.al2023 base)"
```

---

### Task 6: `iac-video-processor-infra` — add `ecr_authorizer`

**Files:**
- Modify: `video-processor-hackathon/iac-video-processor-infra/dev/ecr.tf`
- Modify: `video-processor-hackathon/iac-video-processor-infra/prod/ecr.tf`

**Interfaces:**
- Produces: an ECR repository named `video-processor-authorizer-${var.environment}` in each environment — consumed by Task 7/8's `data.aws_ecr_repository.this`.

- [ ] **Step 1: Add the module block to `dev/ecr.tf`**

Read the current file first to confirm the existing `ecr_users_api` block position:

```bash
cat /home/juliovaz/workspaces/video-processor-hackathon/iac-video-processor-infra/dev/ecr.tf
```

Append this block to the end of `dev/ecr.tf` (same file, new module — mirrors `ecr_users_api` exactly, only the repository name changes):

```hcl

module "ecr_authorizer" {
  source  = "terraform-aws-modules/ecr/aws"
  version = "~> 3.2"

  repository_name = "video-processor-authorizer-${var.environment}"

  repository_image_tag_mutability = "MUTABLE"
  repository_image_scan_on_push   = true

  repository_lifecycle_policy = jsonencode({
    rules = [
      {
        rulePriority = 1
        description  = "Keep last 10 images"
        selection = {
          tagStatus   = "any"
          countType   = "imageCountMoreThan"
          countNumber = 10
        }
        action = {
          type = "expire"
        }
      }
    ]
  })

  tags = {
    Project     = "video-processor"
    Environment = var.environment
  }
}
```

- [ ] **Step 2: Add the identical module block to `prod/ecr.tf`**

Append the exact same block (shown in Step 1) to the end of `prod/ecr.tf`.

- [ ] **Step 3: Validate both environments**

```bash
cd /home/juliovaz/workspaces/video-processor-hackathon/iac-video-processor-infra/dev
terraform init -backend=false
terraform validate

cd /home/juliovaz/workspaces/video-processor-hackathon/iac-video-processor-infra/prod
terraform init -backend=false
terraform validate
```

Expected: `Success! The configuration is valid.` for both.

- [ ] **Step 4: Commit (in the `iac-video-processor-infra` repo)**

```bash
cd /home/juliovaz/workspaces/video-processor-hackathon/iac-video-processor-infra
git add dev/ecr.tf prod/ecr.tf
git commit -m "feat: add ECR repository for video-processor-authorizer"
```

---

### Task 7: `terraform/dev/` — LocalStack environment

**Files:**
- Create: `video-processor-hackathon/video-processor-authorizer/terraform/dev/main.tf`
- Create: `video-processor-hackathon/video-processor-authorizer/terraform/dev/variables.tf`
- Create: `video-processor-hackathon/video-processor-authorizer/terraform/dev/data.tf`
- Create: `video-processor-hackathon/video-processor-authorizer/terraform/dev/lambda.tf`
- Create: `video-processor-hackathon/video-processor-authorizer/terraform/dev/outputs.tf`

**Interfaces:**
- Consumes: `iac-video-processor-infra`'s `video-processor-authorizer-${var.environment}` ECR repo (Task 6) and the pre-existing `jwt-signing-key-${var.environment}` secret.
- Produces: Lambda function `video-processor-authorizer` — consumed by `iac-video-processor-gateway`'s `data.aws_lambda_function.authorizer` (already wired, no change needed there).

- [ ] **Step 1: Write `main.tf`**

```hcl
terraform {
  required_version = ">= 1.7.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.55"
    }
  }

  backend "s3" {
    bucket = "video-processor-bucket-andromeda-local"
    key    = "video-processor-authorizer/terraform.tfstate"
    region = "us-east-1"
    endpoints = {
      s3  = "http://localhost:4566"
      iam = "http://localhost:4566"
      sts = "http://localhost:4566"
    }
    access_key                  = "test"
    secret_key                  = "test"
    skip_credentials_validation = true
    skip_metadata_api_check     = true
    skip_region_validation      = true
    skip_requesting_account_id  = false
    use_path_style              = true
  }
}

provider "aws" {
  region                      = var.region
  access_key                  = "test"
  secret_key                  = "test"
  skip_credentials_validation = true
  skip_metadata_api_check     = true
  skip_requesting_account_id  = false
  s3_use_path_style           = true

  endpoints {
    ecr            = "http://localhost:4566"
    lambda         = "http://localhost:4566"
    iam            = "http://localhost:4566"
    sts            = "http://localhost:4566"
    secretsmanager = "http://localhost:4566"
    cloudwatchlogs = "http://localhost:4566"
  }

  default_tags {
    tags = {
      Terraform   = "true"
      Environment = var.environment
      Project     = "video-processor"
    }
  }
}
```

- [ ] **Step 2: Write `variables.tf`**

```hcl
variable "environment" {
  description = "Environment name (used in resource naming/tags)"
  type        = string
  default     = "localstack"
}

variable "region" {
  description = "AWS region"
  type        = string
  default     = "us-east-1"
}

variable "image_tag" {
  description = "Tag of the authorizer container image already published to ECR"
  type        = string
}
```

- [ ] **Step 3: Write `data.tf`**

```hcl
data "aws_ecr_repository" "this" {
  name = "video-processor-authorizer-${var.environment}"
}

data "aws_secretsmanager_secret" "jwt_signing_key" {
  name = "jwt-signing-key-${var.environment}"
}
```

- [ ] **Step 4: Write `lambda.tf`**

```hcl
module "authorizer_lambda" {
  source  = "terraform-aws-modules/lambda/aws"
  version = "~> 8.8"

  function_name = "video-processor-authorizer"

  package_type   = "Image"
  create_package = false
  image_uri      = "${data.aws_ecr_repository.this.repository_url}:${var.image_tag}"

  architectures = ["arm64"]
  memory_size   = 128
  timeout       = 3

  environment_variables = {
    JWT_SIGNING_KEY_SECRET_NAME = data.aws_secretsmanager_secret.jwt_signing_key.name
    AWS_REGION                  = var.region
  }

  # dev/LocalStack has no AWS Academy sandbox restriction — create a real
  # least-privilege role scoped only to reading the JWT secret (spec
  # section 7). prod/ uses the fixed LabRole instead (see prod/lambda.tf).
  create_role              = true
  attach_policy_statements = true
  policy_statements = {
    read_jwt_secret = {
      effect    = "Allow"
      actions   = ["secretsmanager:GetSecretValue"]
      resources = [data.aws_secretsmanager_secret.jwt_signing_key.arn]
    }
  }

  tags = {
    Project     = "video-processor"
    Environment = var.environment
  }
}
```

- [ ] **Step 5: Write `outputs.tf`**

```hcl
output "lambda_function_arn" {
  description = "The ARN of the authorizer Lambda function"
  value       = module.authorizer_lambda.lambda_function_arn
}

output "lambda_function_name" {
  description = "The name of the authorizer Lambda function"
  value       = module.authorizer_lambda.lambda_function_name
}
```

- [ ] **Step 6: Validate**

```bash
cd /home/juliovaz/workspaces/video-processor-hackathon/video-processor-authorizer/terraform/dev
terraform init -backend=false
terraform validate
```

Expected: `Success! The configuration is valid.`

- [ ] **Step 7: Commit**

```bash
cd /home/juliovaz/workspaces/video-processor-hackathon/video-processor-authorizer
git add terraform/dev/
git commit -m "feat: add terraform/dev (LocalStack) for the authorizer Lambda"
```

---

### Task 8: `terraform/prod/` — AWS environment (LabRole)

**Files:**
- Create: `video-processor-hackathon/video-processor-authorizer/terraform/prod/main.tf`
- Create: `video-processor-hackathon/video-processor-authorizer/terraform/prod/variables.tf`
- Create: `video-processor-hackathon/video-processor-authorizer/terraform/prod/data.tf`
- Create: `video-processor-hackathon/video-processor-authorizer/terraform/prod/lambda.tf`
- Create: `video-processor-hackathon/video-processor-authorizer/terraform/prod/outputs.tf`

**Interfaces:**
- Consumes: same as Task 7, plus `data.aws_iam_role.lab_role` (AWS Academy sandbox fixed role).
- Produces: same Lambda function, in the real AWS account.

- [ ] **Step 1: Write `main.tf`**

```hcl
terraform {
  required_version = ">= 1.7.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.55"
    }
  }

  backend "s3" {
    key     = "video-processor-authorizer/terraform.tfstate"
    region  = "us-east-1"
    encrypt = true
  }
}

provider "aws" {
  region = var.region

  default_tags {
    tags = {
      Terraform   = "true"
      Environment = var.environment
      Project     = "video-processor"
    }
  }
}
```

- [ ] **Step 2: Write `variables.tf`**

```hcl
variable "environment" {
  description = "Environment name (used in resource naming/tags)"
  type        = string
  default     = "prod"
}

variable "region" {
  description = "AWS region"
  type        = string
  default     = "us-east-1"
}

variable "image_tag" {
  description = "Tag of the authorizer container image already published to ECR"
  type        = string
}
```

- [ ] **Step 3: Write `data.tf`**

```hcl
data "aws_ecr_repository" "this" {
  name = "video-processor-authorizer-${var.environment}"
}

data "aws_secretsmanager_secret" "jwt_signing_key" {
  name = "jwt-signing-key-${var.environment}"
}

data "aws_iam_role" "lab_role" {
  name = "LabRole"
}
```

- [ ] **Step 4: Write `lambda.tf`**

```hcl
module "authorizer_lambda" {
  source  = "terraform-aws-modules/lambda/aws"
  version = "~> 8.8"

  function_name = "video-processor-authorizer"

  package_type   = "Image"
  create_package = false
  image_uri      = "${data.aws_ecr_repository.this.repository_url}:${var.image_tag}"

  architectures = ["arm64"]
  memory_size   = 128
  timeout       = 3

  environment_variables = {
    JWT_SIGNING_KEY_SECRET_NAME = data.aws_secretsmanager_secret.jwt_signing_key.name
    AWS_REGION                  = var.region
  }

  # AWS Academy sandbox does not allow creating custom IAM roles/policies in
  # prod — reuse the fixed LabRole, same pattern already used in
  # iac-video-processor-infra/prod/eks.tf.
  create_role = false
  lambda_role = data.aws_iam_role.lab_role.arn

  tags = {
    Project     = "video-processor"
    Environment = var.environment
  }
}
```

- [ ] **Step 5: Write `outputs.tf`**

```hcl
output "lambda_function_arn" {
  description = "The ARN of the authorizer Lambda function"
  value       = module.authorizer_lambda.lambda_function_arn
}

output "lambda_function_name" {
  description = "The name of the authorizer Lambda function"
  value       = module.authorizer_lambda.lambda_function_name
}
```

- [ ] **Step 6: Validate**

```bash
cd /home/juliovaz/workspaces/video-processor-hackathon/video-processor-authorizer/terraform/prod
terraform init -backend=false
terraform validate
```

Expected: `Success! The configuration is valid.`

- [ ] **Step 7: Commit**

```bash
cd /home/juliovaz/workspaces/video-processor-hackathon/video-processor-authorizer
git add terraform/prod/
git commit -m "feat: add terraform/prod (LabRole) for the authorizer Lambda"
```

---

### Task 9: README + final verification

**Files:**
- Create: `video-processor-hackathon/video-processor-authorizer/README.md`

**Interfaces:**
- Consumes: nothing new — documents the whole repo built in Tasks 1-8.
- Produces: nothing consumed by other tasks — this is the last task.

- [ ] **Step 1: Write `README.md`**

```markdown
# video-processor-authorizer

Lambda `REQUEST` authorizer for the `video-processor` API Gateway. Validates
the JWT session token issued by `video-processor-authentication-api` and
returns `{userId, role}` in the authorizer context. No database, no session
store — 100% stateless, decision based only on the signed JWT claims.

See `docs/superpowers/specs/2026-07-11-authorizer-design.md` for the full
design (contract, business rules, test matrix, Terraform decisions).

## Local development

```bash
go build ./...
go vet ./...
go test ./... -v
```

## Building and deploying

Terraform does **not** build or push the container image — it only
references an already-published tag via `var.image_tag`. Build and push
manually before running `terraform apply`:

```bash
docker build -t video-processor-authorizer:<tag> .
docker tag video-processor-authorizer:<tag> <ecr-repository-url>:<tag>
docker push <ecr-repository-url>:<tag>
```

Then, from `terraform/dev/` or `terraform/prod/`:

```bash
terraform init
terraform apply -var image_tag=<tag>
```

**Open point (2026-07-19):** how the image reaches ECR without a CI
pipeline (this manual flow vs. a future GitHub Actions pipeline) is a team
decision, tracked in the design spec — not resolved by this implementation.

## Environments

- `terraform/dev/` — targets LocalStack (`http://localhost:4566`). Creates
  its own least-privilege IAM role (`secretsmanager:GetSecretValue` only).
- `terraform/prod/` — targets real AWS. Reuses the fixed `LabRole` (AWS
  Academy sandbox forbids custom IAM roles/policies).
```

- [ ] **Step 2: Run the full verification suite one more time**

```bash
cd /home/juliovaz/workspaces/video-processor-hackathon/video-processor-authorizer
go build ./...
go vet ./...
go test ./... -v
```

Expected: build succeeds, vet clean, every test PASSes.

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "docs: add README (build, deploy, environments)"
```

---

## Self-Review Notes

- **Spec coverage:** §1-5 (behavior/contract) → Tasks 3-4. §6 (dependencies: secret, jwt library) → Task 3-4 (library), Task 7-8 (secret data source). §7 (IAM) → Tasks 7-8. §8 (observability: no full token in logs) → Task 4 (`redactToken`). §9.1 (porting table) → Tasks 1, 3, 4 (issuer check removed, session store removed, Datadog removed). §9.2 (Terraform/ECR/Image) → Tasks 5-8. §9.3 (dependencies) → Task 6.
- **No placeholders:** every step has complete, runnable code — no `TBD`/`implement later`.
- **Type consistency checked:** `auth.Claims{UserID, Role}` (Task 3) matches the fields read in `handleRequest` (Task 4, `claims.UserID`/`claims.Role`); `config.Config{JWTSigningKeySecretName, AWSRegion}` (Task 2) is consumed by `cmd/authorizer/main.go` (Task 4) via `config.Load()` — `appConfig.JWTSigningKeySecretName` is passed as `SecretId` to Secrets Manager and `appConfig.AWSRegion` to `awsconfig.WithRegion`, so both fields have a real caller (fixed during self-review — an earlier draft had `main()` reading `os.Getenv` directly, leaving `config.Load()` dead code).
