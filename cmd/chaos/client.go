package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/redis/go-redis/v9"
)

// JobStatus values mirrored from internal/jobs.
const (
	statusPending    = "pending"
	statusProcessing = "processing"
	statusCompleted  = "completed"
	statusFailed     = "failed"
)

// jobResponse mirrors the API's dto.JobResponse fields we care about.
type jobResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// liveClient talks to the deployed stack: HTTP API, Redis, and optionally Docker.
type liveClient struct {
	apiURL     string
	apiKey     string
	redis      *redis.Client
	docker     *client.Client
	dockerErr  error
	httpClient *http.Client
	cfg        *Config
}

func newLiveClient(cfg *Config) (*liveClient, error) {
	ctx := context.Background()

	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis %s unreachable: %w", cfg.RedisAddr, err)
	}

	dc, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	var dockerErr error
	if err == nil {
		if _, pingErr := dc.Ping(ctx); pingErr != nil {
			dockerErr = fmt.Errorf("docker daemon unreachable: %w", pingErr)
		}
	} else {
		dockerErr = fmt.Errorf("docker client: %w", err)
	}

	return &liveClient{
		apiURL:     cfg.APIURL,
		apiKey:     cfg.APIKey,
		redis:      rdb,
		docker:     dc,
		dockerErr:  dockerErr,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		cfg:        cfg,
	}, nil
}

func (c *liveClient) Close() {
	_ = c.redis.Close()
	if c.docker != nil {
		_ = c.docker.Close()
	}
}

// requireDocker returns the docker client or a descriptive error.
func (c *liveClient) requireDocker() (*client.Client, error) {
	if c.docker == nil || c.dockerErr != nil {
		return nil, fmt.Errorf("scenario requires Docker, but %v", c.dockerErr)
	}
	return c.docker, nil
}

// ── API ────────────────────────────────────────────────────────────────────────

func (c *liveClient) doJSON(ctx context.Context, method, path string, body, out interface{}) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.apiURL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.apiKey != "" {
		req.Header.Set("X-API-Key", c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s: status %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if out != nil && len(data) > 0 {
		return json.Unmarshal(data, out)
	}
	return nil
}

// ready returns true if the API reports ready.
func (c *liveClient) apiReady(ctx context.Context) bool {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, c.apiURL+"/readyz", nil)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode == http.StatusOK
}

// createJob enqueues a job via the HTTP API and returns its ID.
func (c *liveClient) createJob(ctx context.Context, payload map[string]interface{}) (string, error) {
	body := map[string]interface{}{
		"type":        c.cfg.JobType,
		"payload":     payload,
		"priority":    c.cfg.Priority,
		"max_retries": c.cfg.MaxRetries,
		"tenant_id":   "chaos-cli",
	}
	if c.cfg.TimeoutSec > 0 {
		body["timeout"] = c.cfg.TimeoutSec
	}
	var job jobResponse
	if err := c.doJSON(ctx, http.MethodPost, "/jobs", body, &job); err != nil {
		return "", err
	}
	if job.ID == "" {
		return "", fmt.Errorf("create job response missing id: %+v", job)
	}
	return job.ID, nil
}

// jobStatus returns the current status string for a job ID.
func (c *liveClient) jobStatus(ctx context.Context, id string) (string, error) {
	var job jobResponse
	if err := c.doJSON(ctx, http.MethodGet, "/jobs/"+id, nil, &job); err != nil {
		return "", err
	}
	return job.Status, nil
}

// dlqLen returns the number of failed jobs reported by the API DLQ endpoint.
func (c *liveClient) dlqLen(ctx context.Context) (int, error) {
	var jobs []jobResponse
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/dlq?limit=1000", nil, &jobs); err != nil {
		return 0, err
	}
	return len(jobs), nil
}

// ── Redis observation ─────────────────────────────────────────────────────────

// snapshot captures queue lengths and DLQ size from Redis directly.
type snapshot struct {
	Pending  int64
	InFlight int64
	DLQ      int64
	Delayed  int64
}

var partitionKeys = []string{
	"task_queue:jobs:high:1", "task_queue:jobs:high:2", "task_queue:jobs:high:3",
	"task_queue:jobs:medium:1", "task_queue:jobs:medium:2", "task_queue:jobs:medium:3",
	"task_queue:jobs:low:1", "task_queue:jobs:low:2", "task_queue:jobs:low:3",
}

func (c *liveClient) snapshot(ctx context.Context) (snapshot, error) {
	var s snapshot
	for _, k := range partitionKeys {
		n, err := c.redis.LLen(ctx, k).Result()
		if err != nil {
			return s, err
		}
		s.Pending += n
	}
	inFlight, err := c.redis.ZCard(ctx, "task_queue:in_flight").Result()
	if err != nil {
		return s, err
	}
	s.InFlight = inFlight
	dlq, err := c.redis.LLen(ctx, "task_queue:jobs:dead_letter").Result()
	if err != nil {
		return s, err
	}
	s.DLQ = dlq
	delayed, err := c.redis.ZCard(ctx, "delayed_jobs").Result()
	if err != nil {
		return s, err
	}
	s.Delayed = delayed
	return s, nil
}

// setInFlightScore forges (or overrides) the visibility timestamp of an in-flight
// job, used by the orphan-reclaim scenario to simulate clock skew/stalls.
func (c *liveClient) setInFlightScore(ctx context.Context, jobID string, visibleAt time.Time) error {
	return c.redis.ZAdd(ctx, "task_queue:in_flight", redis.Z{
		Score:  float64(visibleAt.Unix()),
		Member: jobID,
	}).Err()
}

