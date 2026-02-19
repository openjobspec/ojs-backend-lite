package memory

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/openjobspec/ojs-backend-lite/internal/core"
)

func TestConcurrentPush(t *testing.T) {
	backend, _ := New()
	defer backend.Close()

	ctx := context.Background()
	numGoroutines := 100
	jobsPerGoroutine := 10

	var wg sync.WaitGroup
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < jobsPerGoroutine; j++ {
				job := &core.Job{
					Type:  "test",
					Queue: "default",
					Args:  []byte(`[]`),
				}
				if _, err := backend.Push(ctx, job); err != nil {
					t.Errorf("Push failed: %v", err)
				}
			}
		}(i)
	}

	wg.Wait()

	backend.mu.RLock()
	total := len(backend.jobs)
	backend.mu.RUnlock()

	expected := numGoroutines * jobsPerGoroutine
	if total != expected {
		t.Errorf("Expected %d jobs, got %d", expected, total)
	}
}

func TestConcurrentFetchExclusive(t *testing.T) {
	backend, _ := New()
	defer backend.Close()

	ctx := context.Background()
	numJobs := 100

	for i := 0; i < numJobs; i++ {
		job := &core.Job{
			Type:  "test",
			Queue: "default",
		}
		if _, err := backend.Push(ctx, job); err != nil {
			t.Fatalf("Push failed: %v", err)
		}
	}

	numWorkers := 50
	var totalFetched atomic.Int32
	var wg sync.WaitGroup
	seenJobs := sync.Map{}

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for {
				jobs, err := backend.Fetch(ctx, []string{"default"}, 5, core.NewUUIDv7(), 30000)
				if err != nil {
					t.Errorf("Fetch failed: %v", err)
					return
				}
				if len(jobs) == 0 {
					return
				}
				for _, job := range jobs {
					if _, loaded := seenJobs.LoadOrStore(job.ID, true); loaded {
						t.Errorf("Job %s fetched multiple times", job.ID)
					}
					totalFetched.Add(1)
				}
			}
		}(i)
	}

	wg.Wait()

	if totalFetched.Load() != int32(numJobs) {
		t.Errorf("Expected %d fetched jobs, got %d", numJobs, totalFetched.Load())
	}
}

func TestConcurrentAckSameJob(t *testing.T) {
	backend, _ := New()
	defer backend.Close()

	ctx := context.Background()

	job := &core.Job{
		Type:  "test",
		Queue: "default",
	}
	pushed, err := backend.Push(ctx, job)
	if err != nil {
		t.Fatalf("Push failed: %v", err)
	}

	jobs, err := backend.Fetch(ctx, []string{"default"}, 1, "worker-1", 30000)
	if err != nil || len(jobs) != 1 {
		t.Fatalf("Fetch failed")
	}

	jobID := pushed.ID
	numGoroutines := 10
	var successCount atomic.Int32
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := backend.Ack(ctx, jobID, nil)
			if err == nil {
				successCount.Add(1)
			}
		}()
	}

	wg.Wait()

	if successCount.Load() != 1 {
		t.Errorf("Expected exactly 1 successful Ack, got %d", successCount.Load())
	}
}

func TestConcurrentNackSameJob(t *testing.T) {
	backend, _ := New()
	defer backend.Close()

	ctx := context.Background()

	job := &core.Job{
		Type:  "test",
		Queue: "default",
	}
	pushed, err := backend.Push(ctx, job)
	if err != nil {
		t.Fatalf("Push failed: %v", err)
	}

	jobs, err := backend.Fetch(ctx, []string{"default"}, 1, "worker-1", 30000)
	if err != nil || len(jobs) != 1 {
		t.Fatalf("Fetch failed")
	}

	jobID := pushed.ID
	numGoroutines := 10
	var successCount atomic.Int32
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := backend.Nack(ctx, jobID, nil, true)
			if err == nil {
				successCount.Add(1)
			}
		}()
	}

	wg.Wait()

	if successCount.Load() != 1 {
		t.Errorf("Expected exactly 1 successful Nack, got %d", successCount.Load())
	}
}

