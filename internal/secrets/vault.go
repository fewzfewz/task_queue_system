package secrets

import (
	"context"
	"fmt"
	"sync"
	"time"

	vault "github.com/hashicorp/vault/api"
)

type cachedSecret struct {
	value     string
	expiresAt time.Time
}

// VaultSecretsProvider fetches secrets from HashiCorp Vault with AppRole auth and caching.
type VaultSecretsProvider struct {
	client *vault.Client
	roleID string
	secretID string
	cache  sync.Map
	ttl    time.Duration
}

// NewVaultSecretsProvider initialises the Vault client and sets up AppRole authentication.
func NewVaultSecretsProvider(address, roleID, secretID string) (*VaultSecretsProvider, error) {
	config := vault.DefaultConfig()
	config.Address = address

	client, err := vault.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("vault: failed to create client: %w", err)
	}

	p := &VaultSecretsProvider{
		client:   client,
		roleID:   roleID,
		secretID: secretID,
		ttl:      5 * time.Minute,
	}

	// Login immediately to verify credentials
	if err := p.login(); err != nil {
		return nil, fmt.Errorf("vault: initial login failed: %w", err)
	}

	return p, nil
}

func (p *VaultSecretsProvider) login() error {
	data := map[string]interface{}{
		"role_id":   p.roleID,
		"secret_id": p.secretID,
	}

	resp, err := p.client.Logical().Write("auth/approle/login", data)
	if err != nil {
		return err
	}

	if resp.Auth == nil {
		return fmt.Errorf("vault: no auth info returned")
	}

	p.client.SetToken(resp.Auth.ClientToken)
	return nil
}

func (p *VaultSecretsProvider) GetSecret(ctx context.Context, tenantID string) (string, error) {
	// 1. Check Cache
	if val, ok := p.cache.Load(tenantID); ok {
		s := val.(cachedSecret)
		// If secret is close to expiry (within 30s), we'll fetch a fresh one but return the cached one now
		if time.Now().Before(s.expiresAt) {
			if time.Until(s.expiresAt) < 30*time.Second {
				go p.fetchAndCache(context.Background(), tenantID)
			}
			return s.value, nil
		}
	}

	// 2. Cache Miss or Expired
	return p.fetchAndCache(ctx, tenantID)
}

func (p *VaultSecretsProvider) fetchAndCache(ctx context.Context, tenantID string) (string, error) {
	path := fmt.Sprintf("secret/data/taskqueue/%s", tenantID)
	
	secret, err := p.client.Logical().Read(path)
	if err != nil {
		// If token expired, try to re-login once
		if err := p.login(); err == nil {
			secret, err = p.client.Logical().Read(path)
		}
		if err != nil {
			return "", fmt.Errorf("vault: failed to read path %s: %w", path, err)
		}
	}

	if secret == nil || secret.Data == nil {
		return "", fmt.Errorf("vault: secret not found at path %s", path)
	}

	// Vault KV v2 structure has data inside 'data' field
	data, ok := secret.Data["data"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("vault: invalid KV v2 structure at path %s", path)
	}

	apiKey, ok := data["api_key"].(string)
	if !ok {
		return "", fmt.Errorf("vault: api_key not found in secret data at %s", path)
	}

	p.cache.Store(tenantID, cachedSecret{
		value:     apiKey,
		expiresAt: time.Now().Add(p.ttl),
	})

	return apiKey, nil
}
