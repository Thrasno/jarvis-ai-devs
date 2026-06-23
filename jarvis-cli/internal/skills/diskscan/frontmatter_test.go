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
