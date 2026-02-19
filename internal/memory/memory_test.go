package memory

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/openjobspec/ojs-backend-lite/internal/core"
)

// TestPushMinimalJob verifies pushing a minimal job generates ID, timestamps, and state.
func TestPushMinimalJob(t *testing.T) {
	b, _ := New()
	ctx := context.Background()

	job := &core.Job{
		Type:  "test",
		Queue: "default",
		Args:  json.RawMessage(`{"foo":"bar"}`),
	}

	pushed, err := b.Push(ctx, job)
	if err != nil {
		t.Fatalf("Push failed: %v", err)
	}

	if pushed.ID == "" {
		t.Error("Job ID should be generated")
	}

	// Verify UUIDv7 format (starts with timestamp prefix)
	if len(pushed.ID) < 36 {
		t.Errorf("Job ID should be UUIDv7 format, got: %s", pushed.ID)
	}

	if pushed.State != core.StateAvailable {
		t.Errorf("Expected state %s, got %s", core.StateAvailable, pushed.State)
	}

	if pushed.CreatedAt == "" {
		t.Error("CreatedAt should be set")
	}

	if pushed.EnqueuedAt == "" {
		t.Error("EnqueuedAt should be set")
	}

	if pushed.Attempt != 0 {
		t.Errorf("Expected attempt 0, got %d", pushed.Attempt)
	}
}

// TestPushWithScheduledAt verifies scheduled jobs start in scheduled state.
func TestPushWithScheduledAt(t *testing.T) {
	b, _ := New()
	ctx := context.Background()

	futureTime := time.Now().Add(1 * time.Hour).Format(time.RFC3339)
	job := &core.Job{
		Type:        "test",
		Queue:       "default",
		Args:        json.RawMessage(`[]`),
		ScheduledAt: futureTime,
	}

	pushed, err := b.Push(ctx, job)
	if err != nil {
		t.Fatalf("Push failed: %v", err)
	}

	if pushed.State != core.StateScheduled {
		t.Errorf("Expected state %s, got %s", core.StateScheduled, pushed.State)
	}

	if pushed.ScheduledAt != futureTime {
		t.Errorf("Expected scheduled_at %s, got %s", futureTime, pushed.ScheduledAt)
	}
}

// TestPushWithExplicitID verifies explicit IDs are preserved.
func TestPushWithExplicitID(t *testing.T) {
	b, _ := New()
	ctx := context.Background()

	explicitID := "my-custom-id-12345"
	job := &core.Job{
		ID:    explicitID,
		Type:  "test",
		Queue: "default",
		Args:  json.RawMessage(`[]`),
	}

	pushed, err := b.Push(ctx, job)
	if err != nil {
		t.Fatalf("Push failed: %v", err)
	}

	if pushed.ID != explicitID {
		t.Errorf("Expected ID %s, got %s", explicitID, pushed.ID)
	}
}

// TestPushBatchSuccess verifies batch push creates all jobs atomically.
func TestPushBatchSuccess(t *testing.T) {
	b, _ := New()
	ctx := context.Background()

	jobs := []*core.Job{
		{Type: "test1", Queue: "default", Args: json.RawMessage(`[]`)},
		{Type: "test2", Queue: "default", Args: json.RawMessage(`[]`)},
		{Type: "test3", Queue: "default", Args: json.RawMessage(`[]`)},
	}

	pushed, err := b.PushBatch(ctx, jobs)
	if err != nil {
		t.Fatalf("PushBatch failed: %v", err)
	}

	if len(pushed) != 3 {
		t.Fatalf("Expected 3 jobs, got %d", len(pushed))
	}

	for i, job := range pushed {
		if job.ID == "" {
			t.Errorf("Job %d: ID should be generated", i)
		}
		if job.State != core.StateAvailable {
			t.Errorf("Job %d: expected state %s, got %s", i, core.StateAvailable, job.State)
		}
	}
}

// TestPushBatchInvalidJob verifies batch fails atomically on invalid input.
func TestPushBatchInvalidJob(t *testing.T) {
	b, _ := New()
	ctx := context.Background()

	jobs := []*core.Job{
		{Type: "test1", Queue: "default", Args: json.RawMessage(`[]`)},
		{Type: "", Queue: "default", Args: json.RawMessage(`[]`)}, // Invalid: no type
		{Type: "test3", Queue: "default", Args: json.RawMessage(`[]`)},
	}

	_, err := b.PushBatch(ctx, jobs)
	if err == nil {
		t.Fatal("Expected PushBatch to fail with invalid job")
	}

	// Verify no jobs were created
	queues, _ := b.ListQueues(ctx)
	if len(queues) > 0 {
		t.Error("No jobs should be created when batch fails")
	}
}

