package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// JobLatency tracks the execution time of jobs in seconds.
	JobLatency = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "task_queue_job_latency_seconds",
		Help:    "Execution time of jobs in seconds.",
		Buckets: prometheus.DefBuckets,
	}, []string{"job_type", "tenant_id"})

	// JobTotal tracks the total number of jobs processed.
	JobTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "task_queue_jobs_total",
		Help: "Total number of jobs processed.",
	}, []string{"job_type", "tenant_id", "status"})

	// QueueLength tracks the current number of pending jobs.
	// This usually needs to be updated periodically from the queue backend.
	QueueLength = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "task_queue_length",
		Help: "Current number of pending jobs in the queue.",
	})

	// WorkerUtilization tracks the number of busy workers.
	WorkerUtilization = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "task_queue_worker_busy",
		Help: "Number of workers currently executing a job.",
	})
)
