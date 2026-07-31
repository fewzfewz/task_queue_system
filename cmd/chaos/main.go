package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
)

const usage = `chaos — standalone chaos scenarios against a live task-queue deployment

Usage:
  chaos list
  chaos run <scenario> [flags]
  chaos run <scenario> --dry-run

Commands:
  list            List available chaos scenarios
  run             Run a scenario against the live deployment

Scenarios:
  redis-crash     Kill/stop the Redis container while jobs are being processed,
                  then restart it and verify recovery within the SLO.
  worker-kill     Kill a worker container mid-processing and verify the pool
                  and scheduler reclaim finish the jobs.
  redis-partition Isolate Redis from the deployment network for a duration,
                  then restore it and verify recovery.
  orphan-reclaim  Forge stale/fresh in-flight visibility timestamps and verify
                  the scheduler reclaims only the stale job.

Run 'chaos run <scenario> -h' for scenario flags.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}

	switch os.Args[1] {
	case "list":
		fmt.Println(strings.Join(knownScenarios, "\n"))
	case "run":
		if err := runScenario(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "chaos: %v\n", err)
			os.Exit(1)
		}
	case "-h", "--help", "help":
		fmt.Fprint(os.Stdout, usage)
	case "version":
		fmt.Println("chaos (task-queue-system)")
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}
}

func runScenario(args []string) error {
	cfg, err := parseRun(args)
	if err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}
	if err := cfg.validate(); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	c, err := newLiveClient(cfg)
	if err != nil {
		return err
	}
	defer c.Close()

	if cfg.DryRun {
		return dryRun(cfg, c, ctx)
	}

	report := NewReport(cfg.Scenario, cfg)
	r := &runner{c: c, cfg: cfg, report: report}

	var runErr error
	switch cfg.Scenario {
	case "redis-crash":
		runErr = r.runRedisCrash(ctx)
	case "worker-kill":
		runErr = r.runWorkerKill(ctx)
	case "redis-partition":
		runErr = r.runRedisPartition(ctx)
	case "orphan-reclaim":
		runErr = r.runOrphanReclaim(ctx)
	}
	if runErr != nil {
		report.AddFailure("scenario aborted: %v", runErr)
	}
	report.Finish()

	out, jsonErr := report.JSON()
	if jsonErr != nil {
		return fmt.Errorf("encode report: %w", jsonErr)
	}
	if err := writeOutput(cfg.Output, out); err != nil {
		return err
	}
	if report.Verdict == VerdictFail {
		return fmt.Errorf("scenario verdict: FAIL")
	}
	return nil
}

// dryRun validates configuration and reachability without injecting faults.
func dryRun(cfg *Config, c *liveClient, ctx context.Context) error {
	fmt.Printf("dry-run: scenario %q validated (no faults injected)\n", cfg.Scenario)
	fmt.Printf("  api-url: %s\n  redis-addr: %s\n", cfg.APIURL, cfg.RedisAddr)
	fmt.Printf("  jobs: %d  job-type: %s  priority: %s\n", cfg.Jobs, cfg.JobType, cfg.Priority)
	fmt.Printf("  fault-duration: %s  sla-timeout: %s  slo-success: %.2f  slo-max-dlq: %d\n",
		cfg.FaultDuration, cfg.SLATimeout, cfg.SLOJobSuccess, cfg.SLOMaxDLQ)

	checks := c.dryRunReport(ctx)
	allOK := true
	for _, line := range checks {
		fmt.Printf("  probe: %s\n", line)
		if strings.Contains(line, "UNREACHABLE") || strings.Contains(line, "NOT FOUND") {
			allOK = false
		}
	}
	if !allOK {
		return fmt.Errorf("dry-run failed: one or more targets unreachable")
	}
	return nil
}

func writeOutput(path string, data []byte) error {
	if path == "" || path == "-" {
		_, err := os.Stdout.Write(append(data, '\n'))
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