// TestInfoExistingJob verifies Info returns all job fields.
func TestInfoExistingJob(t *testing.T) {
	b, _ := New()
	ctx := context.Background()

	priority := 5
	job := &core.Job{
		Type:     "test",
		Queue:    "default",
		Args:     json.RawMessage(`{"key":"value"}`),
		Priority: &priority,
	}

	pushed, err := b.Push(ctx, job)
	if err != nil {
		t.Fatalf("Push failed: %v", err)
	}

	retrieved, err := b.Info(ctx, pushed.ID)
	if err != nil {
		t.Fatalf("Info failed: %v", err)
	}

	if retrieved.ID != pushed.ID {
		t.Errorf("Expected ID %s, got %s", pushed.ID, retrieved.ID)
	}

	if retrieved.Type != "test" {
		t.Errorf("Expected type test, got %s", retrieved.Type)
	}

	if retrieved.Queue != "default" {
		t.Errorf("Expected queue default, got %s", retrieved.Queue)
	}

	if retrieved.Priority == nil || *retrieved.Priority != 5 {
		t.Errorf("Expected priority 5, got %v", retrieved.Priority)
	}

	if string(retrieved.Args) != `{"key":"value"}` {
		t.Errorf("Expected args {\"key\":\"value\"}, got %s", string(retrieved.Args))
	}
}

// TestInfoNonExistentJob verifies Info returns not_found error.
func TestInfoNonExistentJob(t *testing.T) {
	b, _ := New()
	ctx := context.Background()

	_, err := b.Info(ctx, "non-existent-id")
	if err == nil {
		t.Fatal("Expected error for non-existent job")
	}

	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Expected 'not found' error, got: %v", err)
	}
}

// TestCancelAvailableJob verifies available jobs can be cancelled.
func TestCancelAvailableJob(t *testing.T) {
	b, _ := New()
	ctx := context.Background()

	job := &core.Job{Type: "test", Queue: "default", Args: json.RawMessage(`[]`)}
	pushed, _ := b.Push(ctx, job)

	cancelled, err := b.Cancel(ctx, pushed.ID)
	if err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}

	if cancelled.State != core.StateCancelled {
		t.Errorf("Expected state %s, got %s", core.StateCancelled, cancelled.State)
	}

	if cancelled.CancelledAt == "" {
		t.Error("CancelledAt should be set")
	}
}

// TestCancelActiveJob verifies active jobs can be cancelled.
func TestCancelActiveJob(t *testing.T) {
	b, _ := New()
	ctx := context.Background()

	job := &core.Job{Type: "test", Queue: "default", Args: json.RawMessage(`[]`)}
	pushed, _ := b.Push(ctx, job)

	// Fetch to make it active
	fetched, _ := b.Fetch(ctx, []string{"default"}, 1, "worker1", 30000)
	if len(fetched) != 1 {
		t.Fatal("Expected to fetch 1 job")
	}

	cancelled, err := b.Cancel(ctx, pushed.ID)
	if err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}

	if cancelled.State != core.StateCancelled {
		t.Errorf("Expected state %s, got %s", core.StateCancelled, cancelled.State)
	}
}

// TestCancelCompletedJob verifies completed jobs cannot be cancelled.
func TestCancelCompletedJob(t *testing.T) {
	b, _ := New()
	ctx := context.Background()

	job := &core.Job{Type: "test", Queue: "default", Args: json.RawMessage(`[]`)}
	pushed, _ := b.Push(ctx, job)

	// Fetch and ack
	fetched, _ := b.Fetch(ctx, []string{"default"}, 1, "worker1", 30000)
	b.Ack(ctx, fetched[0].ID, json.RawMessage(`[]`))

	_, err := b.Cancel(ctx, pushed.ID)
	if err == nil {
		t.Fatal("Expected error when cancelling completed job")
	}

	if !strings.Contains(err.Error(), "conflict") && !strings.Contains(err.Error(), "terminal") {
		t.Errorf("Expected conflict/terminal error, got: %v", err)
	}
}

// TestFetchFromEmptyQueue verifies empty fetch result.
func TestFetchFromEmptyQueue(t *testing.T) {
	b, _ := New()
	ctx := context.Background()

	jobs, err := b.Fetch(ctx, []string{"default"}, 1, "worker1", 30000)
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}

	if len(jobs) != 0 {
		t.Errorf("Expected 0 jobs, got %d", len(jobs))
	}
}

