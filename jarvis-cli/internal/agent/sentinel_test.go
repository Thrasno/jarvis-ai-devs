package agent

import (
	"strings"
	"testing"
)

const testFullContent = `# My custom notes above the sentinel — DO NOT TOUCH

<!-- JARVIS:LAYER1:START -->
## Hive Persistent Memory — Protocol

This is Layer 1 content. It is immutable by persona set.
<!-- JARVIS:LAYER1:END -->

<!-- JARVIS:LAYER2:START -->
## Persona: Argentino

This is the Layer 2 content — persona driven.
<!-- JARVIS:LAYER2:END -->

Some trailing user content here.
`

func TestExtractLayer2(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		wantErr  bool
		wantText string
	}{
		{
			name:     "extracts Layer2 content correctly",
			content:  testFullContent,
			wantText: "## Persona: Argentino\n\nThis is the Layer 2 content — persona driven.",
		},
		{
			name:    "missing Layer2 markers returns error",
			content: "no markers here",
			wantErr: true,
		},
		{
			name:    "reversed markers returns error",
			content: "<!-- JARVIS:LAYER2:END -->some<!-- JARVIS:LAYER2:START -->",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractLayer2(tt.content)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.wantText {
				t.Errorf("extracted text mismatch:\ngot:  %q\nwant: %q", got, tt.wantText)
			}
		})
	}
}

func TestPatchLayer2(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		newLayer2 string
		wantErr   bool
		checks    []func(t *testing.T, result string)
	}{
		{
			name:      "PatchLayer2 replaces only Layer2 block",
			content:   testFullContent,
			newLayer2: "## Persona: Tony Stark\n\nNew Layer 2 content.",
			checks: []func(t *testing.T, result string){
				func(t *testing.T, result string) {
					// Layer1 must be unchanged
					if !strings.Contains(result, "This is Layer 1 content. It is immutable by persona set.") {
						t.Error("Layer1 content was modified — must be preserved")
					}
				},
				func(t *testing.T, result string) {
					// New Layer2 must be present
					if !strings.Contains(result, "## Persona: Tony Stark") {
						t.Error("new Layer2 content not found")
					}
					if strings.Contains(result, "## Persona: Argentino") {
						t.Error("old Layer2 content still present after patch")
					}
				},
				func(t *testing.T, result string) {
					// User content outside sentinels must be preserved
					if !strings.Contains(result, "# My custom notes above the sentinel — DO NOT TOUCH") {
						t.Error("user content above sentinel was modified")
					}
					if !strings.Contains(result, "Some trailing user content here.") {
						t.Error("user content below sentinel was modified")
					}
				},
			},
		},
		{
			name:      "PatchLayer2 returns error if sentinels missing",
			content:   "no sentinel markers here",
			newLayer2: "new content",
			wantErr:   true,
		},
		{
			name: "PatchLayer2 returns error if Layer2 END before START",
			content: `<!-- JARVIS:LAYER1:START -->
layer1
<!-- JARVIS:LAYER1:END -->

<!-- JARVIS:LAYER2:END -->
<!-- JARVIS:LAYER2:START -->`,
			newLayer2: "content",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := PatchLayer2(tt.content, tt.newLayer2)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			for _, check := range tt.checks {
				check(t, result)
			}
		})
	}
}

func TestValidateSentinels(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr bool
	}{
		{
			name:    "valid sentinels",
			content: testFullContent,
		},
		{
			name:    "missing Layer1 START",
			content: "<!-- JARVIS:LAYER1:END -->\n<!-- JARVIS:LAYER2:START -->\n<!-- JARVIS:LAYER2:END -->",
			wantErr: true,
		},
		{
			name:    "reversed order Layer1 END before START",
			content: "<!-- JARVIS:LAYER1:END -->\n<!-- JARVIS:LAYER1:START -->\n<!-- JARVIS:LAYER2:START -->\n<!-- JARVIS:LAYER2:END -->",
			wantErr: true,
		},
		{
			name:    "Layer2 before Layer1",
			content: "<!-- JARVIS:LAYER2:START -->\n<!-- JARVIS:LAYER2:END -->\n<!-- JARVIS:LAYER1:START -->\n<!-- JARVIS:LAYER1:END -->",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSentinels(tt.content)
			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestBuildFullContent(t *testing.T) {
	content := BuildFullContent("# Preamble", "Layer1 content", "Layer2 content")

	if err := ValidateSentinels(content); err != nil {
		t.Fatalf("built content has invalid sentinels: %v", err)
	}

	if !strings.Contains(content, "# Preamble") {
		t.Error("preamble not found in output")
	}

	layer2, err := ExtractLayer2(content)
	if err != nil {
		t.Fatalf("ExtractLayer2 failed: %v", err)
	}
	if layer2 != "Layer2 content" {
		t.Errorf("expected 'Layer2 content', got %q", layer2)
	}
}
