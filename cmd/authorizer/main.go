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
