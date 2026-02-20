package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/openjobspec/ojs-backend-lite/internal/core"
	"github.com/openjobspec/ojs-backend-lite/internal/events"
	"github.com/openjobspec/ojs-backend-lite/internal/memory"
)

func setupTestServer() (*httptest.Server, *memory.MemoryBackend) {
	backend, _ := memory.New()
	broker := events.NewBroker()
	
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(OJSHeaders)
	r.Use(LimitRequestBody)
	r.Use(ValidateContentType)

	jobHandler := NewJobHandler(backend)
	workerHandler := NewWorkerHandler(backend)
	systemHandler := NewSystemHandler(backend)
	queueHandler := NewQueueHandler(backend)
	deadLetterHandler := NewDeadLetterHandler(backend)
	cronHandler := NewCronHandler(backend)
	workflowHandler := NewWorkflowHandler(backend)
	batchHandler := NewBatchHandler(backend)

	jobHandler.SetEventPublisher(broker)
	workerHandler.SetEventPublisher(broker)

	r.Get("/ojs/manifest", systemHandler.Manifest)
	r.Get("/ojs/v1/health", systemHandler.Health)
	r.Post("/ojs/v1/jobs", jobHandler.Create)
	r.Get("/ojs/v1/jobs/{id}", jobHandler.Get)
	r.Delete("/ojs/v1/jobs/{id}", jobHandler.Cancel)
	r.Post("/ojs/v1/jobs/batch", batchHandler.Create)
	r.Post("/ojs/v1/workers/fetch", workerHandler.Fetch)
	r.Post("/ojs/v1/workers/ack", workerHandler.Ack)
	r.Post("/ojs/v1/workers/nack", workerHandler.Nack)
	r.Post("/ojs/v1/workers/heartbeat", workerHandler.Heartbeat)
	r.Get("/ojs/v1/queues", queueHandler.List)
	r.Get("/ojs/v1/queues/{name}/stats", queueHandler.Stats)
	r.Post("/ojs/v1/queues/{name}/pause", queueHandler.Pause)
	r.Post("/ojs/v1/queues/{name}/resume", queueHandler.Resume)
	r.Get("/ojs/v1/dead-letter", deadLetterHandler.List)
	r.Get("/ojs/v1/cron", cronHandler.List)
	r.Post("/ojs/v1/cron", cronHandler.Register)
	r.Delete("/ojs/v1/cron/{name}", cronHandler.Delete)
	r.Post("/ojs/v1/workflows", workflowHandler.Create)
	r.Get("/ojs/v1/workflows/{id}", workflowHandler.Get)
	r.Delete("/ojs/v1/workflows/{id}", workflowHandler.Cancel)

	return httptest.NewServer(r), backend
}

// TestPostJob verifies POST /ojs/v1/jobs returns 201 with job.
func TestPostJob(t *testing.T) {
	srv, _ := setupTestServer()
	defer srv.Close()

	reqBody := map[string]interface{}{
		"type":  "test",
		"queue": "default",
		"args":  []string{"arg1"},
	}
	body, _ := json.Marshal(reqBody)

	resp, err := http.Post(srv.URL+"/ojs/v1/jobs", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("Expected status 201, got %d", resp.StatusCode)
	}

	var result struct {
		Job *core.Job `json:"job"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if result.Job.ID == "" {
		t.Error("Job ID should be present")
	}

	if result.Job.Type != "test" {
		t.Errorf("Expected type test, got %s", result.Job.Type)
	}

	if result.Job.State != core.StateAvailable {
		t.Errorf("Expected state %s, got %s", core.StateAvailable, result.Job.State)
	}

	// Verify OJS-Version header
	if version := resp.Header.Get("OJS-Version"); version != "1.0" {
		t.Errorf("Expected OJS-Version 1.0, got %s", version)
	}
}

// TestPostJobInvalidBody verifies 400 on invalid JSON.
func TestPostJobInvalidBody(t *testing.T) {
	srv, _ := setupTestServer()
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/ojs/v1/jobs", "application/json", bytes.NewReader([]byte("invalid json")))
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", resp.StatusCode)
	}
}

// TestPostJobMissingType verifies 400 on missing required field.
func TestPostJobMissingType(t *testing.T) {
	srv, _ := setupTestServer()
	defer srv.Close()

	reqBody := map[string]interface{}{
		"queue": "default",
		"args":  []string{"arg1"},
	}
	body, _ := json.Marshal(reqBody)

	resp, err := http.Post(srv.URL+"/ojs/v1/jobs", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", resp.StatusCode)
	}
}

// TestGetJob verifies GET /ojs/v1/jobs/{id} returns 200.
func TestGetJob(t *testing.T) {
	srv, backend := setupTestServer()
	defer srv.Close()

	// Create a job
	job := &core.Job{Type: "test", Queue: "default", Args: json.RawMessage(`[]`)}
	pushed, _ := backend.Push(nil, job)

	resp, err := http.Get(srv.URL + "/ojs/v1/jobs/" + pushed.ID)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var result struct {
		Job *core.Job `json:"job"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if result.Job.ID != pushed.ID {
		t.Errorf("Expected ID %s, got %s", pushed.ID, result.Job.ID)
	}
}

// TestGetJobNotFound verifies 404 on non-existent job.
func TestGetJobNotFound(t *testing.T) {
	srv, _ := setupTestServer()
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/ojs/v1/jobs/non-existent-id")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", resp.StatusCode)
	}
}

