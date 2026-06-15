package hook

import (
	"encoding/json"
	"io"
)

// HookResponse is the JSON object written to stdout after hook processing.
// Both fields are optional; omitempty ensures empty fields are not emitted.
type HookResponse struct {
	AdditionalContext string `json:"additionalContext,omitempty"`
	SystemMessage     string `json:"systemMessage,omitempty"`
}

// WriteResponse marshals r to JSON and writes it to w.
// If both fields are empty, writes "{}".
// Never returns an error to the caller — any write failure is silently swallowed
// because a hook must always produce valid output on stdout.
func WriteResponse(w io.Writer, r HookResponse) {
	if r.AdditionalContext == "" && r.SystemMessage == "" {
		_, _ = io.WriteString(w, "{}")
		return
	}
	data, err := json.Marshal(r)
	if err != nil {
		_, _ = io.WriteString(w, "{}")
		return
	}
	_, _ = w.Write(data)
}

// WriteEmpty writes "{}" to w. Used when a hook has nothing to inject.
func WriteEmpty(w io.Writer) {
	_, _ = io.WriteString(w, "{}")
}
