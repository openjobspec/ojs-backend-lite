package grpc

import (
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/openjobspec/ojs-backend-lite/internal/core"
	"github.com/openjobspec/ojs-backend-lite/internal/events"
	"github.com/openjobspec/ojs-backend-lite/internal/memory"

	ojsv1 "github.com/openjobspec/ojs-proto/gen/go/ojs/v1"
)

func startTestServer(t *testing.T) (ojsv1.OJSServiceClient, *memory.MemoryBackend, func()) {
	t.Helper()

	backend, _ := memory.New()
	broker := events.NewBroker()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	s := grpc.NewServer()
	Register(s, backend, WithEventSubscriber(broker))
	go s.Serve(lis)

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}

	client := ojsv1.NewOJSServiceClient(conn)
	cleanup := func() {
		conn.Close()
		s.Stop()
		lis.Close()
		broker.Close()
		backend.Close()
	}

	return client, backend, cleanup
}

func TestManifest(t *testing.T) {
	client, _, cleanup := startTestServer(t)
	defer cleanup()

	resp, err := client.Manifest(context.Background(), &ojsv1.ManifestRequest{})
	if err != nil {
		t.Fatalf("Manifest failed: %v", err)
	}

	if resp.OjsVersion == "" {
		t.Error("expected non-empty OJS version")
	}
	if resp.ConformanceLevel != 4 {
		t.Errorf("expected conformance level 4, got %d", resp.ConformanceLevel)
	}
	if resp.Backend != "memory" {
		t.Errorf("expected backend memory, got %s", resp.Backend)
	}
}

func TestHealth(t *testing.T) {
	client, _, cleanup := startTestServer(t)
	defer cleanup()

	resp, err := client.Health(context.Background(), &ojsv1.HealthRequest{})
	if err != nil {
		t.Fatalf("Health failed: %v", err)
	}

	if resp.Status != ojsv1.HealthStatus_HEALTH_STATUS_OK {
		t.Errorf("expected OK status, got %v", resp.Status)
	}
}

func TestEnqueueAndGetJob(t *testing.T) {
	client, _, cleanup := startTestServer(t)
	defer cleanup()

	enqResp, err := client.Enqueue(context.Background(), &ojsv1.EnqueueRequest{
		Type: "test.job",
		Options: &ojsv1.EnqueueOptions{
			Queue: "default",
		},
	})
	if err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}
	if enqResp.Job == nil || enqResp.Job.Id == "" {
		t.Fatal("expected job with ID in response")
	}
	if enqResp.Job.Type != "test.job" {
		t.Errorf("expected type test.job, got %s", enqResp.Job.Type)
	}

	getResp, err := client.GetJob(context.Background(), &ojsv1.GetJobRequest{
		JobId: enqResp.Job.Id,
	})
	if err != nil {
		t.Fatalf("GetJob failed: %v", err)
	}
	if getResp.Job.Id != enqResp.Job.Id {
		t.Errorf("expected ID %s, got %s", enqResp.Job.Id, getResp.Job.Id)
	}
}

func TestGetJobNotFound(t *testing.T) {
	client, _, cleanup := startTestServer(t)
	defer cleanup()

	_, err := client.GetJob(context.Background(), &ojsv1.GetJobRequest{
		JobId: "nonexistent",
	})
	if err == nil {
		t.Fatal("expected error for nonexistent job")
	}
	if st, ok := status.FromError(err); ok {
		if st.Code() != codes.NotFound {
			t.Errorf("expected NotFound, got %v", st.Code())
		}
	}
}

func TestCancelJob(t *testing.T) {
	client, _, cleanup := startTestServer(t)
	defer cleanup()

	enqResp, _ := client.Enqueue(context.Background(), &ojsv1.EnqueueRequest{
		Type:    "test.cancel",
		Options: &ojsv1.EnqueueOptions{Queue: "default"},
	})

	cancelResp, err := client.CancelJob(context.Background(), &ojsv1.CancelJobRequest{
		JobId: enqResp.Job.Id,
	})
	if err != nil {
		t.Fatalf("CancelJob failed: %v", err)
	}
	if cancelResp.Job.State != ojsv1.JobState_JOB_STATE_CANCELLED {
		t.Errorf("expected cancelled state, got %v", cancelResp.Job.State)
	}
}

func TestEnqueueBatch(t *testing.T) {
	client, _, cleanup := startTestServer(t)
	defer cleanup()

	arg1, _ := structpb.NewValue("hello")
	resp, err := client.EnqueueBatch(context.Background(), &ojsv1.EnqueueBatchRequest{
		Jobs: []*ojsv1.BatchJobEntry{
			{Type: "batch_one", Args: []*structpb.Value{arg1}, Options: &ojsv1.EnqueueOptions{Queue: "default"}},
			{Type: "batch_two", Args: []*structpb.Value{arg1}, Options: &ojsv1.EnqueueOptions{Queue: "default"}},
		},
	})
	if err != nil {
		t.Fatalf("EnqueueBatch failed: %v", err)
	}
	if len(resp.Jobs) != 2 {
		t.Errorf("expected 2 jobs, got %d", len(resp.Jobs))
	}
}

