sed -i '/Enqueue(ctx context.Context, tenantID, shardKey, jobID string, priority int) error/a \	EnqueueBatch(ctx context.Context, jobs []struct{ TenantID, ShardKey, JobID string; Priority int }) error' internal/queue/interface.go

sed -i '/func (q \*RedisQueue) Enqueue/i \
func (q *RedisQueue) EnqueueBatch(ctx context.Context, jobs []struct{ TenantID, ShardKey, JobID string; Priority int }) error {\
	if len(jobs) == 0 { return nil }\
	pipe := q.client.Pipeline()\
	for _, j := range jobs {\
		queueKey := "task_queue:queue:" + j.TenantID\
		if j.ShardKey != "" {\
			queueKey += ":" + j.ShardKey\
		}\
		pipe.ZAdd(ctx, queueKey, redis.Z{Score: float64(j.Priority), Member: j.JobID})\
	}\
	_, err := pipe.Exec(ctx)\
	return err\
}\
' internal/queue/redis/redis.go

sed -i '/func (m \*MockQueue) Enqueue/i \
func (m *MockQueue) EnqueueBatch(ctx context.Context, jobs []struct{ TenantID, ShardKey, JobID string; Priority int }) error {\
	for _, j := range jobs {\
		m.Enqueue(ctx, j.TenantID, j.ShardKey, j.JobID, j.Priority)\
	}\
	return nil\
}\
' internal/queue/mock.go
