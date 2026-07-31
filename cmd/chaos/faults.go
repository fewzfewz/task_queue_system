package main

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"time"

	"github.com/docker/docker/api/types/network"
)

// faultInjector is a reversible fault applied to the live deployment.
type faultInjector interface {
	inject(ctx context.Context, c *liveClient) (token string, err error)
	clear(ctx context.Context, c *liveClient, token string) error
}

// redisPartitionFault disconnects the Redis container from the deployment
// network for a duration, then reconnects it.
type redisPartitionFault struct {
	networkName string
}

func (f redisPartitionFault) inject(ctx context.Context, c *liveClient) (string, error) {
	dc, err := c.requireDocker()
	if err != nil {
		return "", err
	}
	if f.networkName == "" {
		f.networkName = "task_queue_net"
	}
	redisID, err := c.containerByName(c.cfg.RedisContainer)
	if err != nil {
		redisID, err = c.findContainerByComposeService("redis")
		if err != nil {
			return "", err
		}
	}
	if err := dc.NetworkDisconnect(ctx, f.networkName, redisID, false); err != nil {
		return "", fmt.Errorf("network disconnect failed: %w", err)
	}
	return redisID, nil
}

func (f redisPartitionFault) clear(ctx context.Context, c *liveClient, redisID string) error {
	dc, err := c.requireDocker()
	if err != nil {
		return err
	}
	if err := dc.NetworkConnect(ctx, f.networkName, redisID, &network.EndpointSettings{}); err != nil {
		return fmt.Errorf("network connect failed: %w", err)
	}
	return nil
}

// iptablesDropFault blocks outbound TCP to the Redis port for a duration using
// iptables (for host-run deployments). Requires root.
type iptablesDropFault struct {
	port string
}

func redisPort(addr string) string {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "6379"
	}
	return port
}

func (f iptablesDropFault) inject(ctx context.Context, c *liveClient) (string, error) {
	if err := exec.Command("iptables", "-I", "OUTPUT", "-p", "tcp", "--dport", f.port, "-j", "DROP").Run(); err != nil {
		return "", fmt.Errorf("iptables DROP install failed: %w", err)
	}
	return f.port, nil
}

func (f iptablesDropFault) clear(ctx context.Context, c *liveClient, _ string) error {
	if err := exec.Command("iptables", "-D", "OUTPUT", "-p", "tcp", "--dport", f.port, "-j", "DROP").Run(); err != nil {
		return fmt.Errorf("iptables DROP removal failed: %w", err)
	}
	return nil
}

// waitRedisReachable polls the redis client until it can PING again.
func (c *liveClient) waitRedisReachable(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := c.redis.Ping(ctx).Err(); err == nil {
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("redis did not become reachable within %s", timeout)
}