// TestDeleteJob verifies DELETE /ojs/v1/jobs/{id} cancels job.
func TestDeleteJob(t *testing.T) {
	srv, backend := setupTestServer()
	defer srv.Close()

	// Create a job
	job := &core.Job{Type: "test", Queue: "default", Args: json.RawMessage(`[]`)}
	pushed, _ := backend.Push(nil, job)

	req, _ := http.NewRequest("DELETE", srv.URL+"/ojs/v1/jobs/"+pushed.ID, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var result struct {
		Job *core.Job `json:"job"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if result.Job.State != core.StateCancelled {
		t.Errorf("Expected state %s, got %s", core.StateCancelled, result.Job.State)
	}
}

// TestPostJobsBatch verifies batch enqueue.
func TestPostJobsBatch(t *testing.T) {
	srv, _ := setupTestServer()
	defer srv.Close()

	reqBody := map[string]interface{}{
		"jobs": []map[string]interface{}{
			{"type": "test1", "queue": "default", "args": []string{}},
			{"type": "test2", "queue": "default", "args": []string{}},
		},
	}
	body, _ := json.Marshal(reqBody)

	resp, err := http.Post(srv.URL+"/ojs/v1/jobs/batch", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("Expected status 201, got %d", resp.StatusCode)
	}

	var result struct {
		Jobs []*core.Job `json:"jobs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(result.Jobs) != 2 {
		t.Errorf("Expected 2 jobs, got %d", len(result.Jobs))
	}
}

// TestPostWorkersFetch verifies worker fetch endpoint.
func TestPostWorkersFetch(t *testing.T) {
	srv, backend := setupTestServer()
	defer srv.Close()

	// Create a job
	job := &core.Job{Type: "test", Queue: "default", Args: json.RawMessage(`[]`)}
	backend.Push(nil, job)

	reqBody := map[string]interface{}{
		"queues":          []string{"default"},
		"count":           1,
		"worker_id":       "worker1",
		"visibility_ms":   30000,
	}
	body, _ := json.Marshal(reqBody)

	resp, err := http.Post(srv.URL+"/ojs/v1/workers/fetch", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var result struct {
		Jobs []*core.Job `json:"jobs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(result.Jobs) != 1 {
		t.Errorf("Expected 1 job, got %d", len(result.Jobs))
	}

	if result.Jobs[0].State != core.StateActive {
		t.Errorf("Expected state %s, got %s", core.StateActive, result.Jobs[0].State)
	}
}

// TestPostWorkersAck verifies ack endpoint.
func TestPostWorkersAck(t *testing.T) {
	srv, backend := setupTestServer()
	defer srv.Close()

	// Create and fetch a job
	job := &core.Job{Type: "test", Queue: "default", Args: json.RawMessage(`[]`)}
	backend.Push(nil, job)
	fetched, _ := backend.Fetch(nil, []string{"default"}, 1, "worker1", 30000)

	reqBody := map[string]interface{}{
		"job_id": fetched[0].ID,
		"result": map[string]string{"status": "success"},
	}
	body, _ := json.Marshal(reqBody)

	resp, err := http.Post(srv.URL+"/ojs/v1/workers/ack", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var ackResp core.AckResponse
	if err := json.NewDecoder(resp.Body).Decode(&ackResp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if !ackResp.Acknowledged {
		t.Error("Job should be acknowledged")
	}

	if ackResp.State != core.StateCompleted {
		t.Errorf("Expected state %s, got %s", core.StateCompleted, ackResp.State)
	}
}

// TestPostWorkersNack verifies nack endpoint.
func TestPostWorkersNack(t *testing.T) {
	srv, backend := setupTestServer()
	defer srv.Close()

	maxAttempts := 3
	// Create and fetch a job
	job := &core.Job{Type: "test", Queue: "default", Args: json.RawMessage(`[]`), MaxAttempts: &maxAttempts}
	backend.Push(nil, job)
	fetched, _ := backend.Fetch(nil, []string{"default"}, 1, "worker1", 30000)

	reqBody := map[string]interface{}{
		"job_id":  fetched[0].ID,
		"error":   map[string]string{"message": "temporary error"},
		"requeue": false,
	}
	body, _ := json.Marshal(reqBody)

	resp, err := http.Post(srv.URL+"/ojs/v1/workers/nack", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var nackResp core.NackResponse
	if err := json.NewDecoder(resp.Body).Decode(&nackResp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if nackResp.State != core.StateRetryable {
		t.Errorf("Expected state %s, got %s", core.StateRetryable, nackResp.State)
	}
}

// TestPostWorkersHeartbeat verifies heartbeat endpoint.
func TestPostWorkersHeartbeat(t *testing.T) {
	srv, backend := setupTestServer()
	defer srv.Close()

	// Create and fetch a job
	job := &core.Job{Type: "test", Queue: "default", Args: json.RawMessage(`[]`)}
	backend.Push(nil, job)
	fetched, _ := backend.Fetch(nil, []string{"default"}, 1, "worker1", 30000)

	reqBody := map[string]interface{}{
		"worker_id":     "worker1",
		"active_jobs":   []string{fetched[0].ID},
		"visibility_ms": 30000,
	}
	body, _ := json.Marshal(reqBody)

	resp, err := http.Post(srv.URL+"/ojs/v1/workers/heartbeat", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var hbResp core.HeartbeatResponse
	if err := json.NewDecoder(resp.Body).Decode(&hbResp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if hbResp.State != "active" {
		t.Errorf("Expected state active, got %s", hbResp.State)
	}

	if hbResp.Directive != "continue" {
		t.Errorf("Expected directive continue, got %s", hbResp.Directive)
	}
}

// TestGetQueues verifies queue listing endpoint.
func TestGetQueues(t *testing.T) {
	srv, backend := setupTestServer()
	defer srv.Close()

	// Create jobs in different queues
	job1 := &core.Job{Type: "test", Queue: "queue1", Args: json.RawMessage(`[]`)}
	job2 := &core.Job{Type: "test", Queue: "queue2", Args: json.RawMessage(`[]`)}
	backend.Push(nil, job1)
	backend.Push(nil, job2)

	resp, err := http.Get(srv.URL + "/ojs/v1/queues")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var result struct {
		Queues     []core.QueueInfo `json:"queues"`
		Pagination map[string]any   `json:"pagination"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(result.Queues) != 2 {
		t.Errorf("Expected 2 queues, got %d", len(result.Queues))
	}
}

// TestGetQueueStats verifies queue stats endpoint.
func TestGetQueueStats(t *testing.T) {
	srv, backend := setupTestServer()
	defer srv.Close()

	// Create jobs
	for i := 0; i < 3; i++ {
		job := &core.Job{Type: "test", Queue: "default", Args: json.RawMessage(`[]`)}
		backend.Push(nil, job)
	}

	resp, err := http.Get(srv.URL + "/ojs/v1/queues/default/stats")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var result struct {
		Queue map[string]any `json:"queue"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if name, ok := result.Queue["name"].(string); !ok || name != "default" {
		t.Errorf("Expected queue name default, got %v", result.Queue["name"])
	}

	if available, ok := result.Queue["available"].(float64); !ok || int(available) != 3 {
		t.Errorf("Expected 3 available, got %v", result.Queue["available"])
	}
}

// TestPostQueuePause verifies pause endpoint.
func TestPostQueuePause(t *testing.T) {
	srv, backend := setupTestServer()
	defer srv.Close()

	// Create a job
	job := &core.Job{Type: "test", Queue: "default", Args: json.RawMessage(`[]`)}
	backend.Push(nil, job)

	resp, err := http.Post(srv.URL+"/ojs/v1/queues/default/pause", "application/json", nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// Verify queue is paused
	fetchReq := map[string]interface{}{
		"queues":        []string{"default"},
		"count":         1,
		"worker_id":     "worker1",
		"visibility_ms": 30000,
	}
	body, _ := json.Marshal(fetchReq)
	fetchResp, fetchErr := http.Post(srv.URL+"/ojs/v1/workers/fetch", "application/json", bytes.NewReader(body))
	if fetchErr != nil {
		t.Fatalf("Fetch request failed: %v", fetchErr)
	}
	defer fetchResp.Body.Close()

	var result struct {
		Jobs []*core.Job `json:"jobs"`
	}
	json.NewDecoder(fetchResp.Body).Decode(&result)

	if len(result.Jobs) != 0 {
		t.Error("Should not fetch from paused queue")
	}
}

// TestPostQueueResume verifies resume endpoint.
func TestPostQueueResume(t *testing.T) {
	srv, backend := setupTestServer()
	defer srv.Close()

	// Create and pause queue
	job := &core.Job{Type: "test", Queue: "default", Args: json.RawMessage(`[]`)}
	backend.Push(nil, job)
	backend.PauseQueue(nil, "default")

	resp, err := http.Post(srv.URL+"/ojs/v1/queues/default/resume", "application/json", nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// Verify queue is active
	fetchReq := map[string]interface{}{
		"queues":        []string{"default"},
		"count":         1,
		"worker_id":     "worker1",
		"visibility_ms": 30000,
	}
	body, _ := json.Marshal(fetchReq)
	fetchResp, fetchErr := http.Post(srv.URL+"/ojs/v1/workers/fetch", "application/json", bytes.NewReader(body))
	if fetchErr != nil {
		t.Fatalf("Fetch request failed: %v", fetchErr)
	}
	defer fetchResp.Body.Close()

	var result struct {
		Jobs []*core.Job `json:"jobs"`
	}
	json.NewDecoder(fetchResp.Body).Decode(&result)

	if len(result.Jobs) != 1 {
		t.Error("Should fetch from resumed queue")
	}
}

// TestGetDeadLetter verifies dead letter listing endpoint.
func TestGetDeadLetter(t *testing.T) {
	srv, backend := setupTestServer()
	defer srv.Close()

	maxAttempts := 0
	// Create a job and nack to discard it immediately (maxAttempts=0 means no retries)
	job := &core.Job{Type: "test", Queue: "default", Args: json.RawMessage(`[]`), MaxAttempts: &maxAttempts}
	backend.Push(nil, job)
	fetched, _ := backend.Fetch(nil, []string{"default"}, 1, "worker1", 30000)
	backend.Nack(nil, fetched[0].ID, &core.JobError{Message: "error"}, false)

	resp, err := http.Get(srv.URL + "/ojs/v1/dead-letter")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var result struct {
		Jobs       []*core.Job    `json:"jobs"`
		Pagination map[string]any `json:"pagination"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	total := int(result.Pagination["total"].(float64))
	if total != 1 {
		t.Errorf("Expected 1 dead letter job, got %d", total)
	}

	if len(result.Jobs) != 1 {
		t.Errorf("Expected 1 job in response, got %d", len(result.Jobs))
	}
}

// TestGetCron verifies cron listing endpoint.
func TestGetCron(t *testing.T) {
	srv, backend := setupTestServer()
	defer srv.Close()

	// Register a cron
	cron := &core.CronJob{
		Name:        "test-cron",
		Schedule:    "* * * * *",
		JobTemplate: &core.CronJobTemplate{Type: "test", Args: json.RawMessage(`[]`), Options: &core.EnqueueOptions{Queue: "default"}},
	}
	backend.RegisterCron(nil, cron)

	resp, err := http.Get(srv.URL + "/ojs/v1/cron")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var result struct {
		Crons []*core.CronJob `json:"crons"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(result.Crons) != 1 {
		t.Errorf("Expected 1 cron, got %d", len(result.Crons))
	}

	if result.Crons[0].Name != "test-cron" {
		t.Errorf("Expected name test-cron, got %s", result.Crons[0].Name)
	}
}

// TestPostCron verifies cron registration endpoint.
func TestPostCron(t *testing.T) {
	srv, _ := setupTestServer()
	defer srv.Close()

	reqBody := map[string]interface{}{
		"name":       "test-cron",
		"expression": "*/5 * * * *",
		"job_template": map[string]interface{}{
			"type": "test",
			"args": []string{},
			"options": map[string]string{
				"queue": "default",
			},
		},
	}
	body, _ := json.Marshal(reqBody)

	resp, err := http.Post(srv.URL+"/ojs/v1/cron", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("Expected status 201, got %d", resp.StatusCode)
	}

	var result struct {
		Cron *core.CronJob `json:"cron"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if result.Cron.Name != "test-cron" {
		t.Errorf("Expected name test-cron, got %s", result.Cron.Name)
	}

	if result.Cron.NextRunAt == "" {
		t.Error("NextRunAt should be set")
	}
}

// TestDeleteCron verifies cron deletion endpoint.
func TestDeleteCron(t *testing.T) {
	srv, backend := setupTestServer()
	defer srv.Close()

	// Register a cron
	cron := &core.CronJob{
		Name:        "test-cron",
		Schedule:    "* * * * *",
		JobTemplate: &core.CronJobTemplate{Type: "test", Args: json.RawMessage(`[]`), Options: &core.EnqueueOptions{Queue: "default"}},
	}
	backend.RegisterCron(nil, cron)

	req, _ := http.NewRequest("DELETE", srv.URL+"/ojs/v1/cron/test-cron", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var result struct {
		Cron *core.CronJob `json:"cron"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if result.Cron.Name != "test-cron" {
		t.Errorf("Expected name test-cron, got %s", result.Cron.Name)
	}
}

// TestPostWorkflows verifies workflow creation endpoint.
func TestPostWorkflows(t *testing.T) {
	srv, _ := setupTestServer()
	defer srv.Close()

	reqBody := map[string]interface{}{
		"name": "test-workflow",
		"type": "chain",
		"steps": []map[string]interface{}{
			{"type": "step1", "args": []string{}, "options": map[string]string{"queue": "default"}},
			{"type": "step2", "args": []string{}, "options": map[string]string{"queue": "default"}},
		},
	}
	body, _ := json.Marshal(reqBody)

	resp, err := http.Post(srv.URL+"/ojs/v1/workflows", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("Expected status 201, got %d", resp.StatusCode)
	}

	var result struct {
		Workflow *core.Workflow `json:"workflow"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if result.Workflow.Name != "test-workflow" {
		t.Errorf("Expected name test-workflow, got %s", result.Workflow.Name)
	}

	if result.Workflow.Type != "chain" {
		t.Errorf("Expected type chain, got %s", result.Workflow.Type)
	}

	if result.Workflow.State != "running" {
		t.Errorf("Expected state running, got %s", result.Workflow.State)
	}
}

// TestGetWorkflow verifies workflow retrieval endpoint.
func TestGetWorkflow(t *testing.T) {
	srv, backend := setupTestServer()
	defer srv.Close()

	// Create a workflow
	req := &core.WorkflowRequest{
		Name: "test-workflow",
		Type: "chain",
		Steps: []core.WorkflowJobRequest{
			{Type: "step1", Args: json.RawMessage(`[]`), Options: &core.EnqueueOptions{Queue: "default"}},
		},
	}
	workflow, _ := backend.CreateWorkflow(nil, req)

	resp, err := http.Get(srv.URL + "/ojs/v1/workflows/" + workflow.ID)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var result struct {
		Workflow *core.Workflow `json:"workflow"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if result.Workflow.ID != workflow.ID {
		t.Errorf("Expected ID %s, got %s", workflow.ID, result.Workflow.ID)
	}
}

// TestDeleteWorkflow verifies workflow cancellation endpoint.
func TestDeleteWorkflow(t *testing.T) {
	srv, backend := setupTestServer()
	defer srv.Close()

	// Create a workflow
	req := &core.WorkflowRequest{
		Name: "test-workflow",
		Type: "chain",
		Steps: []core.WorkflowJobRequest{
			{Type: "step1", Args: json.RawMessage(`[]`), Options: &core.EnqueueOptions{Queue: "default"}},
		},
	}
	workflow, _ := backend.CreateWorkflow(nil, req)

	httpReq, _ := http.NewRequest("DELETE", srv.URL+"/ojs/v1/workflows/"+workflow.ID, nil)
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var result struct {
		Workflow *core.Workflow `json:"workflow"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if result.Workflow.State != "cancelled" {
		t.Errorf("Expected state cancelled, got %s", result.Workflow.State)
	}
}

// TestGetManifest verifies manifest endpoint returns correct fields.
func TestGetManifest(t *testing.T) {
	srv, _ := setupTestServer()
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/ojs/manifest")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var manifest map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Verify required fields (based on actual handler_system.go structure)
	requiredFields := []string{"specversion", "implementation", "backend", "capabilities"}
	for _, field := range requiredFields {
		if _, ok := manifest[field]; !ok {
			t.Errorf("Manifest missing required field: %s", field)
		}
	}

	if impl, ok := manifest["implementation"].(map[string]interface{}); ok {
		if name, ok := impl["name"].(string); !ok || name == "" {
			t.Error("Manifest should have non-empty implementation name")
		}
		if version, ok := impl["version"].(string); !ok || version == "" {
			t.Error("Manifest should have non-empty implementation version")
		}
	} else {
		t.Error("Manifest should have implementation object")
	}
}

// TestGetHealth verifies health endpoint.
func TestGetHealth(t *testing.T) {
	srv, _ := setupTestServer()
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/ojs/v1/health")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var health core.HealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if health.Status != "ok" {
		t.Errorf("Expected status ok, got %s", health.Status)
	}

	if health.Backend.Type != "memory" {
		t.Errorf("Expected backend type memory, got %s", health.Backend.Type)
	}
}

// TestOJSVersionHeader verifies OJS-Version header is present.
func TestOJSVersionHeader(t *testing.T) {
	srv, _ := setupTestServer()
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/ojs/v1/health")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	version := resp.Header.Get("OJS-Version")
	if version == "" {
		t.Error("OJS-Version header should be present")
	}

	if version != "1.0" {
		t.Errorf("Expected OJS-Version 1.0, got %s", version)
	}
}

// TestRequestIDHeader verifies X-Request-Id header is generated.
func TestRequestIDHeader(t *testing.T) {
	srv, _ := setupTestServer()
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/ojs/v1/health")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	requestID := resp.Header.Get("X-Request-Id")
	if requestID == "" {
		t.Error("X-Request-Id header should be generated")
	}
}

// TestContentTypeValidation verifies Content-Type validation on POST.
func TestContentTypeValidation(t *testing.T) {
	srv, _ := setupTestServer()
	defer srv.Close()

	// Missing Content-Type should fail
	reqBody := `{"type":"test","queue":"default","args":{}}`
	req, _ := http.NewRequest("POST", srv.URL+"/ojs/v1/jobs", bytes.NewReader([]byte(reqBody)))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400 for missing Content-Type, got %d", resp.StatusCode)
	}

	// Invalid Content-Type should fail
	req, _ = http.NewRequest("POST", srv.URL+"/ojs/v1/jobs", bytes.NewReader([]byte(reqBody)))
	req.Header.Set("Content-Type", "text/plain")
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400 for invalid Content-Type, got %d", resp2.StatusCode)
	}
}

// TestPaginationDeadLetter verifies pagination parameters.
func TestPaginationDeadLetter(t *testing.T) {
	srv, backend := setupTestServer()
	defer srv.Close()

	maxAttempts := 0
	// Create 5 dead letter jobs (maxAttempts=0 means no retries, immediate discard)
	for i := 0; i < 5; i++ {
		job := &core.Job{Type: fmt.Sprintf("test%d", i), Queue: "default", Args: json.RawMessage(`[]`), MaxAttempts: &maxAttempts}
		backend.Push(nil, job)
		fetched, _ := backend.Fetch(nil, []string{"default"}, 1, "worker1", 30000)
		backend.Nack(nil, fetched[0].ID, &core.JobError{Message: "error"}, false)
	}

	// Fetch first page
	resp, err := http.Get(srv.URL + "/ojs/v1/dead-letter?limit=2&offset=0")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	var result struct {
		Jobs       []*core.Job    `json:"jobs"`
		Pagination map[string]any `json:"pagination"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	total := int(result.Pagination["total"].(float64))
	if total != 5 {
		t.Errorf("Expected total 5, got %d", total)
	}

	if len(result.Jobs) != 2 {
		t.Errorf("Expected 2 jobs in page, got %d", len(result.Jobs))
	}

	// Fetch second page
	resp2, err := http.Get(srv.URL + "/ojs/v1/dead-letter?limit=2&offset=2")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp2.Body.Close()

	json.NewDecoder(resp2.Body).Decode(&result)

	if len(result.Jobs) != 2 {
		t.Errorf("Expected 2 jobs in page 2, got %d", len(result.Jobs))
	}
}
