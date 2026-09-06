package standard

import (
	"context"
	"fmt"
	"log/slog"

	"task-queue-system/internal/jobs"
	"task-queue-system/internal/worker/plugin"
)

// MapReducePlugin implements plugin.JobPlugin for jobs of type "map_reduce".
// It takes a list of items, spawns a map job for each item, and schedules a
// final reduce job that waits (using DAG dependencies) for all map jobs to finish.
type MapReducePlugin struct {
	logger *slog.Logger
}

func NewMapReducePlugin(logger *slog.Logger) *MapReducePlugin {
	return &MapReducePlugin{logger: logger}
}

func init() {
	plugin.RegisterGlobal(NewMapReducePlugin(slog.Default()))
}

func (p *MapReducePlugin) Type() string {
	return "map_reduce"
}

func (p *MapReducePlugin) Execute(ctx context.Context, job *jobs.Job) (interface{}, error) {
	itemsRaw, ok := job.Payload["items"].([]interface{})
	if !ok || len(itemsRaw) == 0 {
		return nil, fmt.Errorf("map_reduce plugin: missing or empty 'items' array")
	}

	mapType, _ := job.Payload["map_job_type"].(string)
	if mapType == "" {
		return nil, fmt.Errorf("map_reduce plugin: missing 'map_job_type'")
	}

	reduceType, _ := job.Payload["reduce_job_type"].(string)
	if reduceType == "" {
		return nil, fmt.Errorf("map_reduce plugin: missing 'reduce_job_type'")
	}

	submitter := plugin.GetSubmitter(ctx)
	if submitter == nil {
		return nil, fmt.Errorf("map_reduce plugin: job submitter not found in context (ensure worker injected it)")
	}

	p.logger.Info("starting map_reduce fan-out", "items", len(itemsRaw), "map_type", mapType, "job_id", job.ID)

	var mapJobIDs []string

	// Fan-out: Create a map job for each item
	for i, item := range itemsRaw {
		payload := map[string]interface{}{
			"item":        item,
			"map_index":   i,
			"parent_job":  job.ID,
		}

		// Inherit priority from parent if not specified
		priority := string(job.Priority)
		if priority == "" {
			priority = "medium"
		}

		mapJob, err := submitter.CreateJob(ctx, mapType, payload, nil, priority, 3, "", "", "", "", job.CorrelationID, 0, 1, job.TenantID, nil, "", nil, "")
		if err != nil {
			return nil, fmt.Errorf("map_reduce plugin: failed to spawn map job %d: %w", i, err)
		}
		mapJobIDs = append(mapJobIDs, mapJob.ID)

		// Report progress on fan-out
		progress := float64(i+1) / float64(len(itemsRaw)) * 50.0
		plugin.ReportProgress(ctx, progress)
	}

	// Fan-in: Create the reduce job with dependencies on ALL map jobs
	reducePayload := map[string]interface{}{
		"parent_job": job.ID,
		"map_jobs":   mapJobIDs,
		"item_count": len(itemsRaw),
	}
	reducePriority := string(job.Priority)
	if reducePriority == "" {
		reducePriority = "medium"
	}

	reduceJob, err := submitter.CreateJob(ctx, reduceType, reducePayload, nil, reducePriority, 3, "", "", "", "", job.CorrelationID, 0, 1, job.TenantID, nil, "", mapJobIDs, "")
	if err != nil {
		return nil, fmt.Errorf("map_reduce plugin: failed to spawn reduce job: %w", err)
	}

	p.logger.Info("map_reduce fan-out complete", "map_jobs_spawned", len(mapJobIDs), "reduce_job_id", reduceJob.ID)
	plugin.ReportProgress(ctx, 100.0)

	return map[string]interface{}{
		"status":          "fan_out_complete",
		"map_jobs_count":  len(mapJobIDs),
		"reduce_job_id":   reduceJob.ID,
		"reduce_job_type": reduceType,
	}, nil
}
