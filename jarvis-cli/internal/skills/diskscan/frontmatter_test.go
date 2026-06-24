package diskscan

import (
	"testing"
)

func TestParseFrontmatter_AllFields(t *testing.T) {
	content := []byte(`---
name: my-skill
Trigger: When doing something
scope: core
---

# My Skill

Some content here.
`)
	result, warn := ParseFrontmatter(content, "my-skill/SKILL.md")
	if warn != nil {
		t.Fatalf("unexpected warning: %+v", warn)
	}
	if result.Name != "my-skill" {
		t.Errorf("Name = %q, want %q", result.Name, "my-skill")
	}
	if result.Trigger != "When doing something" {
		t.Errorf("Trigger = %q, want %q", result.Trigger, "When doing something")
	}
	if result.Scope != "core" {
		t.Errorf("Scope = %q, want %q", result.Scope, "core")
	}
}

func TestParseFrontmatter_MissingName(t *testing.T) {
	content := []byte(`---
Trigger: When doing something
scope: optional
---

# No Name Skill
`)
	result, warn := ParseFrontmatter(content, "no-name/SKILL.md")
	if warn == nil {
		t.Fatal("expected warning for missing name, got nil")
	}
	if warn.Code != "missing-name" {
		t.Errorf("warning Code = %q, want %q", warn.Code, "missing-name")
	}
	if warn.Path != "no-name/SKILL.md" {
		t.Errorf("warning Path = %q, want %q", warn.Path, "no-name/SKILL.md")
	}
	// Trigger and Scope still parsed even on warning
	if result.Trigger != "When doing something" {
		t.Errorf("Trigger = %q, want %q", result.Trigger, "When doing something")
	}
	if result.Scope != "optional" {
		t.Errorf("Scope = %q, want %q", result.Scope, "optional")
	}
}

func TestParseFrontmatter_MissingTrigger(t *testing.T) {
	content := []byte(`---
name: trigger-less
scope: optional
---

# Trigger-less Skill
`)
	result, warn := ParseFrontmatter(content, "trigger-less/SKILL.md")
	if warn == nil {
		t.Fatal("expected warning for missing trigger, got nil")
	}
	if warn.Code != "missing-trigger" {
		t.Errorf("warning Code = %q, want %q", warn.Code, "missing-trigger")
	}
	if result.Name != "trigger-less" {
		t.Errorf("Name = %q, want %q", result.Name, "trigger-less")
	}
}

func TestParseFrontmatter_MissingScope_NoWarning(t *testing.T) {
	content := []byte(`---
name: scope-less
Trigger: When something happens
---

# Scope-less Skill
`)
	result, warn := ParseFrontmatter(content, "scope-less/SKILL.md")
	if warn != nil {
		t.Fatalf("expected no warning for missing scope, got: %+v", warn)
	}
	if result.Name != "scope-less" {
		t.Errorf("Name = %q, want %q", result.Name, "scope-less")
	}
	if result.Trigger != "When something happens" {
		t.Errorf("Trigger = %q, want %q", result.Trigger, "When something happens")
	}
	if result.Scope != "" {
		t.Errorf("Scope = %q, want empty string", result.Scope)
	}
}

func TestParseFrontmatter_NoFrontmatter(t *testing.T) {
	content := []byte(`# Just a plain markdown file

No frontmatter here.
`)
	_, warn := ParseFrontmatter(content, "plain/SKILL.md")
	if warn == nil {
		t.Fatal("expected warning when no frontmatter block found, got nil")
	}
	// missing name takes precedence (no frontmatter means no name)
	if warn.Code != "missing-name" && warn.Code != "missing-trigger" {
		t.Errorf("unexpected warning code %q for no-frontmatter case", warn.Code)
	}
}

func TestParseFrontmatter_MissingBothNameAndTrigger(t *testing.T) {
	content := []byte(`---
scope: core
---
`)
	_, warn := ParseFrontmatter(content, "incomplete/SKILL.md")
	if warn == nil {
		t.Fatal("expected warning for missing name and trigger, got nil")
	}
	// missing-name takes precedence over missing-trigger
	if warn.Code != "missing-name" {
		t.Errorf("warning Code = %q, want %q", warn.Code, "missing-name")
	}
}

