package events

import (
	"sync"

	"github.com/openjobspec/ojs-backend-lite/internal/core"
)

// Broker is an in-process event broker implementing both EventPublisher and EventSubscriber.
type Broker struct {
	mu          sync.RWMutex
	subscribers map[string][]chan *core.JobEvent // channel key → subscriber channels
	allSubs     []chan *core.JobEvent            // global subscribers
	bufSize     int
}

// NewBroker creates a new in-process event broker.
func NewBroker() *Broker {
	return &Broker{
		subscribers: make(map[string][]chan *core.JobEvent),
		allSubs:     make([]chan *core.JobEvent, 0),
		bufSize:     64,
	}
}

// PublishJobEvent publishes an event to all matching subscribers.
func (b *Broker) PublishJobEvent(event *core.JobEvent) error {
	if event == nil {
		return nil
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	// Send to job-specific subscribers
	jobKey := "job:" + event.JobID
	if subs, ok := b.subscribers[jobKey]; ok {
		for _, ch := range subs {
			select {
			case ch <- event:
			default:
				// Channel full, skip to prevent blocking
			}
		}
	}

	// Send to queue-specific subscribers
	if event.Queue != "" {
		queueKey := "queue:" + event.Queue
		if subs, ok := b.subscribers[queueKey]; ok {
			for _, ch := range subs {
				select {
				case ch <- event:
				default:
					// Channel full, skip
				}
			}
		}
	}

	// Send to all global subscribers
	for _, ch := range b.allSubs {
		select {
		case ch <- event:
		default:
			// Channel full, skip
		}
	}

	return nil
}

// SubscribeJob subscribes to events for a specific job ID.
func (b *Broker) SubscribeJob(jobID string) (<-chan *core.JobEvent, func(), error) {
	ch := make(chan *core.JobEvent, b.bufSize)
	key := "job:" + jobID

	b.mu.Lock()
	b.subscribers[key] = append(b.subscribers[key], ch)
	b.mu.Unlock()

	unsub := func() {
		b.mu.Lock()
		defer b.mu.Unlock()

		subs := b.subscribers[key]
		for i, c := range subs {
			if c == ch {
				b.subscribers[key] = append(subs[:i], subs[i+1:]...)
				break
			}
		}
		if len(b.subscribers[key]) == 0 {
			delete(b.subscribers, key)
		}
		close(ch)
	}

	return ch, unsub, nil
}

// SubscribeQueue subscribes to events for a specific queue.
func (b *Broker) SubscribeQueue(queue string) (<-chan *core.JobEvent, func(), error) {
	ch := make(chan *core.JobEvent, b.bufSize)
	key := "queue:" + queue

	b.mu.Lock()
	b.subscribers[key] = append(b.subscribers[key], ch)
	b.mu.Unlock()

	unsub := func() {
		b.mu.Lock()
		defer b.mu.Unlock()

		subs := b.subscribers[key]
		for i, c := range subs {
			if c == ch {
				b.subscribers[key] = append(subs[:i], subs[i+1:]...)
				break
			}
		}
		if len(b.subscribers[key]) == 0 {
			delete(b.subscribers, key)
		}
		close(ch)
	}

	return ch, unsub, nil
}

// SubscribeAll subscribes to all events.
func (b *Broker) SubscribeAll() (<-chan *core.JobEvent, func(), error) {
	ch := make(chan *core.JobEvent, b.bufSize)

	b.mu.Lock()
	b.allSubs = append(b.allSubs, ch)
	b.mu.Unlock()

	unsub := func() {
		b.mu.Lock()
		defer b.mu.Unlock()

		for i, c := range b.allSubs {
			if c == ch {
				b.allSubs = append(b.allSubs[:i], b.allSubs[i+1:]...)
				break
			}
		}
		close(ch)
	}

	return ch, unsub, nil
}

// Close closes the broker and all subscriber channels.
func (b *Broker) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Close all channels
	for _, subs := range b.subscribers {
		for _, ch := range subs {
			close(ch)
		}
	}
	for _, ch := range b.allSubs {
		close(ch)
	}

	// Clear maps
	b.subscribers = make(map[string][]chan *core.JobEvent)
	b.allSubs = make([]chan *core.JobEvent, 0)

	return nil
}
