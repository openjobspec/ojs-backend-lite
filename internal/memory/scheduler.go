package memory

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/openjobspec/ojs-backend-lite/internal/core"
)

func (b *MemoryBackend) PromoteScheduled(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()

	for _, job := range b.jobs {
		if job.State != core.StateScheduled {
			continue
		}

		if job.ScheduledAt == "" {
			continue
		}

		scheduledTime, err := time.Parse(time.RFC3339, job.ScheduledAt)
		if err != nil {
			continue
		}

		if scheduledTime.After(now) {
			continue
		}

		job.State = core.StateAvailable
		b.addToQueueIndex(job)

		if b.persist != nil {
			b.persist.SaveJob(job)
		}
	}

	return nil
}

func (b *MemoryBackend) PromoteRetries(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()

	for _, job := range b.jobs {
		if job.State != core.StateRetryable {
			continue
		}

		if job.ScheduledAt == "" {
			continue
		}

		retryTime, err := time.Parse(time.RFC3339, job.ScheduledAt)
		if err != nil {
			continue
		}

		if retryTime.After(now) {
			continue
		}

		job.State = core.StateAvailable
		job.ScheduledAt = ""
		b.addToQueueIndex(job)

		if b.persist != nil {
			b.persist.SaveJob(job)
		}
	}

	return nil
}

func (b *MemoryBackend) RequeueStalled(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()

	for _, job := range b.jobs {
		if job.State != core.StateActive {
			continue
		}

		if job.VisibilityTimeoutMs == nil {
			continue
		}

		if job.StartedAt == "" {
			continue
		}

		startedTime, err := time.Parse(time.RFC3339, job.StartedAt)
		if err != nil {
			continue
		}

		timeout := time.Duration(*job.VisibilityTimeoutMs) * time.Millisecond
		deadline := startedTime.Add(timeout)

		if now.After(deadline) {
			workerID := job.WorkerID
			job.State = core.StateAvailable
			job.StartedAt = ""
			job.WorkerID = ""
			b.addToQueueIndex(job)

			if b.persist != nil {
				b.persist.SaveJob(job)
			}

			if worker, ok := b.workers[workerID]; ok {
				removeJobFromWorker(worker, job.ID)
			}
		}
	}

	return nil
}

func (b *MemoryBackend) FireCronJobs(ctx context.Context) error {
	b.mu.Lock()
	cronsCopy := make([]*core.CronJob, 0, len(b.crons))
	for _, cron := range b.crons {
		cronCopy := *cron
		cronsCopy = append(cronsCopy, &cronCopy)
	}
	b.mu.Unlock()

	now := time.Now()
	parser := newCronParser()

	for _, cronJob := range cronsCopy {
		if !cronJob.Enabled {
			continue
		}

		if cronJob.NextRunAt == "" {
			continue
		}

		nextRun, err := time.Parse(core.TimeFormat, cronJob.NextRunAt)
		if err != nil {
			continue
		}

		if now.Before(nextRun) {
			continue
		}

		var jobType string
		var args json.RawMessage
		var queue string

		if cronJob.JobTemplate != nil {
			jobType = cronJob.JobTemplate.Type
			args = cronJob.JobTemplate.Args
			if cronJob.JobTemplate.Options != nil {
				queue = cronJob.JobTemplate.Options.Queue
			}
		}
		if queue == "" {
			queue = "default"
		}

		b.mu.RLock()
		shouldSkip := false
		if cronJob.OverlapPolicy == "skip" {
			for _, job := range b.jobs {
				if job.Meta == nil {
					continue
				}
				var metaObj map[string]any
				if json.Unmarshal(job.Meta, &metaObj) == nil {
					if cn, ok := metaObj["cron_name"]; ok {
						if cnStr, ok := cn.(string); ok && cnStr == cronJob.Name {
							if !core.IsTerminalState(job.State) {
								shouldSkip = true
								break
							}
						}
					}
				}
			}
		}
		b.mu.RUnlock()

		if shouldSkip {
			b.updateCronNextRun(cronJob.Name, cronJob, parser, now)
			continue
		}

		job := &core.Job{
			Type:  jobType,
			Args:  args,
			Queue: queue,
		}

		if cronJob.OverlapPolicy == "skip" {
			meta := map[string]any{"cron_name": cronJob.Name}
			metaJSON, marshalErr := json.Marshal(meta)
			if marshalErr != nil {
				slog.Warn("fire cron: failed to marshal cron metadata", "name", cronJob.Name, "error", marshalErr)
			}
			job.Meta = metaJSON
		}

		b.Push(ctx, job)

		b.updateCronNextRun(cronJob.Name, cronJob, parser, now)
	}

	return nil
}

func (b *MemoryBackend) updateCronNextRun(name string, cronJob *core.CronJob, parser cron.Parser, now time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()

	expr := cronJob.Expression
	if expr == "" {
		expr = cronJob.Schedule
	}

	schedule, err := parser.Parse(expr)
	if err != nil {
		return
	}

	cronJob.LastRunAt = core.FormatTime(now)
	cronJob.NextRunAt = core.FormatTime(schedule.Next(now))

	if storedCron, ok := b.crons[name]; ok {
		storedCron.LastRunAt = cronJob.LastRunAt
		storedCron.NextRunAt = cronJob.NextRunAt

		if b.persist != nil {
			b.persist.SaveCron(storedCron)
		}
	}
}

func (b *MemoryBackend) PurgeExpired(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()

	for id, job := range b.jobs {
		if job.State == core.StateCompleted && job.CompletedAt != "" {
			completedTime, err := time.Parse(time.RFC3339, job.CompletedAt)
			if err == nil && now.Sub(completedTime) > b.retentionCompleted {
				delete(b.jobs, id)
				if b.persist != nil {
					b.persist.DeleteJob(id)
				}
			}
		} else if job.State == core.StateCancelled && job.CancelledAt != "" {
			cancelledTime, err := time.Parse(time.RFC3339, job.CancelledAt)
			if err == nil && now.Sub(cancelledTime) > b.retentionCancelled {
				delete(b.jobs, id)
				if b.persist != nil {
					b.persist.DeleteJob(id)
				}
			}
		} else if job.State == core.StateDiscarded && job.CompletedAt != "" {
			discardedTime, err := time.Parse(time.RFC3339, job.CompletedAt)
			if err == nil && now.Sub(discardedTime) > b.retentionDiscarded {
				delete(b.jobs, id)
				delete(b.deadJobs, id)
				if b.persist != nil {
					b.persist.DeleteJob(id)
					b.persist.DeleteDeadJob(id)
				}
			}
		}
	}

	return nil
}
