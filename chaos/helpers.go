//go:build chaos
// +build chaos

package chaos

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"task-queue-system/internal/jobs"
	"task-queue-system/internal/storage/models"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	docker "github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

var (
	buildOnce sync.Once
	workerBin string
	buildErr  error
	randSrc   = rand.NewSource(time.Now().UnixNano())
)

func requireDocker(t *testing.T) *docker.Client {
	t.Helper()
	cli, err := docker.NewClientWithOpts(docker.FromEnv, docker.WithAPIVersionNegotiation())
	if err != nil {
		t.Skipf("docker client unavailable: %v", err)
	}
	_, err = cli.Ping(context.Background())
	if err != nil {
		t.Skipf("docker not available: %v", err)
	}
	return cli
}

func buildWorkerBinary(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		wd, err := os.Getwd()
		if err != nil {
			buildErr = err
			return
		}
		binDir := os.TempDir()
		binPath := filepath.Join(binDir, fmt.Sprintf("worker-chaos-%d", time.Now().UnixNano()))
		cmd := exec.Command("go", "build", "-o", binPath, "../cmd/worker")
		cmd.Dir = wd
		cmd.Env = append(os.Environ(), "GOFLAGS=")
		out, err := cmd.CombinedOutput()
		if err != nil {
			buildErr = fmt.Errorf("failed to build worker binary: %w output=%s", err, string(out))
			return
		}
		workerBin = binPath
	})
	if buildErr != nil {
		t.Fatalf("build worker binary: %v", buildErr)
	}
	return workerBin
}

func startWorkerProcess(t *testing.T, binary, redisHost string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(binary)
	cmd.Env = append(os.Environ(),
		"REDIS_HOST="+redisHost,
		"REDIS_DB=0",
		"STORE_BACKEND=redis",
		"PORT=8080",
		"API_KEY=secret-api-key",
		"LOG_LEVEL=info",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start worker process: %v", err)
	}
	return cmd
}

func stopProcess(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Signal(os.Interrupt)
	finished := make(chan error, 1)
	go func() {
		finished <- cmd.Wait()
	}()
	select {
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
	case <-finished:
	}
}

func killProcess(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	if cmd == nil || cmd.Process == nil {
		return
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatalf("failed to kill worker process: %v", err)
	}
	_, _ = cmd.Process.Wait()
}

func waitForJobsComplete(t *testing.T, store models.Store, ids []string, timeout time.Duration) int {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		count := 0
		for _, id := range ids {
			job, err := store.GetByID(context.Background(), id)
			if err == nil && job != nil && job.Status == jobs.StatusCompleted {
				count++
			}
		}
		if count == len(ids) {
			return count
		}
		time.Sleep(250 * time.Millisecond)
	}
	count := 0
	for _, id := range ids {
		job, err := store.GetByID(context.Background(), id)
		if err == nil && job != nil && job.Status == jobs.StatusCompleted {
			count++
		}
	}
	return count
}

func randomSuffix() string {
	return strconv.FormatInt(rand.New(randSrc).Int63(), 36)
}

func newRedisContainer(t *testing.T, cli *docker.Client) (string, string, func()) {
	t.Helper()
	ctx := context.Background()
	config := &container.Config{
		Image: "redis:7-alpine",
		ExposedPorts: nat.PortSet{
			nat.Port("6379/tcp"): struct{}{},
		},
	}
	reader, err := cli.ImagePull(ctx, "docker.io/library/redis:7-alpine", image.PullOptions{})
	if err != nil {
		t.Fatalf("failed to pull redis image: %v", err)
	}
	_ = reader.Close()
	hostConfig := &container.HostConfig{
		PortBindings: nat.PortMap{
			nat.Port("6379/tcp"): []nat.PortBinding{{HostIP: "127.0.0.1", HostPort: ""}},
		},
	}
	resp, err := cli.ContainerCreate(ctx, config, hostConfig, &network.NetworkingConfig{}, nil, "")
	if err != nil {
		t.Fatalf("failed to create redis container: %v", err)
	}
	cleanup := func() {
		_ = cli.ContainerRemove(context.Background(), resp.ID, container.RemoveOptions{Force: true, RemoveVolumes: true})
	}
	if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		cleanup()
		t.Fatalf("failed to start redis container: %v", err)
	}
	inspect, err := cli.ContainerInspect(ctx, resp.ID)
	if err != nil {
		cleanup()
		t.Fatalf("failed to inspect redis container: %v", err)
	}
	hostPort := inspect.NetworkSettings.Ports[nat.Port("6379/tcp")][0].HostPort
	return resp.ID, "127.0.0.1:" + hostPort, cleanup
}

func blockRedisTraffic(t *testing.T, port string) func() {
	t.Helper()
	iptablesPath, err := exec.LookPath("iptables")
	if err != nil {
		t.Skip("iptables not available, skipping network partition scenario")
	}
	if os.Geteuid() != 0 {
		t.Skip("network partition scenario requires root privileges for iptables")
	}

	rule := []string{"-I", "OUTPUT", "-p", "tcp", "--dport", port, "-j", "DROP"}
	if err := exec.Command(iptablesPath, rule...).Run(); err != nil {
		t.Fatalf("failed to install iptables DROP rule: %v", err)
	}

	return func() {
		_ = exec.Command(iptablesPath, append([]string{"-D"}, rule[1:]...)...).Run()
	}
}
