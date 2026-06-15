package hook

import (
	"strings"
	"testing"
)

func TestParsePayload_ValidJSON(t *testing.T) {
	input := `{
		"session_id": "abc123",
		"sessionId": "alt456",
		"transcript_path": "/tmp/transcripts/session.jsonl",
		"cwd": "/home/user/project",
		"prompt": "hello world",
		"stdout": "subagent output here",
		"project": "my-project",
		"directory": "/home/user/project"
	}`

	p, err := ParsePayload(strings.NewReader(input))
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if p.SessionID != "abc123" {
		t.Errorf("SessionID: got %q, want %q", p.SessionID, "abc123")
	}
	if p.SessionId != "alt456" {
		t.Errorf("SessionId: got %q, want %q", p.SessionId, "alt456")
	}
	if p.TranscriptPath != "/tmp/transcripts/session.jsonl" {
		t.Errorf("TranscriptPath: got %q", p.TranscriptPath)
	}
	if p.CWD != "/home/user/project" {
		t.Errorf("CWD: got %q", p.CWD)
	}
	if p.Prompt != "hello world" {
		t.Errorf("Prompt: got %q", p.Prompt)
	}
	if p.Stdout != "subagent output here" {
		t.Errorf("Stdout: got %q", p.Stdout)
	}
	if p.Project != "my-project" {
		t.Errorf("Project: got %q", p.Project)
	}
	if p.Directory != "/home/user/project" {
		t.Errorf("Directory: got %q", p.Directory)
	}
}

func TestParsePayload_InvalidJSON_ReturnsZeroValue(t *testing.T) {
	p, err := ParsePayload(strings.NewReader("{not valid json"))
	// invalid JSON: must return zero-value, no panic
	_ = err
	if p.SessionID != "" {
		t.Errorf("expected empty SessionID, got %q", p.SessionID)
	}
	if p.Prompt != "" {
		t.Errorf("expected empty Prompt, got %q", p.Prompt)
	}
}

func TestParsePayload_EmptyReader_ReturnsZeroValue(t *testing.T) {
	p, err := ParsePayload(strings.NewReader(""))
	_ = err
	if p.SessionID != "" {
		t.Errorf("expected empty SessionID, got %q", p.SessionID)
	}
}

func TestParsePayload_PartialJSON_ReturnsFields(t *testing.T) {
	p, err := ParsePayload(strings.NewReader(`{"prompt":"test prompt"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Prompt != "test prompt" {
		t.Errorf("Prompt: got %q, want %q", p.Prompt, "test prompt")
	}
	if p.SessionID != "" {
		t.Errorf("expected empty SessionID for partial payload")
	}
}
