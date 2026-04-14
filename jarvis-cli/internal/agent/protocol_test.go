package agent

import (
	"strings"
	"testing"
)

func TestCleanupOldProtocol(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "no old protocol markers",
			content: "some content\nmore content",
			want:    "some content\nmore content",
		},
		{
			name: "single old protocol block",
			content: `before content
<!-- gentle-ai:engram-protocol -->
old protocol content
<!-- /gentle-ai:engram-protocol -->
after content`,
			want: `before content

after content`,
		},
		{
			name: "multiple old protocol blocks (duplicates)",
			content: `start
<!-- gentle-ai:engram-protocol -->
first block
<!-- /gentle-ai:engram-protocol -->
middle
<!-- gentle-ai:engram-protocol -->
second block
<!-- /gentle-ai:engram-protocol -->
end`,
			want: `start

middle

end`,
		},
		{
			name: "malformed: start marker without end",
			content: `content
<!-- gentle-ai:engram-protocol -->
orphaned content`,
			want: `content

orphaned content`,
		},
		{
			name:    "empty content",
			content: "",
			want:    "",
		},
		{
			name: "old protocol at start of file",
			content: `<!-- gentle-ai:engram-protocol -->
protocol
<!-- /gentle-ai:engram-protocol -->
rest of file`,
			want: `
rest of file`,
		},
		{
			name: "old protocol at end of file",
			content: `beginning
<!-- gentle-ai:engram-protocol -->
protocol
<!-- /gentle-ai:engram-protocol -->`,
			want: `beginning
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CleanupOldProtocol(tt.content)
			if got != tt.want {
				t.Errorf("CleanupOldProtocol() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestInjectProtocol(t *testing.T) {
	protocol := "## Hive Protocol\nThis is the protocol content."

	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "empty content - append at EOF",
			content: "",
			want: `<!-- jarvis:hive-protocol -->
## Hive Protocol
This is the protocol content.
<!-- /jarvis:hive-protocol -->
`,
		},
		{
			name:    "no markers - append at EOF",
			content: "existing content\n",
			want: `existing content
<!-- jarvis:hive-protocol -->
## Hive Protocol
This is the protocol content.
<!-- /jarvis:hive-protocol -->
`,
		},
		{
			name: "markers exist - replace content between them",
			content: `before
<!-- jarvis:hive-protocol -->
old protocol content
<!-- /jarvis:hive-protocol -->
after`,
			want: `before
<!-- jarvis:hive-protocol -->
## Hive Protocol
This is the protocol content.
<!-- /jarvis:hive-protocol -->
after`,
		},
		{
			name: "markers exist with different content - idempotent",
			content: `before
<!-- jarvis:hive-protocol -->
different old content
with multiple lines
<!-- /jarvis:hive-protocol -->
after`,
			want: `before
<!-- jarvis:hive-protocol -->
## Hive Protocol
This is the protocol content.
<!-- /jarvis:hive-protocol -->
after`,
		},
		{
			name:    "content without trailing newline",
			content: "existing content",
			want: `existing content
<!-- jarvis:hive-protocol -->
## Hive Protocol
This is the protocol content.
<!-- /jarvis:hive-protocol -->
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := InjectProtocol(tt.content, protocol)
			if got != tt.want {
				t.Errorf("InjectProtocol() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestInjectProtocol_Idempotent(t *testing.T) {
	protocol := "## Protocol Content\nLine 2"
	content := "Some existing content\n"

	// First injection
	result1 := InjectProtocol(content, protocol)

	// Second injection on same result
	result2 := InjectProtocol(result1, protocol)

	if result1 != result2 {
		t.Errorf("InjectProtocol is not idempotent.\nFirst: %q\nSecond: %q", result1, result2)
	}

	// Verify the protocol appears exactly once
	count := strings.Count(result2, HiveProtocolStart)
	if count != 1 {
		t.Errorf("Expected exactly 1 HiveProtocolStart marker, got %d", count)
	}
}

func TestCleanupAndInject_Integration(t *testing.T) {
	// This test simulates the full workflow: cleanup old protocol, then inject new one
	initial := `# AGENTS.md

## Rules
Some rules here

<!-- gentle-ai:engram-protocol -->
OLD Engram protocol content
<!-- /gentle-ai:engram-protocol -->

## Personality
Some personality content

<!-- gentle-ai:engram-protocol -->
Duplicate old protocol
<!-- /gentle-ai:engram-protocol -->

Footer content`

	protocol := "## Hive Memory Protocol\nNew protocol content"

	// Clean up old markers
	cleaned := CleanupOldProtocol(initial)

	// Verify old markers are gone
	if strings.Contains(cleaned, OldEngramStart) {
		t.Error("CleanupOldProtocol should remove all OldEngramStart markers")
	}
	if strings.Contains(cleaned, OldEngramEnd) {
		t.Error("CleanupOldProtocol should remove all OldEngramEnd markers")
	}

	// Inject new protocol
	result := InjectProtocol(cleaned, protocol)

	// Verify new markers are present
	if !strings.Contains(result, HiveProtocolStart) {
		t.Error("InjectProtocol should add HiveProtocolStart marker")
	}
	if !strings.Contains(result, HiveProtocolEnd) {
		t.Error("InjectProtocol should add HiveProtocolEnd marker")
	}

	// Verify protocol content is present
	if !strings.Contains(result, protocol) {
		t.Error("InjectProtocol should include protocol content")
	}

	// Verify original content is preserved
	if !strings.Contains(result, "Some rules here") {
		t.Error("Original content should be preserved")
	}
	if !strings.Contains(result, "Footer content") {
		t.Error("Footer content should be preserved")
	}
}

func TestGetHiveProtocol(t *testing.T) {
	protocol := getHiveProtocol()

	// Verify it's not empty
	if protocol == "" {
		t.Error("getHiveProtocol() should return non-empty content")
	}

	// Verify it contains expected headers (based on hive-protocol.md structure)
	if !strings.Contains(protocol, "Hive Persistent Memory") {
		t.Error("Protocol should contain 'Hive Persistent Memory' header")
	}
	if !strings.Contains(protocol, "PROACTIVE SAVE TRIGGERS") {
		t.Error("Protocol should contain 'PROACTIVE SAVE TRIGGERS' section")
	}
	if !strings.Contains(protocol, "mem_save") {
		t.Error("Protocol should mention mem_save tool")
	}
}