// TestFetchTransitionsToActive verifies fetch changes state and sets fields.
func TestFetchTransitionsToActive(t *testing.T) {
	b, _ := New()
	ctx := context.Background()

	job := &core.Job{Type: "test", Queue: "default", Args: json.RawMessage(`[]`)}
	pushed, _ := b.Push(ctx, job)

	fetched, err := b.Fetch(ctx, []string{"default"}, 1, "worker1", 30000)
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}

	if len(fetched) != 1 {
		t.Fatalf("Expected 1 job, got %d", len(fetched))
	}

	f := fetched[0]
	if f.ID != pushed.ID {
		t.Errorf("Expected job ID %s, got %s", pushed.ID, f.ID)
	}

	if f.State != core.StateActive {
		t.Errorf("Expected state %s, got %s", core.StateActive, f.State)
	}

	if f.StartedAt == "" {
		t.Error("StartedAt should be set")
	}

	if f.WorkerID != "worker1" {
		t.Errorf("Expected worker_id worker1, got %s", f.WorkerID)
	}

	if f.Attempt != 1 {
		t.Errorf("Expected attempt 1, got %d", f.Attempt)
	}
}

// TestFetchRespectsPausedQueues verifies paused queues are skipped.
func TestFetchRespectsPausedQueues(t *testing.T) {
	b, _ := New()
	ctx := context.Background()

	job := &core.Job{Type: "test", Queue: "default", Args: json.RawMessage(`[]`)}
	b.Push(ctx, job)

	// Pause queue
	err := b.PauseQueue(ctx, "default")
	if err != nil {
		t.Fatalf("PauseQueue failed: %v", err)
	}

	// Fetch should return empty
	fetched, err := b.Fetch(ctx, []string{"default"}, 1, "worker1", 30000)
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}

	if len(fetched) != 0 {
		t.Errorf("Expected 0 jobs from paused queue, got %d", len(fetched))
	}
}

// TestFetchRespectsPriority verifies higher priority jobs are fetched first.
func TestFetchRespectsPriority(t *testing.T) {
	b, _ := New()
	ctx := context.Background()

	// Push low priority first
	priority1 := 1
	job1 := &core.Job{Type: "test", Queue: "default", Args: json.RawMessage(`[]`), Priority: &priority1}
	b.Push(ctx, job1)

	// Push high priority second
	priority2 := 10
	job2 := &core.Job{Type: "test", Queue: "default", Args: json.RawMessage(`[]`), Priority: &priority2}
	pushed2, _ := b.Push(ctx, job2)

	// Fetch should return high priority job
	fetched, err := b.Fetch(ctx, []string{"default"}, 1, "worker1", 30000)
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}

	if len(fetched) != 1 {
		t.Fatalf("Expected 1 job, got %d", len(fetched))
	}

	if fetched[0].ID != pushed2.ID {
		t.Error("Expected high priority job to be fetched first")
	}
}

// TestFetchMultipleJobs verifies count parameter is respected.
func TestFetchMultipleJobs(t *testing.T) {
	b, _ := New()
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		job := &core.Job{Type: "test", Queue: "default", Args: json.RawMessage(`[]`)}
		b.Push(ctx, job)
	}

	fetched, err := b.Fetch(ctx, []string{"default"}, 3, "worker1", 30000)
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}

	if len(fetched) != 3 {
		t.Errorf("Expected 3 jobs, got %d", len(fetched))
	}
}

// TestAckSuccess verifies ack transitions to completed and stores result.
func TestAckSuccess(t *testing.T) {
	b, _ := New()
	ctx := context.Background()

	job := &core.Job{Type: "test", Queue: "default", Args: json.RawMessage(`[]`)}
	b.Push(ctx, job)

	fetched, _ := b.Fetch(ctx, []string{"default"}, 1, "worker1", 30000)
	result := json.RawMessage(`{"status":"success"}`)

	ackResp, err := b.Ack(ctx, fetched[0].ID, result)
	if err != nil {
		t.Fatalf("Ack failed: %v", err)
	}

	if !ackResp.Acknowledged {
		t.Error("Job should be acknowledged")
	}

	if ackResp.State != core.StateCompleted {
		t.Errorf("Expected state %s, got %s", core.StateCompleted, ackResp.State)
	}

	// Verify result stored
	info, _ := b.Info(ctx, fetched[0].ID)
	if string(info.Result) != string(result) {
		t.Errorf("Expected result %s, got %s", string(result), string(info.Result))
	}

	if info.CompletedAt == "" {
		t.Error("CompletedAt should be set")
	}
}

// TestAckNonActiveJob verifies ack fails for non-active jobs.
func TestAckNonActiveJob(t *testing.T) {
	b, _ := New()
	ctx := context.Background()

	job := &core.Job{Type: "test", Queue: "default", Args: json.RawMessage(`[]`)}
	pushed, _ := b.Push(ctx, job)

	// Try to ack without fetching
	_, err := b.Ack(ctx, pushed.ID, json.RawMessage(`[]`))
	if err == nil {
		t.Fatal("Expected error when acking non-active job")
	}
}

