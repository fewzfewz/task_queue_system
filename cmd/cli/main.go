package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"task-queue-system/internal/api/dto"
)

const (
	defaultBaseURL = "http://localhost:8080"
	defaultAPIKey  = "secret-api-key"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "submit":
		doSubmit()
	case "status":
		doStatus()
	case "help":
		printUsage()
	default:
		fmt.Printf("Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Task Queue CLI")
	fmt.Println("Usage:")
	fmt.Println("  tq submit --type <type> [--payload '{\"key\":\"val\"}'] [--priority high]")
	fmt.Println("  tq status <job-id>")
	fmt.Println("  tq help")
}

func doSubmit() {
	submitCmd := flag.NewFlagSet("submit", flag.ExitOnError)
	jobType := submitCmd.String("type", "", "Type of job (email, image, etc)")
	payloadStr := submitCmd.String("payload", "{}", "JSON payload string")
	priority := submitCmd.String("priority", "medium", "Job priority (low, medium, high)")
	baseURL := submitCmd.String("url", defaultBaseURL, "Base API URL")
	apiKey := submitCmd.String("key", defaultAPIKey, "API Key for authentication")

	_ = submitCmd.Parse(os.Args[2:])

	if *jobType == "" {
		fmt.Println("Error: --type is required")
		submitCmd.Usage()
		os.Exit(1)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(*payloadStr), &payload); err != nil {
		fmt.Printf("Error: invalid JSON payload: %v\n", err)
		os.Exit(1)
	}

	reqBody := dto.CreateJobRequest{
		Type:     *jobType,
		Payload:  payload,
		Priority: *priority,
	}

	jsonData, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", *baseURL+"/jobs", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", *apiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Error: API call failed: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("Error: server responded with %d: %s\n", resp.StatusCode, string(body))
		os.Exit(1)
	}

	var jobResp dto.JobResponse
	_ = json.NewDecoder(resp.Body).Decode(&jobResp)

	fmt.Printf("✔ Job submitted successfully!\n")
	fmt.Printf("  ID      : %s\n", jobResp.ID)
	fmt.Printf("  Status  : %s\n", jobResp.Status)
}

func doStatus() {
	if len(os.Args) < 3 {
		fmt.Println("Error: job ID is required")
		fmt.Println("Usage: tq status <job-id> [--url http://...] [--key secret]")
		os.Exit(1)
	}

	jobID := os.Args[2]
	
	// Quick manual parse for remaining flags if any
	baseURL := defaultBaseURL
	apiKey := defaultAPIKey
	
	for i, arg := range os.Args {
		if arg == "--url" && i+1 < len(os.Args) {
			baseURL = os.Args[i+1]
		}
		if arg == "--key" && i+1 < len(os.Args) {
			apiKey = os.Args[i+1]
		}
	}

	req, _ := http.NewRequest("GET", fmt.Sprintf("%s/jobs/%s", baseURL, jobID), nil)
	req.Header.Set("X-API-Key", apiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Error: API call failed: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("Error: server responded with %d: %s\n", resp.StatusCode, string(body))
		os.Exit(1)
	}

	var jobResp dto.JobResponse
	_ = json.NewDecoder(resp.Body).Decode(&jobResp)

	fmt.Printf("Job Details:\n")
	fmt.Printf("  ID         : %s\n", jobResp.ID)
	fmt.Printf("  Type       : %s\n", jobResp.Type)
	fmt.Printf("  Status     : %s\n", strings.ToUpper(jobResp.Status))
	fmt.Printf("  Retries    : %d/%d\n", jobResp.Retries, jobResp.MaxRetries)
	fmt.Printf("  Created    : %s\n", jobResp.CreatedAt)
	fmt.Printf("  Updated    : %s\n", jobResp.UpdatedAt)
}
