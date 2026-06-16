package agent

import (
	"encoding/json"
	"strings"
)

// removeHookEntriesByCommand strips every entry from the named hook event array
// whose nested "hooks" list contains a hook with the given command string.
//
// This enables idempotent re-installation when legacy entries lack a "name"
// field and therefore cannot be deduplicated by MergeJSON's name-based identity
// check. Running jarvis init more than once with a legacy settings.json would
// otherwise produce duplicate hook entries.
//
// Returns the original bytes unchanged when settings is empty, unparseable, or
// contains no matching entries.
func removeHookEntriesByCommand(settings []byte, event, command string) []byte {
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
		if !hookEntryContainsCommand(entry, command) {
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

// hookEntryContainsCommand reports whether a hook group entry's nested "hooks"
// array contains a hook whose "command" field equals the given string.
func hookEntryContainsCommand(entry any, command string) bool {
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
		if hMap["command"] == command {
			return true
		}
	}
	return false
}
