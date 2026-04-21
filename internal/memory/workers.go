package memory

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/openjobspec/ojs-backend-lite/internal/core"
)

func (b *MemoryBackend) Fetch(ctx context.Context, queues []string, count int, workerID string, visibilityTimeoutMs int) ([]*core.Job, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if count <= 0 {
		count = 1
	}

	if visibilityTimeoutMs <= 0 {
		visibilityTimeoutMs = core.DefaultVisibilityTimeoutMs
	}

	now := time.Now()

	fetched := make([]*core.Job, 0, count)

	for _, queue := range queues {
		if len(fetched) >= count {
			break
		}

		if b.queueState[queue] == "paused" {
			continue
		}

		idx, exists := b.queueIdx[queue]
		if !exists {
			continue
		}

		for len(idx.available) > 0 && len(fetched) < count {
			job := idx.available[0]
			idx.available = idx.available[1:]

			job.State = core.StateActive
			job.StartedAt = core.FormatTime(now)
			job.WorkerID = workerID
			job.Attempt++
			job.VisibilityTimeoutMs = &visibilityTimeoutMs

			if b.persist != nil {
				b.persist.SaveJob(job)
			}

			jobCopy := *job
			fetched = append(fetched, &jobCopy)
		}
	}

	if worker, ok := b.workers[workerID]; ok {
		for _, job := range fetched {
			worker.ActiveJobs = append(worker.ActiveJobs, job.ID)
		}
	} else {
		activeJobs := make([]string, 0, len(fetched))
		for _, job := range fetched {
			activeJobs = append(activeJobs, job.ID)
		}
		b.workers[workerID] = &workerState{
			ID:            workerID,
			State:         "active",
			Directive:     "continue",
			ActiveJobs:    activeJobs,
			LastHeartbeat: core.FormatTime(now),
		}
	}

	return fetched, nil
}

func (b *MemoryBackend) Ack(ctx context.Context, jobID string, result []byte) (*core.AckResponse, error) {
	b.mu.Lock()

	job, exists := b.jobs[jobID]
	if !exists {
		b.mu.Unlock()
		return nil, core.NewNotFoundError("Job", jobID)
	}

	if job.State != core.StateActive {
		b.mu.Unlock()
		return nil, core.NewConflictError(
			"Job is not in active state.",
			nil,
		)
	}

	now := time.Now()
	job.State = core.StateCompleted
	job.CompletedAt = core.FormatTime(now)
	if result != nil {
		job.Result = json.RawMessage(result)
	}

	if worker, ok := b.workers[job.WorkerID]; ok {
		removeJobFromWorker(worker, jobID)
	}

	// Clean up unique key for completed jobs
	b.cleanupUniqueKey(job)

	if b.persist != nil {
		b.persist.SaveJob(job)
		if job.Unique != nil {
			fingerprint := computeFingerprint(job)
			b.persist.DeleteUniqueKey(fingerprint)
		}
	}

	workflowID := job.WorkflowID
	b.mu.Unlock()

	// Advance workflow outside the lock
	if workflowID != "" {
		b.AdvanceWorkflow(ctx, workflowID, jobID, result, false)
	}

	return &core.AckResponse{
		Acknowledged: true,
		ID:        jobID,
		State:        core.StateCompleted,
		CompletedAt:  job.CompletedAt,
	}, nil
}