func (c *liveClient) inFlightScore(ctx context.Context, jobID string) (float64, bool) {
	score, err := c.redis.ZScore(ctx, "task_queue:in_flight", jobID).Result()
	if err == redis.Nil {
		return 0, false
	}
	return score, true
}

// ── Docker ─────────────────────────────────────────────────────────────────────

func (c *liveClient) containerByName(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("no container name provided")
	}
	dc, err := c.requireDocker()
	if err != nil {
		return "", err
	}
	ctx := context.Background()
	containers, err := dc.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return "", err
	}
	for _, cnt := range containers {
		for _, n := range cnt.Names {
			if strings.TrimPrefix(n, "/") == name {
				return cnt.ID, nil
			}
		}
	}
	return "", fmt.Errorf("container %q not found (run 'docker ps' to confirm the deployment)", name)
}

// findContainerByComposeService returns the ID of the first container whose
// compose labels identify it as the given service.
func (c *liveClient) findContainerByComposeService(service string) (string, error) {
	dc, err := c.requireDocker()
	if err != nil {
		return "", err
	}
	ctx := context.Background()
	containers, err := dc.ContainerList(ctx, container.ListOptions{})
	if err != nil {
		return "", err
	}
	for _, cnt := range containers {
		labels := cnt.Labels
		if labels["com.docker.compose.service"] == service {
			return cnt.ID, nil
		}
		if strings.HasPrefix(cnt.Image, service) && strings.Contains(cnt.Image, "task-queue") {
			return cnt.ID, nil
		}
	}
	return "", fmt.Errorf("no running container found for compose service %q", service)
}

func (c *liveClient) containerName(ctx context.Context, id string) (string, error) {
	dc, err := c.requireDocker()
	if err != nil {
		return "", err
	}
	info, err := dc.ContainerInspect(ctx, id)
	if err != nil {
		return "", err
	}
	name := strings.TrimPrefix(info.Name, "/")
	if name == "" {
		return id[:12], nil
	}
	return name, nil
}

func (c *liveClient) killContainer(ctx context.Context, id string) error {
	dc, err := c.requireDocker()
	if err != nil {
		return err
	}
	return dc.ContainerKill(ctx, id, "KILL")
}

func (c *liveClient) stopContainer(ctx context.Context, id string) error {
	dc, err := c.requireDocker()
	if err != nil {
		return err
	}
	return dc.ContainerStop(ctx, id, container.StopOptions{})
}

func (c *liveClient) startContainer(ctx context.Context, id string) error {
	dc, err := c.requireDocker()
	if err != nil {
		return err
	}
	return dc.ContainerStart(ctx, id, container.StartOptions{})
}

// waitContainerRunning polls until the container is running again.
func (c *liveClient) waitContainerRunning(ctx context.Context, id string, timeout time.Duration) error {
	dc, err := c.requireDocker()
	if err != nil {
		return err
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		info, err := dc.ContainerInspect(ctx, id)
		if err == nil && info.State != nil && info.State.Running {
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("container did not come back up within %s", timeout)
}

// ── Health probes ─────────────────────────────────────────────────────────────

// probeURL returns true if the URL responds 200 within the timeout.
func (c *liveClient) probeURL(ctx context.Context, url string) bool {
	if url == "" {
		return false
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode == http.StatusOK
}

// dryRunReport validates that all configured targets are reachable.
func (c *liveClient) dryRunReport(ctx context.Context) []string {
	var lines []string
	if c.apiReady(ctx) {
		lines = append(lines, fmt.Sprintf("api %s: reachable (/readyz 200)", c.apiURL))
	} else {
		lines = append(lines, fmt.Sprintf("api %s: UNREACHABLE", c.apiURL))
	}
	if c.probeURL(ctx, c.cfg.SchedulerURL) {
		lines = append(lines, fmt.Sprintf("scheduler %s: reachable", c.cfg.SchedulerURL))
	}
	for _, u := range c.cfg.WorkerURLs {
		if c.probeURL(ctx, u) {
			lines = append(lines, fmt.Sprintf("worker %s: reachable", u))
		} else {
			lines = append(lines, fmt.Sprintf("worker %s: UNREACHABLE", u))
		}
	}
	if c.cfg.RedisContainer != "" {
		if id, err := c.containerByName(c.cfg.RedisContainer); err == nil {
			lines = append(lines, fmt.Sprintf("redis container %s: found (%s)", c.cfg.RedisContainer, id[:12]))
		} else {
			lines = append(lines, fmt.Sprintf("redis container %s: NOT FOUND (%v)", c.cfg.RedisContainer, err))
		}
	} else {
		if id, err := c.findContainerByComposeService("redis"); err == nil {
			lines = append(lines, fmt.Sprintf("redis container: discovered (%s)", id[:12]))
		} else {
			lines = append(lines, fmt.Sprintf("redis container: NOT FOUND (%v)", err))
		}
	}
	workerName := c.cfg.WorkerContainer
	if workerName == "" {
		if id, err := c.findContainerByComposeService("worker"); err == nil {
			if name, err2 := c.containerName(ctx, id); err2 == nil {
				workerName = name
			}
		}
	}
	if workerName != "" {
		lines = append(lines, fmt.Sprintf("worker container: %s", workerName))
	} else {
		lines = append(lines, fmt.Sprintf("worker container: NOT FOUND (set --worker-container)"))
	}
	return lines
}
