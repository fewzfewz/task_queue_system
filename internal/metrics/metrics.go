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
	QueueLength = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "task_queue_length",
		Help: "Current number of pending jobs in the queue.",
	}, []string{"queue", "tenant_id"})

	// WorkerBusyRatio tracks the ratio of busy workers to total workers.
	WorkerBusyRatio = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "task_queue_worker_busy_ratio",
		Help: "Ratio of busy workers to total workers (0.0 to 1.0).",
	})

	// WorkerGracefulShutdownTotal tracks the number of successful graceful shutdowns.
	WorkerGracefulShutdownTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "worker_graceful_shutdown_total",
		Help: "Total number of worker processes that drained cleanly before shutdown.",
	})


	// WorkerUtilization tracks the number of busy workers.
	WorkerUtilization = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "task_queue_worker_busy",
		Help: "Number of workers currently executing a job.",
	})

	// JobSLACompliance tracks if jobs meet the execution time target (e.g. < 5s).
	JobSLACompliance = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "task_queue_job_sla_compliance_total",
		Help: "Total number of jobs segmented by SLA compliance status (true/false).",
	}, []string{"job_type", "tenant_id", "compliant"})

	// WebhookDeliveryFailuresTotal tracks failed webhook delivery attempts.
	WebhookDeliveryFailuresTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "task_queue_webhook_delivery_failures_total",
		Help: "Total number of failed webhook delivery attempts.",
	}, []string{"tenant_id", "reason"})
)


