#!/bin/bash
sed -i '/type Pool struct {/a \
\	activeJobs   map[string]context.CancelFunc\
\	activeJobsMu sync.Mutex' internal/worker/pool/pool.go

sed -i '/return &Pool{/a \
\		activeJobs:   make(map[string]context.CancelFunc),' internal/worker/pool/pool.go
