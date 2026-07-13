package hook

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestWriteResponse_WithAdditionalContext(t *testing.T) {
	var buf bytes.Buffer
	r := HookResponse{AdditionalContext: "some context text"}
	WriteResponse(&buf, r)

	var got map[string]string
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON output: %v\noutput: %q", err, buf.String())
	}
	if got["additionalContext"] != "some context text" {
		t.Errorf("additionalContext: got %q, want %q", got["additionalContext"], "some context text")
	}
}

func TestWriteResponse_WithSystemMessage(t *testing.T) {
	var buf bytes.Buffer
	r := HookResponse{SystemMessage: "do this now"}
	WriteResponse(&buf, r)

	var got map[string]string
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	if got["systemMessage"] != "do this now" {
		t.Errorf("systemMessage: got %q, want %q", got["systemMessage"], "do this now")
	}
}

func TestWriteResponse_EmptyResponse_WritesEmptyObject(t *testing.T) {
	var buf bytes.Buffer
	r := HookResponse{}
	WriteResponse(&buf, r)

	out := buf.String()
	if out != "{}" {
		t.Errorf("expected {}, got %q", out)
	}
}

func TestWriteEmpty_WritesEmptyObject(t *testing.T) {
	var buf bytes.Buffer
	WriteEmpty(&buf)

	out := buf.String()
	if out != "{}" {
		t.Errorf("expected {}, got %q", out)
	}
}

func TestWriteResponse_OmitsEmptyFields(t *testing.T) {
	var buf bytes.Buffer
	r := HookResponse{AdditionalContext: "ctx"}
	WriteResponse(&buf, r)

	var got map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if _, ok := got["systemMessage"]; ok {
		t.Error("systemMessage should be omitted when empty")
	}
	if _, ok := got["hookSpecificOutput"]; ok {
		t.Error("hookSpecificOutput should be omitted when nil")
	}
}

// TestPromptSubmitMessage_EmitsNestedAndSystemMessage verifies the builder used
// for UserPromptSubmit delivers the message to BOTH the model (nested
// hookSpecificOutput.additionalContext) and the user (top-level systemMessage).
func TestPromptSubmitMessage_EmitsNestedAndSystemMessage(t *testing.T) {
	var buf bytes.Buffer
	msg := "call mem_save now"
	WriteResponse(&buf, PromptSubmitMessage(msg))

	var got struct {
		SystemMessage      string `json:"systemMessage"`
		HookSpecificOutput *struct {
			HookEventName     string `json:"hookEventName"`
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON output: %v\noutput: %q", err, buf.String())
	}
	if got.SystemMessage != msg {
		t.Errorf("systemMessage: got %q, want %q", got.SystemMessage, msg)
	}
	if got.HookSpecificOutput == nil {
		t.Fatalf("hookSpecificOutput missing; output: %q", buf.String())
	}
	if got.HookSpecificOutput.HookEventName != "UserPromptSubmit" {
		t.Errorf("hookEventName: got %q, want %q", got.HookSpecificOutput.HookEventName, "UserPromptSubmit")
	}
	if got.HookSpecificOutput.AdditionalContext != msg {
		t.Errorf("nested additionalContext: got %q, want %q", got.HookSpecificOutput.AdditionalContext, msg)
	}
}

// TestPromptSubmitMessage_NoTopLevelAdditionalContext ensures the builder does
// NOT emit a top-level additionalContext (that field is reserved for SessionStart).
func TestPromptSubmitMessage_NoTopLevelAdditionalContext(t *testing.T) {
	var buf bytes.Buffer
	WriteResponse(&buf, PromptSubmitMessage("x"))

	var got map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if _, ok := got["additionalContext"]; ok {
		t.Error("top-level additionalContext must NOT be present for prompt-submit builder")
	}
}
