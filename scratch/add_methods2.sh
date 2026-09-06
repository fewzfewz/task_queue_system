sed -i '/func (s \*InMemoryStore) GetByIDs/i \
func (s *InMemoryStore) GetReadyDAGJobs(ctx context.Context, limit int) ([]*jobs.Job, error) {\
	return nil, nil\
}\
func (s *InMemoryStore) GetDependentJobs(ctx context.Context, parentID string) ([]*jobs.Job, error) {\
	return nil, nil\
}\
' internal/storage/models/models.go

sed -i '/func (s \*DualStore) GetByIDs/i \
func (s *DualStore) GetReadyDAGJobs(ctx context.Context, limit int) ([]*jobs.Job, error) {\
	return s.primary.GetReadyDAGJobs(ctx, limit)\
}\
func (s *DualStore) GetDependentJobs(ctx context.Context, parentID string) ([]*jobs.Job, error) {\
	return s.primary.GetDependentJobs(ctx, parentID)\
}\
' internal/storage/models/models.go
