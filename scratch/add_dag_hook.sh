sed -i '/s.publishWebhookEvents/a \
	// ── DAG Promotion ─────────────────────────────────────────────────────────\
	if status == jobs.StatusCompleted {\
		dependentJobs, err := s.store.GetDependentJobs(ctx, jobID)\
		if err == nil \&\& len(dependentJobs) > 0 {\
			for _, dJob := range dependentJobs {\
				if err := s.queue.Enqueue(ctx, dJob); err != nil {\
					s.logger.Error("failed to enqueue dependent job", "job_id", dJob.ID, "error", err)\
				} else {\
					s.logger.Info("dependent job promoted to queue", "parent_id", jobID, "dependent_id", dJob.ID)\
				}\
			}\
		}\
	}' internal/service/job_service.go
