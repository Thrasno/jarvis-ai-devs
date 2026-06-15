package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// executeHookSubcommand runs a hook subcommand with the given stdin content
// and returns the stdout output and any error.
func executeHookSubcommand(t *testing.T, subcommand string, stdinContent string) string {
	t.Helper()

	stdin := strings.NewReader(stdinContent)
	var stdout bytes.Buffer

	// Reset root command output and inject test IO.
	rootCmd.SetIn(stdin)
	rootCmd.SetOut(&stdout)

	rootCmd.SetArgs([]string{"hook", subcommand})
	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("hook %s: unexpected error: %v", subcommand, err)
	}

	return stdout.String()
}

// assertValidJSON verifies the output is non-empty, valid JSON, and returns the decoded map.
func assertValidJSON(t *testing.T, name, output string) map[string]any {
	t.Helper()
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		t.Fatalf("%s: stdout is empty, want JSON", name)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(trimmed), &m); err != nil {
		t.Fatalf("%s: stdout is not valid JSON: %v\noutput: %q", name, err, output)
	}
	return m
}

// TestHookCmd_SessionStart_OutputsAdditionalContext verifies that
// "jarvis hook session-start" reads stdin and writes JSON with additionalContext.
func TestHookCmd_SessionStart_OutputsAdditionalContext(t *testing.T) {
	payload := `{"session_id":"test-session-1"}`
	output := executeHookSubcommand(t, "session-start", payload)

	decoded := assertValidJSON(t, "session-start", output)
	if _, ok := decoded["additionalContext"]; !ok {
		t.Fatalf("session-start: expected 'additionalContext' key in output, got %v", decoded)
	}
}

// TestHookCmd_SessionCompact_OutputsAdditionalContext verifies that
// "jarvis hook session-compact" reads stdin and writes JSON with additionalContext.
func TestHookCmd_SessionCompact_OutputsAdditionalContext(t *testing.T) {
	payload := `{"session_id":"test-session-2"}`
	output := executeHookSubcommand(t, "session-compact", payload)

	decoded := assertValidJSON(t, "session-compact", output)
	if _, ok := decoded["additionalContext"]; !ok {
		t.Fatalf("session-compact: expected 'additionalContext' key in output, got %v", decoded)
	}
}

// TestHookCmd_PromptSubmit_OutputsValidJSON verifies that
// "jarvis hook prompt-submit" reads stdin and writes valid JSON.
func TestHookCmd_PromptSubmit_OutputsValidJSON(t *testing.T) {
	payload := `{"prompt":"hello","session_id":"test-session-3"}`
	output := executeHookSubcommand(t, "prompt-submit", payload)
	assertValidJSON(t, "prompt-submit", output)
}

// TestHookCmd_SubagentStop_OutputsValidJSON verifies that
// "jarvis hook subagent-stop" reads stdin and writes valid JSON.
func TestHookCmd_SubagentStop_OutputsValidJSON(t *testing.T) {
	payload := `{"stdout":"some output","session_id":"test-session-4"}`
	output := executeHookSubcommand(t, "subagent-stop", payload)
	assertValidJSON(t, "subagent-stop", output)
}

// TestHookCmd_SessionStop_OutputsValidJSON verifies that
// "jarvis hook session-stop" reads stdin and writes valid JSON.
func TestHookCmd_SessionStop_OutputsValidJSON(t *testing.T) {
	payload := `{"session_id":"test-session-5"}`
	output := executeHookSubcommand(t, "session-stop", payload)
	assertValidJSON(t, "session-stop", output)
}

// TestHookCmd_MalformedStdin_DoesNotCrash verifies that all hook subcommands
// handle malformed stdin gracefully and still emit valid JSON.
func TestHookCmd_MalformedStdin_DoesNotCrash(t *testing.T) {
	for _, sub := range []string{"session-start", "session-compact", "prompt-submit", "subagent-stop", "session-stop"} {
		t.Run(sub, func(t *testing.T) {
			output := executeHookSubcommand(t, sub, "not valid json {{{{")
			assertValidJSON(t, sub+" malformed stdin", output)
		})
	}
}

// TestHookCmd_IsHidden verifies the hook command is not surfaced in the root help.
func TestHookCmd_IsHidden(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "hook" {
			found = true
			if !cmd.Hidden {
				t.Error("hook command must be Hidden=true so it does not appear in normal help output")
			}
			break
		}
	}
	if !found {
		t.Error("hook command not registered in rootCmd")
	}
}
