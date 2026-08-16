package agent

import (
	"encoding/json"
	"strings"
)

// hookCommandMatcher reports whether a nested hooks[].command string identifies
// a managed entry that should be stripped.
type hookCommandMatcher func(command string) bool

// removeHookEntries strips every entry from the named hook event array whose
// nested "hooks" list contains a hook whose "command" satisfies match.
//
// This enables idempotent re-installation when prior entries cannot be
// deduplicated by MergeJSON's name-based identity check — either because a
// legacy entry lacks a "name" field, or because Claude Code normalized
// settings.json and stripped the non-native "name" key. Running jarvis init
// more than once would otherwise produce duplicate hook entries.
//
// Returns the original bytes unchanged when settings is empty, unparseable, or
// contains no matching entries.
func removeHookEntries(settings []byte, event string, match hookCommandMatcher) []byte {
	if len(strings.TrimSpace(string(settings))) == 0 {
		return settings
	}
	var root map[string]any
	if err := json.Unmarshal(settings, &root); err != nil {
		return settings
	}
	hooks, ok := root["hooks"].(map[string]any)
	if !ok {
		return settings
	}
	entries, ok := hooks[event].([]any)
	if !ok {
		return settings
	}

	filtered := make([]any, 0, len(entries))
	for _, entry := range entries {
		if !hookEntryMatches(entry, match) {
			filtered = append(filtered, entry)
		}
	}
	if len(filtered) == len(entries) {
		return settings
	}

	hooks[event] = filtered
	root["hooks"] = hooks
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return settings
	}
	return append(out, '\n')
}

// removeHookEntriesByCommand strips entries whose nested command equals the
// given string exactly. Retained for backward compatibility and exact-match
// callers/tests; delegates to removeHookEntries.
func removeHookEntriesByCommand(settings []byte, event, command string) []byte {
	return removeHookEntries(settings, event, func(c string) bool { return c == command })
}

// removeHookEntriesByCommandToken strips entries whose nested command CONTAINS
// the stable managed subcommand token (e.g. " hook subagent-stop") at a word
// boundary. Unlike exact-match, this survives binary-path drift across
// upgrades and a stripped "name" field, while a leading-space-anchored,
// right-anchored token avoids matching unrelated user hooks that invoke
// jarvis with a different (or merely longer) subcommand.
func removeHookEntriesByCommandToken(settings []byte, event, token string) []byte {
	return removeHookEntries(settings, event, func(c string) bool { return containsTokenAtWordBoundary(c, token) })
}

// containsTokenAtWordBoundary reports whether command contains token such
// that the character immediately following the match, if any, is a word
// boundary: a space, a shell quote ('\” or '"'), or end-of-string. This
// prevents a managed token like " hook subagent-stop" from over-matching a
// longer subcommand such as " hook subagent-stopwatch".
func containsTokenAtWordBoundary(command, token string) bool {
	for i := 0; i+len(token) <= len(command); i++ {
		idx := strings.Index(command[i:], token)
		if idx < 0 {
			return false
		}
		i += idx
		end := i + len(token)
		if end == len(command) {
			return true
		}
		switch command[end] {
		case ' ', '\'', '"':
			return true
		}
	}
	return false
}

// hookEntryMatches reports whether a hook group entry's nested "hooks" array
// contains a hook whose "command" field satisfies match.
func hookEntryMatches(entry any, match hookCommandMatcher) bool {
	entryMap, ok := entry.(map[string]any)
	if !ok {
		return false
	}
	hooksList, ok := entryMap["hooks"].([]any)
	if !ok {
		return false
	}
	for _, h := range hooksList {
		hMap, ok := h.(map[string]any)
		if !ok {
			continue
		}
		if command, ok := hMap["command"].(string); ok && match(command) {
			return true
		}
	}
	return false
}

// hookEntryContainsCommand reports whether a hook group entry's nested "hooks"
// array contains a hook whose "command" field equals the given string.
func hookEntryContainsCommand(entry any, command string) bool {
	return hookEntryMatches(entry, func(c string) bool { return c == command })
}
