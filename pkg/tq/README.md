# Go Client SDK for Task Queue

This package provides a strongly-typed, native Go client for interacting with the Task Queue system. It abstracts away HTTP requests, JSON parsing, and authentication boilerplate.

## Installation

```bash
go get github.com/your-org/task-queue-system/pkg/tq
```

*(Note: update the import path to match your module name)*

## Usage Example

```go
package main

import (
	"context"
	"fmt"
	"log"

	"task-queue-system/pkg/tq"
)

func main() {
	// Initialize the client with the API base URL and an API Key.
	// You can generate API Keys from the Operator Dashboard.
	client := tq.NewClient("http://localhost:8080", "tq_live_...")

	// 1. Submit a one-off job
	job, err := client.Submit(context.Background(), tq.SubmitJobRequest{
		Type: "email",
		Payload: map[string]interface{}{
			"to":      "user@example.com",
			"subject": "Welcome!",
		},
	})
	if err != nil {
		log.Fatalf("Submit failed: %v", err)
	}
	fmt.Println("Job queued with ID:", job.ID)

	// 2. Submit a recurring cron job
	cronJob, err := client.Submit(context.Background(), tq.SubmitJobRequest{
		Type: "daily-report",
		Payload: map[string]interface{}{},
		CronExpr: "0 8 * * *", // run daily at 8 AM
	})
	if err != nil {
		log.Fatalf("Submit failed: %v", err)
	}
	fmt.Println("Cron scheduled with ID:", cronJob.ID)

	// 3. Fetch Job Status
	status, err := client.GetJob(context.Background(), job.ID)
	if err != nil {
		log.Fatalf("GetJob failed: %v", err)
	}
	fmt.Printf("Job Status: %s (Progress: %.0f%%)\n", status.Status, status.Progress)
}
```

## Features

- **Jobs API**: Submit, Batch Submit, Retrieve, Search, Pause, Resume, and Cancel jobs.
- **Webhooks API**: Register, List, Delete, and Retrieve Webhooks.
- **Strong Typing**: Full Go structs for all job parameters, filters, and webhook configurations.
- **Automatic Auth**: Client transparently attaches the `X-API-Key` header to all outgoing requests.
