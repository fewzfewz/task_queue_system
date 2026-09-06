sed -i 's/s.primary/s.Primary/g' internal/storage/models/dual_store.go
sed -i 's/s.cache/s.Cache/g' internal/storage/models/dual_store.go

# Fix mock.go EnqueueBatch
cat << 'MOCK' > scratch/patch_mock.patch
--- internal/queue/mock.go
+++ internal/queue/mock.go
@@ -25,7 +25,12 @@
 
 func (m *MockQueue) EnqueueBatch(ctx context.Context, jobs []struct{ TenantID, ShardKey, JobID string; Priority int }) error {
 	for _, j := range jobs {
-		m.Enqueue(ctx, j.TenantID, j.ShardKey, j.JobID, j.Priority)
+		m.Enqueue(ctx, &jobspkg.Job{
+			ID: j.JobID,
+			TenantID: j.TenantID,
+			ShardKey: j.ShardKey,
+			Priority: jobspkg.JobPriority(fmt.Sprintf("%d", j.Priority)),
+		})
 	}
 	return nil
 }
MOCK
patch internal/queue/mock.go < scratch/patch_mock.patch
