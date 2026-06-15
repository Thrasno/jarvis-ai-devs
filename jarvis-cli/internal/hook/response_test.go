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
}
