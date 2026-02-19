package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/openjobspec/ojs-backend-lite/internal/core"
	"github.com/openjobspec/ojs-backend-lite/internal/events"
	"github.com/openjobspec/ojs-backend-lite/internal/memory"
)

// setupFullTestServer creates a server with all routes including admin and metrics.
func setupFullTestServer() (*httptest.Server, *memory.MemoryBackend) {
	backend, _ := memory.New()
	broker := events.NewBroker()

	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(Metrics)
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
	adminHandler := NewAdminHandler(backend)
	healthDetailHandler := NewHealthDetailHandler(backend)

	jobHandler.SetEventPublisher(broker)
	workerHandler.SetEventPublisher(broker)

	r.Get("/ojs/manifest", systemHandler.Manifest)
	r.Get("/ojs/v1/health", systemHandler.Health)
	r.Get("/ojs/v1/health/detail", healthDetailHandler.HealthDetail)
	r.Get("/metrics", MetricsHandler)
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
	r.Post("/ojs/v1/dead-letter/{id}/retry", deadLetterHandler.Retry)
	r.Delete("/ojs/v1/dead-letter/{id}", deadLetterHandler.Delete)
	r.Get("/ojs/v1/cron", cronHandler.List)
	r.Post("/ojs/v1/cron", cronHandler.Register)
	r.Delete("/ojs/v1/cron/{name}", cronHandler.Delete)
	r.Post("/ojs/v1/workflows", workflowHandler.Create)
	r.Get("/ojs/v1/workflows/{id}", workflowHandler.Get)
	r.Delete("/ojs/v1/workflows/{id}", workflowHandler.Cancel)
	r.Get("/ojs/v1/admin/stats", adminHandler.Stats)
	r.Get("/ojs/v1/admin/queues", adminHandler.ListQueues)
	r.Get("/ojs/v1/admin/jobs", adminHandler.ListJobs)
	r.Get("/ojs/v1/admin/workers", adminHandler.ListWorkers)

	return httptest.NewServer(r), backend
}

// TestFullJobLifecycle tests create → fetch → ack full cycle.
func TestFullJobLifecycle(t *testing.T) {
	srv, _ := setupFullTestServer()
	defer srv.Close()

	// Step 1: Create a job
	createBody := `{"type":"lifecycle_test","queue":"default","args":["value1"]}`
	resp := doPost(t, srv, "/ojs/v1/jobs", createBody)
	assertStatus(t, resp, http.StatusCreated)

	var createResult struct {
		Job *core.Job `json:"job"`
	}
	decodeJSON(t, resp, &createResult)
	jobID := createResult.Job.ID

	if createResult.Job.State != core.StateAvailable {
		t.Errorf("Expected state %s, got %s", core.StateAvailable, createResult.Job.State)
	}

	// Step 2: Get the job
	resp = doGet(t, srv, "/ojs/v1/jobs/"+jobID)
	assertStatus(t, resp, http.StatusOK)

	// Step 3: Fetch the job as a worker
	fetchBody := `{"queues":["default"],"count":1,"worker_id":"lifecycle-worker","visibility_ms":30000}`
	resp = doPost(t, srv, "/ojs/v1/workers/fetch", fetchBody)
	assertStatus(t, resp, http.StatusOK)

	var fetchResult struct {
		Jobs []*core.Job `json:"jobs"`
	}
	decodeJSON(t, resp, &fetchResult)

	if len(fetchResult.Jobs) != 1 {
		t.Fatalf("Expected 1 fetched job, got %d", len(fetchResult.Jobs))
	}
	if fetchResult.Jobs[0].State != core.StateActive {
		t.Errorf("Expected state %s, got %s", core.StateActive, fetchResult.Jobs[0].State)
	}

	// Step 4: Ack the job
	ackBody := `{"job_id":"` + jobID + `","result":{"status":"done"}}`
	resp = doPost(t, srv, "/ojs/v1/workers/ack", ackBody)
	assertStatus(t, resp, http.StatusOK)

	var ackResult core.AckResponse
	decodeJSON(t, resp, &ackResult)

	if !ackResult.Acknowledged {
		t.Error("Expected acknowledged")
	}
	if ackResult.State != core.StateCompleted {
		t.Errorf("Expected state %s, got %s", core.StateCompleted, ackResult.State)
	}

	// Step 5: Verify the job is completed
	resp = doGet(t, srv, "/ojs/v1/jobs/"+jobID)
	assertStatus(t, resp, http.StatusOK)

	var getResult struct {
		Job *core.Job `json:"job"`
	}
	decodeJSON(t, resp, &getResult)

	if getResult.Job.State != core.StateCompleted {
		t.Errorf("Expected completed state, got %s", getResult.Job.State)
	}
}