// TestNackWithRetriesRemaining verifies nack transitions to retryable.
func TestNackWithRetriesRemaining(t *testing.T) {
	b, _ := New()
	ctx := context.Background()

	maxAttempts := 3
	job := &core.Job{
		Type:        "test",
		Queue:       "default",
		Args:        json.RawMessage(`[]`),
		MaxAttempts: &maxAttempts,
	}
	b.Push(ctx, job)

	fetched, _ := b.Fetch(ctx, []string{"default"}, 1, "worker1", 30000)

	jobErr := &core.JobError{Message: "temporary error"}
	nackResp, err := b.Nack(ctx, fetched[0].ID, jobErr, false)
	if err != nil {
		t.Fatalf("Nack failed: %v", err)
	}

	if nackResp.State != core.StateRetryable {
		t.Errorf("Expected state %s, got %s", core.StateRetryable, nackResp.State)
	}

	if nackResp.Attempt != 1 {
		t.Errorf("Expected attempt 1, got %d", nackResp.Attempt)
	}

	if nackResp.MaxAttempts != 3 {
		t.Errorf("Expected max_attempts 3, got %d", nackResp.MaxAttempts)
	}
}

// TestNackWithNoRetries verifies nack transitions to discarded.
func TestNackWithNoRetries(t *testing.T) {
	b, _ := New()
	ctx := context.Background()

	maxAttempts := 1
	job := &core.Job{
		Type:        "test",
		Queue:       "default",
		Args:        json.RawMessage(`[]`),
		MaxAttempts: &maxAttempts,
	}
	b.Push(ctx, job)

	fetched, _ := b.Fetch(ctx, []string{"default"}, 1, "worker1", 30000)

	jobErr := &core.JobError{Message: "permanent error"}
	nackResp, err := b.Nack(ctx, fetched[0].ID, jobErr, true)
	if err != nil {
		t.Fatalf("Nack failed: %v", err)
	}

	if nackResp.State != core.StateDiscarded {
		t.Errorf("Expected state %s, got %s", core.StateDiscarded, nackResp.State)
	}

	// Verify job is in dead letter queue
	deadJobs, total, _ := b.ListDeadLetter(ctx, 10, 0)
	if total != 1 {
		t.Errorf("Expected 1 dead letter job, got %d", total)
	}

	if len(deadJobs) != 1 || deadJobs[0].ID != fetched[0].ID {
		t.Error("Job should be in dead letter queue")
	}
}

// TestHeartbeat verifies worker tracking.
func TestHeartbeat(t *testing.T) {
	b, _ := New()
	ctx := context.Background()

	job := &core.Job{Type: "test", Queue: "default", Args: json.RawMessage(`[]`)}
	b.Push(ctx, job)

	fetched, _ := b.Fetch(ctx, []string{"default"}, 1, "worker1", 30000)

	hbResp, err := b.Heartbeat(ctx, "worker1", []string{fetched[0].ID}, 30000)
	if err != nil {
		t.Fatalf("Heartbeat failed: %v", err)
	}

	if hbResp.State != "active" {
		t.Errorf("Expected state active, got %s", hbResp.State)
	}

	if hbResp.Directive != "continue" {
		t.Errorf("Expected directive continue, got %s", hbResp.Directive)
	}
}

// TestListQueuesAfterPush verifies queue discovery.
func TestListQueuesAfterPush(t *testing.T) {
	b, _ := New()
	ctx := context.Background()

	job1 := &core.Job{Type: "test", Queue: "queue1", Args: json.RawMessage(`[]`)}
	job2 := &core.Job{Type: "test", Queue: "queue2", Args: json.RawMessage(`[]`)}

	b.Push(ctx, job1)
	b.Push(ctx, job2)

	queues, err := b.ListQueues(ctx)
	if err != nil {
		t.Fatalf("ListQueues failed: %v", err)
	}

	if len(queues) != 2 {
		t.Errorf("Expected 2 queues, got %d", len(queues))
	}

	foundQueue1 := false
	foundQueue2 := false
	for _, q := range queues {
		if q.Name == "queue1" {
			foundQueue1 = true
		}
		if q.Name == "queue2" {
			foundQueue2 = true
		}
	}

	if !foundQueue1 || !foundQueue2 {
		t.Error("Expected both queue1 and queue2 to be listed")
	}
}

