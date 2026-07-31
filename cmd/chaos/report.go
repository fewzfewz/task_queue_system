package main

import (
	"encoding/json"
	"fmt"
	"time"
)

// Verdict values emitted in the JSON report.
const (
	VerdictPass = "pass"
	VerdictFail = "fail"
)

// ConfigSnapshot records the flags/env that produced a run so reports are reproducible.
type ConfigSnapshot struct {
	APIURL          string  `json:"api_url"`
	RedisAddr       string  `json:"redis_addr"`
	RedisContainer  string  `json:"redis_container"`
	WorkerContainer string  `json:"worker_container"`
	Jobs            int     `json:"jobs"`
	JobType         string  `json:"job_type"`
	Priority        string  `json:"priority"`
	MaxRetries      int     `json:"max_retries"`
	TimeoutSec      int     `json:"timeout_seconds"`
	SleepMs         int     `json:"sleep_ms"`
	FaultDurationMS int64   `json:"fault_duration_ms"`
	CrashKind       string  `json:"crash_kind"`
}

// FaultDetails describes the fault that was injected and when.
type FaultDetails struct {
	Type       string            `json:"type"`
	Target     string            `json:"target"`
	InjectedAt string            `json:"injected_at,omitempty"`
	ClearedAt  string            `json:"cleared_at,omitempty"`
	Details    map[string]string `json:"details,omitempty"`
}

// Observations captures what the system did during the scenario.
type Observations struct {
	JobsEnqueued            int     `json:"jobs_enqueued"`
	JobsCompleted           int     `json:"jobs_completed"`
	JobsFailed              int     `json:"jobs_failed"`
	JobsDropped             int     `json:"jobs_dropped"`
	DLQGrowth               int64   `json:"dlq_growth"`
	RecoveryTimeMS          int64   `json:"recovery_time_ms,omitempty"`
	APIUnavailableSeconds   float64 `json:"api_unavailable_seconds"`
	PendingBefore           int64   `json:"pending_before"`
	PendingAfter            int64   `json:"pending_after"`
	InFlightBefore          int64   `json:"in_flight_before"`
	InFlightAfter           int64   `json:"in_flight_after"`
	StaleReclaimed          int     `json:"stale_reclaimed,omitempty"`
	FreshReclaimedEarly     int     `json:"fresh_reclaimed_early,omitempty"`
	WorkerEndpointsProbed   int     `json:"worker_endpoints_probed"`
	WorkerEndpointsHealthy  int     `json:"worker_endpoints_healthy"`
}

// SLO captures the thresholds the verdict is measured against.
type SLO struct {
	JobSuccessRatio   float64 `json:"job_success_ratio"`
	RecoveryTimeoutMS int64   `json:"recovery_timeout_ms"`
	MaxDLQGrowth      int64   `json:"max_dlq_growth"`
}

// Report is the structured JSON output of a chaos run.
type Report struct {
	Scenario     string          `json:"scenario"`
	StartedAt    string          `json:"started_at"`
	EndedAt      string          `json:"ended_at"`
	DurationMS   int64           `json:"duration_ms"`
	Config       ConfigSnapshot  `json:"config"`
	Fault        FaultDetails    `json:"fault"`
	Observations Observations    `json:"observations"`
	SLO          SLO             `json:"slo"`
	Verdict      string          `json:"verdict"`
	Failures     []string        `json:"failures,omitempty"`
}

// NewReport initialises a report with timestamps and the run configuration.
func NewReport(scenario string, cfg *Config) *Report {
	return &Report{
		Scenario:  scenario,
		StartedAt: time.Now().UTC().Format(time.RFC3339),
		Config: ConfigSnapshot{
			APIURL:          cfg.APIURL,
			RedisAddr:       cfg.RedisAddr,
			RedisContainer:  cfg.RedisContainer,
			WorkerContainer: cfg.WorkerContainer,
			Jobs:            cfg.Jobs,
			JobType:         cfg.JobType,
			Priority:        cfg.Priority,
			MaxRetries:      cfg.MaxRetries,
			TimeoutSec:      cfg.TimeoutSec,
			SleepMs:         cfg.SleepMs,
			FaultDurationMS: cfg.FaultDuration.Milliseconds(),
			CrashKind:       cfg.CrashKind,
		},
		SLO: SLO{
			JobSuccessRatio:   cfg.SLOJobSuccess,
			RecoveryTimeoutMS: cfg.SLATimeout.Milliseconds(),
			MaxDLQGrowth:      int64(cfg.SLOMaxDLQ),
		},
	}
}

// Finish stamps the end time and computes the verdict from any recorded failures.
func (r *Report) Finish() {
	r.EndedAt = time.Now().UTC().Format(time.RFC3339)
	r.DurationMS = time.Since(parseTime(r.StartedAt)).Milliseconds()
	if len(r.Failures) == 0 {
		r.Verdict = VerdictPass
	} else {
		r.Verdict = VerdictFail
	}
}

// AddFailure records a failed SLO check.
func (r *Report) AddFailure(format string, args ...interface{}) {
	r.Failures = append(r.Failures, fmt.Sprintf(format, args...))
}

func parseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Now()
	}
	return t
}

// JSON returns the indented JSON encoding of the report.
func (r *Report) JSON() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}
