package tq

import (
	"context"
	"net/url"
	"strconv"
	"time"
)

// Submit submits a new job to the queue.
func (c *Client) Submit(ctx context.Context, req SubmitJobRequest) (*Job, error) {
	var out Job
	if err := c.doReq(ctx, "POST", "/jobs", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SubmitBatch submits multiple jobs in a single request.
func (c *Client) SubmitBatch(ctx context.Context, reqs []SubmitJobRequest) (*BatchJobResponse, error) {
	payload := map[string]interface{}{
		"jobs": reqs,
	}
	var out BatchJobResponse
	if err := c.doReq(ctx, "POST", "/jobs/batch", payload, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetJob fetches the current state of a job by ID.
func (c *Client) GetJob(ctx context.Context, jobID string) (*Job, error) {
	var out Job
	if err := c.doReq(ctx, "GET", "/jobs/"+url.PathEscape(jobID), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// JobFilter provides search criteria for listing jobs.
type JobFilter struct {
	Status        JobStatus
	Type          string
	LabelKey      string
	LabelValue    string
	CreatedAfter  *time.Time
	CreatedBefore *time.Time
	Limit         int
	Page          int
}

// ListJobs searches for jobs based on the provided filter.
func (c *Client) ListJobs(ctx context.Context, filter JobFilter) (*ListJobsResponse, error) {
	q := url.Values{}
	if filter.Status != "" {
		q.Set("status", string(filter.Status))
	}
	if filter.Type != "" {
		q.Set("type", filter.Type)
	}
	if filter.LabelKey != "" {
		q.Set("label_key", filter.LabelKey)
		if filter.LabelValue != "" {
			q.Set("label_value", filter.LabelValue)
		}
	}
	if filter.CreatedAfter != nil {
		q.Set("created_after", filter.CreatedAfter.Format(time.RFC3339))
	}
	if filter.CreatedBefore != nil {
		q.Set("created_before", filter.CreatedBefore.Format(time.RFC3339))
	}
	if filter.Limit > 0 {
		q.Set("limit", strconv.Itoa(filter.Limit))
	}
	if filter.Page > 0 {
		q.Set("page", strconv.Itoa(filter.Page))
	}

	path := "/jobs"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}

	var out ListJobsResponse
	if err := c.doReq(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Pause prevents a job from being executed until resumed.
func (c *Client) Pause(ctx context.Context, jobID string) error {
	return c.doReq(ctx, "POST", "/jobs/"+url.PathEscape(jobID)+"/pause", nil, nil)
}

// Resume allows a previously paused job to be executed.
func (c *Client) Resume(ctx context.Context, jobID string) error {
	return c.doReq(ctx, "POST", "/jobs/"+url.PathEscape(jobID)+"/resume", nil, nil)
}

// Cancel permanently aborts a pending or executing job.
func (c *Client) Cancel(ctx context.Context, jobID string) error {
	return c.doReq(ctx, "POST", "/jobs/"+url.PathEscape(jobID)+"/cancel", nil, nil)
}