// TestQueueStats verifies count accuracy.
func TestQueueStats(t *testing.T) {
	b, _ := New()
	ctx := context.Background()

	// Push 3 jobs
	for i := 0; i < 3; i++ {
		job := &core.Job{Type: "test", Queue: "default", Args: json.RawMessage(`[]`)}
		b.Push(ctx, job)
	}

	// Fetch 1 job (2 available, 1 active)
	b.Fetch(ctx, []string{"default"}, 1, "worker1", 30000)

	stats, err := b.QueueStats(ctx, "default")
	if err != nil {
		t.Fatalf("QueueStats failed: %v", err)
	}

	if stats.Queue != "default" {
		t.Errorf("Expected queue name default, got %s", stats.Queue)
	}

	if stats.Stats.Available != 2 {
		t.Errorf("Expected 2 available, got %d", stats.Stats.Available)
	}

	if stats.Stats.Active != 1 {
		t.Errorf("Expected 1 active, got %d", stats.Stats.Active)
	}
}

// TestPauseResumeQueue verifies queue state changes.
func TestPauseResumeQueue(t *testing.T) {
	b, _ := New()
	ctx := context.Background()

	job := &core.Job{Type: "test", Queue: "default", Args: json.RawMessage(`[]`)}
	b.Push(ctx, job)

	// Pause
	err := b.PauseQueue(ctx, "default")
	if err != nil {
		t.Fatalf("PauseQueue failed: %v", err)
	}

	fetched, _ := b.Fetch(ctx, []string{"default"}, 1, "worker1", 30000)
	if len(fetched) != 0 {
		t.Error("Should not fetch from paused queue")
	}

	// Resume
	err = b.ResumeQueue(ctx, "default")
	if err != nil {
		t.Fatalf("ResumeQueue failed: %v", err)
	}

	fetched, _ = b.Fetch(ctx, []string{"default"}, 1, "worker1", 30000)
	if len(fetched) != 1 {
		t.Error("Should fetch from resumed queue")
	}
}

// TestListDeadLetter verifies pagination.
func TestListDeadLetter(t *testing.T) {
	b, _ := New()
	ctx := context.Background()

	maxAttempts := 1
	// Create 5 dead letter jobs
	for i := 0; i < 5; i++ {
		job := &core.Job{Type: "test", Queue: "default", Args: json.RawMessage(`[]`), MaxAttempts: &maxAttempts}
		b.Push(ctx, job)
		fetched, _ := b.Fetch(ctx, []string{"default"}, 1, "worker1", 30000)
		b.Nack(ctx, fetched[0].ID, &core.JobError{Message: "error"}, true)
	}

	// Fetch first page
	jobs, total, err := b.ListDeadLetter(ctx, 2, 0)
	if err != nil {
		t.Fatalf("ListDeadLetter failed: %v", err)
	}

	if total != 5 {
		t.Errorf("Expected total 5, got %d", total)
	}

	if len(jobs) != 2 {
		t.Errorf("Expected 2 jobs, got %d", len(jobs))
	}

	// Fetch second page
	jobs, total, err = b.ListDeadLetter(ctx, 2, 2)
	if err != nil {
		t.Fatalf("ListDeadLetter page 2 failed: %v", err)
	}

	if len(jobs) != 2 {
		t.Errorf("Expected 2 jobs on page 2, got %d", len(jobs))
	}
}

// TestRetryDeadLetter verifies job moves back to available.
func TestRetryDeadLetter(t *testing.T) {
	b, _ := New()
	ctx := context.Background()

	maxAttempts := 1
	job := &core.Job{Type: "test", Queue: "default", Args: json.RawMessage(`[]`), MaxAttempts: &maxAttempts}
	b.Push(ctx, job)

	fetched, _ := b.Fetch(ctx, []string{"default"}, 1, "worker1", 30000)
	b.Nack(ctx, fetched[0].ID, &core.JobError{Message: "error"}, true)

	// Retry from dead letter
	retried, err := b.RetryDeadLetter(ctx, fetched[0].ID)
	if err != nil {
		t.Fatalf("RetryDeadLetter failed: %v", err)
	}

	if retried.State != core.StateAvailable {
		t.Errorf("Expected state %s, got %s", core.StateAvailable, retried.State)
	}

	// Verify not in dead letter anymore
	deadJobs, total, _ := b.ListDeadLetter(ctx, 10, 0)
	if total != 0 {
		t.Errorf("Expected 0 dead letter jobs, got %d", total)
	}

	if len(deadJobs) != 0 {
		t.Error("Job should not be in dead letter queue after retry")
	}
}

