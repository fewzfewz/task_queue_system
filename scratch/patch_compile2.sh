sed -i 's/s.Cache/s.Secondary/g' internal/storage/models/dual_store.go

# Remove duplicate SaveBatch in dual_store.go
sed -i '34,42d' internal/storage/models/dual_store.go

# Fix mock.go EnqueueBatch
cat << 'MOCK' > scratch/patch_mock2.patch
--- internal/queue/mock.go
+++ internal/queue/mock.go
@@ -28,10 +28,10 @@
 func (m *MockQueue) EnqueueBatch(ctx context.Context, jobs []struct{ TenantID, ShardKey, JobID string; Priority int }) error {
 	for _, j := range jobs {
-		m.Enqueue(ctx, &jobspkg.Job{
+		m.Enqueue(ctx, j.TenantID, j.ShardKey, j.JobID, j.Priority)
-			ID: j.JobID,
-			TenantID: j.TenantID,
-			ShardKey: j.ShardKey,
-			Priority: jobspkg.JobPriority(fmt.Sprintf("%d", j.Priority)),
-		})
 	}
 	return nil
 }
MOCK
patch internal/queue/mock.go < scratch/patch_mock2.patch
