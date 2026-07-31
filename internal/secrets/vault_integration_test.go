package secrets

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	vault "github.com/hashicorp/vault/api"
)

const (
	vaultDefaultAddr  = "http://127.0.0.1:8200"
	vaultDefaultToken = "root"
)

type vaultTestTransport struct {
	base http.RoundTripper

	mu          sync.Mutex
	secretReads int
	fail        atomic.Bool
}

func (t *vaultTestTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if strings.Contains(r.URL.Path, "/v1/secret/data/taskqueue/") {
		t.mu.Lock()
		t.secretReads++
		t.mu.Unlock()
	}
	if t.fail.Load() {
		return nil, errors.New("simulated: vault unreachable")
	}
	return t.base.RoundTrip(r)
}

func (t *vaultTestTransport) secretReadCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.secretReads
}

type vaultHarness struct {
	addr      string
	token     string
	root      *vault.Client
	roleID    string
	secretID  string
	transport *vaultTestTransport
}

func vaultAddrAndToken(t *testing.T) (string, string) {
	t.Helper()
	addr := os.Getenv("VAULT_ADDR")
	if addr == "" {
		addr = vaultDefaultAddr
	}
	token := os.Getenv("VAULT_DEV_ROOT_TOKEN_ID")
	if token == "" {
		token = vaultDefaultToken
	}
	return addr, token
}

func setupVault(t *testing.T) (*vaultHarness, error) {
	t.Helper()
	addr, token := vaultAddrAndToken(t)

	if err := probeVault(addr); err != nil {
		return nil, err
	}

	cfg := vault.DefaultConfig()
	cfg.Address = addr
	root, err := vault.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("new root client: %w", err)
	}
	root.SetToken(token)

	deadline := time.Now().Add(15 * time.Second)
	for {
		health, err := root.Sys().Health()
		if err == nil && health != nil && !health.Sealed {
			break
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("vault not unsealed at %s: %w", addr, err)
		}
		time.Sleep(250 * time.Millisecond)
	}

	if _, err := root.Logical().Write("sys/auth/approle", map[string]interface{}{"type": "approle"}); err != nil {
		if !strings.Contains(err.Error(), "in use") {
			return nil, fmt.Errorf("enable approle: %w", err)
		}
	}

	policy := `path "secret/data/taskqueue/*" { capabilities = ["read"] }`
	if _, err := root.Logical().Write("sys/policies/acl/taskqueue-test", map[string]interface{}{"policy": policy}); err != nil {
		return nil, fmt.Errorf("write policy: %w", err)
	}

	if _, err := root.Logical().Write("auth/approle/role/taskqueue-test", map[string]interface{}{
		"token_policies": []string{"taskqueue-test"},
	}); err != nil {
		return nil, fmt.Errorf("create role: %w", err)
	}

	roleResp, err := root.Logical().Read("auth/approle/role/taskqueue-test/role-id")
	if err != nil {
		return nil, fmt.Errorf("read role-id: %w", err)
	}
	roleID, _ := roleResp.Data["role_id"].(string)

	secretResp, err := root.Logical().Write("auth/approle/role/taskqueue-test/secret-id", nil)
	if err != nil {
		return nil, fmt.Errorf("create secret-id: %w", err)
	}
	secretID, _ := secretResp.Data["secret_id"].(string)

	return &vaultHarness{addr: addr, token: token, root: root, roleID: roleID, secretID: secretID}, nil
}

// probeVault does a fast reachability check so the skip path is near-instant
// when no Vault server is running.
func probeVault(addr string) error {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(addr + "/v1/sys/health")
	if err != nil {
		return fmt.Errorf("vault not reachable at %s: %w", addr, err)
	}
	defer resp.Body.Close()
	return nil
}

func requireVault(t *testing.T) *vaultHarness {
	t.Helper()
	h, err := setupVault(t)
	if err != nil {
		t.Skipf("skipping: vault not available: %v", err)
	}
	return h
}