func TestParseFrontmatter_UTF8BOM(t *testing.T) {
	// UTF-8 BOM (\xEF\xBB\xBF) immediately before the opening --- delimiter.
	// ParseFrontmatter must strip it transparently and still extract all fields.
	bom := []byte{0xEF, 0xBB, 0xBF}
	body := []byte("---\nname: Foo\nTrigger: when X\n---\n\n# Foo\n")
	content := append(bom, body...)

	result, warn := ParseFrontmatter(content, "bom-skill/SKILL.md")
	if warn != nil {
		t.Fatalf("unexpected warning with BOM content: %+v", warn)
	}
	if result.Name != "Foo" {
		t.Errorf("Name = %q, want %q", result.Name, "Foo")
	}
	if result.Trigger != "when X" {
		t.Errorf("Trigger = %q, want %q", result.Trigger, "when X")
	}
}

func TestParseFrontmatter_CRLFLineEndings(t *testing.T) {
	// SKILL.md files authored on Windows/WSL use CRLF line endings.
	// ParseFrontmatter must extract clean values without trailing \r.
	content := []byte("---\r\nname: my-skill\r\nTrigger: When doing something\r\nscope: core\r\n---\r\n\r\n# My Skill\r\n")

	result, warn := ParseFrontmatter(content, "crlf-skill/SKILL.md")
	if warn != nil {
		t.Fatalf("unexpected warning with CRLF content: %+v", warn)
	}
	if result.Name != "my-skill" {
		t.Errorf("Name = %q, want %q (trailing \\r not stripped?)", result.Name, "my-skill")
	}
	if result.Trigger != "When doing something" {
		t.Errorf("Trigger = %q, want %q", result.Trigger, "When doing something")
	}
	if result.Scope != "core" {
		t.Errorf("Scope = %q, want %q", result.Scope, "core")
	}
}

// TestParseFrontmatter_FoldedDescriptionWithTrigger validates that a folded
// YAML scalar (description: >) with the trigger on a continuation line is
// correctly extracted. This matches the real go-testing / branch-pr skill format.
func TestParseFrontmatter_FoldedDescriptionWithTrigger(t *testing.T) {
	content := []byte(`---
name: go-testing
description: >
  Go testing patterns for Gentleman.Dots, including Bubbletea TUI testing.
  Trigger: When writing Go tests, using teatest, or adding test coverage.
license: Apache-2.0
---

# Go Testing
`)
	result, warn := ParseFrontmatter(content, "go-testing/SKILL.md")
	if warn != nil {
		t.Fatalf("unexpected warning for folded description with trigger: %+v", warn)
	}
	if result.Name != "go-testing" {
		t.Errorf("Name = %q, want %q", result.Name, "go-testing")
	}
	want := "When writing Go tests, using teatest, or adding test coverage."
	if result.Trigger != want {
		t.Errorf("Trigger = %q, want %q", result.Trigger, want)
	}
}

// TestParseFrontmatter_SingleLineDescriptionWithTrigger validates that a
// single-line quoted description containing "Trigger: ..." extracts the
// trigger text correctly. This is the case that fails with the current parser.
func TestParseFrontmatter_SingleLineDescriptionWithTrigger(t *testing.T) {
	content := []byte(`---
name: sdd-apply
description: "Implement SDD tasks from specs and design. Trigger: orchestrator launches apply for one or more change tasks."
---

# SDD Apply
`)
	result, warn := ParseFrontmatter(content, "sdd-apply/SKILL.md")
	if warn != nil {
		t.Fatalf("unexpected warning for single-line description with trigger: %+v", warn)
	}
	if result.Name != "sdd-apply" {
		t.Errorf("Name = %q, want %q", result.Name, "sdd-apply")
	}
	want := "orchestrator launches apply for one or more change tasks."
	if result.Trigger != want {
		t.Errorf("Trigger = %q, want %q", result.Trigger, want)
	}
}

// TestParseFrontmatter_FoldedDescriptionSummaryThenTrigger validates that when
// the description has a summary sentence before the Trigger: line, only the
// trigger portion is extracted, not the whole description.
func TestParseFrontmatter_FoldedDescriptionSummaryThenTrigger(t *testing.T) {
	content := []byte(`---
name: branch-pr
description: >
  Create Gentle AI pull requests with issue-first checks for Jarvis-packaged workflows.
  Trigger: When creating a pull request, opening a PR, or preparing changes for review.
license: Apache-2.0
---

# Branch PR
`)
	result, warn := ParseFrontmatter(content, "branch-pr/SKILL.md")
	if warn != nil {
		t.Fatalf("unexpected warning for description with summary + trigger: %+v", warn)
	}
	if result.Name != "branch-pr" {
		t.Errorf("Name = %q, want %q", result.Name, "branch-pr")
	}
	want := "When creating a pull request, opening a PR, or preparing changes for review."
	if result.Trigger != want {
		t.Errorf("Trigger = %q, want %q", result.Trigger, want)
	}
}

