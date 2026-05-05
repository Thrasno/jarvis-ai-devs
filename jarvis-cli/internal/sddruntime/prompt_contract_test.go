package sddruntime

import "testing"

func TestDefaultPromptContract_ComposesCanonicalRequiredSourcesInOrder(t *testing.T) {
	tests := []struct {
		name      string
		agent     string
		phase     string
		wantIDs   []string
		wantOrder []int
	}{
		{
			name:    "claude orchestrator composition",
			agent:   "claude",
			phase:   "orchestrator",
			wantIDs: []string{"layer1.behavior", "layer2.persona", "skill.sdd-orchestrator", "registry.compact-rules", "protocol.hive"},
			wantOrder: []int{1, 2, 3, 4, 5},
		},
		{
			name:    "opencode apply composition",
			agent:   "opencode",
			phase:   "sdd-apply",
			wantIDs: []string{"layer1.behavior", "layer2.persona", "skill.sdd-apply", "registry.compact-rules", "protocol.hive"},
			wantOrder: []int{1, 2, 3, 4, 5},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contract := DefaultPromptContract(tt.agent, tt.phase)
			sources, err := contract.OrderedRequiredSources()
			if err != nil {
				t.Fatalf("OrderedRequiredSources() error = %v", err)
			}

			if len(sources) != len(tt.wantIDs) {
				t.Fatalf("required source count mismatch: got %d want %d", len(sources), len(tt.wantIDs))
			}

			for i := range sources {
				if sources[i].ID != tt.wantIDs[i] {
					t.Fatalf("source id at index %d mismatch: got %q want %q", i, sources[i].ID, tt.wantIDs[i])
				}
				if sources[i].Order != tt.wantOrder[i] {
					t.Fatalf("source order at index %d mismatch: got %d want %d", i, sources[i].Order, tt.wantOrder[i])
				}
			}
		})
	}
}

func TestPromptContract_OrderedRequiredSourcesRejectsMissingRequiredSource(t *testing.T) {
	contract := PromptContract{
		Agent: "claude",
		Phase: "sdd-spec",
		Sources: []PromptSource{
			{ID: "layer1.behavior", Required: true, Order: 1},
			{ID: "layer2.persona", Required: true, Order: 2},
			{ID: "skill.sdd-spec", Required: false, Order: 3},
			{ID: "registry.compact-rules", Required: true, Order: 4},
			{ID: "protocol.hive", Required: true, Order: 5},
		},
	}

	_, err := contract.OrderedRequiredSources()
	if err == nil {
		t.Fatal("expected error for missing required source")
	}
}
