package core

import "time"

const (
	EventJobStateChanged = "job.state_changed"
	EventJobProgress     = "job.progress"
	EventServerShutdown  = "server.shutdown"
)

type JobEvent struct {
	EventType string `json:"event"`
	JobID     string `json:"job_id"`
	Queue     string `json:"queue"`
	JobType   string `json:"type"`
	From      string `json:"from,omitempty"`
	To        string `json:"to,omitempty"`
	Progress  int    `json:"progress,omitempty"`
	Message   string `json:"message,omitempty"`
	Timestamp string `json:"timestamp"`
}

func NewStateChangedEvent(jobID, queue, jobType, from, to string) *JobEvent {
	return &JobEvent{
		EventType: EventJobStateChanged,
		JobID:     jobID,
		Queue:     queue,
		JobType:   jobType,
		From:      from,
		To:        to,
		Timestamp: FormatTime(time.Now()),
	}
}

type EventPublisher interface {
	PublishJobEvent(event *JobEvent) error
	Close() error
}

type EventSubscriber interface {
	SubscribeJob(jobID string) (<-chan *JobEvent, func(), error)
	SubscribeQueue(queue string) (<-chan *JobEvent, func(), error)
	SubscribeAll() (<-chan *JobEvent, func(), error)
}
