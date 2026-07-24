package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	awstrace "github.com/DataDog/dd-trace-go/contrib/aws/aws-sdk-go-v2/v2/aws"
	ddlambda "github.com/DataDog/dd-trace-go/contrib/aws/datadog-lambda-go/v2"
	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"
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
		utils.ErrorLogger.Printf("failed to load AWS config: %v", err)
		os.Exit(1)
	}
	// Instruments every AWS SDK call made with awsCfg (here: Secrets
	// Manager) as a span nested under the current trace — gives call-level
	// latency/errors in APM without needing any AWS-side IAM integration.
	awstrace.AppendMiddleware(&awsCfg)

	// JWT signing key is fetched once here, before lambda.Start — never
	// inside the per-request handler — so it's loaded once per cold start
	// and reused across every warm invocation (same pattern as
	// video-processor-authentication-api/cmd/api/main.go).
	secretsClient := secretsmanager.NewFromConfig(awsCfg)
	secretValue, err := secretsClient.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: &appConfig.JWTSigningKeySecretName,
	})
	if err != nil {
		utils.ErrorLogger.Printf("failed to load jwt signing key: %v", err)
		os.Exit(1)
	}
	jwtSecret := []byte(*secretValue.SecretString)

	// ddlambda.WrapFunction lazily calls tracer.Start(...) itself on the
	// first invocation of a cold execution environment (see
	// internal/trace/listener.go in
	// github.com/DataDog/dd-trace-go/contrib/aws/datadog-lambda-go/v2) and
	// flushes/tears the tracer down around every invocation to survive
	// freeze/thaw between them. Unlike a long-running service (contrast
	// with video-processor-users-api/cmd/api/main.go, which calls
	// tracer.Start explicitly before serving requests), this Lambda must
	// NOT also call tracer.Start itself — that would race with the
	// wrapper's own lazy start. Service/env/version are instead threaded
	// in via ddlambda.Config.TracerOptions, which the wrapper appends to
	// its own tracer.Start call.
	ddCfg := &ddlambda.Config{
		DDTraceEnabled: true,
		TracerOptions: []tracer.StartOption{
			tracer.WithService(appConfig.DDService),
			tracer.WithEnv(appConfig.DDEnv),
			tracer.WithServiceVersion(appConfig.DDVersion),
		},
	}

	handler := func(ctx context.Context, request events.APIGatewayV2CustomAuthorizerV2Request) (events.APIGatewayV2CustomAuthorizerSimpleResponse, error) {
		return handleRequest(ctx, request, jwtSecret), nil
	}

	lambda.Start(ddlambda.WrapFunction(handler, ddCfg))
}

func handleRequest(ctx context.Context, request events.APIGatewayV2CustomAuthorizerV2Request, jwtSecret []byte) events.APIGatewayV2CustomAuthorizerSimpleResponse {
	// API Gateway v2 always lowercases header names.
	authHeader := request.Headers["authorization"]

	tokenString, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		utils.ErrorLogger.PrintfContext(ctx, "rejected request: %v", err)
		ddlambda.Metric("video_processor.authorizer.validations", 1, "result:deny", "reason:"+denyReason(err))
		return deny()
	}

	claims, err := auth.ValidateToken(tokenString, jwtSecret)
	if err != nil {
		utils.ErrorLogger.PrintfContext(ctx, "rejected token %s: %v", redactToken(tokenString), err)
		ddlambda.Metric("video_processor.authorizer.validations", 1, "result:deny", "reason:"+denyReason(err))
		return deny()
	}

	ddlambda.Metric("video_processor.authorizer.validations", 1, "result:allow")

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

// denyReason maps a validation error to a low-cardinality tag value for the
// video_processor.authorizer.validations metric.
func denyReason(err error) string {
	switch {
	case errors.Is(err, auth.ErrMissingAuthHeader):
		return "missing_auth_header"
	case errors.Is(err, auth.ErrInvalidAuthHeader):
		return "invalid_auth_header"
	case errors.Is(err, auth.ErrTokenNotSession):
		return "token_not_session"
	default:
		return "invalid_token"
	}
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
