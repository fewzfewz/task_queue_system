# Chaos Tests

Resilience tests that introduce network faults, process kills, and resource
pressure to verify the task queue system recovers correctly.

## Prerequisites

- Go 1.25+
- Docker (for `pumba` network chaos)
- Root / `sudo` (for iptables rules)
- A running Redis instance (for queue operations)

## Running

```bash
# Basic chaos test (requires root for iptables)
sudo -E go test -tags=chaos -v ./chaos/ -run TestNetworkPartition -timeout 120s

# All chaos scenarios
sudo -E go test -tags=chaos -v ./chaos/ -timeout 300s
```

## Scenarios

| Test | What it does | Recovery verified |
|------|-------------|-------------------|
| `TestNetworkPartition` | Blocks Redis port 6379 via iptables for 10s | Reconnection, orphan reconciliation |
| `TestProcessKill` | Kills worker process mid-job | Scheduler reaps timed-out jobs |
| `TestResourcePressure` | Runs under memory/cpu constraints | Graceful degradation, no crash |

## Notes

- These tests use `//go:build chaos` build tag and are excluded from normal `go test ./...`
- Docker's `pumba` is used for container-level network faults
- `iptables` rules are cleaned up in deferred `teardown()` calls
- A real Redis on localhost:6379 is required (miniredis does not support BRPOP/Lua)

## CI Integration

To run in CI, add a step with root access and Docker:

```yaml
- name: Chaos tests
  run: sudo -E go test -tags=chaos -v ./chaos/ -timeout 300s
  env:
    REDIS_HOST: localhost:6379
  services:
    redis:
      image: redis:7-alpine
      ports:
        - 6379:6379
```
