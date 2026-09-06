sed -i '/func (s \*JobService) CreateJob(/i \
func (s *JobService) CreateJobBatch(ctx context.Context, requests []struct{\
	Type             string\
	Payload          map[string]interface{}\
	Labels           map[string]string\
	Priority         string\
	MaxRetries       int\
	BackoffAlgorithm string\
	BackoffJitter    string\
	CronExpr         string\
	RunAt            string\
	CorrelationID    string\
	Timeout          int\
	Version          int\
	TenantID         string\
	Webhook          *jobs.WebhookConfig\
	DedupKey         string\
	Dependencies     []string\
	ShardKey         string\
}) ([]*jobs.Job, error) {\
	if len(requests) == 0 { return nil, nil }\
	var batch []*jobs.Job\
	now := time.Now().UTC()\
	var toEnqueue []struct{ TenantID, ShardKey, JobID string; Priority int }\
\
	for _, req := range requests {\
		prioInt := jobs.GetPriorityInt(jobs.PriorityMedium)\
		if req.Priority != "" {\
			prioInt = jobs.GetPriorityInt(jobs.JobPriority(req.Priority))\
		}\
		maxR := req.MaxRetries\
		if maxR == 0 { maxR = 3 }\
\
		j := &jobs.Job{\
			ID:               uuid.New().String(),\
			Type:             req.Type,\
			Payload:          req.Payload,\
			Status:           jobs.StatusPending,\
			Priority:         jobs.JobPriority(req.Priority),\
			MaxRetries:       maxR,\
			CorrelationID:    req.CorrelationID,\
			Timeout:          req.Timeout,\
			Version:          req.Version,\
			TenantID:         req.TenantID,\
			Webhook:          req.Webhook,\
			DedupKey:         req.DedupKey,\
			Dependencies:     req.Dependencies,\
			ShardKey:         req.ShardKey,\
			CronExpr:         req.CronExpr,\
			RunAt:            req.RunAt,\
			CreatedAt:        now,\
			UpdatedAt:        now,\
		}\
		if j.Priority == "" { j.Priority = jobs.PriorityMedium }\
		if j.Timeout == 0 { j.Timeout = 60 }\
		if j.Version == 0 { j.Version = 1 }\
\
		batch = append(batch, j)\
		if len(req.Dependencies) == 0 {\
			toEnqueue = append(toEnqueue, struct{ TenantID, ShardKey, JobID string; Priority int }{\
				TenantID: j.TenantID, ShardKey: j.ShardKey, JobID: j.ID, Priority: prioInt,\
			})\
		}\
	}\
\
	if err := s.store.SaveBatch(ctx, batch); err != nil {\
		return nil, apperr.NewInternal("failed to persist job batch", err)\
	}\
\
	if len(toEnqueue) > 0 {\
		if err := s.queue.EnqueueBatch(ctx, toEnqueue); err != nil {\
			s.logger.Error("failed to enqueue batch to active queue", "error", err)\
		}\
	}\
	for _, j := range batch {\
		s.publishWebhookEvent(ctx, j, "created", map[string]interface{}{"job_id": j.ID})\
	}\
	return batch, nil\
}\
' internal/service/job_service.go
