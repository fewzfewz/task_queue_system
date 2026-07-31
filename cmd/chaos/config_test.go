package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestKnownScenarios(t *testing.T) {
	want := []string{"redis-crash", "worker-kill", "redis-partition", "orphan-reclaim"}
	if len(knownScenarios) != len(want) {
		t.Fatalf("expected %d scenarios, got %d: %v", len(want), len(knownScenarios), knownScenarios)
	}
	for _, w := range want {
		if !isKnownScenario(w) {
			t.Fatalf("scenario %q missing from knownScenarios", w)
		}
	}
	if isKnownScenario("nope") {
		t.Fatal("unknown scenario should not be known")
	}
}

func TestParseRunDefaults(t *testing.T) {
	cfg, err := parseRun([]string{"redis-crash"})
	if err != nil {
		t.Fatalf("parseRun: %v", err)
	}
	if cfg.Scenario != "redis-crash" {
		t.Fatalf("scenario: %q", cfg.Scenario)
	}
	if cfg.APIURL != "http://localhost:8080" {
		t.Fatalf("api-url default: %q", cfg.APIURL)
	}
	if cfg.Jobs != 20 {
		t.Fatalf("jobs default: %d", cfg.Jobs)
	}
	if cfg.CrashKind != "stop" {
		t.Fatalf("crash-kind default: %q", cfg.CrashKind)
	}
	if cfg.SLOJobSuccess != 1.0 {
		t.Fatalf("slo-success default: %f", cfg.SLOJobSuccess)
	}
	if cfg.Output != "-" {
		t.Fatalf("output default: %q", cfg.Output)
	}
}

func TestParseRunFlagsAndEnv(t *testing.T) {
	t.Setenv("CHAOS_API_URL", "http://api.example:9999")
	t.Setenv("CHAOS_API_KEY", "env-key")
	defer os.Unsetenv("CHAOS_API_URL")
	defer os.Unsetenv("CHAOS_API_KEY")

	cfg, err := parseRun([]string{
		"worker-kill",
		"--jobs", "7",
		"--slo-success", "0.95",
		"--fault-duration", "10s",
		"--output", "/tmp/out.json",
		"--dry-run",
		"--verbose",
		"--crash-kind", "kill",
	})
	if err != nil {
		t.Fatalf("parseRun: %v", err)
	}
	if cfg.APIURL != "http://api.example:9999" {
		t.Fatalf("api-url env: %q", cfg.APIURL)
	}
	if cfg.APIKey != "env-key" {
		t.Fatalf("api-key env: %q", cfg.APIKey)
	}
	if cfg.Jobs != 7 {
		t.Fatalf("jobs: %d", cfg.Jobs)
	}
	if cfg.SLOJobSuccess != 0.95 {
		t.Fatalf("slo-success: %f", cfg.SLOJobSuccess)
	}
	if cfg.FaultDuration != 10*time.Second {
		t.Fatalf("fault-duration: %s", cfg.FaultDuration)
	}
	if cfg.Output != "/tmp/out.json" {
		t.Fatalf("output: %q", cfg.Output)
	}
	if !cfg.DryRun || !cfg.Verbose {
		t.Fatal("dry-run/verbose flags not set")
	}
	if cfg.CrashKind != "kill" {
		t.Fatalf("crash-kind: %q", cfg.CrashKind)
	}
}

func TestParseRunRepeatedWorkerURLs(t *testing.T) {
	cfg, err := parseRun([]string{"redis-crash", "--worker-url", "http://w1:8081/readyz", "--worker-url", "http://w2:8081/readyz"})
	if err != nil {
		t.Fatalf("parseRun: %v", err)
	}
	if len(cfg.WorkerURLs) != 2 {
		t.Fatalf("worker urls: %v", cfg.WorkerURLs)
	}
}

