package events

import (
	"testing"
	"time"

	"github.com/openjobspec/ojs-backend-lite/internal/core"
)

// TestPublishAndSubscribeJob verifies job-specific subscription.
func TestPublishAndSubscribeJob(t *testing.T) {
	broker := NewBroker()
	defer broker.Close()

	jobID := "test-job-123"
	ch, unsub, err := broker.SubscribeJob(jobID)
	if err != nil {
		t.Fatalf("SubscribeJob failed: %v", err)
	}
	defer unsub()

	event := &core.JobEvent{
		EventType: core.EventJobStateChanged,
		JobID:     jobID,
		Queue:     "default",
		JobType:   "test",
		From:      "available",
		To:        "active",
		Timestamp: time.Now().Format(time.RFC3339),
	}

	err = broker.PublishJobEvent(event)
	if err != nil {
		t.Fatalf("PublishJobEvent failed: %v", err)
	}

	select {
	case received := <-ch:
		if received.JobID != jobID {
			t.Errorf("Expected job ID %s, got %s", jobID, received.JobID)
		}
		if received.EventType != core.EventJobStateChanged {
			t.Errorf("Expected event type %s, got %s", core.EventJobStateChanged, received.EventType)
		}
		if received.From != "available" {
			t.Errorf("Expected from available, got %s", received.From)
		}
		if received.To != "active" {
			t.Errorf("Expected to active, got %s", received.To)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout waiting for event")
	}
}

// TestSubscribeJobOnlyReceivesMatchingEvents verifies filtering.
func TestSubscribeJobOnlyReceivesMatchingEvents(t *testing.T) {
	broker := NewBroker()
	defer broker.Close()

	jobID := "test-job-123"
	ch, unsub, err := broker.SubscribeJob(jobID)
	if err != nil {
		t.Fatalf("SubscribeJob failed: %v", err)
	}
	defer unsub()

	// Publish event for different job
	otherEvent := &core.JobEvent{
		EventType: core.EventJobStateChanged,
		JobID:     "other-job-456",
		Queue:     "default",
		Timestamp: time.Now().Format(time.RFC3339),
	}
	broker.PublishJobEvent(otherEvent)

	// Should not receive event
	select {
	case event := <-ch:
		t.Errorf("Should not receive event for other job, got: %+v", event)
	case <-time.After(50 * time.Millisecond):
		// Expected
	}

	// Publish event for subscribed job
	matchingEvent := &core.JobEvent{
		EventType: core.EventJobStateChanged,
		JobID:     jobID,
		Queue:     "default",
		Timestamp: time.Now().Format(time.RFC3339),
	}
	broker.PublishJobEvent(matchingEvent)

	// Should receive event
	select {
	case received := <-ch:
		if received.JobID != jobID {
			t.Errorf("Expected job ID %s, got %s", jobID, received.JobID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout waiting for matching event")
	}
}

// TestSubscribeQueue verifies queue-specific subscription.
func TestSubscribeQueue(t *testing.T) {
	broker := NewBroker()
	defer broker.Close()

	queue := "default"
	ch, unsub, err := broker.SubscribeQueue(queue)
	if err != nil {
		t.Fatalf("SubscribeQueue failed: %v", err)
	}
	defer unsub()

	event := &core.JobEvent{
		EventType: core.EventJobStateChanged,
		JobID:     "job-123",
		Queue:     queue,
		JobType:   "test",
		Timestamp: time.Now().Format(time.RFC3339),
	}

	broker.PublishJobEvent(event)

	select {
	case received := <-ch:
		if received.Queue != queue {
			t.Errorf("Expected queue %s, got %s", queue, received.Queue)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout waiting for event")
	}
}

// TestSubscribeQueueFiltering verifies queue filtering.
func TestSubscribeQueueFiltering(t *testing.T) {
	broker := NewBroker()
	defer broker.Close()

	queue := "queue1"
	ch, unsub, err := broker.SubscribeQueue(queue)
	if err != nil {
		t.Fatalf("SubscribeQueue failed: %v", err)
	}
	defer unsub()

	// Publish event for different queue
	otherEvent := &core.JobEvent{
		EventType: core.EventJobStateChanged,
		JobID:     "job-123",
		Queue:     "queue2",
		Timestamp: time.Now().Format(time.RFC3339),
	}
	broker.PublishJobEvent(otherEvent)

	// Should not receive
	select {
	case event := <-ch:
		t.Errorf("Should not receive event for other queue, got: %+v", event)
	case <-time.After(50 * time.Millisecond):
		// Expected
	}

	// Publish event for subscribed queue
	matchingEvent := &core.JobEvent{
		EventType: core.EventJobStateChanged,
		JobID:     "job-456",
		Queue:     queue,
		Timestamp: time.Now().Format(time.RFC3339),
	}
	broker.PublishJobEvent(matchingEvent)

	// Should receive
	select {
	case received := <-ch:
		if received.Queue != queue {
			t.Errorf("Expected queue %s, got %s", queue, received.Queue)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout waiting for matching event")
	}
}

// TestSubscribeAll verifies global subscription.
func TestSubscribeAll(t *testing.T) {
	broker := NewBroker()
	defer broker.Close()

	ch, unsub, err := broker.SubscribeAll()
	if err != nil {
		t.Fatalf("SubscribeAll failed: %v", err)
	}
	defer unsub()

	// Publish events for different jobs
	event1 := &core.JobEvent{
		EventType: core.EventJobStateChanged,
		JobID:     "job-1",
		Queue:     "queue1",
		Timestamp: time.Now().Format(time.RFC3339),
	}
	event2 := &core.JobEvent{
		EventType: core.EventJobStateChanged,
		JobID:     "job-2",
		Queue:     "queue2",
		Timestamp: time.Now().Format(time.RFC3339),
	}

	broker.PublishJobEvent(event1)
	broker.PublishJobEvent(event2)

	// Should receive both
	receivedIDs := make(map[string]bool)
	for i := 0; i < 2; i++ {
		select {
		case received := <-ch:
			receivedIDs[received.JobID] = true
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("Timeout waiting for event %d", i+1)
		}
	}

	if !receivedIDs["job-1"] {
		t.Error("Should receive event for job-1")
	}
	if !receivedIDs["job-2"] {
		t.Error("Should receive event for job-2")
	}
}

// TestUnsubscribe verifies unsubscription stops events.
func TestUnsubscribe(t *testing.T) {
	broker := NewBroker()
	defer broker.Close()

	jobID := "test-job-123"
	ch, unsub, err := broker.SubscribeJob(jobID)
	if err != nil {
		t.Fatalf("SubscribeJob failed: %v", err)
	}

	event := &core.JobEvent{
		EventType: core.EventJobStateChanged,
		JobID:     jobID,
		Queue:     "default",
		Timestamp: time.Now().Format(time.RFC3339),
	}

	broker.PublishJobEvent(event)

	// Receive first event
	select {
	case <-ch:
		// Expected
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout waiting for first event")
	}

	// Unsubscribe
	unsub()

	// Wait for channel to close
	time.Sleep(10 * time.Millisecond)

	// Publish another event
	broker.PublishJobEvent(event)

	// Should not receive (channel closed)
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("Channel should be closed after unsubscribe")
		}
	case <-time.After(50 * time.Millisecond):
		t.Error("Channel should be closed immediately")
	}
}

// TestMultipleSubscribers verifies all subscribers receive events.
func TestMultipleSubscribers(t *testing.T) {
	broker := NewBroker()
	defer broker.Close()

	jobID := "test-job-123"

	// Subscribe 3 times
	ch1, unsub1, _ := broker.SubscribeJob(jobID)
	defer unsub1()
	ch2, unsub2, _ := broker.SubscribeJob(jobID)
	defer unsub2()
	ch3, unsub3, _ := broker.SubscribeJob(jobID)
	defer unsub3()

	event := &core.JobEvent{
		EventType: core.EventJobStateChanged,
		JobID:     jobID,
		Queue:     "default",
		Timestamp: time.Now().Format(time.RFC3339),
	}

	broker.PublishJobEvent(event)

	// All should receive
	channels := []<-chan *core.JobEvent{ch1, ch2, ch3}
	for i, ch := range channels {
		select {
		case received := <-ch:
			if received.JobID != jobID {
				t.Errorf("Subscriber %d: expected job ID %s, got %s", i+1, jobID, received.JobID)
			}
		case <-time.After(100 * time.Millisecond):
			t.Errorf("Subscriber %d: timeout waiting for event", i+1)
		}
	}
}

// TestPublishNilEvent verifies nil events are handled gracefully.
func TestPublishNilEvent(t *testing.T) {
	broker := NewBroker()
	defer broker.Close()

	err := broker.PublishJobEvent(nil)
	if err != nil {
		t.Errorf("Publishing nil event should not error, got: %v", err)
	}
}

// TestMixedSubscriptions verifies job, queue, and global subscriptions work together.
func TestMixedSubscriptions(t *testing.T) {
	broker := NewBroker()
	defer broker.Close()

	jobID := "test-job-123"
	queue := "default"

	chJob, unsubJob, _ := broker.SubscribeJob(jobID)
	defer unsubJob()
	chQueue, unsubQueue, _ := broker.SubscribeQueue(queue)
	defer unsubQueue()
	chAll, unsubAll, _ := broker.SubscribeAll()
	defer unsubAll()

	event := &core.JobEvent{
		EventType: core.EventJobStateChanged,
		JobID:     jobID,
		Queue:     queue,
		JobType:   "test",
		Timestamp: time.Now().Format(time.RFC3339),
	}

	broker.PublishJobEvent(event)

	// All three should receive
	select {
	case <-chJob:
		// Expected
	case <-time.After(100 * time.Millisecond):
		t.Error("Job subscriber did not receive event")
	}

	select {
	case <-chQueue:
		// Expected
	case <-time.After(100 * time.Millisecond):
		t.Error("Queue subscriber did not receive event")
	}

	select {
	case <-chAll:
		// Expected
	case <-time.After(100 * time.Millisecond):
		t.Error("Global subscriber did not receive event")
	}
}

// TestBufferedChannel verifies events are buffered.
func TestBufferedChannel(t *testing.T) {
	broker := NewBroker()
	defer broker.Close()

	jobID := "test-job-123"
	ch, unsub, _ := broker.SubscribeJob(jobID)
	defer unsub()

	// Publish multiple events without reading
	for i := 0; i < 10; i++ {
		event := &core.JobEvent{
			EventType: core.EventJobStateChanged,
			JobID:     jobID,
			Queue:     "default",
			Timestamp: time.Now().Format(time.RFC3339),
		}
		broker.PublishJobEvent(event)
	}

	// Read all events
	received := 0
	timeout := time.After(200 * time.Millisecond)
loop:
	for {
		select {
		case <-ch:
			received++
		case <-timeout:
			break loop
		}
	}

	if received != 10 {
		t.Errorf("Expected to receive 10 buffered events, got %d", received)
	}
}

// TestCloseBroker verifies broker cleanup.
func TestCloseBroker(t *testing.T) {
	broker := NewBroker()

	jobID := "test-job-123"
	ch, _, _ := broker.SubscribeJob(jobID)

	err := broker.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Channel should be closed
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("Channel should be closed after broker close")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Channel should be closed immediately")
	}
}

// TestSubscribeAfterClose verifies subscriptions work after close.
func TestSubscribeAfterClose(t *testing.T) {
	broker := NewBroker()

	broker.Close()

	// Subscribe after close should still work (creates new channels)
	ch, unsub, err := broker.SubscribeJob("test-job")
	if err != nil {
		t.Fatalf("Subscribe after close failed: %v", err)
	}
	defer unsub()

	event := &core.JobEvent{
		EventType: core.EventJobStateChanged,
		JobID:     "test-job",
		Queue:     "default",
		Timestamp: time.Now().Format(time.RFC3339),
	}

	broker.PublishJobEvent(event)

	select {
	case received := <-ch:
		if received.JobID != "test-job" {
			t.Errorf("Expected job ID test-job, got %s", received.JobID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout waiting for event after close/reopen")
	}
}

// TestEventTypes verifies different event types.
func TestEventTypes(t *testing.T) {
	broker := NewBroker()
	defer broker.Close()

	jobID := "test-job-123"
	ch, unsub, _ := broker.SubscribeJob(jobID)
	defer unsub()

	// State change event
	stateEvent := core.NewStateChangedEvent(jobID, "default", "test", "available", "active")
	broker.PublishJobEvent(stateEvent)

	select {
	case received := <-ch:
		if received.EventType != core.EventJobStateChanged {
			t.Errorf("Expected event type %s, got %s", core.EventJobStateChanged, received.EventType)
		}
		if received.From != "available" {
			t.Errorf("Expected from available, got %s", received.From)
		}
		if received.To != "active" {
			t.Errorf("Expected to active, got %s", received.To)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout waiting for state change event")
	}

	// Progress event
	progressEvent := &core.JobEvent{
		EventType: core.EventJobProgress,
		JobID:     jobID,
		Queue:     "default",
		Progress:  50,
		Message:   "Halfway done",
		Timestamp: time.Now().Format(time.RFC3339),
	}
	broker.PublishJobEvent(progressEvent)

	select {
	case received := <-ch:
		if received.EventType != core.EventJobProgress {
			t.Errorf("Expected event type %s, got %s", core.EventJobProgress, received.EventType)
		}
		if received.Progress != 50 {
			t.Errorf("Expected progress 50, got %d", received.Progress)
		}
		if received.Message != "Halfway done" {
			t.Errorf("Expected message 'Halfway done', got %s", received.Message)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout waiting for progress event")
	}
}