func TestFetchAndAck(t *testing.T) {
	client, _, cleanup := startTestServer(t)
	defer cleanup()

	client.Enqueue(context.Background(), &ojsv1.EnqueueRequest{
		Type:    "test.fetch",
		Options: &ojsv1.EnqueueOptions{Queue: "work"},
	})

	fetchResp, err := client.Fetch(context.Background(), &ojsv1.FetchRequest{
		Queues:   []string{"work"},
		WorkerId: "worker-1",
		Count:    1,
	})
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}
	if len(fetchResp.Jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(fetchResp.Jobs))
	}
	if fetchResp.Jobs[0].State != ojsv1.JobState_JOB_STATE_ACTIVE {
		t.Errorf("expected active state, got %v", fetchResp.Jobs[0].State)
	}

	ackResp, err := client.Ack(context.Background(), &ojsv1.AckRequest{
		JobId: fetchResp.Jobs[0].Id,
	})
	if err != nil {
		t.Fatalf("Ack failed: %v", err)
	}
	if !ackResp.Acknowledged {
		t.Error("expected acknowledged")
	}
}

func TestNack(t *testing.T) {
	client, _, cleanup := startTestServer(t)
	defer cleanup()

	client.Enqueue(context.Background(), &ojsv1.EnqueueRequest{
		Type:    "test.nack",
		Options: &ojsv1.EnqueueOptions{Queue: "work"},
	})

	fetchResp, _ := client.Fetch(context.Background(), &ojsv1.FetchRequest{
		Queues:   []string{"work"},
		WorkerId: "worker-1",
		Count:    1,
	})

	nackResp, err := client.Nack(context.Background(), &ojsv1.NackRequest{
		JobId: fetchResp.Jobs[0].Id,
		Error: &ojsv1.JobError{
			Message: "something failed",
			Code:    "TEMP_ERROR",
		},
	})
	if err != nil {
		t.Fatalf("Nack failed: %v", err)
	}
	if nackResp.State != ojsv1.JobState_JOB_STATE_RETRYABLE {
		t.Errorf("expected retryable state, got %v", nackResp.State)
	}
}

func TestListQueues(t *testing.T) {
	client, _, cleanup := startTestServer(t)
	defer cleanup()

	client.Enqueue(context.Background(), &ojsv1.EnqueueRequest{
		Type:    "test",
		Options: &ojsv1.EnqueueOptions{Queue: "queue-a"},
	})
	client.Enqueue(context.Background(), &ojsv1.EnqueueRequest{
		Type:    "test",
		Options: &ojsv1.EnqueueOptions{Queue: "queue-b"},
	})

	resp, err := client.ListQueues(context.Background(), &ojsv1.ListQueuesRequest{})
	if err != nil {
		t.Fatalf("ListQueues failed: %v", err)
	}
	if len(resp.Queues) < 2 {
		t.Errorf("expected at least 2 queues, got %d", len(resp.Queues))
	}
}

func TestQueueStats(t *testing.T) {
	client, _, cleanup := startTestServer(t)
	defer cleanup()

	client.Enqueue(context.Background(), &ojsv1.EnqueueRequest{
		Type:    "test",
		Options: &ojsv1.EnqueueOptions{Queue: "stats-q"},
	})

	resp, err := client.QueueStats(context.Background(), &ojsv1.QueueStatsRequest{Queue: "stats-q"})
	if err != nil {
		t.Fatalf("QueueStats failed: %v", err)
	}
	if resp.Stats.Available != 1 {
		t.Errorf("expected 1 available, got %d", resp.Stats.Available)
	}
}

func TestPauseResumeQueue(t *testing.T) {
	client, _, cleanup := startTestServer(t)
	defer cleanup()

	_, err := client.PauseQueue(context.Background(), &ojsv1.PauseQueueRequest{Queue: "pq"})
	if err != nil {
		t.Fatalf("PauseQueue failed: %v", err)
	}

	_, err = client.ResumeQueue(context.Background(), &ojsv1.ResumeQueueRequest{Queue: "pq"})
	if err != nil {
		t.Fatalf("ResumeQueue failed: %v", err)
	}
}

