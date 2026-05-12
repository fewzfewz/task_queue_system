package secrets

import (
	"context"
	"os"
)

// EnvSecretsProvider reads secrets from environment variables.
// Use this for local development or legacy deployments.
type EnvSecretsProvider struct{}

func NewEnvSecretsProvider() *EnvSecretsProvider {
	return &EnvSecretsProvider{}
}

func (p *EnvSecretsProvider) GetSecret(_ context.Context, key string) (string, error) {
	return os.Getenv(key), nil
}
