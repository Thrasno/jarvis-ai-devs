package opencode

import "testing"

func TestEffortOptions_AreProviderAware(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		model    Model
		want     []string
	}{
		{name: "non reasoning model", provider: "openai", model: Model{Reasoning: false}, want: []string{""}},
		{name: "openai reasoning model", provider: " openAI ", model: Model{Reasoning: true}, want: []string{"", "minimal", "low", "medium", "high", "xhigh"}},
		{name: "anthropic reasoning model", provider: "anthropic", model: Model{Reasoning: true}, want: []string{"", "high", "max"}},
		{name: "unknown reasoning provider", provider: "custom", model: Model{Reasoning: true}, want: []string{""}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EffortOptions(tt.provider, tt.model)
			if !sameStringSlice(got, tt.want) {
				t.Fatalf("EffortOptions() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func sameStringSlice(got []string, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
