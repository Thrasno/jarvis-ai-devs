package hook

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// envGetter returns a function that looks up keys from the provided map,
// falling back to an empty string. Used to inject controlled env values in tests.
func makeEnvGetter(env map[string]string) func(string) string {
	return func(key string) string {
		return env[key]
	}
}

func TestResolveSessionID_HiveCLaudeSessionID_TakesPriority(t *testing.T) {
	env := map[string]string{
		"HIVE_CLAUDE_SESSION_ID": "hive-session-1",
		"CLAUDE_SESSION_ID":      "claude-session-2",
		"SESSION_ID":             "session-3",
	}
	p := HookPayload{SessionID: "payload-session"}
	got := resolveSessionID(makeEnvGetter(env), p)
	if got != "hive-session-1" {
		t.Errorf("got %q, want %q", got, "hive-session-1")
	}
}

func TestResolveSessionID_ClaudeSessionID_SecondPriority(t *testing.T) {
	env := map[string]string{
		"HIVE_CLAUDE_SESSION_ID": "",
		"CLAUDE_SESSION_ID":      "claude-session-2",
		"SESSION_ID":             "session-3",
	}
	p := HookPayload{SessionID: "payload-session"}
	got := resolveSessionID(makeEnvGetter(env), p)
	if got != "claude-session-2" {
		t.Errorf("got %q, want %q", got, "claude-session-2")
	}
}

func TestResolveSessionID_SessionID_ThirdPriority(t *testing.T) {
	env := map[string]string{
		"SESSION_ID": "session-3",
	}
	p := HookPayload{SessionID: "payload-session"}
	got := resolveSessionID(makeEnvGetter(env), p)
	if got != "session-3" {
		t.Errorf("got %q, want %q", got, "session-3")
	}
}

func TestResolveSessionID_PayloadSessionID_FourthPriority(t *testing.T) {
	env := map[string]string{}
	p := HookPayload{SessionID: "from-payload"}
	got := resolveSessionID(makeEnvGetter(env), p)
	if got != "from-payload" {
		t.Errorf("got %q, want %q", got, "from-payload")
	}
}

func TestResolveSessionID_PayloadSessionId_FifthPriority(t *testing.T) {
	env := map[string]string{}
	p := HookPayload{SessionId: "from-payload-camel"}
	got := resolveSessionID(makeEnvGetter(env), p)
	if got != "from-payload-camel" {
		t.Errorf("got %q, want %q", got, "from-payload-camel")
	}
}

func TestResolveSessionID_TranscriptPath_SixthPriority(t *testing.T) {
	env := map[string]string{}
	p := HookPayload{TranscriptPath: "/tmp/sessions/my-session.jsonl"}
	got := resolveSessionID(makeEnvGetter(env), p)
	// basename of transcript_path
	if got != "my-session.jsonl" {
		t.Errorf("got %q, want %q", got, "my-session.jsonl")
	}
}

func TestResolveSessionID_Fallback_PPID(t *testing.T) {
	env := map[string]string{}
	p := HookPayload{}
	got := resolveSessionID(makeEnvGetter(env), p)
	expected := fmt.Sprintf("ppid-%d", os.Getppid())
	if got != expected {
		t.Errorf("got %q, want %q", got, expected)
	}
}

func TestResolveSessionID_NeverEmpty(t *testing.T) {
	env := map[string]string{}
	p := HookPayload{}
	got := resolveSessionID(makeEnvGetter(env), p)
	if got == "" {
		t.Error("resolveSessionID must never return empty string")
	}
}

func TestResolveSessionID_EnvTakesPrecedenceOverPayload(t *testing.T) {
	env := map[string]string{
		"CLAUDE_SESSION_ID": "env-wins",
	}
	p := HookPayload{SessionID: "payload-loses"}
	got := resolveSessionID(makeEnvGetter(env), p)
	if got != "env-wins" {
		t.Errorf("env should take precedence: got %q, want %q", got, "env-wins")
	}
}

func TestResolveSessionID_TranscriptPath_UsesBasename(t *testing.T) {
	env := map[string]string{}
	p := HookPayload{TranscriptPath: "/home/user/.claude/transcript-abc123.jsonl"}
	got := resolveSessionID(makeEnvGetter(env), p)
	if got != "transcript-abc123.jsonl" {
		t.Errorf("got %q, want basename %q", got, "transcript-abc123.jsonl")
	}
}

// ResolveSessionID (exported) uses os.Getenv — verify it works with real env.
func TestResolveSessionID_Exported_UsesOSEnv(t *testing.T) {
	t.Setenv("HIVE_CLAUDE_SESSION_ID", "os-env-session")
	p := HookPayload{}
	got := ResolveSessionID(p)
	if !strings.HasPrefix(got, "os-env-session") && got != "os-env-session" {
		t.Errorf("exported ResolveSessionID should use os.Getenv: got %q", got)
	}
}
