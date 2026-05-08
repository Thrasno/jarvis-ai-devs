package sanitize_test

import (
	"strings"
	"testing"

	"github.com/Thrasno/jarvis-dev/hive-daemon/internal/sanitize"
)

// TestStrip covers all scanner cases (Phase 1 non-nested + Phase 2 nested/orphan).
func TestStrip(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantClean  string
		wantCount  int
	}{
		// --- Phase 1: non-nested cases ---
		{
			name:      "#1 empty string",
			input:     "",
			wantClean: "",
			wantCount: 0,
		},
		{
			name:      "#2 no tags",
			input:     "hello world",
			wantClean: "hello world",
			wantCount: 0,
		},
		{
			name:      "#3 bare tag",
			input:     "a <private>secret</private> b",
			wantClean: "a [REDACTED] b",
			wantCount: 1,
		},
		{
			name:      "#4 multiple adjacent bare tags",
			input:     "<private>one</private> and <private>two</private>",
			wantClean: "[REDACTED] and [REDACTED]",
			wantCount: 2,
		},
		{
			name:      "#5 labelled tag",
			input:     `<private label="todoist">tok</private>`,
			wantClean: "[REDACTED:todoist]",
			wantCount: 1,
		},
		{
			name:      "#6 label with spaces",
			input:     `<private label="my secret">x</private>`,
			wantClean: "[REDACTED:my-secret]",
			wantCount: 1,
		},
		{
			name:      "#7 label with invalid chars",
			input:     `<private label="key!@#">x</private>`,
			wantClean: "[REDACTED:key---]",
			wantCount: 1,
		},
		{
			name:      "#8 label truncated to 32 chars",
			input:     `<private label="` + strings.Repeat("a", 40) + `">x</private>`,
			wantClean: "[REDACTED:" + strings.Repeat("a", 32) + "]",
			wantCount: 1,
		},
		{
			name:      "#9 empty label attr -> bare marker",
			input:     `<private label="">x</private>`,
			wantClean: "[REDACTED]",
			wantCount: 1,
		},
		{
			name:      "#10 mixed bare + labelled adjacent",
			input:     `<private>a</private><private label="k">b</private>`,
			wantClean: "[REDACTED][REDACTED:k]",
			wantCount: 2,
		},
		{
			name:      "#11 multiline content",
			input:     "<private>line1\nline2\nline3</private>",
			wantClean: "[REDACTED]",
			wantCount: 1,
		},
		{
			name:      "#12 case-insensitive open tag",
			input:     "<PRIVATE>secret</PRIVATE>",
			wantClean: "[REDACTED]",
			wantCount: 1,
		},
		{
			name:      "#13 case-insensitive close tag",
			input:     "<private>secret</PRIVATE>",
			wantClean: "[REDACTED]",
			wantCount: 1,
		},
		{
			name:      "#14 mixed case both",
			input:     "<Private>secret</pRiVaTe>",
			wantClean: "[REDACTED]",
			wantCount: 1,
		},
		{
			name:      "#15 whitespace inside open tag",
			input:     `<private   label="x"   >y</private>`,
			wantClean: "[REDACTED:x]",
			wantCount: 1,
		},
		{
			name:      "#16 unicode content",
			input:     "<private>こんにちは世界</private>",
			wantClean: "[REDACTED]",
			wantCount: 1,
		},
		{
			name:      "#30 empty private block",
			input:     "<private></private>",
			wantClean: "",
			wantCount: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := sanitize.Strip(tc.input)
			if r.Clean != tc.wantClean {
				t.Errorf("Clean: got %q, want %q", r.Clean, tc.wantClean)
			}
			if r.Count != tc.wantCount {
				t.Errorf("Count: got %d, want %d", r.Count, tc.wantCount)
			}
		})
	}
}

