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