func (h *vaultHarness) writeSecret(t *testing.T, tenant, apiKey string) {
	t.Helper()
	if _, err := h.root.Logical().Write(fmt.Sprintf("secret/data/taskqueue/%s", tenant), map[string]interface{}{
		"data": map[string]interface{}{"api_key": apiKey},
	}); err != nil {
		t.Fatalf("write secret %s: %v", tenant, err)
	}
}

func (h *vaultHarness) provider(t *testing.T, ttl time.Duration) (*VaultSecretsProvider, *vaultTestTransport) {
	t.Helper()
	tr := &vaultTestTransport{base: http.DefaultTransport}

	cfg := vault.DefaultConfig()
	cfg.Address = h.addr
	cfg.HttpClient.Transport = tr
	client, err := vault.NewClient(cfg)
	if err != nil {
		t.Fatalf("new provider client: %v", err)
	}
	client.SetToken(h.token)
	client.SetMaxRetries(0)

	p := &VaultSecretsProvider{
		client:        client,
		roleID:        h.roleID,
		secretID:      h.secretID,
		ttl:           ttl,
		refreshWindow: defaultRefreshWindow,
	}
	return p, tr
}

func TestVaultAppRoleLoginAndFetch(t *testing.T) {
	h := requireVault(t)
	h.writeSecret(t, "tenant-login", "approle-secret-value")

	p, err := NewVaultSecretsProvider(h.addr, h.roleID, h.secretID)
	if err != nil {
		t.Fatalf("NewVaultSecretsProvider: %v", err)
	}

	val, err := p.GetSecret(context.Background(), "tenant-login")
	if err != nil {
		t.Fatalf("GetSecret: %v", err)
	}
	if val != "approle-secret-value" {
		t.Fatalf("expected approle-secret-value, got %q", val)
	}
}

func TestVaultSecretCachedBeforeTTL(t *testing.T) {
	h := requireVault(t)
	h.writeSecret(t, "tenant-cached", "value-v1")

	p, tr := h.provider(t, defaultTTL)
	ctx := context.Background()

	v1, err := p.GetSecret(ctx, "tenant-cached")
	if err != nil {
		t.Fatalf("first GetSecret: %v", err)
	}
	if v1 != "value-v1" {
		t.Fatalf("expected value-v1, got %q", v1)
	}

	v2, err := p.GetSecret(ctx, "tenant-cached")
	if err != nil {
		t.Fatalf("second GetSecret: %v", err)
	}
	if v2 != v1 {
		t.Fatalf("expected cached value %q, got %q", v1, v2)
	}

	h.writeSecret(t, "tenant-cached", "value-v2")

	v3, err := p.GetSecret(ctx, "tenant-cached")
	if err != nil {
		t.Fatalf("third GetSecret: %v", err)
	}
	if v3 != "value-v1" {
		t.Fatalf("expected stale cached value-v1 before TTL expiry, got %q", v3)
	}
	if got := tr.secretReadCount(); got != 1 {
		t.Fatalf("expected exactly 1 Vault read before TTL expiry, got %d", got)
	}
}

func TestVaultSecretRefetchedAfterTTL(t *testing.T) {
	h := requireVault(t)
	h.writeSecret(t, "tenant-refresh", "value-v1")

	p, tr := h.provider(t, 300*time.Millisecond)
	ctx := context.Background()

	v1, err := p.GetSecret(ctx, "tenant-refresh")
	if err != nil {
		t.Fatalf("first GetSecret: %v", err)
	}
	if v1 != "value-v1" {
		t.Fatalf("expected value-v1, got %q", v1)
	}

	h.writeSecret(t, "tenant-refresh", "value-v2")
	time.Sleep(600 * time.Millisecond)

	v2, err := p.GetSecret(ctx, "tenant-refresh")
	if err != nil {
		t.Fatalf("GetSecret after TTL expiry: %v", err)
	}
	if v2 != "value-v2" {
		t.Fatalf("expected refreshed value-v2 after TTL expiry, got %q", v2)
	}
	if got := tr.secretReadCount(); got != 2 {
		t.Fatalf("expected 2 Vault reads (initial + after expiry), got %d", got)
	}
}

