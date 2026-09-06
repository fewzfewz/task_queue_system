cat << 'INNER_EOF' >> internal/service/job_service.go

// PromoteReadyDAGJobs finds pending jobs with met dependencies and enqueues them.
func (s *JobService) PromoteReadyDAGJobs(ctx context.Context) (int64, error) {
	jobs, err := s.store.GetReadyDAGJobs(ctx, 100)
	if err != nil {
		return 0, err
	}
	var count int64
	for _, j := range jobs {
		// Only enqueue if it's not already in Redis (checked implicitly by Redis logic or safe to double-push if deduped by ID)
		// Wait, Redis Enqueue might just overwrite. But if it's already in pending, ZAdd might update score, LPUSH might duplicate if not careful.
		// However, it's only retrieved if it's in Postgres 'pending'. When popped by worker, it stays pending?
		// No, when popped by worker it becomes 'processing'.
		// What if it's in Redis queue right now? Postgres says 'pending'.
		// We could add a status like 'queued' but our system uses 'pending' for both.
		// So we must be careful not to push multiple times.
		// Wait! s.store.GetReadyDAGJobs gets 'pending' jobs. Any 'pending' job with met dependencies SHOULD be in Redis.
		// We can safely Enqueue it. Redis deduplication or idempotent processing handles the rest, OR we can just let it be.
		// Actually, Redis Queue doesn't natively deduplicate pending queue (it's a List or Zset).
		// Wait, for delayed jobs it's ZSET (deduped). For ready jobs it's LIST (NOT deduped)!
		// If we push it 5 times, it gets processed 5 times? No, the postgres UpdateStatus to 'processing' uses `WHERE status = 'pending'`. 
		// The 2nd pop will fail to UpdateStatus and be ignored! So it's safe.
		if err := s.queue.Enqueue(ctx, j); err == nil {
			count++
		}
	}
	return count, nil
}
INNER_EOF
