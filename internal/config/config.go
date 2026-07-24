package config

import "os"

type Config struct {
	JWTSigningKeySecretName string
	AWSRegion               string

	// DDService/DDEnv/DDVersion configure the Datadog tracer (see
	// cmd/authorizer/main.go). DDVersion is deliberately left empty by
	// default rather than "unknown" here — the Terraform module fills it
	// from the deployed image tag; an empty string just means the tracer
	// omits the version tag on spans/logs.
	DDService string
	DDEnv     string
	DDVersion string
}

func Load() Config {
	return Config{
		JWTSigningKeySecretName: os.Getenv("JWT_SIGNING_KEY_SECRET_NAME"),
		AWSRegion:               getEnv("AWS_REGION", "us-east-1"),
		DDService:               getEnv("DD_SERVICE", "video-processor-authorizer"),
		DDEnv:                   os.Getenv("DD_ENV"),
		DDVersion:               os.Getenv("DD_VERSION"),
	}
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}
