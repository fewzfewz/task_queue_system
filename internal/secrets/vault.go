package secrets

import (
	"context"
	"fmt"
	"sync"
	"time"

	vault "github.com/hashicorp/vault/api"
)

const (
	defaultTTL           = 5 * time.Minute
	defaultRefreshWindow = 30 * time.Second
)

type cachedSecret struct {
	value     string
	expiresAt time.Time
}

type inflightFetch struct {
	done  chan struct{}
	value string
	err   error
}

// VaultSecretsProvider fetches secrets from HashiCorp Vault with AppRole auth and caching.
type VaultSecretsProvider struct {
	client        *vault.Client
	roleID        string
	secretID      string
	cache         sync.Map
	ttl           time.Duration
	refreshWindow time.Duration

	inflight sync.Map
	loginMu  sync.Mutex
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
		client:        client,
		roleID:        roleID,
		secretID:      secretID,
		ttl:           defaultTTL,
		refreshWindow: defaultRefreshWindow,
	}

	// Login immediately to verify credentials
	if err := p.login(); err != nil {
		return nil, fmt.Errorf("vault: initial login failed: %w", err)
	}

	return p, nil
}

func (p *VaultSecretsProvider) login() error {
	p.loginMu.Lock()
	defer p.loginMu.Unlock()

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
		// If secret is close to expiry (within refreshWindow), we'll fetch a fresh
		// one in the background but return the cached one now (stale-while-revalidate).
		if time.Now().Before(s.expiresAt) {
			if time.Until(s.expiresAt) < p.refreshWindow {
				go p.refresh(tenantID)
			}
			return s.value, nil
		}
	}

	// 2. Cache Miss or Expired
	return p.fetch(ctx, tenantID)
}

// refresh re-fetches the secret in the background, coalesced with any in-flight
// fetch. Errors are intentionally not surfaced here; a failed refresh leaves the
// stale cached entry in place until full expiry.
func (p *VaultSecretsProvider) refresh(tenantID string) {
	_, _ = p.fetch(context.Background(), tenantID)
}

// fetch returns a cached-fresh secret, coalescing concurrent requests for the
// same tenant so only one Vault read happens (singleflight).
func (p *VaultSecretsProvider) fetch(ctx context.Context, tenantID string) (string, error) {
	if v, ok := p.inflight.Load(tenantID); ok {
		return p.awaitFetch(ctx, v.(*inflightFetch))
	}

	f := &inflightFetch{done: make(chan struct{})}
	actual, loaded := p.inflight.LoadOrStore(tenantID, f)
	if loaded {
		return p.awaitFetch(ctx, actual.(*inflightFetch))
	}

	value, err := p.fetchAndCache(ctx, tenantID)

	f.value, f.err = value, err
	close(f.done)
	p.inflight.Delete(tenantID)
	return value, err
}

func (p *VaultSecretsProvider) awaitFetch(ctx context.Context, f *inflightFetch) (string, error) {
	select {
	case <-f.done:
		return f.value, f.err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (p *VaultSecretsProvider) fetchAndCache(ctx context.Context, tenantID string) (string, error) {
	path := fmt.Sprintf("secret/data/taskqueue/%s", tenantID)

	secret, err := p.client.Logical().ReadWithContext(ctx, path)
	if err != nil {
		// If token expired, try to re-login once
		if err := p.login(); err == nil {
			secret, err = p.client.Logical().ReadWithContext(ctx, path)
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