// TestDeleteDeadLetter verifies permanent removal.
func TestDeleteDeadLetter(t *testing.T) {
	b, _ := New()
	ctx := context.Background()

	maxAttempts := 1
	job := &core.Job{Type: "test", Queue: "default", Args: json.RawMessage(`[]`), MaxAttempts: &maxAttempts}
	b.Push(ctx, job)

	fetched, _ := b.Fetch(ctx, []string{"default"}, 1, "worker1", 30000)
	b.Nack(ctx, fetched[0].ID, &core.JobError{Message: "error"}, false)

	// Delete from dead letter
	err := b.DeleteDeadLetter(ctx, fetched[0].ID)
	if err != nil {
		t.Fatalf("DeleteDeadLetter failed: %v", err)
	}

	// Verify not in dead letter
	deadJobs, total, _ := b.ListDeadLetter(ctx, 10, 0)
	if total != 0 {
		t.Errorf("Expected 0 dead letter jobs, got %d", total)
	}

	if len(deadJobs) != 0 {
		t.Error("Job should be permanently deleted")
	}

	// Verify not retrievable via Info
	_, err = b.Info(ctx, fetched[0].ID)
	if err == nil {
		t.Error("Job should not be retrievable after delete")
	}
}

// TestRegisterCron verifies cron creation with next_run_at.
func TestRegisterCron(t *testing.T) {
	b, _ := New()
	ctx := context.Background()

	cron := &core.CronJob{
		Name:     "test-cron",
		Schedule: "*/5 * * * *",
		JobTemplate: &core.CronJobTemplate{
			Type: "test",
			Args: json.RawMessage(`[]`),
			Options: &core.EnqueueOptions{
				Queue: "default",
			},
		},
	}

	registered, err := b.RegisterCron(ctx, cron)
	if err != nil {
		t.Fatalf("RegisterCron failed: %v", err)
	}

	if registered.Name != "test-cron" {
		t.Errorf("Expected name test-cron, got %s", registered.Name)
	}

	if registered.NextRunAt == "" {
		t.Error("NextRunAt should be set")
	}
}

// TestListCron verifies cron listing.
func TestListCron(t *testing.T) {
	b, _ := New()
	ctx := context.Background()

	cron1 := &core.CronJob{Name: "cron1", Schedule: "* * * * *", JobTemplate: &core.CronJobTemplate{Type: "test", Args: json.RawMessage(`[]`), Options: &core.EnqueueOptions{Queue: "default"}}}
	cron2 := &core.CronJob{Name: "cron2", Schedule: "* * * * *", JobTemplate: &core.CronJobTemplate{Type: "test", Args: json.RawMessage(`[]`), Options: &core.EnqueueOptions{Queue: "default"}}}

	b.RegisterCron(ctx, cron1)
	b.RegisterCron(ctx, cron2)

	crons, err := b.ListCron(ctx)
	if err != nil {
		t.Fatalf("ListCron failed: %v", err)
	}

	if len(crons) != 2 {
		t.Errorf("Expected 2 crons, got %d", len(crons))
	}
}

// TestDeleteCron verifies cron removal.
func TestDeleteCron(t *testing.T) {
	b, _ := New()
	ctx := context.Background()

	cron := &core.CronJob{Name: "test-cron", Schedule: "* * * * *", JobTemplate: &core.CronJobTemplate{Type: "test", Args: json.RawMessage(`[]`), Options: &core.EnqueueOptions{Queue: "default"}}}
	b.RegisterCron(ctx, cron)

	deleted, err := b.DeleteCron(ctx, "test-cron")
	if err != nil {
		t.Fatalf("DeleteCron failed: %v", err)
	}

	if deleted.Name != "test-cron" {
		t.Errorf("Expected name test-cron, got %s", deleted.Name)
	}

	// Verify not listed
	crons, _ := b.ListCron(ctx)
	if len(crons) != 0 {
		t.Error("Cron should be deleted")
	}
}

// TestFireCronJobs verifies job enqueued on cron trigger.
func TestFireCronJobs(t *testing.T) {
	b, _ := New()
	ctx := context.Background()

	cron := &core.CronJob{
		Name:       "test-cron",
		Expression: "* * * * *",
		JobTemplate: &core.CronJobTemplate{
			Type: "test",
			Args: json.RawMessage(`[]`),
			Options: &core.EnqueueOptions{
				Queue: "default",
			},
		},
	}
	registered, _ := b.RegisterCron(ctx, cron)
	
	// Manually set NextRunAt to the past to trigger firing
	b.mu.Lock()
	if storedCron, ok := b.crons[registered.Name]; ok {
		pastTime := core.FormatTime(time.Now().Add(-1 * time.Minute))
		storedCron.NextRunAt = pastTime
		storedCron.Enabled = true
		t.Logf("Set cron NextRunAt to %s, Enabled=%v", storedCron.NextRunAt, storedCron.Enabled)
	}
	b.mu.Unlock()

	err := b.FireCronJobs(ctx)
	if err != nil {
		t.Fatalf("FireCronJobs failed: %v", err)
	}

	// Verify job was enqueued
	stats, _ := b.QueueStats(ctx, "default")
	t.Logf("Available jobs: %d", stats.Stats.Available)
	if stats.Stats.Available < 1 {
		t.Error("Cron job should be enqueued")
	}
}

