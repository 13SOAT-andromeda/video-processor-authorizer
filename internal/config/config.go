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
