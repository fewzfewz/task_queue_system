sed -i '/func (s \*DualStore) GetByIDs/i \
func (s *DualStore) GetReadyDAGJobs(ctx context.Context, limit int) ([]*jobs.Job, error) {\
	return s.db.GetReadyDAGJobs(ctx, limit)\
}\
func (s *DualStore) GetDependentJobs(ctx context.Context, parentID string) ([]*jobs.Job, error) {\
	return s.db.GetDependentJobs(ctx, parentID)\
}\
' internal/storage/dual_store.go