// TestJobLifecycleNackRetry tests create → fetch → nack → retry cycle.
func TestJobLifecycleNackRetry(t *testing.T) {
	srv, _ := setupFullTestServer()
	defer srv.Close()

	// Create job with max_attempts
	createBody := `{"type":"nack_test","queue":"default","args":[],"options":{"retry":{"max_attempts":3}}}`
	resp := doPost(t, srv, "/ojs/v1/jobs", createBody)
	assertStatus(t, resp, http.StatusCreated)

	var createResult struct {
		Job *core.Job `json:"job"`
	}
	decodeJSON(t, resp, &createResult)
	jobID := createResult.Job.ID

	// Fetch the job
	fetchBody := `{"queues":["default"],"count":1,"worker_id":"nack-worker","visibility_ms":30000}`
	resp = doPost(t, srv, "/ojs/v1/workers/fetch", fetchBody)
	assertStatus(t, resp, http.StatusOK)

	// Nack with requeue
	nackBody := `{"job_id":"` + jobID + `","error":{"message":"temp failure"},"requeue":true}`
	resp = doPost(t, srv, "/ojs/v1/workers/nack", nackBody)
	assertStatus(t, resp, http.StatusOK)

	var nackResult core.NackResponse
	decodeJSON(t, resp, &nackResult)

	if nackResult.State != core.StateAvailable {
		t.Errorf("Expected state %s after requeue, got %s", core.StateAvailable, nackResult.State)
	}
}

// TestFetchEmptyQueue verifies fetching from empty queue returns empty array.
func TestFetchEmptyQueue(t *testing.T) {
	srv, _ := setupFullTestServer()
	defer srv.Close()

	fetchBody := `{"queues":["empty-queue"],"count":1,"worker_id":"worker1","visibility_ms":30000}`
	resp := doPost(t, srv, "/ojs/v1/workers/fetch", fetchBody)
	assertStatus(t, resp, http.StatusOK)

	var result struct {
		Jobs []*core.Job `json:"jobs"`
	}
	decodeJSON(t, resp, &result)

	if len(result.Jobs) != 0 {
		t.Errorf("Expected 0 jobs from empty queue, got %d", len(result.Jobs))
	}
}

// TestFetchMissingQueues verifies 400 when queues is empty.
func TestFetchMissingQueues(t *testing.T) {
	srv, _ := setupFullTestServer()
	defer srv.Close()

	resp := doPost(t, srv, "/ojs/v1/workers/fetch", `{"count":1,"worker_id":"w1","visibility_ms":30000}`)
	assertStatus(t, resp, http.StatusBadRequest)
}

// TestAckMissingJobID verifies 400 when job_id is missing.
func TestAckMissingJobID(t *testing.T) {
	srv, _ := setupFullTestServer()
	defer srv.Close()

	resp := doPost(t, srv, "/ojs/v1/workers/ack", `{"result":{}}`)
	assertStatus(t, resp, http.StatusBadRequest)
}

// TestAckNonExistentJob verifies 404 for non-existent job.
func TestAckNonExistentJob(t *testing.T) {
	srv, _ := setupFullTestServer()
	defer srv.Close()

	resp := doPost(t, srv, "/ojs/v1/workers/ack", `{"job_id":"non-existent"}`)
	assertStatus(t, resp, http.StatusNotFound)
}

// TestNackMissingJobID verifies 400 when job_id is missing.
func TestNackMissingJobID(t *testing.T) {
	srv, _ := setupFullTestServer()
	defer srv.Close()

	resp := doPost(t, srv, "/ojs/v1/workers/nack", `{"error":{"message":"err"}}`)
	assertStatus(t, resp, http.StatusBadRequest)
}

// TestHeartbeatMissingWorkerID verifies 400 when worker_id is missing.
func TestHeartbeatMissingWorkerID(t *testing.T) {
	srv, _ := setupFullTestServer()
	defer srv.Close()

	resp := doPost(t, srv, "/ojs/v1/workers/heartbeat", `{"active_jobs":[]}`)
	assertStatus(t, resp, http.StatusBadRequest)
}

