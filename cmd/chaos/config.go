package main

import (
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Known chaos scenarios exposed by the CLI.
var knownScenarios = []string{"redis-crash", "worker-kill", "redis-partition", "orphan-reclaim"}

// Config holds all options for a chaos run, sourced from flags with CHAOS_* env
// fallbacks.
type Config struct {
	APIURL           string
	APIKey           string
	RedisAddr        string
	RedisContainer   string
	WorkerContainer  string
	WorkerURLs       []string
	SchedulerURL     string
	Scenario         string
	Jobs             int
	JobType          string
	Priority         string
	MaxRetries       int
	TimeoutSec       int
	SleepMs          int
	FaultDuration    time.Duration
	SLATimeout       time.Duration
	SLOJobSuccess    float64
	SLOMaxDLQ        int
	CrashKind        string
	PartitionMethod  string
	DockerNetwork    string
	Output           string
	DryRun           bool
	Verbose          bool
}

// newFlagSet registers the run subcommand flags, wiring CHAOS_* env fallbacks.
func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.StringVar(&cfgValues.APIURL, "api-url", envOr("CHAOS_API_URL", "http://localhost:8080"), "API base URL")
	fs.StringVar(&cfgValues.APIKey, "api-key", envOr("CHAOS_API_KEY", "secret-api-key"), "X-API-Key value for protected endpoints")
	fs.StringVar(&cfgValues.RedisAddr, "redis-addr", envOr("CHAOS_REDIS_ADDR", "localhost:6379"), "Redis address for direct observation")
	fs.StringVar(&cfgValues.RedisContainer, "redis-container", envOr("CHAOS_REDIS_CONTAINER", "task_queue_redis"), "Redis container name (empty = auto-discover by compose labels)")
	fs.StringVar(&cfgValues.WorkerContainer, "worker-container", envOr("CHAOS_WORKER_CONTAINER", ""), "Worker container name (empty = auto-discover first compose worker)")
	fs.Var(&multiFlag{&cfgValues.WorkerURLs}, "worker-url", "Worker health endpoint (repeatable, e.g. --worker-url http://h:8081/readyz)")
	fs.StringVar(&cfgValues.SchedulerURL, "scheduler-url", envOr("CHAOS_SCHEDULER_URL", ""), "Scheduler health endpoint")
	fs.IntVar(&cfgValues.Jobs, "jobs", 20, "Number of jobs to enqueue for throughput scenarios")
	fs.StringVar(&cfgValues.JobType, "job-type", "image", "Job type ('image' or 'email')")
	fs.StringVar(&cfgValues.Priority, "priority", "medium", "Job priority (low, medium, high)")
	fs.IntVar(&cfgValues.MaxRetries, "max-retries", 0, "Job max retries")
	fs.IntVar(&cfgValues.TimeoutSec, "timeout-seconds", 0, "Job execution timeout seconds")
	fs.IntVar(&cfgValues.SleepMs, "sleep-ms", 3000, "Simulated work duration in payload sleep_ms")
	fs.DurationVar(&cfgValues.FaultDuration, "fault-duration", 5*time.Second, "How long the fault persists (e.g. 10s)")
	fs.DurationVar(&cfgValues.SLATimeout, "sla-timeout", 120*time.Second, "Recovery SLO: max time for the system to recover")
	fs.Float64Var(&cfgValues.SLOJobSuccess, "slo-success", 1.0, "Minimum fraction of jobs that must complete (0.0-1.0)")
	fs.IntVar(&cfgValues.SLOMaxDLQ, "slo-max-dlq", 0, "Maximum allowed DLQ growth")
	fs.StringVar(&cfgValues.CrashKind, "crash-kind", "stop", "Redis crash style: 'stop' (graceful SIGTERM) or 'kill' (SIGKILL)")
	fs.StringVar(&cfgValues.PartitionMethod, "partition-method", "docker", "redis-partition method: 'docker' (network disconnect/reconnect) or 'iptables' (root host firewall)")
	fs.StringVar(&cfgValues.DockerNetwork, "docker-network", "task_queue_net", "Docker network to disconnect for redis-partition")
	fs.StringVar(&cfgValues.Output, "output", "-", "Report output path ('-' = stdout)")
	fs.BoolVar(&cfgValues.DryRun, "dry-run", false, "Validate config and reachability without injecting any fault")
	fs.BoolVar(&cfgValues.Verbose, "verbose", false, "Emit progress lines to stderr")
	return fs
}

// configValues accumulates raw flag values; parseRun builds a Config from it.
var cfgValues = struct {
	APIURL          string
	APIKey          string
	RedisAddr       string
	RedisContainer  string
	WorkerContainer string
	WorkerURLs      []string
	SchedulerURL    string
	Jobs            int
	JobType         string
	Priority        string
	MaxRetries      int
	TimeoutSec      int
	SleepMs         int
	FaultDuration   time.Duration
	SLATimeout      time.Duration
	SLOJobSuccess   float64
	SLOMaxDLQ       int
	CrashKind       string
	PartitionMethod string
	DockerNetwork   string
	Output          string
	DryRun          bool
	Verbose         bool
}{}

// multiFlag collects repeated string flags into a slice.
type multiFlag struct{ targets *[]string }

func (m *multiFlag) String() string {
	if m.targets == nil {
		return ""
	}
	return strings.Join(*m.targets, ",")
}

