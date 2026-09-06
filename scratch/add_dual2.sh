sed -i '/func (s \*DualStore) GetByIDs/i \
func (s *DualStore) GetReadyDAGJobs(ctx context.Context, limit int) ([]*jobs.Job, error) {\
	return s.primary.GetReadyDAGJobs(ctx, limit)\
}\
func (s *DualStore) GetDependentJobs(ctx context.Context, parentID string) ([]*jobs.Job, error) {\
	return s.primary.GetDependentJobs(ctx, parentID)\
}\
' internal/storage/models/dual_store.go