// TestCancelNonExistentJob verifies 404 for non-existent job cancel.
func TestCancelNonExistentJob(t *testing.T) {
	srv, _ := setupFullTestServer()
	defer srv.Close()

	req, _ := http.NewRequest("DELETE", srv.URL+"/ojs/v1/jobs/non-existent", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	assertStatus(t, resp, http.StatusNotFound)
}

// TestDeadLetterRetry verifies dead letter retry creates a new available job.
func TestDeadLetterRetry(t *testing.T) {
	srv, backend := setupFullTestServer()
	defer srv.Close()

	maxAttempts := 0
	job := &core.Job{Type: "dl_test", Queue: "default", Args: json.RawMessage(`{}`), MaxAttempts: &maxAttempts}
	backend.Push(nil, job)
	fetched, _ := backend.Fetch(nil, []string{"default"}, 1, "w1", 30000)
	backend.Nack(nil, fetched[0].ID, &core.JobError{Message: "fatal"}, false)

	// Retry the dead letter
	resp := doPost(t, srv, "/ojs/v1/dead-letter/"+fetched[0].ID+"/retry", "")
	assertStatus(t, resp, http.StatusOK)

	var result struct {
		Job *core.Job `json:"job"`
	}
	decodeJSON(t, resp, &result)

	if result.Job.State != core.StateAvailable {
		t.Errorf("Expected state %s after retry, got %s", core.StateAvailable, result.Job.State)
	}
}

// TestDeadLetterDelete verifies dead letter deletion.
func TestDeadLetterDelete(t *testing.T) {
	srv, backend := setupFullTestServer()
	defer srv.Close()

	maxAttempts := 0
	job := &core.Job{Type: "dl_test", Queue: "default", Args: json.RawMessage(`{}`), MaxAttempts: &maxAttempts}
	backend.Push(nil, job)
	fetched, _ := backend.Fetch(nil, []string{"default"}, 1, "w1", 30000)
	backend.Nack(nil, fetched[0].ID, &core.JobError{Message: "fatal"}, false)

	req, _ := http.NewRequest("DELETE", srv.URL+"/ojs/v1/dead-letter/"+fetched[0].ID, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)
}

// TestGetMetrics verifies /metrics endpoint returns Prometheus format.
func TestGetMetrics(t *testing.T) {
	srv, _ := setupFullTestServer()
	defer srv.Close()

	// Make a request to generate some metrics
	doGet(t, srv, "/ojs/v1/health")

	resp := doGet(t, srv, "/metrics")
	assertStatus(t, resp, http.StatusOK)

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	if !strings.Contains(bodyStr, "ojs_http_requests_total") {
		t.Error("Metrics should contain ojs_http_requests_total")
	}
	if !strings.Contains(bodyStr, "ojs_http_errors_total") {
		t.Error("Metrics should contain ojs_http_errors_total")
	}
	if !strings.Contains(bodyStr, "ojs_http_active_requests") {
		t.Error("Metrics should contain ojs_http_active_requests")
	}
}

// TestGetHealthDetail verifies /ojs/v1/health/detail returns detailed info.
func TestGetHealthDetail(t *testing.T) {
	srv, _ := setupFullTestServer()
	defer srv.Close()

	resp := doGet(t, srv, "/ojs/v1/health/detail")
	assertStatus(t, resp, http.StatusOK)

	var detail map[string]any
	decodeJSON(t, resp, &detail)

	if _, ok := detail["runtime"]; !ok {
		t.Error("Health detail should contain runtime info")
	}
	if _, ok := detail["memory"]; !ok {
		t.Error("Health detail should contain memory info")
	}
	if _, ok := detail["metrics"]; !ok {
		t.Error("Health detail should contain metrics info")
	}

	runtime := detail["runtime"].(map[string]any)
	if runtime["go_version"] == "" {
		t.Error("Runtime should have go_version")
	}
	if runtime["goroutines"].(float64) <= 0 {
		t.Error("Runtime should report goroutines > 0")
	}
}

// TestAdminStats verifies admin stats endpoint.
func TestAdminStats(t *testing.T) {
	srv, backend := setupFullTestServer()
	defer srv.Close()

	// Create some jobs
	for i := 0; i < 3; i++ {
		job := &core.Job{Type: "test", Queue: "default", Args: json.RawMessage(`{}`)}
		backend.Push(nil, job)
	}

	resp := doGet(t, srv, "/ojs/v1/admin/stats")
	assertStatus(t, resp, http.StatusOK)

	var stats map[string]any
	decodeJSON(t, resp, &stats)

	if stats["queues"].(float64) != 1 {
		t.Errorf("Expected 1 queue, got %v", stats["queues"])
	}

	jobs := stats["jobs"].(map[string]any)
	if jobs["available"].(float64) != 3 {
		t.Errorf("Expected 3 available jobs, got %v", jobs["available"])
	}
}

// TestAdminListQueues verifies admin queue listing.
func TestAdminListQueues(t *testing.T) {
	srv, backend := setupFullTestServer()
	defer srv.Close()

	backend.Push(nil, &core.Job{Type: "t", Queue: "q1", Args: json.RawMessage(`{}`)})
	backend.Push(nil, &core.Job{Type: "t", Queue: "q2", Args: json.RawMessage(`{}`)})

	resp := doGet(t, srv, "/ojs/v1/admin/queues")
	assertStatus(t, resp, http.StatusOK)

	var result struct {
		Items []map[string]any `json:"items"`
	}
	decodeJSON(t, resp, &result)

	if len(result.Items) != 2 {
		t.Errorf("Expected 2 queues, got %d", len(result.Items))
	}
}

// TestAdminListJobs verifies admin job listing with pagination.
func TestAdminListJobs(t *testing.T) {
	srv, backend := setupFullTestServer()
	defer srv.Close()

	for i := 0; i < 5; i++ {
		backend.Push(nil, &core.Job{Type: "t", Queue: "default", Args: json.RawMessage(`{}`)})
	}

	resp := doGet(t, srv, "/ojs/v1/admin/jobs?per_page=2&page=1")
	assertStatus(t, resp, http.StatusOK)

	var result struct {
		Items      []*core.Job    `json:"items"`
		Pagination map[string]any `json:"pagination"`
	}
	decodeJSON(t, resp, &result)

	if len(result.Items) != 2 {
		t.Errorf("Expected 2 items on page 1, got %d", len(result.Items))
	}
	if result.Pagination["total"].(float64) != 5 {
		t.Errorf("Expected total 5, got %v", result.Pagination["total"])
	}
}

// TestAdminListWorkers verifies admin worker listing.
func TestAdminListWorkers(t *testing.T) {
	srv, backend := setupFullTestServer()
	defer srv.Close()

	// Create a worker by fetching a job
	backend.Push(nil, &core.Job{Type: "t", Queue: "default", Args: json.RawMessage(`{}`)})
	backend.Fetch(nil, []string{"default"}, 1, "test-worker", 30000)

	resp := doGet(t, srv, "/ojs/v1/admin/workers")
	assertStatus(t, resp, http.StatusOK)

	var result struct {
		Items   []*core.WorkerInfo `json:"items"`
		Summary map[string]any     `json:"summary"`
	}
	decodeJSON(t, resp, &result)

	if len(result.Items) != 1 {
		t.Errorf("Expected 1 worker, got %d", len(result.Items))
	}
	if result.Items[0].ID != "test-worker" {
		t.Errorf("Expected worker ID test-worker, got %s", result.Items[0].ID)
	}
}

// TestKeyAuthMiddleware verifies API key authentication.
func TestKeyAuthMiddleware(t *testing.T) {
	backend, _ := memory.New()
	r := chi.NewRouter()
	r.Use(KeyAuth("test-secret-key", "/ojs/v1/health"))

	systemHandler := NewSystemHandler(backend)
	r.Get("/ojs/v1/health", systemHandler.Health)
	r.Get("/ojs/v1/jobs/{id}", func(w http.ResponseWriter, r *http.Request) {
		WriteJSON(w, http.StatusOK, map[string]string{"ok": "true"})
	})

	srv := httptest.NewServer(r)
	defer srv.Close()

	// Health endpoint should bypass auth
	resp := doGet(t, srv, "/ojs/v1/health")
	assertStatus(t, resp, http.StatusOK)

	// Protected endpoint without auth should fail
	resp = doGet(t, srv, "/ojs/v1/jobs/test")
	assertStatus(t, resp, http.StatusUnauthorized)

	// Protected endpoint with wrong key should fail
	req, _ := http.NewRequest("GET", srv.URL+"/ojs/v1/jobs/test", nil)
	req.Header.Set("Authorization", "Bearer wrong-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusForbidden)

	// Protected endpoint with correct key should succeed
	req, _ = http.NewRequest("GET", srv.URL+"/ojs/v1/jobs/test", nil)
	req.Header.Set("Authorization", "Bearer test-secret-key")
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp2.Body.Close()
	assertStatus(t, resp2, http.StatusOK)
}

// --- Helpers ---

func doGet(t *testing.T, srv *httptest.Server, path string) *http.Response {
	t.Helper()
	resp, err := http.Get(srv.URL + path)
	if err != nil {
		t.Fatalf("GET %s failed: %v", path, err)
	}
	return resp
}

func doPost(t *testing.T, srv *httptest.Server, path string, body string) *http.Response {
	t.Helper()
	resp, err := http.Post(srv.URL+path, "application/json", bytes.NewReader([]byte(body)))
	if err != nil {
		t.Fatalf("POST %s failed: %v", path, err)
	}
	return resp
}

func assertStatus(t *testing.T, resp *http.Response, expected int) {
	t.Helper()
	if resp.StatusCode != expected {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("Expected status %d, got %d. Body: %s", expected, resp.StatusCode, string(body))
	}
}

func decodeJSON(t *testing.T, resp *http.Response, v any) {
	t.Helper()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("Failed to decode JSON: %v", err)
	}
}
