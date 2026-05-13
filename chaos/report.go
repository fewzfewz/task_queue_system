//go:build chaos
// +build chaos

package chaos

import (
	"fmt"
	"time"
)

// Report captures the outcome of a chaos scenario.
type Report struct {
	Scenario      string        `json:"scenario"`
	JobsEnqueued  int           `json:"jobs_enqueued"`
	JobsCompleted int           `json:"jobs_completed"`
	JobsLost      int           `json:"jobs_lost"`
	Duration      time.Duration `json:"duration"`
	Passed        bool          `json:"passed"`
	Error         string        `json:"error,omitempty"`
}

func (r Report) String() string {
	return fmt.Sprintf(
		"scenario=%s jobs_enqueued=%d jobs_completed=%d jobs_lost=%d duration=%s passed=%t error=%q",
		r.Scenario, r.JobsEnqueued, r.JobsCompleted, r.JobsLost, r.Duration.String(), r.Passed, r.Error,
	)
}