// TestParseFrontmatter_StandaloneKeyTakesPrecedenceOverDescription verifies
// back-compat: when both a standalone Trigger: key and description-embedded
// trigger exist, the standalone key wins.
func TestParseFrontmatter_StandaloneKeyTakesPrecedenceOverDescription(t *testing.T) {
	content := []byte(`---
name: explicit-trigger
description: "Some description. Trigger: trigger from description."
Trigger: explicit standalone trigger
---
`)
	result, warn := ParseFrontmatter(content, "explicit-trigger/SKILL.md")
	if warn != nil {
		t.Fatalf("unexpected warning: %+v", warn)
	}
	want := "explicit standalone trigger"
	if result.Trigger != want {
		t.Errorf("Trigger = %q, want %q (standalone key should win)", result.Trigger, want)
	}
}

// TestParseFrontmatter_DescriptionWithNoTrigger validates that a description
// field without any "Trigger:" text still produces a missing-trigger warning.
func TestParseFrontmatter_DescriptionWithNoTrigger(t *testing.T) {
	content := []byte(`---
name: no-trigger-in-desc
description: "Just a summary with no trigger text here."
---
`)
	_, warn := ParseFrontmatter(content, "no-trigger-in-desc/SKILL.md")
	if warn == nil {
		t.Fatal("expected missing-trigger warning when description has no Trigger: text")
	}
	if warn.Code != "missing-trigger" {
		t.Errorf("warning Code = %q, want %q", warn.Code, "missing-trigger")
	}
}

// TestParseFrontmatter_FoldedDescriptionCRLFBlankLine validates that a folded
// YAML scalar with a CRLF blank line (\r\n\r\n) between the summary sentence
// and the "Trigger:" text does NOT prematurely end the folded scalar. The blank
// CRLF line ("\r" after split-by-\n) must be treated as empty/blank, not as a
// non-indented line that terminates the description block.
//
// The exact failure scenario: the Trigger text is embedded INLINE in the
// description folded scalar on a single continuation line that starts with
// "Summary sentence. Trigger: value". There is no standalone top-level
// Trigger: key. When the CRLF blank line ("\r") prematurely sets inDesc=false,
// the continuation line "  Summary. Trigger: value\r" is never accumulated into
// descLines, so extractTriggerFromDescription is never called with that text,
// producing a missing-trigger warning.
func TestParseFrontmatter_FoldedDescriptionCRLFBlankLine(t *testing.T) {
	// Build with explicit CRLF line endings. The blank line between the two
	// description paragraphs becomes "\r" after split-by-\n. The trigger is
	// embedded inline on the second continuation line — no standalone top-level
	// Trigger: key exists. If inDesc is terminated by the \r blank line, the
	// second paragraph never reaches descLines and the trigger is lost.
	content := "---\r\nname: crlf-blank-skill\r\ndescription: >\r\n  Go testing patterns for the project.\r\n\r\n  Summary before trigger. Trigger: When writing Go tests or adding test coverage.\r\n---\r\n"

	result, warn := ParseFrontmatter([]byte(content), "crlf-blank-skill/SKILL.md")
	if warn != nil {
		t.Fatalf("unexpected warning: %+v (CRLF blank line must not drop embedded trigger)", warn)
	}
	if result.Name != "crlf-blank-skill" {
		t.Errorf("Name = %q, want %q", result.Name, "crlf-blank-skill")
	}
	want := "When writing Go tests or adding test coverage."
	if result.Trigger != want {
		t.Errorf("Trigger = %q, want %q (CRLF blank line terminated folded scalar prematurely)", result.Trigger, want)
	}
}

// TestExtractTriggerFromDescription_FirstWins verifies that when the description
// text contains two "Trigger:" occurrences, the FIRST one wins and its value is
// terminated at the first sentence boundary (period-space or period-end).
func TestExtractTriggerFromDescription_FirstWins(t *testing.T) {
	text := "Some summary. Trigger: first trigger value. Extra text. Trigger: second trigger value."
	got := extractTriggerFromDescription(text)
	// After FIX 3: trigger must stop at the first sentence boundary.
	want := "first trigger value."
	if got != want {
		t.Errorf("extractTriggerFromDescription = %q, want %q", got, want)
	}
}

