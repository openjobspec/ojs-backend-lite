package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/openjobspec/ojs-backend-lite/internal/core"
)

func (b *MemoryBackend) CreateWorkflow(ctx context.Context, req *core.WorkflowRequest) (*core.Workflow, error) {
	b.mu.Lock()

	now := time.Now()
	wfID := core.NewUUIDv7()

	jobs := req.Jobs
	if req.Type == "chain" {
		jobs = req.Steps
	}

	total := len(jobs)

	wf := &core.Workflow{
		ID:        wfID,
		Name:      req.Name,
		Type:      req.Type,
		State:     "running",
		CreatedAt: core.FormatTime(now),
	}

	if req.Type == "chain" {
		wf.StepsTotal = &total
		zero := 0
		wf.StepsCompleted = &zero
	} else {
		wf.JobsTotal = &total
		zero := 0
		wf.JobsCompleted = &zero
	}

	if req.Callbacks != nil {
		wf.Callbacks = req.Callbacks
	}

	state := &workflowState{
		ID:        wfID,
		Name:      req.Name,
		Type:      req.Type,
		State:     "running",
		Total:     total,
		Completed: 0,
		Failed:    0,
		CreatedAt: core.FormatTime(now),
		JobDefs:   jobs,
		Callbacks: req.Callbacks,
		JobIDs:    []string{},
		Results:   make(map[int][]byte),
	}

	b.workflows[wfID] = state

	if b.persist != nil {
		b.persist.SaveWorkflow(wfID, state)
	}

	// Collect jobs to enqueue, then release lock before pushing
	var jobsToEnqueue []*core.Job
	if req.Type == "chain" {
		jobsToEnqueue = append(jobsToEnqueue, workflowJobToJob(jobs[0], wfID, 0))
	} else {
		for i, step := range jobs {
			jobsToEnqueue = append(jobsToEnqueue, workflowJobToJob(step, wfID, i))
		}
	}
	b.mu.Unlock()

	// Push jobs outside the lock
	for _, job := range jobsToEnqueue {
		created, err := b.Push(ctx, job)
		if err != nil {
			return nil, err
		}
		b.mu.Lock()
		state.JobIDs = append(state.JobIDs, created.ID)
		b.mu.Unlock()
	}

	return wf, nil
}

func (b *MemoryBackend) GetWorkflow(ctx context.Context, id string) (*core.Workflow, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	state, exists := b.workflows[id]
	if !exists {
		return nil, core.NewNotFoundError("Workflow", id)
	}

	wf := &core.Workflow{
		ID:        state.ID,
		Name:      state.Name,
		Type:      state.Type,
		State:     state.State,
		CreatedAt: state.CreatedAt,
	}

	if state.CompletedAt != "" {
		wf.CompletedAt = state.CompletedAt
	}

	if state.Type == "chain" {
		wf.StepsTotal = &state.Total
		wf.StepsCompleted = &state.Completed
	} else {
		wf.JobsTotal = &state.Total
		wf.JobsCompleted = &state.Completed
	}

	if state.Callbacks != nil {
		wf.Callbacks = state.Callbacks
	}

	return wf, nil
}

func (b *MemoryBackend) CancelWorkflow(ctx context.Context, id string) (*core.Workflow, error) {
	b.mu.Lock()

	state, exists := b.workflows[id]
	if !exists {
		b.mu.Unlock()
		return nil, core.NewNotFoundError("Workflow", id)
	}

	if state.State == "completed" || state.State == "failed" || state.State == "cancelled" {
		b.mu.Unlock()
		return nil, core.NewConflictError(
			fmt.Sprintf("Cannot cancel workflow in state '%s'.", state.State),
			nil,
		)
	}

	// Collect non-terminal job IDs to cancel
	jobsToCancel := make([]string, 0)
	for _, jobID := range state.JobIDs {
		job, exists := b.jobs[jobID]
		if exists && !core.IsTerminalState(job.State) {
			jobsToCancel = append(jobsToCancel, jobID)
		}
	}
	b.mu.Unlock()

	// Cancel jobs outside the lock
	for _, jobID := range jobsToCancel {
		b.Cancel(ctx, jobID)
	}

	b.mu.Lock()
	now := time.Now()
	state.State = "cancelled"
	state.CompletedAt = core.FormatTime(now)

	if b.persist != nil {
		b.persist.SaveWorkflow(id, state)
	}

	wf := &core.Workflow{
		ID:          state.ID,
		Name:        state.Name,
		Type:        state.Type,
		State:       "cancelled",
		CreatedAt:   state.CreatedAt,
		CompletedAt: state.CompletedAt,
	}

	if state.Type == "chain" {
		wf.StepsTotal = &state.Total
		wf.StepsCompleted = &state.Completed
	} else {
		wf.JobsTotal = &state.Total
		wf.JobsCompleted = &state.Completed
	}
	b.mu.Unlock()

	return wf, nil
}