func TestConcurrentPushAndFetch(t *testing.T) {
	backend, _ := New()
	defer backend.Close()

	ctx := context.Background()
	numProducers := 20
	numConsumers := 20
	jobsPerProducer := 10

	var pushCount atomic.Int32
	var fetchCount atomic.Int32
	var wg sync.WaitGroup
	done := make(chan struct{})

	for i := 0; i < numProducers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < jobsPerProducer; j++ {
				job := &core.Job{
					Type:  "test",
					Queue: "default",
				}
				if _, err := backend.Push(ctx, job); err == nil {
					pushCount.Add(1)
				}
				time.Sleep(1 * time.Millisecond)
			}
		}()
	}

	for i := 0; i < numConsumers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
				}

				jobs, err := backend.Fetch(ctx, []string{"default"}, 5, core.NewUUIDv7(), 30000)
				if err != nil {
					time.Sleep(1 * time.Millisecond)
					continue
				}
				if len(jobs) == 0 {
					time.Sleep(5 * time.Millisecond)
					continue
				}
				fetchCount.Add(int32(len(jobs)))
				for _, job := range jobs {
					backend.Ack(ctx, job.ID, nil)
				}
			}
		}(i)
	}

	go func() {
		maxWait := time.After(10 * time.Second)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-maxWait:
				close(done)
				return
			case <-ticker.C:
				if fetchCount.Load() >= int32(numProducers*jobsPerProducer) {
					close(done)
					return
				}
			}
		}
	}()

	wg.Wait()

	if pushCount.Load() != int32(numProducers*jobsPerProducer) {
		t.Errorf("Expected %d pushed jobs, got %d", numProducers*jobsPerProducer, pushCount.Load())
	}
}

func TestConcurrentWorkflowAdvancement(t *testing.T) {
	backend, _ := New()
	defer backend.Close()

	ctx := context.Background()

	req := &core.WorkflowRequest{
		Type: "group",
		Jobs: []core.WorkflowJobRequest{
			{Type: "job1", Args: []byte(`{}`)},
			{Type: "job2", Args: []byte(`{}`)},
			{Type: "job3", Args: []byte(`{}`)},
		},
	}

	wf, err := backend.CreateWorkflow(ctx, req)
	if err != nil {
		t.Fatalf("CreateWorkflow failed: %v", err)
	}

	jobs, err := backend.Fetch(ctx, []string{"default"}, 10, "worker-1", 30000)
	if err != nil || len(jobs) != 3 {
		t.Fatalf("Fetch failed: expected 3 jobs")
	}

	var wg sync.WaitGroup
	for _, job := range jobs {
		wg.Add(1)
		go func(j *core.Job) {
			defer wg.Done()
			backend.Ack(ctx, j.ID, []byte(`{"result":"ok"}`))
		}(job)
	}

	wg.Wait()

	time.Sleep(100 * time.Millisecond)

	finalWF, err := backend.GetWorkflow(ctx, wf.ID)
	if err != nil {
		t.Fatalf("GetWorkflow failed: %v", err)
	}

	if finalWF.State != "completed" {
		t.Errorf("Expected workflow state 'completed', got '%s'", finalWF.State)
	}

	if *finalWF.JobsCompleted != 3 {
		t.Errorf("Expected 3 completed jobs, got %d", *finalWF.JobsCompleted)
	}
}

func TestConcurrentBatchPush(t *testing.T) {
	backend, _ := New()
	defer backend.Close()

	ctx := context.Background()
	numGoroutines := 20
	batchSize := 5

	var totalPushed atomic.Int32
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			batch := make([]*core.Job, batchSize)
			for j := 0; j < batchSize; j++ {
				batch[j] = &core.Job{
					Type:  "test",
					Queue: "default",
					Args:  []byte(`[]`),
				}
			}
			if _, err := backend.PushBatch(ctx, batch); err == nil {
				totalPushed.Add(int32(batchSize))
			} else {
				t.Errorf("PushBatch failed: %v", err)
			}
		}()
	}

	wg.Wait()

	expected := int32(numGoroutines * batchSize)
	if totalPushed.Load() != expected {
		t.Errorf("Expected %d pushed jobs, got %d", expected, totalPushed.Load())
	}
}
