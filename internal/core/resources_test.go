package core

import (
	"encoding/json"
	"testing"
)

func TestExtractResourceRequirements(t *testing.T) {
	tests := []struct {
		name     string
		meta     json.RawMessage
		wantNil  bool
		wantGPU  bool
	}{
		{"nil meta", nil, true, false},
		{"empty meta", json.RawMessage(`{}`), true, false},
		{"invalid json", json.RawMessage(`{bad`), true, false},
		{"with GPU", json.RawMessage(`{"resources":{"gpu":{"count":1,"type":"nvidia-a100"}}}`), false, true},
		{"no resources key", json.RawMessage(`{"other":"data"}`), true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractResourceRequirements(tt.meta)
			if tt.wantNil && result != nil {
				t.Errorf("expected nil, got %+v", result)
			}
			if !tt.wantNil && result == nil {
				t.Error("expected non-nil result")
			}
			if tt.wantGPU && result != nil && result.GPU == nil {
				t.Error("expected GPU requirement")
			}
		})
	}
}

func TestExtractAffinityRules(t *testing.T) {
	tests := []struct {
		name    string
		meta    json.RawMessage
		wantNil bool
	}{
		{"nil meta", nil, true},
		{"empty meta", json.RawMessage(`{}`), true},
		{"with affinity", json.RawMessage(`{"affinity":{"required":[{"key":"gpu","operator":"Exists"}]}}`), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractAffinityRules(tt.meta)
			if tt.wantNil && result != nil {
				t.Errorf("expected nil, got %+v", result)
			}
			if !tt.wantNil && result == nil {
				t.Error("expected non-nil result")
			}
		})
	}
}

func TestParseCapabilities(t *testing.T) {
	caps := ParseCapabilities([]string{"gpu:nvidia-a100", "region:us-east", "ssd"})

	if caps.Labels["gpu"] != "nvidia-a100" {
		t.Errorf("expected gpu=nvidia-a100, got %s", caps.Labels["gpu"])
	}
	if caps.Labels["region"] != "us-east" {
		t.Errorf("expected region=us-east, got %s", caps.Labels["region"])
	}
	if _, ok := caps.Labels["ssd"]; !ok {
		t.Error("expected ssd label to exist")
	}
}

func TestMatchesCapabilities(t *testing.T) {
	tests := []struct {
		name   string
		meta   json.RawMessage
		caps   []string
		expect bool
	}{
		{"no caps matches all", json.RawMessage(`{"resources":{"gpu":{"count":1}}}`), nil, true},
		{"gpu required with gpu cap", json.RawMessage(`{"resources":{"gpu":{"count":1,"type":"nvidia-a100"}}}`), []string{"gpu:nvidia-a100"}, true},
		{"gpu required wrong type", json.RawMessage(`{"resources":{"gpu":{"count":1,"type":"nvidia-a100"}}}`), []string{"gpu:nvidia-v100"}, false},
		{"gpu required no cap", json.RawMessage(`{"resources":{"gpu":{"count":1}}}`), []string{"region:us"}, false},
		{"affinity In match", json.RawMessage(`{"affinity":{"required":[{"key":"region","operator":"In","values":["us","eu"]}]}}`), []string{"region:us"}, true},
		{"affinity In no match", json.RawMessage(`{"affinity":{"required":[{"key":"region","operator":"In","values":["us","eu"]}]}}`), []string{"region:ap"}, false},
		{"affinity Exists match", json.RawMessage(`{"affinity":{"required":[{"key":"gpu","operator":"Exists"}]}}`), []string{"gpu:any"}, true},
		{"affinity DoesNotExist match", json.RawMessage(`{"affinity":{"required":[{"key":"gpu","operator":"DoesNotExist"}]}}`), []string{"region:us"}, true},
		{"affinity NotIn match", json.RawMessage(`{"affinity":{"required":[{"key":"region","operator":"NotIn","values":["ap"]}]}}`), []string{"region:us"}, true},
		{"no meta matches", nil, []string{"gpu:nvidia"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := &Job{Meta: tt.meta}
			caps := ParseCapabilities(tt.caps)
			result := MatchesCapabilities(job, caps)
			if result != tt.expect {
				t.Errorf("expected %v, got %v", tt.expect, result)
			}
		})
	}
}

func TestNewStateChangedEvent(t *testing.T) {
	event := NewStateChangedEvent("job-1", "default", "test.type", "available", "active")

	if event.EventType != EventJobStateChanged {
		t.Errorf("expected event type %s, got %s", EventJobStateChanged, event.EventType)
	}
	if event.JobID != "job-1" {
		t.Errorf("expected job ID job-1, got %s", event.JobID)
	}
	if event.Queue != "default" {
		t.Errorf("expected queue default, got %s", event.Queue)
	}
	if event.From != "available" {
		t.Errorf("expected from available, got %s", event.From)
	}
	if event.To != "active" {
		t.Errorf("expected to active, got %s", event.To)
	}
	if event.Timestamp == "" {
		t.Error("expected non-empty timestamp")
	}
}
