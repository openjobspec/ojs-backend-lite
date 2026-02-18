package memory

import (
	"context"
	"fmt"
	"time"

	"github.com/openjobspec/ojs-backend-lite/internal/core"
)

func (b *MemoryBackend) Push(ctx context.Context, job *core.Job) (*core.Job, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()

	if job.ID == "" {
		job.ID = core.NewUUIDv7()
	}

	job.CreatedAt = core.FormatTime(now)
	job.Attempt = 0

	if job.Unique != nil {
		fingerprint := computeFingerprint(job)
		
		if existingID, exists := b.uniqueKeys[fingerprint]; exists {
			existingJob := b.jobs[existingID]
			if existingJob != nil && shouldCheckUnique(existingJob, job.Unique) {
				conflict := job.Unique.OnConflict
				if conflict == "" {
					conflict = "reject"
				}

				switch conflict {
				case "reject":
					return nil, &core.OJSError{
						Code:    core.ErrCodeDuplicate,
						Message: "A job with the same unique key already exists.",
						Details: map[string]any{
							"existing_job_id": existingID,
							"unique_key":      fingerprint,
						},
					}
				case "ignore":
					existingJob.IsExisting = true
					jobCopy := *existingJob
					return &jobCopy, nil
				case "replace":
					if core.IsCancellableState(existingJob.State) {
						existingJob.State = core.StateCancelled
						existingJob.CancelledAt = core.FormatTime(now)
						if b.persist != nil {
							b.persist.SaveJob(existingJob)
						}
					}
				}
			}
		}

		b.uniqueKeys[fingerprint] = job.ID
		if b.persist != nil {
			b.persist.SaveUniqueKey(fingerprint, job.ID)
		}
	}

	if job.ScheduledAt != "" {
		scheduledTime, err := time.Parse(time.RFC3339, job.ScheduledAt)
		if err == nil && scheduledTime.After(now) {
			job.State = core.StateScheduled
			job.EnqueuedAt = core.FormatTime(now)
			b.jobs[job.ID] = job

			if b.persist != nil {
				b.persist.SaveJob(job)
			}

			return job, nil
		}
	}

	job.State = core.StateAvailable
	job.EnqueuedAt = core.FormatTime(now)
	b.jobs[job.ID] = job
	b.addToQueueIndex(job)

	if b.persist != nil {
		b.persist.SaveJob(job)
	}

	return job, nil
}

func (b *MemoryBackend) PushBatch(ctx context.Context, jobs []*core.Job) ([]*core.Job, error) {
	// Validate all jobs first (no lock needed for validation)
	for _, job := range jobs {
		if err := core.ValidateEnqueueRequest(&core.EnqueueRequest{
			Type: job.Type,
			Args: job.Args,
		}); err != nil {
			return nil, err
		}
	}

	// Push each job individually — Push() handles its own locking
	result := make([]*core.Job, 0, len(jobs))
	for _, job := range jobs {
		pushed, err := b.Push(ctx, job)
		if err != nil {
			return nil, err
		}
		result = append(result, pushed)
	}

	return result, nil
}

func (b *MemoryBackend) Info(ctx context.Context, jobID string) (*core.Job, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	job, exists := b.jobs[jobID]
	if !exists {
		job = b.deadJobs[jobID]
		if job == nil {
			return nil, core.NewNotFoundError("Job", jobID)
		}
	}

	jobCopy := *job
	return &jobCopy, nil
}

func (b *MemoryBackend) Cancel(ctx context.Context, jobID string) (*core.Job, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	job, exists := b.jobs[jobID]
	if !exists {
		return nil, core.NewNotFoundError("Job", jobID)
	}

	if !core.IsCancellableState(job.State) {
		return nil, core.NewConflictError(
			fmt.Sprintf("Cannot cancel job in state '%s'.", job.State),
			nil,
		)
	}

	job.State = core.StateCancelled
	job.CancelledAt = core.FormatTime(time.Now())

	b.cleanupUniqueKey(job)

	if b.persist != nil {
		b.persist.SaveJob(job)
	}

	jobCopy := *job
	return &jobCopy, nil
}

func computeFingerprint(job *core.Job) string {
	keys := job.Unique.Keys
	if len(keys) == 0 {
		keys = []string{"type", "queue"}
	}

	fingerprint := ""
	for _, key := range keys {
		switch key {
		case "type":
			fingerprint += "type:" + job.Type + ";"
		case "queue":
			fingerprint += "queue:" + job.Queue + ";"
		case "args":
			if job.Args != nil {
				fingerprint += "args:" + string(job.Args) + ";"
			}
		case "meta":
			if job.Meta != nil {
				fingerprint += "meta:" + string(job.Meta) + ";"
			}
		}
	}

	return fingerprint
}

func shouldCheckUnique(existingJob *core.Job, policy *core.UniquePolicy) bool {
	if len(policy.States) == 0 {
		return !core.IsTerminalState(existingJob.State)
	}

	for _, state := range policy.States {
		if existingJob.State == state {
			return true
		}
	}
	return false
}

// cleanupUniqueKey removes the unique key mapping for a job reaching a terminal state.
// Must be called while holding b.mu.Lock().
func (b *MemoryBackend) cleanupUniqueKey(job *core.Job) {
	if job.Unique == nil {
		return
	}
	fingerprint := computeFingerprint(job)
	if existingID, ok := b.uniqueKeys[fingerprint]; ok && existingID == job.ID {
		delete(b.uniqueKeys, fingerprint)
	}
}