// TestExtractTriggerFromDescription_SentenceBoundary verifies that
// extractTriggerFromDescription stops at the first sentence boundary (period
// followed by space or end-of-string) after "Trigger:".
func TestExtractTriggerFromDescription_SentenceBoundary(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{
			name: "clean sentence",
			text: "Skill summary. Trigger: When X, Y, or Z.",
			want: "When X, Y, or Z.",
		},
		{
			name: "trailing summary after period-space",
			text: "Trigger: improve skills. Audit and upgrade...",
			want: "improve skills.",
		},
		{
			name: "no trailing text",
			text: "Trigger: When doing something",
			want: "When doing something",
		},
		{
			name: "period at end of string",
			text: "Trigger: When doing something.",
			want: "When doing something.",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractTriggerFromDescription(tc.text)
			if got != tc.want {
				t.Errorf("extractTriggerFromDescription(%q) = %q, want %q", tc.text, got, tc.want)
			}
		})
	}
}

// TestParseFrontmatter_DisplayName verifies that a display_name: frontmatter key
// is parsed correctly and returned in FrontmatterResult.DisplayName.
func TestParseFrontmatter_DisplayName(t *testing.T) {
	content := []byte(`---
name: go-testing
display_name: Go Testing
description: >
  Go testing patterns.
  Trigger: When writing Go tests.
scope: optional
---

# Go Testing
`)
	result, warn := ParseFrontmatter(content, "go-testing/SKILL.md")
	if warn != nil {
		t.Fatalf("unexpected warning: %+v", warn)
	}
	if result.DisplayName != "Go Testing" {
		t.Errorf("DisplayName = %q, want %q", result.DisplayName, "Go Testing")
	}
	if result.Name != "go-testing" {
		t.Errorf("Name = %q, want %q (name: key unchanged)", result.Name, "go-testing")
	}
}

// TestParseFrontmatter_DisplayNameAbsent verifies that when display_name: is absent,
// DisplayName is empty (caller uses Name as fallback).
func TestParseFrontmatter_DisplayNameAbsent(t *testing.T) {
	content := []byte(`---
name: my-skill
Trigger: When doing something
scope: optional
---

# My Skill
`)
	result, warn := ParseFrontmatter(content, "my-skill/SKILL.md")
	if warn != nil {
		t.Fatalf("unexpected warning: %+v", warn)
	}
	if result.DisplayName != "" {
		t.Errorf("DisplayName = %q, want empty string when absent", result.DisplayName)
	}
}

// TestParseFrontmatter_RealSkillFrontmatters validates that the two real-world
// skill formats (go-testing folded, sdd-apply single-line) both yield
// non-empty triggers with no warnings.
func TestParseFrontmatter_RealSkillFrontmatters(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name: "go-testing (folded)",
			content: "---\nname: go-testing\ndescription: >\n  Go testing patterns for Gentleman.Dots, including Bubbletea TUI testing.\n  Trigger: When writing Go tests, using teatest, or adding test coverage.\nlicense: Apache-2.0\n---\n",
			want: "When writing Go tests, using teatest, or adding test coverage.",
		},
		{
			name:    "branch-pr (folded)",
			content: "---\nname: branch-pr\ndescription: >\n  Create Gentle AI pull requests with issue-first checks for Jarvis-packaged workflows.\n  Trigger: When creating a pull request, opening a PR, or preparing changes for review.\nlicense: Apache-2.0\n---\n",
			want:    "When creating a pull request, opening a PR, or preparing changes for review.",
		},
		{
			name:    "sdd-apply (single-line quoted)",
			content: "---\nname: sdd-apply\ndescription: \"Implement SDD tasks from specs and design. Trigger: orchestrator launches apply for one or more change tasks.\"\n---\n",
			want:    "orchestrator launches apply for one or more change tasks.",
		},
		{
			name:    "skill-registry (folded with long summary)",
			content: "---\nname: skill-registry\ndescription: >\n  Create or update the skill registry for the current project. Scans user skills and project conventions, writes .jarvis/skill-registry.md, and saves to Hive if available.\n  Trigger: When user says \"update skills\", \"skill registry\", \"actualizar skills\", \"update registry\", or after installing/removing skills.\n---\n",
			want:    "When user says \"update skills\", \"skill registry\", \"actualizar skills\", \"update registry\", or after installing/removing skills.",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, warn := ParseFrontmatter([]byte(tc.content), tc.name+"/SKILL.md")
			if warn != nil {
				t.Errorf("unexpected warning %+v for skill %q", warn, tc.name)
			}
			if result.Trigger != tc.want {
				t.Errorf("Trigger = %q, want %q", result.Trigger, tc.want)
			}
		})
	}
}
