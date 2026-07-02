package secrets

import (
	"context"
	"os"
	"testing"
)

func TestEnvSecretsProvider_GetSecret(t *testing.T) {
	p := NewEnvSecretsProvider()

	os.Setenv("TEST_SECRET_KEY", "super-secret-value")
	t.Cleanup(func() { os.Unsetenv("TEST_SECRET_KEY") })

	val, err := p.GetSecret(context.Background(), "TEST_SECRET_KEY")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "super-secret-value" {
		t.Fatalf("expected super-secret-value, got %s", val)
	}
}

func TestEnvSecretsProvider_GetSecretMissing(t *testing.T) {
	p := NewEnvSecretsProvider()

	val, err := p.GetSecret(context.Background(), "NONEXISTENT_VAR_XYZ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "" {
		t.Fatalf("expected empty string, got %s", val)
	}
}

func TestEnvSecretsProviderImplementsInterface(t *testing.T) {
	var p SecretsProvider = NewEnvSecretsProvider()
	if p == nil {
		t.Fatal("EnvSecretsProvider should implement SecretsProvider")
	}
}
