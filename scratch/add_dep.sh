cat << 'INNER_EOF' >> internal/storage/postgres/postgres.go

func (s *PostgresStore) GetDependentJobs(ctx context.Context, parentID string) ([]*jobs.Job, error) {
	query := `SELECT id FROM jobs WHERE status = 'pending' AND dependencies @> ('"' || $1 || '"')::jsonb`
	rows, err := s.pool.Query(ctx, query, parentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	return s.GetByIDs(ctx, ids)
}
INNER_EOF
