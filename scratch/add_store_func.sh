sed -i '/GetByIDs(ctx context.Context, ids \[\]string) (\[\]\*jobs.Job, error)/a \
	GetReadyDAGJobs(ctx context.Context, limit int) ([]*jobs.Job, error)' internal/storage/models/models.go
