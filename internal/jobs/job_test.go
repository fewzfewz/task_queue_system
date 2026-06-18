package jobs

import (
	"testing"
	"time"
)

func TestNewJobDefaults(t *testing.T) {
	j := NewJob("email", nil, nil, PriorityMedium, 3, time.Time{}, "", 60, 1, "tenant-a")

	if j.BackoffAlgorithm != BackoffExponential {
		t.Errorf("expected exponential backoff, got %s", j.BackoffAlgorithm)
	}
	if j.BackoffJitter != JitterNone {
		t.Errorf("expected no jitter, got %s", j.BackoffJitter)
	}
	if len(j.Labels) != 0 {
		t.Errorf("expected empty labels, got %v", j.Labels)
	}
}

func TestBackoffDelay_Exponential(t *testing.T) {
	job := &Job{
		BackoffAlgorithm: BackoffExponential,
		BackoffJitter:    JitterNone,
	}
	tests := []struct {
		retry int
		want  time.Duration
	}{
		{1, 2 * time.Second},
		{2, 4 * time.Second},
		{3, 8 * time.Second},
		{4, 16 * time.Second},
		{5, 32 * time.Second},
	}
	for _, tt := range tests {
		job.Retries = tt.retry
		got := BackoffDelay(job)
		if got != tt.want {
			t.Errorf("retry=%d: expected %v, got %v", tt.retry, tt.want, got)
		}
	}
}

func TestBackoffDelay_Linear(t *testing.T) {
	job := &Job{
		BackoffAlgorithm: BackoffLinear,
		BackoffJitter:    JitterNone,
	}
	tests := []struct {
		retry int
		want  time.Duration
	}{
		{1, 1 * time.Second},
		{2, 2 * time.Second},
		{3, 3 * time.Second},
		{5, 5 * time.Second},
	}
	for _, tt := range tests {
		job.Retries = tt.retry
		got := BackoffDelay(job)
		if got != tt.want {
			t.Errorf("retry=%d: expected %v, got %v", tt.retry, tt.want, got)
		}
	}
}

func TestBackoffDelay_Fixed(t *testing.T) {
	job := &Job{
		BackoffAlgorithm: BackoffFixed,
		BackoffJitter:    JitterNone,
	}
	for retry := 1; retry <= 10; retry++ {
		job.Retries = retry
		got := BackoffDelay(job)
		if got != 1*time.Second {
			t.Errorf("retry=%d: expected 1s, got %v", retry, got)
		}
	}
}

func TestBackoffDelay_JitterFull(t *testing.T) {
	job := &Job{
		BackoffAlgorithm: BackoffExponential,
		BackoffJitter:    JitterFull,
		Retries:          3,
	}
	// With full jitter at retry 3, delay = random(0..8s)
	for i := 0; i < 100; i++ {
		got := BackoffDelay(job)
		if got < 0 || got > 8*time.Second {
			t.Errorf("jitter full out of range [0, 8s]: got %v", got)
		}
	}
}

func TestBackoffDelay_JitterEqual(t *testing.T) {
	job := &Job{
		BackoffAlgorithm: BackoffExponential,
		BackoffJitter:    JitterEqual,
		Retries:          3,
	}
	// With equal jitter at retry 3, base = 8s, half = 4s, result = 4s + random(0..4s)
	for i := 0; i < 100; i++ {
		got := BackoffDelay(job)
		if got < 4*time.Second || got > 8*time.Second {
			t.Errorf("jitter equal out of range [4s, 8s]: got %v", got)
		}
	}
}

func TestBackoffDelay_NilJob(t *testing.T) {
	got := BackoffDelay(nil)
	if got != 2*time.Second {
		t.Errorf("expected 2s for nil job, got %v", got)
	}
}

func TestBackoffDelay_ZeroRetries(t *testing.T) {
	job := &Job{
		BackoffAlgorithm: BackoffExponential,
		BackoffJitter:    JitterNone,
		Retries:          0,
	}
	got := BackoffDelay(job)
	// Zero retries should be treated as 1 -> 2s
	if got != 2*time.Second {
		t.Errorf("expected 2s for zero retries, got %v", got)
	}
}
