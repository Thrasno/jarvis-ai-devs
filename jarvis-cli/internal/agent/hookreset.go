package agent

// filterHookGroupNestedCommands is the configuration-reset counterpart to
// removeHookEntries in hookcleanup.go. The install path's removeHookEntries
// drops a whole hook group the moment ANY nested command matches, which is
// correct for idempotent re-installation because Jarvis only ever installs
// one nested command per group it owns. A reset must instead survive a group
// a user hand-edited to add an unrelated nested command alongside a
// Jarvis-managed one: it strips only the matching nested commands, preserves
// the group's "matcher" and every other field and every unrelated nested
// command, and drops the group only once its nested "hooks" list becomes
// empty. computeClaudeSettingsReset in reset.go is the single caller: it
// walks every hook event present (not a fixed list, since Jarvis manages
// hooks across several event names) and applies this filter per group.
//
// filterHookGroupNestedCommands removes, from one hook group's nested "hooks"
// list, every entry whose "command" matches any token at a word boundary. It
// returns the (possibly mutated) group and whether anything changed; a nil
// group return means the group's nested list became empty and must be
// dropped entirely.
func filterHookGroupNestedCommands(group orderedValue, tokens []string) (*orderedValue, bool) {
	if group.object == nil {
		return &group, false
	}
	nested, ok := group.object.get("hooks")
	if !ok || nested.array == nil {
		return &group, false
	}

	filtered := make([]orderedValue, 0, len(nested.array))
	changed := false
	for _, hook := range nested.array {
		if hookCommandMatchesAnyToken(hook, tokens) {
			changed = true
			continue
		}
		filtered = append(filtered, hook)
	}
	if !changed {
		return &group, false
	}
	if len(filtered) == 0 {
		return nil, true
	}
	group.object.set("hooks", orderedValue{array: filtered})
	return &group, true
}

func hookCommandMatchesAnyToken(hook orderedValue, tokens []string) bool {
	if hook.object == nil {
		return false
	}
	commandVal, ok := hook.object.get("command")
	if !ok {
		return false
	}
	command, ok := commandVal.scalar.(string)
	if !ok {
		return false
	}
	for _, token := range tokens {
		if containsTokenAtWordBoundary(command, token) {
			return true
		}
	}
	return false
}