func TestCronLifecycle(t *testing.T) {
	client, _, cleanup := startTestServer(t)
	defer cleanup()

	regResp, err := client.RegisterCron(context.Background(), &ojsv1.RegisterCronRequest{
		Name:     "test-cron",
		Cron:     "*/5 * * * *",
		Type:     "cron.test",
		Timezone: "UTC",
	})
	if err != nil {
		t.Fatalf("RegisterCron failed: %v", err)
	}
	if regResp.Name != "test-cron" {
		t.Errorf("expected name test-cron, got %s", regResp.Name)
	}

	listResp, err := client.ListCron(context.Background(), &ojsv1.ListCronRequest{})
	if err != nil {
		t.Fatalf("ListCron failed: %v", err)
	}
	if len(listResp.Entries) != 1 {
		t.Errorf("expected 1 cron entry, got %d", len(listResp.Entries))
	}

	_, err = client.UnregisterCron(context.Background(), &ojsv1.UnregisterCronRequest{
		Name: "test-cron",
	})
	if err != nil {
		t.Fatalf("UnregisterCron failed: %v", err)
	}
}

func TestDeadLetterLifecycle(t *testing.T) {
	client, backend, cleanup := startTestServer(t)
	defer cleanup()

	maxAttempts := 0
	coreJob := &core.Job{Type: "dl.test", Queue: "default", Args: json.RawMessage(`[]`), MaxAttempts: &maxAttempts}
	backend.Push(context.Background(), coreJob)
	fetched, _ := backend.Fetch(context.Background(), []string{"default"}, 1, "w1", 30000)
	backend.Nack(context.Background(), fetched[0].ID, nil, false)

	listResp, err := client.ListDeadLetter(context.Background(), &ojsv1.ListDeadLetterRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListDeadLetter failed: %v", err)
	}
	if listResp.TotalCount < 1 {
		t.Error("expected at least 1 dead letter job")
	}

	retryResp, err := client.RetryDeadLetter(context.Background(), &ojsv1.RetryDeadLetterRequest{
		JobId: fetched[0].ID,
	})
	if err != nil {
		t.Fatalf("RetryDeadLetter failed: %v", err)
	}
	if retryResp.Job == nil {
		t.Error("expected job in retry response")
	}
}

func TestWorkflowLifecycle(t *testing.T) {
	client, _, cleanup := startTestServer(t)
	defer cleanup()

	createResp, err := client.CreateWorkflow(context.Background(), &ojsv1.CreateWorkflowRequest{
		Name: "test-wf",
		Steps: []*ojsv1.WorkflowStep{
			{Id: "step1", Type: "wf.step1"},
			{Id: "step2", Type: "wf.step2"},
		},
	})
	if err != nil {
		t.Fatalf("CreateWorkflow failed: %v", err)
	}
	if createResp.Workflow == nil {
		t.Fatal("expected workflow in response")
	}

	wfID := createResp.Workflow.Id

	getResp, err := client.GetWorkflow(context.Background(), &ojsv1.GetWorkflowRequest{
		WorkflowId: wfID,
	})
	if err != nil {
		t.Fatalf("GetWorkflow failed: %v", err)
	}
	if getResp.Workflow.Id != wfID {
		t.Errorf("expected ID %s, got %s", wfID, getResp.Workflow.Id)
	}

	cancelResp, err := client.CancelWorkflow(context.Background(), &ojsv1.CancelWorkflowRequest{
		WorkflowId: wfID,
	})
	if err != nil {
		t.Fatalf("CancelWorkflow failed: %v", err)
	}
	if cancelResp.Workflow.State != ojsv1.WorkflowState_WORKFLOW_STATE_CANCELLED {
		t.Errorf("expected cancelled state, got %v", cancelResp.Workflow.State)
	}
}

func TestStreamEventsConnection(t *testing.T) {
	client, _, cleanup := startTestServer(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	stream, err := client.StreamEvents(ctx, &ojsv1.StreamEventsRequest{})
	if err != nil {
		t.Fatalf("StreamEvents failed: %v", err)
	}

	// Stream is alive; it will timeout waiting for events, which is expected
	_, recvErr := stream.Recv()
	if recvErr != nil {
		st, ok := status.FromError(recvErr)
		if ok && st.Code() != codes.DeadlineExceeded && st.Code() != codes.Canceled {
			t.Errorf("unexpected error: %v", recvErr)
		}
	}
}

func TestCoreErrorToGRPC(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode codes.Code
	}{
		{"nil error", nil, codes.OK},
		{"not found", core.NewNotFoundError("Job", "123"), codes.NotFound},
		{"conflict", core.NewConflictError("conflict", nil), codes.AlreadyExists},
		{"invalid request", core.NewInvalidRequestError("bad input", nil), codes.InvalidArgument},
		{"validation error", core.NewValidationError("invalid", nil), codes.InvalidArgument},
		{"internal error", core.NewInternalError("boom"), codes.Internal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := coreErrorToGRPC(tt.err)
			if tt.err == nil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
				return
			}
			st, ok := status.FromError(result)
			if !ok {
				t.Fatalf("expected gRPC status error, got %v", result)
			}
			if st.Code() != tt.wantCode {
				t.Errorf("expected code %v, got %v", tt.wantCode, st.Code())
			}
		})
	}
}
