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
