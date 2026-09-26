package sddruntime

import (
	"reflect"
	"testing"
)

func displayLines(surfaces []ResetSurface) []string {
	lines := make([]string, 0, len(surfaces))
	for _, s := range surfaces {
		lines = append(lines, s.Display)
	}
	return lines
}

func TestResetInventory_ClaudeDisplayGolden(t *testing.T) {
	want := []string{
		"settings.json: outputStyle (only when it names a Jarvis-emitted persona style)",
		"settings.json: statusLine (only when it matches the Jarvis statusline command)",
		"settings.json: hook entries by managed command token (prompt-submit, skill-registry refresh, session-start, session-stop, session-compact, subagent-stop)",
		"settings.json: permission allow/deny entries by exact historical literal",
		"settings.json: permissions.defaultMode (only when equal to a Jarvis-issued value)",
		"CLAUDE.md: Jarvis instructions marker block content",
		"agents/: entire directory (SDD phase agents, Judgment Day agents, retired review-* agents, and any user-added agent files)",
		"sdd-orchestrator.md: whole file",
		"statusline-command.sh: whole file",
		"output-styles/: whole directory",
		"skills/: whole directory",
		"hive-hooks/: whole directory",
	}
	got := displayLines(ResetInventory(PlatformClaude))
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Claude reset inventory display =\n%v\nwant\n%v", got, want)
	}
}

func TestResetInventory_OpenCodeDisplayGolden(t *testing.T) {
	want := []string{
		"opencode.json: default_agent, permission, agent (entirely Jarvis-owned keys, removed whole)",
		"opencode.json: mcp.hive, mcp.context7 entries (other mcp servers preserved)",
		"AGENTS.md: Jarvis instructions marker block content",
		"sdd-orchestrator.md: whole file",
		"skills/: whole directory",
		"plugins/hive.ts: whole file",
		"plugins/skill-registry.ts: whole file",
	}
	got := displayLines(ResetInventory(PlatformOpenCode))
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("OpenCode reset inventory display =\n%v\nwant\n%v", got, want)
	}
}

func TestResetInventory_UnknownPlatformReturnsNil(t *testing.T) {
	if got := ResetInventory(Platform("unknown")); got != nil {
		t.Fatalf("ResetInventory(unknown) = %v, want nil", got)
	}
}

func TestResetInventory_SurfaceIDsAreUniqueAndStable(t *testing.T) {
	seen := map[string]bool{}
	for _, platform := range []Platform{PlatformClaude, PlatformOpenCode} {
		for _, surface := range ResetInventory(platform) {
			if surface.ID == "" {
				t.Fatalf("surface %+v has empty ID", surface)
			}
			if seen[surface.ID] {
				t.Fatalf("duplicate surface ID %q", surface.ID)
			}
			seen[surface.ID] = true
			if surface.Platform != platform {
				t.Fatalf("surface %q Platform = %q, want %q", surface.ID, surface.Platform, platform)
			}
		}
	}
}

func TestResetInventory_ClaudeAgentsAndOpenCodeCoreKeysAreReplacedEntirely(t *testing.T) {
	for _, id := range []string{"claude.agents.directory", "opencode.settings.core_keys"} {
		found := false
		for _, platform := range []Platform{PlatformClaude, PlatformOpenCode} {
			for _, surface := range ResetInventory(platform) {
				if surface.ID != id {
					continue
				}
				found = true
				if !surface.ReplacedEntirely {
					t.Fatalf("surface %q ReplacedEntirely = false, want true", id)
				}
			}
		}
		if !found {
			t.Fatalf("surface %q not found in any platform inventory", id)
		}
	}
}

func TestRetiredReviewAgentNames_ClaudeAndOpenCodeShareOneSource(t *testing.T) {
	want := []string{"review-risk", "review-readability", "review-reliability", "review-resilience"}
	if got := RetiredClaudeReviewAgentBaseNames(); !reflect.DeepEqual(got, want) {
		t.Fatalf("RetiredClaudeReviewAgentBaseNames() = %v, want %v", got, want)
	}
	if got := RetiredOpenCodeReviewAgentNames(); !reflect.DeepEqual(got, want) {
		t.Fatalf("RetiredOpenCodeReviewAgentNames() = %v, want %v", got, want)
	}
}

func TestClaudePermissionLiterals_ContainsCurrentAndObsoleteVariantsNotNearMisses(t *testing.T) {
	literals := claudePermissionLiterals()
	set := make(map[string]bool, len(literals))
	for _, l := range literals {
		set[l] = true
	}
	mustContain := []string{
		"Bash(git status:*)",
		"Bash(git diff:*)",
		"Bash(go test:*)",
		"Read(.env*)",
		"Bash(rm -rf /*)",
		"Bash(git push --force*)",
		// obsolete variants an older install may still carry
		"Bash(git push --force*:*)",
		"Bash(git push * --force-with-lease*:*)",
	}
	for _, l := range mustContain {
		if !set[l] {
			t.Errorf("claudePermissionLiterals() missing %q", l)
		}
	}
	nearMisses := []string{
		"Bash(git status)",
		"Bash(git status:*) ",
		"bash(git status:*)",
		"Bash(git push --force)",
	}
	for _, l := range nearMisses {
		if set[l] {
			t.Errorf("claudePermissionLiterals() unexpectedly contains near-miss %q", l)
		}
	}
}

func TestClaudeHookCommandTokens_ContainsAllSixManagedTokens(t *testing.T) {
	want := []string{
		" hook prompt-submit",
		" skill-registry refresh",
		" hook session-start",
		" hook session-stop",
		" hook session-compact",
		" hook subagent-stop",
	}
	if got := claudeHookCommandTokens(); !reflect.DeepEqual(got, want) {
		t.Fatalf("claudeHookCommandTokens() = %v, want %v", got, want)
	}
}

func TestClaudeDefaultModeJarvisValues_DoesNotIncludeNonJarvisValues(t *testing.T) {
	values := claudeDefaultModeJarvisValues()
	set := make(map[string]bool, len(values))
	for _, v := range values {
		set[v] = true
	}
	if !set["bypassPermissions"] {
		t.Fatalf("claudeDefaultModeJarvisValues() missing bypassPermissions")
	}
	for _, nonJarvis := range []string{"default", "acceptEdits", "plan"} {
		if set[nonJarvis] {
			t.Errorf("claudeDefaultModeJarvisValues() unexpectedly contains user value %q", nonJarvis)
		}
	}
}
