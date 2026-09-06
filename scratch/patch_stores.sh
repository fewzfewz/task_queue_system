sed -i '/func (s \*InMemoryStore) Save(/i \
func (s *InMemoryStore) SaveBatch(ctx context.Context, batch []*jobs.Job) error {\
	s.mu.Lock()\
	defer s.mu.Unlock()\
	for _, job := range batch {\
		s.data[job.ID] = job\
	}\
	return nil\
}\
' internal/storage/models/inmemory.go

sed -i '/func (s \*RedisStore) Save(/i \
func (s *RedisStore) SaveBatch(ctx context.Context, batch []*jobs.Job) error {\
	pipe := s.client.Pipeline()\
	for _, job := range batch {\
		data, err := json.Marshal(job)\
		if err != nil {\
			return err\
		}\
		pipe.Set(ctx, "job:"+job.ID, string(data), 0)\
	}\
	_, err := pipe.Exec(ctx)\
	return err\
}\
' internal/storage/models/redis_store.go

sed -i '/func (s \*DualStore) Save(/i \
func (s *DualStore) SaveBatch(ctx context.Context, batch []*jobs.Job) error {\
	if err := s.primary.SaveBatch(ctx, batch); err != nil {\
		return err\
	}\
	s.cache.SaveBatch(ctx, batch)\
	return nil\
}\
' internal/storage/models/dual_store.go