func (b *MemoryBackend) Nack(ctx context.Context, jobID string, jobErr *core.JobError, requeue bool) (*core.NackResponse, error) {
	b.mu.Lock()

	job, exists := b.jobs[jobID]
	if !exists {
		b.mu.Unlock()
		return nil, core.NewNotFoundError("Job", jobID)
	}

	if job.State != core.StateActive {
		b.mu.Unlock()
		return nil, core.NewConflictError(
			"Job is not in active state.",
			nil,
		)
	}

	now := time.Now()

	if jobErr != nil {
		errJSON, marshalErr := json.Marshal(jobErr)
		if marshalErr != nil {
			slog.Warn("nack: failed to marshal job error", "job_id", job.ID, "error", marshalErr)
			errJSON = []byte(`{"message":"marshal error"}`)
		}
		job.Error = errJSON
		if job.Errors == nil {
			job.Errors = []json.RawMessage{}
		}
		job.Errors = append(job.Errors, errJSON)
	}

	retryPolicy := job.Retry
	if retryPolicy == nil {
		defaultPolicy := core.DefaultRetryPolicy()
		retryPolicy = &defaultPolicy
	}

	maxAttempts := retryPolicy.MaxAttempts
	if job.MaxAttempts != nil {
		maxAttempts = *job.MaxAttempts
	}

	resp := &core.NackResponse{
		ID:       jobID,
		Attempt:     job.Attempt,
		MaxAttempts: maxAttempts,
	}

	if worker, ok := b.workers[job.WorkerID]; ok {
		removeJobFromWorker(worker, jobID)
	}

	if job.Attempt >= maxAttempts {
		onExhaustion := retryPolicy.OnExhaustion
		if onExhaustion == "" || onExhaustion == "dead_letter" {
			job.State = core.StateDiscarded
			discardedAt := core.FormatTime(now)
			resp.State = core.StateDiscarded
			resp.DiscardedAt = discardedAt

			deadCopy := *job
			b.deadJobs[jobID] = &deadCopy

			b.cleanupUniqueKey(job)

			if b.persist != nil {
				b.persist.SaveJob(job)
				b.persist.SaveDeadJob(&deadCopy)
				if job.Unique != nil {
					fingerprint := computeFingerprint(job)
					b.persist.DeleteUniqueKey(fingerprint)
				}
			}

			workflowID := job.WorkflowID
			b.mu.Unlock()

			if workflowID != "" {
				b.AdvanceWorkflow(ctx, workflowID, jobID, nil, true)
			}
		} else {
			job.State = core.StateDiscarded
			resp.State = core.StateDiscarded
			b.cleanupUniqueKey(job)

			if b.persist != nil {
				b.persist.SaveJob(job)
				if job.Unique != nil {
					fingerprint := computeFingerprint(job)
					b.persist.DeleteUniqueKey(fingerprint)
				}
			}

			b.mu.Unlock()
		}
		return resp, nil
	}

	if requeue {
		job.State = core.StateAvailable
		resp.State = core.StateAvailable
		b.addToQueueIndex(job)
	} else {
		job.State = core.StateRetryable
		retryDelay := core.CalculateBackoff(retryPolicy, job.Attempt)
		retryAt := now.Add(retryDelay)
		job.ScheduledAt = core.FormatTime(retryAt)
		resp.State = core.StateRetryable
		resp.NextAttemptAt = job.ScheduledAt
	}

	if b.persist != nil {
		b.persist.SaveJob(job)
	}

	b.mu.Unlock()
	return resp, nil
}

func (b *MemoryBackend) Heartbeat(ctx context.Context, workerID string, activeJobs []string, visibilityTimeoutMs int) (*core.HeartbeatResponse, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()

	worker, exists := b.workers[workerID]
	if !exists {
		worker = &workerState{
			ID:        workerID,
			State:     "active",
			Directive: "continue",
		}
		b.workers[workerID] = worker
	}

	worker.LastHeartbeat = core.FormatTime(now)
	worker.ActiveJobs = activeJobs

	extended := make([]string, 0)
	for _, jobID := range activeJobs {
		job, exists := b.jobs[jobID]
		if !exists || job.State != core.StateActive {
			continue
		}

		visTimeout := visibilityTimeoutMs
		job.VisibilityTimeoutMs = &visTimeout
		extended = append(extended, jobID)
	}

	directive := worker.Directive
	if directive == "" {
		directive = "continue"
	}

	for _, jobID := range activeJobs {
		job, exists := b.jobs[jobID]
		if !exists {
			continue
		}
		if job.Meta != nil {
			var metaObj map[string]any
			if json.Unmarshal(job.Meta, &metaObj) == nil {
				if td, ok := metaObj["test_directive"]; ok {
					if tdStr, ok := td.(string); ok && tdStr != "" {
						directive = tdStr
						break
					}
				}
			}
		}
	}

	return &core.HeartbeatResponse{
		State:        "active",
		Directive:    directive,
		JobsExtended: extended,
		ServerTime:   core.FormatTime(now),
	}, nil
}

func (b *MemoryBackend) SetWorkerState(ctx context.Context, workerID string, state string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	worker, exists := b.workers[workerID]
	if !exists {
		worker = &workerState{
			ID:    workerID,
			State: "active",
		}
		b.workers[workerID] = worker
	}

	worker.Directive = state
	return nil
}

func removeJobFromWorker(worker *workerState, jobID string) {
	for i, jid := range worker.ActiveJobs {
		if jid == jobID {
			worker.ActiveJobs = append(worker.ActiveJobs[:i], worker.ActiveJobs[i+1:]...)
			break
		}
	}
}