func TestVaultServesStaleWhenRefreshFails(t *testing.T) {
	h := requireVault(t)
	h.writeSecret(t, "tenant-stale", "stale-value")

	p, tr := h.provider(t, 2*time.Second)
	ctx := context.Background()

	v1, err := p.GetSecret(ctx, "tenant-stale")
	if err != nil {
		t.Fatalf("first GetSecret: %v", err)
	}

	tr.fail.Store(true)
	time.Sleep(1 * time.Second)

	v2, err := p.GetSecret(ctx, "tenant-stale")
	if err != nil {
		t.Fatalf("expected stale cached value when Vault unreachable within TTL, got error: %v", err)
	}
	if v2 != v1 {
		t.Fatalf("expected stale value %q, got %q", v1, v2)
	}

	deadline := time.Now().Add(3 * time.Second)
	for tr.secretReadCount() < 2 {
		if time.Now().After(deadline) {
			t.Fatalf("background refresh attempt never reached Vault: count=%d", tr.secretReadCount())
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestVaultErrorsAfterExpiryWhenUnavailable(t *testing.T) {
	h := requireVault(t)
	h.writeSecret(t, "tenant-expired", "expiring-value")

	p, tr := h.provider(t, 300*time.Millisecond)
	ctx := context.Background()

	if _, err := p.GetSecret(ctx, "tenant-expired"); err != nil {
		t.Fatalf("first GetSecret: %v", err)
	}

	tr.fail.Store(true)
	time.Sleep(600 * time.Millisecond)

	_, err := p.GetSecret(ctx, "tenant-expired")
	if err == nil {
		t.Fatal("expected error after full TTL expiry when Vault is unreachable (no stale fallback)")
	}
	if got := tr.secretReadCount(); got != 2 {
		t.Fatalf("expected a fetch attempt after expiry, got %d reads", got)
	}
}

func TestVaultConcurrentColdMissSingleFetch(t *testing.T) {
	h := requireVault(t)
	h.writeSecret(t, "tenant-concurrent-miss", "conc-value")

	p, tr := h.provider(t, defaultTTL)
	ctx := context.Background()

	const n = 20
	var wg sync.WaitGroup
	errs := make([]error, n)
	vals := make([]string, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			vals[i], errs[i] = p.GetSecret(ctx, "tenant-concurrent-miss")
		}(i)
	}
	wg.Wait()

	for i := 0; i < n; i++ {
		if errs[i] != nil {
			t.Fatalf("reader %d: %v", i, errs[i])
		}
		if vals[i] != "conc-value" {
			t.Fatalf("reader %d: expected conc-value, got %q", i, vals[i])
		}
	}
	if got := tr.secretReadCount(); got != 1 {
		t.Fatalf("expected exactly 1 Vault read under concurrent cold-miss, got %d", got)
	}
}

func TestVaultConcurrentRefreshNoStampede(t *testing.T) {
	h := requireVault(t)
	h.writeSecret(t, "tenant-concurrent-refresh", "refresh-value")

	p, tr := h.provider(t, 2*time.Second)
	ctx := context.Background()

	if _, err := p.GetSecret(ctx, "tenant-concurrent-refresh"); err != nil {
		t.Fatalf("first GetSecret: %v", err)
	}

	time.Sleep(1 * time.Second)

	const n = 20
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = p.GetSecret(ctx, "tenant-concurrent-refresh")
		}(i)
	}
	wg.Wait()

	for i := 0; i < n; i++ {
		if errs[i] != nil {
			t.Fatalf("reader %d: %v", i, errs[i])
		}
	}

	deadline := time.Now().Add(3 * time.Second)
	for tr.secretReadCount() < 2 {
		if time.Now().After(deadline) {
			t.Fatalf("background refresh never completed: count=%d", tr.secretReadCount())
		}
		time.Sleep(50 * time.Millisecond)
	}

	time.Sleep(200 * time.Millisecond)
	if got := tr.secretReadCount(); got != 2 {
		t.Fatalf("expected exactly 1 additional refresh read despite %d concurrent readers, got %d", n, got)
	}
}
