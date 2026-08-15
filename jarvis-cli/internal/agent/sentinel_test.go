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

// PatchFile rebuilds both sentinel blocks, and the line ending it rebuilds them
// with has to be the one the file already uses.
//
// It used to hardcode "\n" at all four boundaries. On a checkout that converts
// the embedded template to CRLF, the renderer produced "\r\n" there and the
// patch produced "\n", so rendering and patching the same content disagreed by
// exactly four bytes. That is a file that differs from itself every time it is
// rewritten, which is why the Windows CI job -- and nothing on Linux -- caught
// it. The defect is about content, not about the platform, so this test builds
// the CRLF content directly and therefore runs everywhere.
//
// The same rule covers the second source of CRLF, which no checkout setting can
// rule out: a user's own instruction file saved by a Windows editor.
func TestPatchFile_PreservesTheLineEndingEachBlockAlreadyUses(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content string
		want    string
	}{
		{name: "LF file", content: testFullContent, want: "\n"},
		{name: "CRLF file", content: strings.ReplaceAll(testFullContent, "\n", "\r\n"), want: "\r\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			patched, err := PatchFile(tc.content, "new layer one", "new layer two")
			if err != nil {
				t.Fatalf("PatchFile: %v", err)
			}

			for _, boundary := range []string{
				Layer1Start + tc.want + "new layer one" + tc.want + Layer1End,
				Layer2Start + tc.want + "new layer two" + tc.want + Layer2End,
			} {
				if !strings.Contains(patched, boundary) {
					t.Errorf("patched block does not read %q", boundary)
				}
			}
			// The other half of the promise: a rebuilt block must not rewrite the
			// line endings of content it does not own. Counting the file's CRLFs
			// would not say this -- replacing a multi-line payload with a one-line
			// one legitimately removes some -- so the untouched regions are compared
			// directly, byte for byte.
			for _, region := range []struct{ name, want, got string }{
				{"content above the first sentinel", before(tc.content, Layer1Start), before(patched, Layer1Start)},
				{"content between the two blocks", between(tc.content, Layer1End, Layer2Start), between(patched, Layer1End, Layer2Start)},
				{"content below the last sentinel", after(tc.content, Layer2End), after(patched, Layer2End)},
			} {
				if region.got != region.want {
					t.Errorf("%s was rewritten:\n got %q\nwant %q", region.name, region.got, region.want)
				}
			}
		})
	}
}

// A boundary that carries no line ending at all is rebuilt with LF, the same
// way it always was. A hand-edited file is not a reason to fail, and guessing
// CRLF for a file that shows no evidence of it would be a fresh claim.
func TestPatchFile_FallsBackToLFWhenABoundaryShowsNoLineEnding(t *testing.T) {
	collapsed := Layer1Start + "old one" + Layer1End + "\n\n" + Layer2Start + "old two" + Layer2End

	patched, err := PatchFile(collapsed, "one", "two")
	if err != nil {
		t.Fatalf("PatchFile: %v", err)
	}

	if want := Layer1Start + "\none\n" + Layer1End; !strings.Contains(patched, want) {
		t.Errorf("patched content does not read %q:\n%s", want, patched)
	}
	if strings.Contains(patched, "\r") {
		t.Errorf("a file with no CRLF anywhere must not gain one:\n%s", patched)
	}
}

// Slicing helpers for the untouched-region comparison above. They return the
// empty string when a marker is absent, which the assertions treat as a
// difference rather than as a pass.
func before(content, marker string) string {
	index := strings.Index(content, marker)
	if index == -1 {
		return ""
	}
	return content[:index]
}

func after(content, marker string) string {
	index := strings.Index(content, marker)
	if index == -1 {
		return ""
	}
	return content[index+len(marker):]
}

func between(content, startMarker, endMarker string) string {
	return before(after(content, startMarker), endMarker)
}