// TestCreateWorkflowChain verifies first job enqueued.
func TestCreateWorkflowChain(t *testing.T) {
	b, _ := New()
	ctx := context.Background()

	req := &core.WorkflowRequest{
		Name: "test-workflow",
		Type: "chain",
		Steps: []core.WorkflowJobRequest{
			{Type: "step1", Args: json.RawMessage(`[]`), Options: &core.EnqueueOptions{Queue: "default"}},
			{Type: "step2", Args: json.RawMessage(`[]`), Options: &core.EnqueueOptions{Queue: "default"}},
		},
	}

	workflow, err := b.CreateWorkflow(ctx, req)
	if err != nil {
		t.Fatalf("CreateWorkflow failed: %v", err)
	}

	if workflow.Name != "test-workflow" {
		t.Errorf("Expected name test-workflow, got %s", workflow.Name)
	}

	if workflow.Type != "chain" {
		t.Errorf("Expected type chain, got %s", workflow.Type)
	}

	if workflow.State != "running" {
		t.Errorf("Expected state running, got %s", workflow.State)
	}

	// Verify first job enqueued
	stats, _ := b.QueueStats(ctx, "default")
	if stats.Stats.Available != 1 {
		t.Error("First job should be enqueued")
	}
}

// TestCreateWorkflowGroup verifies all jobs enqueued.
func TestCreateWorkflowGroup(t *testing.T) {
	b, _ := New()
	ctx := context.Background()

	req := &core.WorkflowRequest{
		Name: "test-group",
		Type: "group",
		Jobs: []core.WorkflowJobRequest{
			{Type: "job1", Args: json.RawMessage(`[]`), Options: &core.EnqueueOptions{Queue: "default"}},
			{Type: "job2", Args: json.RawMessage(`[]`), Options: &core.EnqueueOptions{Queue: "default"}},
			{Type: "job3", Args: json.RawMessage(`[]`), Options: &core.EnqueueOptions{Queue: "default"}},
		},
	}

	workflow, err := b.CreateWorkflow(ctx, req)
	if err != nil {
		t.Fatalf("CreateWorkflow failed: %v", err)
	}

	if workflow.Type != "group" {
		t.Errorf("Expected type group, got %s", workflow.Type)
	}

	// Verify all jobs enqueued
	stats, _ := b.QueueStats(ctx, "default")
	if stats.Stats.Available != 3 {
		t.Errorf("Expected 3 jobs enqueued, got %d", stats.Stats.Available)
	}
}

// TestAdvanceWorkflowChain verifies next step enqueued after ack.
func TestAdvanceWorkflowChain(t *testing.T) {
	b, _ := New()
	ctx := context.Background()

	req := &core.WorkflowRequest{
		Name: "test-chain",
		Type: "chain",
		Steps: []core.WorkflowJobRequest{
			{Type: "step1", Args: json.RawMessage(`[]`), Options: &core.EnqueueOptions{Queue: "default"}},
			{Type: "step2", Args: json.RawMessage(`[]`), Options: &core.EnqueueOptions{Queue: "default"}},
		},
	}

	workflow, _ := b.CreateWorkflow(ctx, req)

	// Fetch and ack first job (this automatically advances the workflow)
	fetched, _ := b.Fetch(ctx, []string{"default"}, 1, "worker1", 30000)
	result := json.RawMessage(`{"result":"success"}`)
	b.Ack(ctx, fetched[0].ID, result)

	// Verify second job enqueued
	stats, _ := b.QueueStats(ctx, "default")
	if stats.Stats.Available != 1 {
		t.Error("Next step should be enqueued")
	}

	// Verify workflow state
	wf, _ := b.GetWorkflow(ctx, workflow.ID)
	if wf.StepsCompleted == nil || *wf.StepsCompleted != 1 {
		t.Errorf("Expected 1 completed, got %v", wf.StepsCompleted)
	}
}

