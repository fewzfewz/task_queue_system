package main

import (
	"context"
	"fmt"
	"os"
	"time"
)

// runner executes one scenario against the live deployment.
type runner struct {
	c      *liveClient
	cfg    *Config
	report *Report
}

func (r *runner) logf(format string, args ...interface{}) {
	if r.cfg.Verbose {
		fmt.Fprintf(os.Stderr, "[chaos] "+format+"\n", args...)
	}
}

// enqueue creates cfg.Jobs jobs via the API and returns their IDs.
func (r *runner) enqueue(ctx context.Context) ([]string, error) {
	r.logf("enqueueing %d %s jobs (sleep_ms=%d)", r.cfg.Jobs, r.cfg.JobType, r.cfg.SleepMs)
	var ids []string
	for i := 0; i < r.cfg.Jobs; i++ {
		id, err := r.c.createJob(ctx, map[string]interface{}{
			"source_url": fmt.Sprintf("http://example.com/chaos/%d.jpg", i),
			"operation":  "process",
			"sleep_ms":   r.cfg.SleepMs,
		})
		if err != nil {
			return ids, fmt.Errorf("create job %d: %w", i, err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// waitForInFlight blocks until at least min jobs are mid-processing.
func (r *runner) waitForInFlight(ctx context.Context, min int64, timeout time.Duration) error {
	r.logf("waiting for %d job(s) in flight...", min)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		s, err := r.c.snapshot(ctx)
		if err == nil && s.InFlight >= min {
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for in-flight jobs (want >= %d)", min)
}

// waitForJobsTerminal polls job statuses until all reach a terminal state or
// the SLO deadline passes. Returns completed/failed counts and recovery time
// measured from `since`.
func (r *runner) waitForJobsTerminal(ctx context.Context, ids []string, since time.Time) (completed, failed int, recoveryMS int64, err error) {
	r.logf("observing %d job(s) until terminal (SLO %s)...", len(ids), r.cfg.SLATimeout)
	deadline := time.Now().Add(r.cfg.SLATimeout)
	terminal := make(map[string]string, len(ids))
	var lastTerminal time.Time

	for time.Now().Before(deadline) {
		if len(terminal) == len(ids) {
			break
		}
		for _, id := range ids {
			if _, ok := terminal[id]; ok {
				continue
			}
			status, err := r.c.jobStatus(ctx, id)
			if err != nil {
				continue // transient (e.g. API still recovering from the fault)
			}
			switch status {
			case statusCompleted, statusFailed:
				terminal[id] = status
				lastTerminal = time.Now()
			}
		}
		time.Sleep(500 * time.Millisecond)
	}

	for _, st := range terminal {
		switch st {
		case statusCompleted:
			completed++
		case statusFailed:
			failed++
		}
	}

	if len(terminal) < len(ids) {
		r.report.AddFailure("only %d/%d jobs reached a terminal state within SLO %s", len(terminal), len(ids), r.cfg.SLATimeout)
		recoveryMS = r.cfg.SLATimeout.Milliseconds()
		return completed, failed, recoveryMS, nil
	}

	if lastTerminal.IsZero() {
		lastTerminal = since
	}
	recoveryMS = lastTerminal.Sub(since).Milliseconds()
	if recoveryMS < 0 {
		recoveryMS = 0
	}
	return completed, failed, recoveryMS, nil
}

// availabilityWatcher samples API readiness and reports total down time.
type availabilityWatcher struct {
	done     chan struct{}
	totalDown time.Duration
}

func startAvailabilityWatcher(c *liveClient) *availabilityWatcher {
	w := &availabilityWatcher{done: make(chan struct{})}
	go func() {
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		lastDown := time.Time{}
		for {
			select {
			case <-w.done:
				if !lastDown.IsZero() {
					w.totalDown += time.Since(lastDown)
				}
				return
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				ok := c.apiReady(ctx)
				cancel()
				if ok {
					if !lastDown.IsZero() {
						w.totalDown += time.Since(lastDown)
						lastDown = time.Time{}
					}
				} else if lastDown.IsZero() {
					lastDown = time.Now()
				}
			}
		}
	}()
	return w
}

func (w *availabilityWatcher) stop() {
	close(w.done)
}

// finalizeVerdict applies the shared SLO checks for throughput scenarios.
func (r *runner) finalizeVerdict(ids []string, completed, failed int, dlqGrowth int64, recoveryMS int64) {
	ratio := 0.0
	if len(ids) > 0 {
		ratio = float64(completed) / float64(len(ids))
	}
	if ratio < r.cfg.SLOJobSuccess {
		r.report.AddFailure("job success ratio %.2f < SLO %.2f (%d/%d completed)", ratio, r.cfg.SLOJobSuccess, completed, len(ids))
	}
	if recoveryMS > r.cfg.SLATimeout.Milliseconds() {
		r.report.AddFailure("recovery time %dms > SLO %dms", recoveryMS, r.cfg.SLATimeout.Milliseconds())
	}
	if dlqGrowth > int64(r.cfg.SLOMaxDLQ) {
		r.report.AddFailure("DLQ growth %d > SLO max %d", dlqGrowth, r.cfg.SLOMaxDLQ)
	}
}

func (r *runner) probeWorkers(ctx context.Context) {
	r.report.Observations.WorkerEndpointsProbed = len(r.cfg.WorkerURLs)
	for _, u := range r.cfg.WorkerURLs {
		if r.c.probeURL(ctx, u) {
			r.report.Observations.WorkerEndpointsHealthy++
		}
	}
}

// ── Scenarios ─────────────────────────────────────────────────────────────────

// runRedisCrash kills/stops the Redis container while jobs are being processed,
// restarts it, and verifies the system recovers within the SLO.
func (r *runner) runRedisCrash(ctx context.Context) error {
	dc, err := r.c.requireDocker()
	if err != nil {
		return err
	}
	_ = dc

	redisID, err := r.c.containerByName(r.cfg.RedisContainer)
	if err != nil {
		redisID, err = r.c.findContainerByComposeService("redis")
		if err != nil {
			return fmt.Errorf("locate redis container: %w", err)
		}
	}
	target, err := r.c.containerName(ctx, redisID)
	if err != nil {
		return err
	}

	r.report.Fault.Type = "docker-" + r.cfg.CrashKind + "-start"
	r.report.Fault.Target = target
	r.report.Fault.Details = map[string]string{"container": target}

	baseline, err := r.c.snapshot(ctx)
	if err != nil {
		return err
	}
	r.report.Observations.PendingBefore = baseline.Pending
	r.report.Observations.InFlightBefore = baseline.InFlight

	ids, err := r.enqueue(ctx)
	if err != nil {
		return err
	}
	if err := r.waitForInFlight(ctx, 1, 60*time.Second); err != nil {
		return err
	}

	watcher := startAvailabilityWatcher(r.c)

	r.logf("injecting fault: %s redis container", r.cfg.CrashKind)
	if r.cfg.CrashKind == "kill" {
		if err := r.c.killContainer(ctx, redisID); err != nil {
			watcher.stop()
			return fmt.Errorf("kill redis container: %w", err)
		}
	} else {
		if err := r.c.stopContainer(ctx, redisID); err != nil {
			watcher.stop()
			return fmt.Errorf("stop redis container: %w", err)
		}
	}
	r.report.Fault.InjectedAt = time.Now().UTC().Format(time.RFC3339)

	r.logf("fault active for %s", r.cfg.FaultDuration)
	select {
	case <-time.After(r.cfg.FaultDuration):
	case <-ctx.Done():
		watcher.stop()
		return ctx.Err()
	}

	r.logf("clearing fault: restarting redis container")
	if err := r.c.startContainer(ctx, redisID); err != nil {
		watcher.stop()
		return fmt.Errorf("restart redis container: %w", err)
	}
	clearedAt := time.Now()
	r.report.Fault.ClearedAt = clearedAt.UTC().Format(time.RFC3339)

	if err := r.c.waitContainerRunning(ctx, redisID, 60*time.Second); err != nil {
		watcher.stop()
		return err
	}
	if err := r.c.waitRedisReachable(ctx, 60*time.Second); err != nil {
		watcher.stop()
		return err
	}

	completed, failed, recoveryMS, _ := r.waitForJobsTerminal(ctx, ids, clearedAt)
	watcher.stop()
	r.probeWorkers(ctx)

	final, err := r.c.snapshot(ctx)
	if err != nil {
		return err
	}
	dlqGrowth := final.DLQ - baseline.DLQ
	if dlqGrowth < 0 {
		dlqGrowth = 0
	}

	r.report.Observations.JobsEnqueued = len(ids)
	r.report.Observations.JobsCompleted = completed
	r.report.Observations.JobsFailed = failed
	r.report.Observations.JobsDropped = len(ids) - completed - failed
	r.report.Observations.DLQGrowth = dlqGrowth
	r.report.Observations.RecoveryTimeMS = recoveryMS
	r.report.Observations.APIUnavailableSeconds = watcher.totalDown.Seconds()
	r.report.Observations.PendingAfter = final.Pending
	r.report.Observations.InFlightAfter = final.InFlight

	r.finalizeVerdict(ids, completed, failed, dlqGrowth, recoveryMS)
	return nil
}

// runWorkerKill kills a worker container mid-processing and verifies the rest
// of the pool (and the scheduler's reclaim) finishes the jobs.
func (r *runner) runWorkerKill(ctx context.Context) error {
	workerID, err := r.c.containerByName(r.cfg.WorkerContainer)
	if err != nil {
		workerID, err = r.c.findContainerByComposeService("worker")
		if err != nil {
			return fmt.Errorf("locate worker container: %w", err)
		}
	}
	target, err := r.c.containerName(ctx, workerID)
	if err != nil {
		return err
	}

	r.report.Fault.Type = "docker-kill-start"
	r.report.Fault.Target = target
	r.report.Fault.Details = map[string]string{"container": target}

	baseline, err := r.c.snapshot(ctx)
	if err != nil {
		return err
	}
	r.report.Observations.PendingBefore = baseline.Pending
	r.report.Observations.InFlightBefore = baseline.InFlight

	ids, err := r.enqueue(ctx)
	if err != nil {
		return err
	}
	if err := r.waitForInFlight(ctx, 1, 60*time.Second); err != nil {
		return err
	}

	watcher := startAvailabilityWatcher(r.c)

	r.logf("injecting fault: killing worker container %s", target)
	if err := r.c.killContainer(ctx, workerID); err != nil {
		watcher.stop()
		return fmt.Errorf("kill worker container: %w", err)
	}
	r.report.Fault.InjectedAt = time.Now().UTC().Format(time.RFC3339)

	select {
	case <-time.After(r.cfg.FaultDuration):
	case <-ctx.Done():
		watcher.stop()
		return ctx.Err()
	}

	r.logf("clearing fault: restarting worker container %s", target)
	if err := r.c.startContainer(ctx, workerID); err != nil {
		watcher.stop()
		return fmt.Errorf("restart worker container: %w", err)
	}
	clearedAt := time.Now()
	r.report.Fault.ClearedAt = clearedAt.UTC().Format(time.RFC3339)
	if err := r.c.waitContainerRunning(ctx, workerID, 60*time.Second); err != nil {
		watcher.stop()
		return err
	}

	completed, failed, recoveryMS, _ := r.waitForJobsTerminal(ctx, ids, clearedAt)
	watcher.stop()
	r.probeWorkers(ctx)

	final, err := r.c.snapshot(ctx)
	if err != nil {
		return err
	}
	dlqGrowth := final.DLQ - baseline.DLQ
	if dlqGrowth < 0 {
		dlqGrowth = 0
	}

	r.report.Observations.JobsEnqueued = len(ids)
	r.report.Observations.JobsCompleted = completed
	r.report.Observations.JobsFailed = failed
	r.report.Observations.JobsDropped = len(ids) - completed - failed
	r.report.Observations.DLQGrowth = dlqGrowth
	r.report.Observations.RecoveryTimeMS = recoveryMS
	r.report.Observations.APIUnavailableSeconds = watcher.totalDown.Seconds()
	r.report.Observations.PendingAfter = final.Pending
	r.report.Observations.InFlightAfter = final.InFlight

	r.finalizeVerdict(ids, completed, failed, dlqGrowth, recoveryMS)
	return nil
}

// runRedisPartition disconnects Redis from the deployment network (or drops
// host iptables traffic) while jobs are processing, then restores it.
func (r *runner) runRedisPartition(ctx context.Context) error {
	var (
		fault     faultInjector
		faultType string
	)
	if r.cfg.PartitionMethod == "iptables" {
		port := redisPort(r.cfg.RedisAddr)
		fault = iptablesDropFault{port: port}
		faultType = "iptables-drop:" + port
		r.report.Fault.Details = map[string]string{"port": port, "method": "iptables"}
	} else {
		fault = redisPartitionFault{networkName: r.cfg.DockerNetwork}
		faultType = "docker-network-disconnect"
		r.report.Fault.Details = map[string]string{"network": r.cfg.DockerNetwork, "method": "docker"}
	}
	r.report.Fault.Type = faultType

	baseline, err := r.c.snapshot(ctx)
	if err != nil {
		return err
	}
	r.report.Observations.PendingBefore = baseline.Pending
	r.report.Observations.InFlightBefore = baseline.InFlight

	ids, err := r.enqueue(ctx)
	if err != nil {
		return err
	}
	if err := r.waitForInFlight(ctx, 1, 60*time.Second); err != nil {
		return err
	}

	watcher := startAvailabilityWatcher(r.c)

	r.logf("injecting fault: %s", faultType)
	token, err := fault.inject(ctx, r.c)
	if err != nil {
		watcher.stop()
		return fmt.Errorf("inject partition: %w", err)
	}
	r.report.Fault.InjectedAt = time.Now().UTC().Format(time.RFC3339)

	select {
	case <-time.After(r.cfg.FaultDuration):
	case <-ctx.Done():
		watcher.stop()
		_ = fault.clear(ctx, r.c, token)
		return ctx.Err()
	}

	r.logf("clearing fault")
	if err := fault.clear(ctx, r.c, token); err != nil {
		watcher.stop()
		return fmt.Errorf("clear partition: %w", err)
	}
	clearedAt := time.Now()
	r.report.Fault.ClearedAt = clearedAt.UTC().Format(time.RFC3339)

	if r.cfg.PartitionMethod == "docker" {
		if err := r.c.waitRedisReachable(ctx, 60*time.Second); err != nil {
			watcher.stop()
			return err
		}
	}

	completed, failed, recoveryMS, _ := r.waitForJobsTerminal(ctx, ids, clearedAt)
	watcher.stop()
	r.probeWorkers(ctx)

	final, err := r.c.snapshot(ctx)
	if err != nil {
		return err
	}
	dlqGrowth := final.DLQ - baseline.DLQ
	if dlqGrowth < 0 {
		dlqGrowth = 0
	}

	r.report.Observations.JobsEnqueued = len(ids)
	r.report.Observations.JobsCompleted = completed
	r.report.Observations.JobsFailed = failed
	r.report.Observations.JobsDropped = len(ids) - completed - failed
	r.report.Observations.DLQGrowth = dlqGrowth
	r.report.Observations.RecoveryTimeMS = recoveryMS
	r.report.Observations.APIUnavailableSeconds = watcher.totalDown.Seconds()
	r.report.Observations.PendingAfter = final.Pending
	r.report.Observations.InFlightAfter = final.InFlight

	r.finalizeVerdict(ids, completed, failed, dlqGrowth, recoveryMS)
	return nil
}

// runOrphanReclaim forges stale + fresh in-flight visibility timestamps and
// verifies the scheduler reclaims only the stale job.
func (r *runner) runOrphanReclaim(ctx context.Context) error {
	r.report.Fault.Type = "forge-in-flight-visibility"
	r.report.Fault.Details = map[string]string{
		"stale_score": "now - 2h",
		"fresh_score": "now + 30s",
	}

	baseline, err := r.c.snapshot(ctx)
	if err != nil {
		return err
	}
	r.report.Observations.PendingBefore = baseline.Pending
	r.report.Observations.InFlightBefore = baseline.InFlight

	ids, err := r.enqueue(ctx)
	if err != nil {
		return err
	}
	if len(ids) < 2 {
		return fmt.Errorf("orphan-reclaim requires at least 2 jobs")
	}
	staleID, freshID := ids[0], ids[1]

	r.logf("waiting for both jobs to enter in-flight set")
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		_, staleOK := r.c.inFlightScore(ctx, staleID)
		_, freshOK := r.c.inFlightScore(ctx, freshID)
		if staleOK && freshOK {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if _, ok := r.c.inFlightScore(ctx, staleID); !ok {
		return fmt.Errorf("stale job %s never entered in-flight (are workers running?)", staleID)
	}
	if _, ok := r.c.inFlightScore(ctx, freshID); !ok {
		return fmt.Errorf("fresh job %s never entered in-flight (are workers running?)", freshID)
	}

	r.logf("forging visibility timestamps: stale=%s (now-2h), fresh=%s (now+30s)", staleID, freshID)
	if err := r.c.setInFlightScore(ctx, staleID, time.Now().Add(-2*time.Hour)); err != nil {
		return fmt.Errorf("forge stale score: %w", err)
	}
	if err := r.c.setInFlightScore(ctx, freshID, time.Now().Add(30*time.Second)); err != nil {
		return fmt.Errorf("forge fresh score: %w", err)
	}
	injectedAt := time.Now()
	r.report.Fault.InjectedAt = injectedAt.UTC().Format(time.RFC3339)
	r.report.Fault.ClearedAt = injectedAt.UTC().Format(time.RFC3339)

	r.logf("watching for stale job reclaim (SLO %s)", r.cfg.SLATimeout)
	reclaimDeadline := time.Now().Add(r.cfg.SLATimeout)
	staleReclaimed := false
	var reclaimedAt time.Time
	for time.Now().Before(reclaimDeadline) {
		if _, ok := r.c.inFlightScore(ctx, staleID); !ok {
			staleReclaimed = true
			reclaimedAt = time.Now()
			break
		}
		time.Sleep(250 * time.Millisecond)
	}

	freshReclaimedEarly := false
	freshWindow := time.Now().Add(10 * time.Second)
	for time.Now().Before(freshWindow) {
		if _, ok := r.c.inFlightScore(ctx, freshID); !ok {
			status, err := r.c.jobStatus(ctx, freshID)
			if err == nil && status == "processing" {
				freshReclaimedEarly = true
			}
			break
		}
		time.Sleep(250 * time.Millisecond)
	}

	r.logf("waiting for jobs to settle...")
	completed, failed, _, _ := r.waitForJobsTerminal(ctx, ids, injectedAt)

	r.report.Observations.JobsEnqueued = len(ids)
	r.report.Observations.JobsCompleted = completed
	r.report.Observations.JobsFailed = failed
	r.report.Observations.JobsDropped = len(ids) - completed - failed
	if staleReclaimed {
		r.report.Observations.StaleReclaimed = 1
		r.report.Observations.RecoveryTimeMS = reclaimedAt.Sub(injectedAt).Milliseconds()
	}
	if freshReclaimedEarly {
		r.report.Observations.FreshReclaimedEarly = 1
	}

	if !staleReclaimed {
		r.report.AddFailure("stale in-flight job was not reclaimed within SLO %s", r.cfg.SLATimeout)
	}
	if freshReclaimedEarly {
		r.report.AddFailure("fresh in-flight job was reclaimed before its visibility window expired")
	}
	r.probeWorkers(ctx)
	return nil
}
