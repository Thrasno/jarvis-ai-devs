package hook

import (
	"encoding/json"
	"io"
)

// hookEventUserPromptSubmit is the hookEventName Claude Code expects inside the
// nested hookSpecificOutput wrapper for the UserPromptSubmit event.
const hookEventUserPromptSubmit = "UserPromptSubmit"

// HookSpecificOutput is the nested object Claude Code reads for MODEL-visible
// context. For UserPromptSubmit, AdditionalContext here is seen by the model
// (unlike the top-level SystemMessage, which is shown ONLY to the user).
type HookSpecificOutput struct {
	HookEventName     string `json:"hookEventName,omitempty"`
	AdditionalContext string `json:"additionalContext,omitempty"`
}

// HookResponse is the JSON object written to stdout after hook processing.
// All fields are optional; omitempty ensures empty fields are not emitted.
//
// Field visibility (per Claude Code hooks docs):
//   - AdditionalContext (top-level): model-visible; used by SessionStart.
//   - SystemMessage: shown ONLY to the user.
//   - HookSpecificOutput.AdditionalContext: model-visible; the supported way to
//     deliver model context for UserPromptSubmit.
type HookResponse struct {
	AdditionalContext  string              `json:"additionalContext,omitempty"`
	SystemMessage      string              `json:"systemMessage,omitempty"`
	HookSpecificOutput *HookSpecificOutput `json:"hookSpecificOutput,omitempty"`
}

// PromptSubmitMessage builds a UserPromptSubmit hook response that delivers msg
// to BOTH the model (nested hookSpecificOutput.additionalContext) and the user
// (top-level systemMessage). It intentionally does NOT set the top-level
// additionalContext — that field is reserved for SessionStart.
func PromptSubmitMessage(msg string) HookResponse {
	return HookResponse{
		SystemMessage: msg,
		HookSpecificOutput: &HookSpecificOutput{
			HookEventName:     hookEventUserPromptSubmit,
			AdditionalContext: msg,
		},
	}
}

// WriteResponse marshals r to JSON and writes it to w.
// If every field is empty, writes "{}".
// Never returns an error to the caller — any write failure is silently swallowed
// because a hook must always produce valid output on stdout.
func WriteResponse(w io.Writer, r HookResponse) {
	if r.AdditionalContext == "" && r.SystemMessage == "" && r.HookSpecificOutput == nil {
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