// TestCancelWorkflow verifies all jobs cancelled.
func TestCancelWorkflow(t *testing.T) {
	b, _ := New()
	ctx := context.Background()

	req := &core.WorkflowRequest{
		Name: "test-workflow",
		Type: "group",
		Jobs: []core.WorkflowJobRequest{
			{Type: "job1", Args: json.RawMessage(`[]`), Options: &core.EnqueueOptions{Queue: "default"}},
			{Type: "job2", Args: json.RawMessage(`[]`), Options: &core.EnqueueOptions{Queue: "default"}},
		},
	}

	workflow, _ := b.CreateWorkflow(ctx, req)

	cancelled, err := b.CancelWorkflow(ctx, workflow.ID)
	if err != nil {
		t.Fatalf("CancelWorkflow failed: %v", err)
	}

	if cancelled.State != "cancelled" {
		t.Errorf("Expected state cancelled, got %s", cancelled.State)
	}

	// Verify jobs are cancelled
	stats, _ := b.QueueStats(ctx, "default")
	if stats.Stats.Available != 0 {
		t.Error("All jobs should be cancelled")
	}
}

// TestPushUniqueJobReject verifies duplicate handling with unique policy.
func TestPushUniqueJobReject(t *testing.T) {
	b, _ := New()
	ctx := context.Background()

	job1 := &core.Job{
		Type:  "test",
		Queue: "default",
		Args:  json.RawMessage(`[]`),
		Unique: &core.UniquePolicy{
			Keys:       []string{"unique-test-key"},
			OnConflict: "reject",
		},
	}

	// First push
	_, err := b.Push(ctx, job1)
	if err != nil {
		t.Fatalf("First push failed: %v", err)
	}

	// Duplicate push
	job2 := &core.Job{
		Type:  "test",
		Queue: "default",
		Args:  json.RawMessage(`[]`),
		Unique: &core.UniquePolicy{
			Keys:       []string{"unique-test-key"},
			OnConflict: "reject",
		},
	}

	pushed2, err := b.Push(ctx, job2)
	if err == nil {
		t.Fatal("Second push should have been rejected")
	}

	// Should be nil when rejected
	if pushed2 != nil {
		t.Error("Rejected unique job should return nil")
	}
}

// TestPromoteScheduled verifies scheduled→available transition.
func TestPromoteScheduled(t *testing.T) {
	b, _ := New()
	ctx := context.Background()

	pastTime := time.Now().Add(-1 * time.Minute).Format(time.RFC3339)
	job := &core.Job{
		Type:        "test",
		Queue:       "default",
		Args:        json.RawMessage(`[]`),
		ScheduledAt: pastTime,
	}

	pushed, _ := b.Push(ctx, job)

	// Promote scheduled jobs
	err := b.PromoteScheduled(ctx)
	if err != nil {
		t.Fatalf("PromoteScheduled failed: %v", err)
	}

	// Verify job is now available
	info, _ := b.Info(ctx, pushed.ID)
	if info.State != core.StateAvailable {
		t.Errorf("Expected state %s, got %s", core.StateAvailable, info.State)
	}
}

// TestPromoteRetries verifies retryable→available transition.
func TestPromoteRetries(t *testing.T) {
	b, _ := New()
	ctx := context.Background()

	maxAttempts := 3
	job := &core.Job{
		Type:        "test",
		Queue:       "default",
		Args:        json.RawMessage(`[]`),
		MaxAttempts: &maxAttempts,
	}
	b.Push(ctx, job)

	fetched, _ := b.Fetch(ctx, []string{"default"}, 1, "worker1", 30000)
	b.Nack(ctx, fetched[0].ID, &core.JobError{Message: "error"}, true)

	// Wait for backoff to expire (assuming minimal backoff)
	time.Sleep(100 * time.Millisecond)

	// Promote retries
	err := b.PromoteRetries(ctx)
	if err != nil {
		t.Fatalf("PromoteRetries failed: %v", err)
	}

	// Verify job is available again
	info, _ := b.Info(ctx, fetched[0].ID)
	if info.State != core.StateAvailable {
		t.Errorf("Expected state %s, got %s", core.StateAvailable, info.State)
	}
}

// TestRequeueStalled verifies stalled active→available transition.
func TestRequeueStalled(t *testing.T) {
	b, _ := New()
	ctx := context.Background()

	job := &core.Job{Type: "test", Queue: "default", Args: json.RawMessage(`[]`)}
	b.Push(ctx, job)

	// Fetch with very short visibility timeout
	fetched, _ := b.Fetch(ctx, []string{"default"}, 1, "worker1", 1)

	// Wait for visibility timeout
	time.Sleep(10 * time.Millisecond)

	// Requeue stalled
	err := b.RequeueStalled(ctx)
	if err != nil {
		t.Fatalf("RequeueStalled failed: %v", err)
	}

	// Verify job is available again
	info, _ := b.Info(ctx, fetched[0].ID)
	if info.State != core.StateAvailable {
		t.Errorf("Expected state %s, got %s", core.StateAvailable, info.State)
	}
}
