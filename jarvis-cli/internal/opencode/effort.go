package opencode

import "strings"

// EffortOptions returns provider-aware reasoning effort options for a model.
// The empty option means no explicit effort: keep the runtime/provider default.
func EffortOptions(providerID string, model Model) []string {
	if !model.Reasoning {
		return []string{""}
	}
	switch strings.ToLower(strings.TrimSpace(providerID)) {
	case "openai":
		return []string{"", "minimal", "low", "medium", "high", "xhigh"}
	case "anthropic":
		return []string{"", "high", "max"}
	default:
		return []string{""}
	}
}
