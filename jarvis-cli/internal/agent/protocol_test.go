package agent

import (
	"os"
	"path/filepath"
	"runtime"
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

func TestCleanupOldOrchestratorLink(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "no legacy line — no-op",
			content: "some content\nmore content",
			want:    "some content\nmore content",
		},
		{
			name:    "empty content — no-op",
			content: "",
			want:    "",
		},
		{
			name:    "line in middle of content",
			content: "before\nFor detailed SDD orchestration logic, see: [sdd-orchestrator.md](./sdd-orchestrator.md)\nafter",
			want:    "before\nafter",
		},
		{
			name:    "line at start of content",
			content: "For detailed SDD orchestration logic, see: [sdd-orchestrator.md](./sdd-orchestrator.md)\nafter",
			want:    "after",
		},
		{
			name:    "line at end of content",
			content: "before\nFor detailed SDD orchestration logic, see: [sdd-orchestrator.md](./sdd-orchestrator.md)",
			want:    "before",
		},
		{
			name:    "line surrounded by blank lines — collapses double blank",
			content: "before\n\nFor detailed SDD orchestration logic, see: [sdd-orchestrator.md](./sdd-orchestrator.md)\n\nafter",
			want:    "before\n\nafter",
		},
		{
			name:    "line with blank line before only",
			content: "before\n\nFor detailed SDD orchestration logic, see: [sdd-orchestrator.md](./sdd-orchestrator.md)\nafter",
			want:    "before\n\nafter",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CleanupOldOrchestratorLink(tt.content)
			if got != tt.want {
				t.Errorf("CleanupOldOrchestratorLink() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestInjectOrchestratorImport(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "empty content — appends block",
			content: "",
			want:    "<!-- jarvis:orchestrator-import -->\n@./sdd-orchestrator.md\n<!-- /jarvis:orchestrator-import -->\n",
		},
		{
			name:    "no markers — appends block at EOF",
			content: "existing content\n",
			want:    "existing content\n<!-- jarvis:orchestrator-import -->\n@./sdd-orchestrator.md\n<!-- /jarvis:orchestrator-import -->\n",
		},
		{
			name:    "content without trailing newline — appends with newline",
			content: "existing content",
			want:    "existing content\n<!-- jarvis:orchestrator-import -->\n@./sdd-orchestrator.md\n<!-- /jarvis:orchestrator-import -->\n",
		},
		{
			name:    "markers exist — replaces content between them",
			content: "before\n<!-- jarvis:orchestrator-import -->\nold content\n<!-- /jarvis:orchestrator-import -->\nafter",
			want:    "before\n<!-- jarvis:orchestrator-import -->\n@./sdd-orchestrator.md\n<!-- /jarvis:orchestrator-import -->\nafter",
		},
		{
			name:    "markers exist with same content — idempotent",
			content: "before\n<!-- jarvis:orchestrator-import -->\n@./sdd-orchestrator.md\n<!-- /jarvis:orchestrator-import -->\nafter",
			want:    "before\n<!-- jarvis:orchestrator-import -->\n@./sdd-orchestrator.md\n<!-- /jarvis:orchestrator-import -->\nafter",
		},
		{
			name:    "orphaned start marker — stripped before appending",
			content: "before\n<!-- jarvis:orchestrator-import -->\norphaned content\n",
			want:    "before\n\norphaned content\n<!-- jarvis:orchestrator-import -->\n@./sdd-orchestrator.md\n<!-- /jarvis:orchestrator-import -->\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := InjectOrchestratorImport(tt.content)
			if got != tt.want {
				t.Errorf("InjectOrchestratorImport() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestInjectOrchestratorImport_Idempotent(t *testing.T) {
	content := "Some existing content\n"

	// First injection
	result1 := InjectOrchestratorImport(content)

	// Second injection on same result
	result2 := InjectOrchestratorImport(result1)

	if result1 != result2 {
		t.Errorf("InjectOrchestratorImport is not idempotent.\nFirst:  %q\nSecond: %q", result1, result2)
	}

	// Verify markers appear exactly once
	if count := strings.Count(result2, OrchestratorImportStart); count != 1 {
		t.Errorf("expected exactly 1 %q marker, got %d", OrchestratorImportStart, count)
	}
	if count := strings.Count(result2, OrchestratorImportEnd); count != 1 {
		t.Errorf("expected exactly 1 %q marker, got %d", OrchestratorImportEnd, count)
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

func TestGetHiveProtocol_UsesRootCanonicalEmbeddedSource(t *testing.T) {
	protocol := getHiveProtocol()
	canonical := readAgentTestFile(t, "embed", "hive-protocol.md")

	if protocol != canonical {
		t.Fatal("getHiveProtocol() must load the canonical embed/hive-protocol.md source, not an internal duplicate")
	}
	if !strings.Contains(protocol, "## SDD ARTIFACT BOUNDARY (MVP CONTRACT)") {
		t.Fatal("canonical Hive protocol must preserve the SDD artifact boundary section")
	}
}

func readAgentTestFile(t *testing.T, parts ...string) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current test file")
	}

	moduleRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	path := filepath.Join(append([]string{moduleRoot}, parts...)...)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
