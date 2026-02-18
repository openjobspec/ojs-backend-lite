package memory

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/openjobspec/ojs-backend-lite/internal/core"
)

func newCronParser() cron.Parser {
	return cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
}

func (b *MemoryBackend) RegisterCron(ctx context.Context, cronJob *core.CronJob) (*core.CronJob, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	expr := cronJob.Expression
	if expr == "" {
		expr = cronJob.Schedule
	}

	parser := newCronParser()
	var schedule cron.Schedule
	var err error

	if cronJob.Timezone != "" {
		loc, locErr := time.LoadLocation(cronJob.Timezone)
		if locErr != nil {
			return nil, core.NewInvalidRequestError(
				fmt.Sprintf("Invalid timezone: %s", cronJob.Timezone),
				map[string]any{"timezone": cronJob.Timezone},
			)
		}
		schedule, err = parser.Parse("CRON_TZ=" + loc.String() + " " + expr)
		if err != nil {
			schedule, err = parser.Parse(expr)
		}
	} else {
		schedule, err = parser.Parse(expr)
	}

	if err != nil {
		return nil, core.NewInvalidRequestError(
			fmt.Sprintf("Invalid cron expression: %s", expr),
			map[string]any{"expression": expr, "error": err.Error()},
		)
	}

	now := time.Now()
	cronJob.CreatedAt = core.FormatTime(now)
	cronJob.NextRunAt = core.FormatTime(schedule.Next(now))
	cronJob.Schedule = expr
	cronJob.Expression = expr

	if cronJob.OverlapPolicy == "" {
		cronJob.OverlapPolicy = "allow"
	}
	cronJob.Enabled = true

	cronCopy := *cronJob
	b.crons[cronJob.Name] = &cronCopy

	if b.persist != nil {
		b.persist.SaveCron(&cronCopy)
	}

	result := *cronJob
	return &result, nil
}

func (b *MemoryBackend) ListCron(ctx context.Context) ([]*core.CronJob, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	crons := make([]*core.CronJob, 0, len(b.crons))
	for _, cron := range b.crons {
		cronCopy := *cron
		crons = append(crons, &cronCopy)
	}

	sort.Slice(crons, func(i, j int) bool {
		return crons[i].Name < crons[j].Name
	})

	return crons, nil
}

func (b *MemoryBackend) DeleteCron(ctx context.Context, name string) (*core.CronJob, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	cron, exists := b.crons[name]
	if !exists {
		return nil, core.NewNotFoundError("Cron job", name)
	}

	delete(b.crons, name)

	if b.persist != nil {
		b.persist.DeleteCron(name)
	}

	cronCopy := *cron
	return &cronCopy, nil
}