func (b *MemoryBackend) AdvanceWorkflow(ctx context.Context, workflowID string, jobID string, result json.RawMessage, failed bool) error {
	b.mu.Lock()

	state, exists := b.workflows[workflowID]
	if !exists {
		b.mu.Unlock()
		return nil
	}

	if state.State != "running" {
		b.mu.Unlock()
		return nil
	}

	job, jobExists := b.jobs[jobID]
	if !jobExists {
		b.mu.Unlock()
		return nil
	}

	stepIdx := job.WorkflowStep
	if result != nil {
		state.Results[stepIdx] = result
	}

	if failed {
		state.Failed++
	} else {
		state.Completed++
	}

	if state.Type == "chain" {
		if failed {
			state.State = "failed"
			state.CompletedAt = core.FormatTime(time.Now())
			if b.persist != nil {
				b.persist.SaveWorkflow(workflowID, state)
			}
			b.mu.Unlock()
			b.fireWorkflowCallbacks(ctx, state, true)
		} else if state.Completed < state.Total {
			nextStepIdx := state.Completed
			if b.persist != nil {
				b.persist.SaveWorkflow(workflowID, state)
			}
			b.mu.Unlock()
			b.enqueueChainStep(ctx, state, nextStepIdx)
		} else {
			state.State = "completed"
			state.CompletedAt = core.FormatTime(time.Now())
			if b.persist != nil {
				b.persist.SaveWorkflow(workflowID, state)
			}
			b.mu.Unlock()
			b.fireWorkflowCallbacks(ctx, state, false)
		}
	} else {
		if state.Completed+state.Failed >= state.Total {
			if state.Failed > 0 {
				state.State = "failed"
			} else {
				state.State = "completed"
			}
			state.CompletedAt = core.FormatTime(time.Now())
			if b.persist != nil {
				b.persist.SaveWorkflow(workflowID, state)
			}
			b.mu.Unlock()
			b.fireWorkflowCallbacks(ctx, state, state.Failed > 0)
		} else {
			if b.persist != nil {
				b.persist.SaveWorkflow(workflowID, state)
			}
			b.mu.Unlock()
		}
	}

	return nil
}

func (b *MemoryBackend) enqueueChainStep(ctx context.Context, state *workflowState, stepIdx int) {
	if stepIdx >= len(state.JobDefs) {
		return
	}

	b.mu.RLock()
	parentResults := make([]json.RawMessage, 0, stepIdx)
	for i := 0; i < stepIdx; i++ {
		if result, ok := state.Results[i]; ok {
			parentResults = append(parentResults, result)
		}
	}
	b.mu.RUnlock()

	job := workflowJobToJob(state.JobDefs[stepIdx], state.ID, stepIdx)
	if len(parentResults) > 0 {
		job.ParentResults = parentResults
	}

	created, err := b.Push(ctx, job)
	if err != nil {
		return
	}

	b.mu.Lock()
	state.JobIDs = append(state.JobIDs, created.ID)
	b.mu.Unlock()
}

func (b *MemoryBackend) fireWorkflowCallbacks(ctx context.Context, state *workflowState, failed bool) {
	if state.Callbacks == nil {
		return
	}

	var callback *core.WorkflowCallback
	if failed && state.Callbacks.OnFailure != nil {
		callback = state.Callbacks.OnFailure
	} else if !failed && state.Callbacks.OnSuccess != nil {
		callback = state.Callbacks.OnSuccess
	}

	if state.Callbacks.OnComplete != nil {
		callback = state.Callbacks.OnComplete
	}

	if callback == nil {
		return
	}

	queue := "default"
	if callback.Options != nil && callback.Options.Queue != "" {
		queue = callback.Options.Queue
	}

	job := &core.Job{
		Type:  callback.Type,
		Args:  callback.Args,
		Queue: queue,
	}

	b.Push(ctx, job)
}

func workflowJobToJob(step core.WorkflowJobRequest, workflowID string, stepIdx int) *core.Job {
	queue := "default"
	if step.Options != nil && step.Options.Queue != "" {
		queue = step.Options.Queue
	}

	job := &core.Job{
		Type:         step.Type,
		Args:         step.Args,
		Queue:        queue,
		WorkflowID:   workflowID,
		WorkflowStep: stepIdx,
	}

	if step.Options != nil && step.Options.RetryPolicy != nil {
		job.Retry = step.Options.RetryPolicy
		job.MaxAttempts = &step.Options.RetryPolicy.MaxAttempts
	} else if step.Options != nil && step.Options.Retry != nil {
		job.Retry = step.Options.Retry
		job.MaxAttempts = &step.Options.Retry.MaxAttempts
	}

	return job
}
