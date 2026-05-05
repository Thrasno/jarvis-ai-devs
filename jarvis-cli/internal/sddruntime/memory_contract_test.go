package sddruntime

import "testing"

func TestClassifyMemoryTopic_DistinguishesSDDArtifactsFromGeneralMemory(t *testing.T) {
	tests := []struct {
		name  string
		topic string
		want  MemoryTopicClass
	}{
		{name: "valid sdd artifact topic", topic: "sdd/jarvis-agent-parity-vs-gentle/spec", want: MemoryTopicSDDArtifact},
		{name: "general memory topic", topic: "project/setup-notes", want: MemoryTopicGeneral},
		{name: "non-sdd namespace", topic: "notes/sdd/jarvis/spec", want: MemoryTopicGeneral},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyMemoryTopic(tt.topic)
			if got != tt.want {
				t.Fatalf("ClassifyMemoryTopic(%q) = %q, want %q", tt.topic, got, tt.want)
			}
		})
	}
}

func TestIsSDDArtifactTopic_EnforcesBoundaryRules(t *testing.T) {
	tests := []struct {
		name  string
		topic string
		want  bool
	}{
		{name: "valid artifact", topic: "sdd/my-change/tasks", want: true},
		{name: "missing artifact segment", topic: "sdd/my-change", want: false},
		{name: "empty change segment", topic: "sdd//tasks", want: false},
		{name: "empty artifact segment", topic: "sdd/my-change/", want: false},
		{name: "nested extra segment", topic: "sdd/my-change/spec/v2", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsSDDArtifactTopic(tt.topic)
			if got != tt.want {
				t.Fatalf("IsSDDArtifactTopic(%q) = %v, want %v", tt.topic, got, tt.want)
			}
		})
	}
}