func TestValidate(t *testing.T) {
	newCfg := func() *Config {
		cfg, err := parseRun([]string{"redis-crash"})
		if err != nil {
			t.Fatalf("parseRun: %v", err)
		}
		return cfg
	}

	bad := newCfg()
	bad.Scenario = "nope"
	if err := bad.validate(); err == nil || !strings.Contains(err.Error(), "unknown scenario") {
		t.Fatalf("expected unknown scenario error, got %v", err)
	}

	bad = newCfg()
	bad.APIURL = "not-a-url"
	if err := bad.validate(); err == nil || !strings.Contains(err.Error(), "invalid api-url") {
		t.Fatalf("expected invalid api-url error, got %v", err)
	}

	bad = newCfg()
	bad.APIKey = ""
	if err := bad.validate(); err == nil || !strings.Contains(err.Error(), "api-key") {
		t.Fatalf("expected api-key error, got %v", err)
	}

	bad = newCfg()
	bad.Jobs = 0
	if err := bad.validate(); err == nil || !strings.Contains(err.Error(), "jobs") {
		t.Fatalf("expected jobs error, got %v", err)
	}

	bad = newCfg()
	bad.SLATimeout = 0
	if err := bad.validate(); err == nil || !strings.Contains(err.Error(), "sla-timeout") {
		t.Fatalf("expected sla-timeout error, got %v", err)
	}

	bad = newCfg()
	bad.SLOJobSuccess = 1.5
	if err := bad.validate(); err == nil || !strings.Contains(err.Error(), "slo-success") {
		t.Fatalf("expected slo-success error, got %v", err)
	}

	bad = newCfg()
	bad.CrashKind = "explode"
	if err := bad.validate(); err == nil || !strings.Contains(err.Error(), "crash-kind") {
		t.Fatalf("expected crash-kind error, got %v", err)
	}

	bad = newCfg()
	bad.Priority = "urgent"
	if err := bad.validate(); err == nil || !strings.Contains(err.Error(), "priority") {
		t.Fatalf("expected priority error, got %v", err)
	}

	bad = newCfg()
	bad.PartitionMethod = "telepathy"
	if err := bad.validate(); err == nil || !strings.Contains(err.Error(), "partition-method") {
		t.Fatalf("expected partition-method error, got %v", err)
	}

	if err := newCfg().validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
}

func TestNewReportSnapshot(t *testing.T) {
	cfg, _ := parseRun([]string{"redis-crash", "--jobs", "5"})
	r := NewReport("redis-crash", cfg)

	if r.Scenario != "redis-crash" {
		t.Fatalf("scenario: %q", r.Scenario)
	}
	if r.Config.Jobs != 5 {
		t.Fatalf("config jobs: %d", r.Config.Jobs)
	}
	if r.SLO.JobSuccessRatio != 1.0 {
		t.Fatalf("slo ratio: %f", r.SLO.JobSuccessRatio)
	}
	if r.StartedAt == "" {
		t.Fatal("started_at empty")
	}
}

func TestReportVerdict(t *testing.T) {
	cfg, _ := parseRun([]string{"redis-crash"})

	pass := NewReport("redis-crash", cfg)
	pass.Finish()
	if pass.Verdict != VerdictPass {
		t.Fatalf("expected pass, got %q", pass.Verdict)
	}
	if pass.DurationMS < 0 {
		t.Fatalf("duration negative: %d", pass.DurationMS)
	}

	fail := NewReport("redis-crash", cfg)
	fail.AddFailure("job success ratio 0.50 < SLO 1.00")
	fail.AddFailure("recovery time 99999ms > SLO 120000ms")
	fail.Finish()
	if fail.Verdict != VerdictFail {
		t.Fatalf("expected fail, got %q", fail.Verdict)
	}
	if len(fail.Failures) != 2 {
		t.Fatalf("failures: %v", fail.Failures)
	}
}

func TestReportJSON(t *testing.T) {
	cfg, _ := parseRun([]string{"orphan-reclaim"})
	r := NewReport("orphan-reclaim", cfg)
	r.Fault.Type = "forge-in-flight-visibility"
	r.Fault.Target = "task_queue_redis"
	r.Observations.JobsEnqueued = 2
	r.Observations.StaleReclaimed = 1
	r.Observations.RecoveryTimeMS = 1500
	r.SLO.RecoveryTimeoutMS = 120000
	r.Finish()

	data, err := r.JSON()
	if err != nil {
		t.Fatalf("JSON: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"scenario", "started_at", "ended_at", "duration_ms", "config", "fault", "observations", "slo", "verdict"} {
		if _, ok := m[key]; !ok {
			t.Fatalf("report missing top-level key %q", key)
		}
	}
	if m["verdict"] != VerdictPass {
		t.Fatalf("verdict: %v", m["verdict"])
	}
}
