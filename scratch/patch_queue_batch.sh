sed -i 's/EnqueueBatch(ctx context.Context, jobs \[\]struct{ TenantID, ShardKey, JobID string; Priority int }) error/EnqueueBatch(ctx context.Context, jobs \[\]\*jobs.Job) error/g' internal/queue/interface.go

cat << 'REDIS' > scratch/patch_redis.patch
--- internal/queue/redis/redis.go
+++ internal/queue/redis/redis.go
@@ -52,14 +52,15 @@
 }
 
-func (q *RedisQueue) EnqueueBatch(ctx context.Context, jobs []struct{ TenantID, ShardKey, JobID string; Priority int }) error {
+func (q *RedisQueue) EnqueueBatch(ctx context.Context, jobs []*jobspkg.Job) error {
 	if len(jobs) == 0 { return nil }
 	pipe := q.client.Pipeline()
 	for _, j := range jobs {
 		queueKey := "task_queue:queue:" + j.TenantID
 		if j.ShardKey != "" {
 			queueKey += ":" + j.ShardKey
 		}
-		pipe.ZAdd(ctx, queueKey, redis.Z{Score: float64(j.Priority), Member: j.JobID})
+		prioInt := jobspkg.GetPriorityInt(j.Priority)
+		pipe.ZAdd(ctx, queueKey, redis.Z{Score: float64(prioInt), Member: j.ID})
 	}
 	_, err := pipe.Exec(ctx)
 	return err
REDIS
patch internal/queue/redis/redis.go < scratch/patch_redis.patch

cat << 'MOCK' > scratch/patch_mock_fixed.patch
--- internal/queue/mock.go
+++ internal/queue/mock.go
@@ -24,8 +24,8 @@
 	return m.client.RPush(ctx, "mock_queue", string(data)).Err()
 }
 
-func (m *MockQueue) EnqueueBatch(ctx context.Context, jobs []struct{ TenantID, ShardKey, JobID string; Priority int }) error {
+func (m *MockQueue) EnqueueBatch(ctx context.Context, jobs []*jobs.Job) error {
 	for _, j := range jobs {
-		m.Enqueue(ctx, j.TenantID, j.ShardKey, j.JobID, j.Priority)
+		m.Enqueue(ctx, j)
 	}
 	return nil
 }
MOCK
patch internal/queue/mock.go < scratch/patch_mock_fixed.patch

# Fix dual_store syntax error
git checkout internal/storage/models/dual_store.go

# Now apply proper dual_store modifications
cat << 'DUAL' > scratch/patch_dual.patch
--- internal/storage/models/dual_store.go
+++ internal/storage/models/dual_store.go
@@ -23,6 +23,13 @@
 	}
 }
 
+func (s *DualStore) SaveBatch(ctx context.Context, batch []*jobs.Job) error {
+	if err := s.Primary.SaveBatch(ctx, batch); err != nil {
+		return err
+	}
+	s.Secondary.SaveBatch(ctx, batch)
+	return nil
+}
+
 // Save writes to both stores.
 func (s *DualStore) Save(ctx context.Context, job *jobs.Job) error {
 	if err := s.Primary.Save(ctx, job); err != nil {
DUAL
patch internal/storage/models/dual_store.go < scratch/patch_dual.patch

# Fix job_service.go CreateJobBatch
cat << 'SERVICE' > scratch/patch_service_queue.patch
--- internal/service/job_service.go
+++ internal/service/job_service.go
@@ -218,7 +218,7 @@
 	if len(requests) == 0 { return nil, nil }
 	var batch []*jobs.Job
 	now := time.Now().UTC()
-	var toEnqueue []struct{ TenantID, ShardKey, JobID string; Priority int }
+	var toEnqueue []*jobs.Job
 
 	for _, req := range requests {
 		prioInt := jobs.GetPriorityInt(jobs.PriorityMedium)
@@ -250,9 +250,7 @@
 
 		batch = append(batch, j)
 		if len(req.Dependencies) == 0 {
-			toEnqueue = append(toEnqueue, struct{ TenantID, ShardKey, JobID string; Priority int }{
-				TenantID: j.TenantID, ShardKey: j.ShardKey, JobID: j.ID, Priority: prioInt,
-			})
+			toEnqueue = append(toEnqueue, j)
 		}
 	}
 
SERVICE
patch internal/service/job_service.go < scratch/patch_service_queue.patch
