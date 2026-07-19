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