// TestStripPhase2 covers nested blocks and orphan tag handling.
func TestStripPhase2(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantClean string
		wantCount int
	}{
		{
			name:      "#17 2-level nest no labels",
			input:     "<private>a <private>b</private> c</private>",
			wantClean: "[REDACTED]",
			wantCount: 1,
		},
		{
			name:      "#18 2-level nest outer+inner labelled -> outer label wins",
			input:     `<private label="outer">a<private label="inner">b</private>c</private>`,
			wantClean: "[REDACTED:outer]",
			wantCount: 1,
		},
		{
			name:      "#19 2-level nest outer no label inner labelled -> bare marker",
			input:     `<private>a<private label="inner">b</private>c</private>`,
			wantClean: "[REDACTED]",
			wantCount: 1,
		},
		{
			name:      "#20 2-level nest outer labelled inner no label -> outer label",
			input:     `<private label="outer">a<private>b</private>c</private>`,
			wantClean: "[REDACTED:outer]",
			wantCount: 1,
		},
		{
			name:      "#21 3-level nest",
			input:     "<private>a <private>b <private>c</private> d</private> e</private>",
			wantClean: "[REDACTED]",
			wantCount: 1,
		},
		{
			name:      "#22 nested + adjacent sibling -> count=2",
			input:     "<private>a <private>b</private> c</private> X <private>d</private>",
			wantClean: "[REDACTED] X [REDACTED]",
			wantCount: 2,
		},
		{
			name:      "#24 orphan open no close -> strip to EOF count=1",
			input:     "hi <private>no close",
			wantClean: "hi [REDACTED]",
			wantCount: 1,
		},
		{
			name:      "#25 orphan close no open -> literal text count=0",
			input:     "hello </private> world",
			wantClean: "hello </private> world",
			wantCount: 0,
		},
		{
			name:      "#26 mixed orphan close + valid block -> count=1",
			input:     "</private> then <private>secret</private>",
			wantClean: "</private> then [REDACTED]",
			wantCount: 1,
		},
		{
			name:      "#27 idempotency: marker not re-processed",
			input:     "a [REDACTED] b",
			wantClean: "a [REDACTED] b",
			wantCount: 0,
		},
		{
			name: "#28 label injection via </private> inside attr -> sanitized slug no leak",
			// label="</private>" -> sanitizeLabel lowercases, replaces '<', '/', '>' each with '-'
			// -> "--private-"; inner content "x" is NOT leaked.
			input:     `<private label="</private>">x</private>`,
			wantClean: "[REDACTED:--private-]",
			wantCount: 1,
		},
		{
			name:      "#29 tag-like substring without close angle -> orphan open",
			input:     "a <private no-angle",
			wantClean: "a [REDACTED]",
			wantCount: 1,
		},
		{
			// Regression for W-01: nested inner tag whose label contains '>'
			// inside quotes must not confuse depth tracking. Without quote-aware
			// scanning of the inner open tag, the outer block would close early.
			name:      "#31 nested inner with label containing > inside quotes",
			input:     `<private label="outer">a <private label="a>b">inner</private> tail</private>`,
			wantClean: "[REDACTED:outer]",
			wantCount: 1,
		},
		{
			name:      "#32 invalid UTF-8 byte before tag still strips",
			input:     "\x80<private>secret</private>",
			wantClean: "\x80[REDACTED]",
			wantCount: 1,
		},
		{
			name:      "#33 invalid UTF-8 byte 0xff before tag still strips",
			input:     "\xff<private>secret</private>",
			wantClean: "\xff[REDACTED]",
			wantCount: 1,
		},
		{
			name:      "#34 invalid UTF-8 in middle still strips both blocks",
			input:     "<private>a</private>\x80<private>b</private>",
			wantClean: "[REDACTED]\x80[REDACTED]",
			wantCount: 2,
		},
		{
			name:      "#35 invalid UTF-8 after orphan-open marker emitted",
			input:     "before \x80 <private>no close",
			wantClean: "before \x80 [REDACTED]",
			wantCount: 1,
		},
		{
			name:      "#36 <private at exact EOF treated as orphan open",
			input:     "leak the rest <private",
			wantClean: "leak the rest [REDACTED]",
			wantCount: 1,
		},
		{
			// Documents fail-closed behavior: unmatched quote in label causes
			// findOpenTagEnd to return -1, and the rest of the input is consumed
			// as an orphan open. Privacy is preserved (over-redact, not leak).
			name:      "#37 unmatched quote in label fails closed (consume rest)",
			input:     `prefix <private label="unclosed>secret content</private> tail`,
			wantClean: "prefix [REDACTED]",
			wantCount: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := sanitize.Strip(tc.input)
			if r.Clean != tc.wantClean {
				t.Errorf("Clean: got %q, want %q", r.Clean, tc.wantClean)
			}
			if r.Count != tc.wantCount {
				t.Errorf("Count: got %d, want %d", r.Count, tc.wantCount)
			}
		})
	}
}

// TestSanitizeLabel tests label normalization via Strip's observable output.
// We test sanitizeLabel indirectly through Strip since it's unexported.
func TestSanitizeLabel(t *testing.T) {
	tests := []struct {
		name      string
		label     string
		wantLabel string // the part after "REDACTED:" in the marker, or "" for bare
	}{
		{
			name:      "empty -> bare marker",
			label:     "",
			wantLabel: "",
		},
		{
			name:      "plain lowercase",
			label:     "todoist",
			wantLabel: "todoist",
		},
		{
			name:      "uppercase -> lowercased",
			label:     "MyKey",
			wantLabel: "mykey",
		},
		{
			name:      "spaces -> hyphens",
			label:     "my key",
			wantLabel: "my-key",
		},
		{
			name:      "invalid chars -> hyphens",
			label:     "a!@#b",
			wantLabel: "a---b",
		},
		{
			name:      "50-char input -> 32-char output",
			label:     strings.Repeat("a", 50),
			wantLabel: strings.Repeat("a", 32),
		},
		{
			name:      "all-invalid -> all-hyphen (acceptable)",
			label:     "!!!",
			wantLabel: "---",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			input := `<private label="` + tc.label + `">x</private>`
			r := sanitize.Strip(input)
			if r.Count != 1 {
				t.Fatalf("Count: got %d, want 1", r.Count)
			}
			var wantClean string
			if tc.wantLabel == "" {
				wantClean = "[REDACTED]"
			} else {
				wantClean = "[REDACTED:" + tc.wantLabel + "]"
			}
			if r.Clean != wantClean {
				t.Errorf("Clean: got %q, want %q", r.Clean, wantClean)
			}
		})
	}
}
