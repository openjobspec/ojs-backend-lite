package core

import (
	"encoding/json"
	"strings"
)

// ResourceRequirements defines compute resources a job needs.
// These are extracted from job.meta.resources.
type ResourceRequirements struct {
	GPU       *GPURequirement `json:"gpu,omitempty"`
	CPUCores  int             `json:"cpu_cores,omitempty"`
	MemoryGB  float64         `json:"memory_gb,omitempty"`
	StorageGB float64         `json:"storage_gb,omitempty"`
}

// GPURequirement defines GPU-specific resource needs.
type GPURequirement struct {
	Count    int     `json:"count,omitempty"`
	Type     string  `json:"type,omitempty"`
	MemoryGB float64 `json:"memory_gb,omitempty"`
}

// AffinityRules defines scheduling affinity for resource-aware dispatch.
type AffinityRules struct {
	Required  []AffinityRule `json:"required,omitempty"`
	Preferred []AffinityRule `json:"preferred,omitempty"`
}

// AffinityRule is a single affinity constraint.
type AffinityRule struct {
	Key      string   `json:"key"`
	Operator string   `json:"operator"` // In, NotIn, Exists, DoesNotExist
	Values   []string `json:"values,omitempty"`
}

// WorkerCapabilities describes what resources a worker can provide.
// Capabilities are declared as key-value label pairs (e.g., "gpu:nvidia-a100").
type WorkerCapabilities struct {
	Labels map[string]string
}

// ParseCapabilities converts a slice of "key:value" strings into WorkerCapabilities.
func ParseCapabilities(labels []string) WorkerCapabilities {
	caps := WorkerCapabilities{Labels: make(map[string]string)}
	for _, label := range labels {
		parts := strings.SplitN(label, ":", 2)
		if len(parts) == 2 {
			caps.Labels[parts[0]] = parts[1]
		} else if len(parts) == 1 {
			caps.Labels[parts[0]] = ""
		}
	}
	return caps
}

// ExtractResourceRequirements parses resource requirements from a job's meta field.
// Returns nil if no resource requirements are present.
func ExtractResourceRequirements(meta json.RawMessage) *ResourceRequirements {
	if len(meta) == 0 {
		return nil
	}

	var parsed struct {
		Resources *ResourceRequirements `json:"resources"`
	}
	if err := json.Unmarshal(meta, &parsed); err != nil {
		return nil
	}
	return parsed.Resources
}

// ExtractAffinityRules parses affinity rules from a job's meta field.
// Returns nil if no affinity rules are present.
func ExtractAffinityRules(meta json.RawMessage) *AffinityRules {
	if len(meta) == 0 {
		return nil
	}

	var parsed struct {
		Affinity *AffinityRules `json:"affinity"`
	}
	if err := json.Unmarshal(meta, &parsed); err != nil {
		return nil
	}
	return parsed.Affinity
}

// MatchesCapabilities checks if a worker's capabilities satisfy a job's
// resource requirements and affinity rules. Jobs without resource requirements
// match any worker.
func MatchesCapabilities(job *Job, caps WorkerCapabilities) bool {
	// No capabilities declared — all jobs match
	if len(caps.Labels) == 0 {
		return true
	}

	resources := ExtractResourceRequirements(job.Meta)
	if resources != nil {
		if !matchResources(resources, caps) {
			return false
		}
	}

	affinity := ExtractAffinityRules(job.Meta)
	if affinity != nil {
		if !matchAffinity(affinity, caps) {
			return false
		}
	}

	return true
}

func matchResources(req *ResourceRequirements, caps WorkerCapabilities) bool {
	if req.GPU != nil && req.GPU.Count > 0 {
		// Check GPU type
		if req.GPU.Type != "" {
			if gpuType, ok := caps.Labels["gpu"]; !ok || gpuType != req.GPU.Type {
				return false
			}
		}
		// Check GPU availability (count label)
		if _, ok := caps.Labels["gpu"]; !ok {
			return false
		}
	}
	return true
}

func matchAffinity(rules *AffinityRules, caps WorkerCapabilities) bool {
	// Required rules are hard constraints
	for _, rule := range rules.Required {
		if !evaluateRule(rule, caps) {
			return false
		}
	}
	// Preferred rules are advisory (not enforced here)
	return true
}

func evaluateRule(rule AffinityRule, caps WorkerCapabilities) bool {
	val, exists := caps.Labels[rule.Key]

	switch rule.Operator {
	case "In":
		if !exists {
			return false
		}
		for _, v := range rule.Values {
			if val == v {
				return true
			}
		}
		return false
	case "NotIn":
		if !exists {
			return true
		}
		for _, v := range rule.Values {
			if val == v {
				return false
			}
		}
		return true
	case "Exists":
		return exists
	case "DoesNotExist":
		return !exists
	default:
		return true
	}
}