func (m *multiFlag) Set(v string) error {
	*m.targets = append(*m.targets, v)
	return nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func parseRun(args []string) (*Config, error) {
	cfgValues = struct {
		APIURL          string
		APIKey          string
		RedisAddr       string
		RedisContainer  string
		WorkerContainer string
		WorkerURLs      []string
		SchedulerURL    string
		Jobs            int
		JobType         string
		Priority        string
		MaxRetries      int
		TimeoutSec      int
		SleepMs         int
		FaultDuration   time.Duration
		SLATimeout      time.Duration
		SLOJobSuccess   float64
		SLOMaxDLQ       int
		CrashKind       string
		PartitionMethod string
		DockerNetwork   string
		Output          string
		DryRun          bool
		Verbose         bool
	}{}

	fs := newFlagSet("run")

	// The stdlib flag package stops at the first positional argument, so pull
	// the scenario out first to allow flags on either side of it.
	var scenario string
	rest := make([]string, 0, len(args))
	for _, a := range args {
		if scenario == "" && !strings.HasPrefix(a, "-") {
			scenario = a
			continue
		}
		rest = append(rest, a)
	}
	if err := fs.Parse(rest); err != nil {
		return nil, err
	}
	if len(fs.Args()) != 0 {
		return nil, fmt.Errorf("unexpected extra arguments: %v", fs.Args())
	}
	if scenario == "" {
		return nil, fmt.Errorf("usage: chaos run <scenario> [flags] (scenarios: %s)", strings.Join(knownScenarios, ", "))
	}

	cfg := &Config{
		APIURL:          strings.TrimRight(cfgValues.APIURL, "/"),
		APIKey:          cfgValues.APIKey,
		RedisAddr:       cfgValues.RedisAddr,
		RedisContainer:  cfgValues.RedisContainer,
		WorkerContainer: cfgValues.WorkerContainer,
		WorkerURLs:      cfgValues.WorkerURLs,
		SchedulerURL:    strings.TrimRight(cfgValues.SchedulerURL, "/"),
		Scenario:        scenario,
		Jobs:            cfgValues.Jobs,
		JobType:         cfgValues.JobType,
		Priority:        cfgValues.Priority,
		MaxRetries:      cfgValues.MaxRetries,
		TimeoutSec:      cfgValues.TimeoutSec,
		SleepMs:         cfgValues.SleepMs,
		FaultDuration:   cfgValues.FaultDuration,
		SLATimeout:      cfgValues.SLATimeout,
		SLOJobSuccess:   cfgValues.SLOJobSuccess,
		SLOMaxDLQ:       cfgValues.SLOMaxDLQ,
		CrashKind:       cfgValues.CrashKind,
		PartitionMethod: cfgValues.PartitionMethod,
		DockerNetwork:   cfgValues.DockerNetwork,
		Output:          cfgValues.Output,
		DryRun:          cfgValues.DryRun,
		Verbose:         cfgValues.Verbose,
	}
	return cfg, nil
}

// validate checks the config is coherent and safe to run.
func (c *Config) validate() error {
	if !isKnownScenario(c.Scenario) {
		return fmt.Errorf("unknown scenario %q (known: %s)", c.Scenario, strings.Join(knownScenarios, ", "))
	}
	if _, err := url.Parse(c.APIURL); err != nil || !strings.HasPrefix(c.APIURL, "http") {
		return fmt.Errorf("invalid api-url %q", c.APIURL)
	}
	if c.APIKey == "" {
		return fmt.Errorf("api-key must not be empty")
	}
	if c.RedisAddr == "" {
		return fmt.Errorf("redis-addr must not be empty")
	}
	if c.Jobs < 1 {
		return fmt.Errorf("jobs must be >= 1")
	}
	if c.SLATimeout <= 0 {
		return fmt.Errorf("sla-timeout must be > 0")
	}
	if c.SLOJobSuccess < 0 || c.SLOJobSuccess > 1 {
		return fmt.Errorf("slo-success must be between 0.0 and 1.0")
	}
	if c.SLOMaxDLQ < 0 {
		return fmt.Errorf("slo-max-dlq must be >= 0")
	}
	if c.FaultDuration <= 0 {
		return fmt.Errorf("fault-duration must be > 0")
	}
	switch c.CrashKind {
	case "stop", "kill":
	default:
		return fmt.Errorf("crash-kind must be 'stop' or 'kill' (got %q)", c.CrashKind)
	}
	switch c.PartitionMethod {
	case "docker", "iptables":
	default:
		return fmt.Errorf("partition-method must be 'docker' or 'iptables' (got %q)", c.PartitionMethod)
	}
	if c.PartitionMethod == "iptables" {
		if os.Geteuid() != 0 {
			return fmt.Errorf("partition-method 'iptables' requires root")
		}
		if _, err := exec.LookPath("iptables"); err != nil {
			return fmt.Errorf("partition-method 'iptables' requires iptables: %w", err)
		}
	}
	switch c.Priority {
	case "low", "medium", "high":
	default:
		return fmt.Errorf("priority must be low, medium, or high (got %q)", c.Priority)
	}
	switch c.JobType {
	case "image", "email":
	default:
		return fmt.Errorf("job-type must be 'image' or 'email' (got %q)", c.JobType)
	}
	if c.Scenario == "redis-partition" && c.PartitionMethod == "iptables" {
		if os.Geteuid() != 0 {
			return fmt.Errorf("scenario 'redis-partition' with iptables requires root")
		}
		if _, err := exec.LookPath("iptables"); err != nil {
			return fmt.Errorf("scenario 'redis-partition' with iptables requires iptables: %w", err)
		}
	}
	return nil
}

func isKnownScenario(s string) bool {
	for _, k := range knownScenarios {
		if k == s {
			return true
		}
	}
	return false
}
