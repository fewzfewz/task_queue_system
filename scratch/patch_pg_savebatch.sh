sed -i '/func (s \*PostgresStore) GetByID/i \
func (s *PostgresStore) SaveBatch(ctx context.Context, batch []*jobs.Job) error {\
	if len(batch) == 0 {\
		return nil\
	}\
\
	query := `\
		INSERT INTO jobs (\
			id, tenant_id, type, payload, status, priority,\
			attempts, max_attempts, correlation_id, timeout_seconds,\
			version, scheduled_at, updated_at, progress,\
			webhook_url, webhook_secret, webhook_events, webhook_last_status, webhook_attempts,\
			dedup_key, dependencies, shard_key, cron_expr\
		) VALUES `\
\
	args := make([]interface{}, 0, len(batch)*23)\
	for i, job := range batch {\
		payload, _ := json.Marshal(job.Payload)\
		depsJSON, _ := json.Marshal(job.Dependencies)\
		var url, secret *string\
		var events []string\
		var lastStatus, attempts int\
		if job.Webhook != nil {\
			url = &job.Webhook.URL\
			secret = &job.Webhook.Secret\
			events = job.Webhook.Events\
			lastStatus = job.Webhook.LastStatus\
			attempts = job.Webhook.Attempts\
		}\
\
		if i > 0 {\
			query += ", "\
		}\
		\
		offset := i * 23\
		query += fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d)",\
			offset+1, offset+2, offset+3, offset+4, offset+5, offset+6, offset+7, offset+8, offset+9, offset+10,\
			offset+11, offset+12, offset+13, offset+14, offset+15, offset+16, offset+17, offset+18, offset+19, offset+20,\
			offset+21, offset+22, offset+23)\
\
		args = append(args, job.ID, job.TenantID, job.Type, payload, string(job.Status), string(job.Priority),\
			job.Retries, job.MaxRetries, job.CorrelationID, job.Timeout, job.Version, job.RunAt, time.Now().UTC(),\
			job.Progress, url, secret, events, lastStatus, attempts, nullIfEmpty(job.DedupKey), string(depsJSON), nullIfEmpty(job.ShardKey), nullIfEmpty(job.CronExpr))\
	}\
\
	_, err := s.pool.Exec(ctx, query, args...)\
	return err\
}\
' internal/storage/postgres/postgres.go
