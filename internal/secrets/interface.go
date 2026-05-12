package secrets

import (
	"context"
)

// SecretsProvider defines the contract for fetching sensitive configuration
// at runtime from various backends (Vault, ENV, AWS SM, etc.)
type SecretsProvider interface {
	// GetSecret retrieves the string value of a secret by its key.
	// Typically key would be the tenantID or a global config key.
	GetSecret(ctx context.Context, key string) (string, error)
}
