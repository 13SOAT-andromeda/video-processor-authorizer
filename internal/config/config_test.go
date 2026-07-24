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

func TestLoad_DatadogDefaults(t *testing.T) {
	t.Setenv("JWT_SIGNING_KEY_SECRET_NAME", "jwt-signing-key-test")
	os.Unsetenv("DD_SERVICE")
	os.Unsetenv("DD_ENV")
	os.Unsetenv("DD_VERSION")

	cfg := config.Load()

	assert.Equal(t, "video-processor-authorizer", cfg.DDService)
	assert.Empty(t, cfg.DDEnv)
	assert.Empty(t, cfg.DDVersion)
}

func TestLoad_DatadogFromEnv(t *testing.T) {
	t.Setenv("JWT_SIGNING_KEY_SECRET_NAME", "jwt-signing-key-test")
	t.Setenv("DD_SERVICE", "custom-service")
	t.Setenv("DD_ENV", "dev")
	t.Setenv("DD_VERSION", "1.2.3")

	cfg := config.Load()

	assert.Equal(t, "custom-service", cfg.DDService)
	assert.Equal(t, "dev", cfg.DDEnv)
	assert.Equal(t, "1.2.3", cfg.DDVersion)
}
