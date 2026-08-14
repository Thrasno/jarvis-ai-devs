package agent

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	jarvis "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/config"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
)

func TestInstallSkillsFromFS(t *testing.T) {
	testCases := []struct {
		name      string
		fsys      fs.FS
		selected  []string
		wantFiles map[string]string
		wantPaths []string
		absentDir string
		wantErr   string
	}{
		{
			name: "installs selected skills shared files and nested references",
			fsys: fstest.MapFS{
				"selected-skill/SKILL.md":                       {Data: []byte("# Skill")},
				"selected-skill/references/examples.md":         {Data: []byte("reference example")},
				"selected-skill/references/nested/deep-link.md": {Data: []byte("deep link")},
				"selected-skill/templates/snippet.txt":          {Data: []byte("snippet")},
				"other-skill/SKILL.md":                          {Data: []byte("# Other")},
				"_shared/hive-convention.md":                    {Data: []byte("shared convention")},
			},
			selected: []string{"selected-skill"},
			wantFiles: map[string]string{
				"selected-skill/SKILL.md":                       "# Skill",
				"selected-skill/references/examples.md":         "reference example",
				"selected-skill/references/nested/deep-link.md": "deep link",
				"selected-skill/templates/snippet.txt":          "snippet",
				"_shared/hive-convention.md":                    "shared convention",
			},
			wantPaths: []string{
				"_shared/hive-convention.md",
				"selected-skill/SKILL.md",
				"selected-skill/references/examples.md",
				"selected-skill/references/nested/deep-link.md",
				"selected-skill/templates/snippet.txt",
			},
			absentDir: "other-skill",
		},
		{
			name: "installs qa checklist and skill creator when selected",
			fsys: fstest.MapFS{
				"qa-checklist/SKILL.md":                          {Data: []byte("# QA Checklist")},
				"skill-creator/SKILL.md":                         {Data: []byte("# Skill Creator")},
				"skill-creator/references/quality-loop.md":       {Data: []byte("quality loop")},
				"unselected-skill/SKILL.md":                      {Data: []byte("# Other")},
				"unselected-skill/references/should-not-copy.md": {Data: []byte("skip")},
			},
			selected: []string{"qa-checklist", "skill-creator"},
			wantFiles: map[string]string{
				"qa-checklist/SKILL.md":                    "# QA Checklist",
				"skill-creator/SKILL.md":                   "# Skill Creator",
				"skill-creator/references/quality-loop.md": "quality loop",
			},
			wantPaths: []string{
				"qa-checklist/SKILL.md",
				"skill-creator/SKILL.md",
				"skill-creator/references/quality-loop.md",
			},
			absentDir: "unselected-skill",
		},
		{
			name:     "returns read errors with path context",
			fsys:     brokenReadFS{},
			selected: []string{"selected-skill"},
			wantErr:  "read skill file selected-skill/SKILL.md",
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			dest := t.TempDir()

			err := installSkillsFromFS(dest, tt.fsys, tt.selected)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatal("expected installSkillsFromFS to fail")
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error to include %q, got %q", tt.wantErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("installSkillsFromFS: %v", err)
			}

			assertInstalledFiles(t, dest, tt.wantFiles)
			assertInstalledRelativePaths(t, dest, tt.wantPaths)
			if tt.absentDir != "" {
				assertDirectoryAbsent(t, filepath.Join(dest, tt.absentDir))
			}
		})
	}
}

// TestInstallSkillsFromFS_Idempotent verifies that calling installSkillsFromFS twice
// produces no error and does not duplicate or append file contents.
func TestInstallSkillsFromFS_Idempotent(t *testing.T) {
	dest := t.TempDir()

	testFS := fstest.MapFS{
		"my-skill/SKILL.md": {Data: []byte("# My Skill")},
	}

	// First call.
	if err := installSkillsFromFS(dest, testFS, []string{"my-skill"}); err != nil {
		t.Fatalf("first installSkillsFromFS: %v", err)
	}

	// Second call (idempotency check).
	if err := installSkillsFromFS(dest, testFS, []string{"my-skill"}); err != nil {
		t.Fatalf("second installSkillsFromFS: %v", err)
	}

	// Content must be exactly what was written, not appended.
	assertFileContent(t, filepath.Join(dest, "my-skill", "SKILL.md"), "# My Skill")
}

func TestInstallSkillsFromFSWithModelSections_RemovesNonMatchingSections(t *testing.T) {
	dest := t.TempDir()
	skillsFS := fstest.MapFS{
		"sdd-verify/SKILL.md": {Data: []byte(strings.Join([]string{
			"Neutral intro",
			"<!-- section:model-capable -->",
			"Capable verification instructions",
			"<!-- /section:model-capable -->",
			"<!-- section:model-small -->",
			"Small verification instructions",
			"<!-- /section:model-small -->",
			"Neutral outro",
		}, "\n"))},
	}

	err := installSkillsFromFSWithModelSections(dest, skillsFS, []string{"sdd-verify"}, func(skillID string) sddruntime.ModelSectionClass {
		if skillID != "sdd-verify" {
			t.Fatalf("unexpected skillID %q", skillID)
		}
		return sddruntime.ModelSectionSmall
	})
	if err != nil {
		t.Fatalf("installSkillsFromFSWithModelSections: %v", err)
	}

	installed, err := os.ReadFile(filepath.Join(dest, "sdd-verify", "SKILL.md"))
	if err != nil {
		t.Fatalf("read installed skill: %v", err)
	}
	content := string(installed)
	for _, want := range []string{"Neutral intro", "Small verification instructions", "Neutral outro"} {
		if !strings.Contains(content, want) {
			t.Fatalf("installed skill missing %q:\n%s", want, content)
		}
	}
	for _, unwanted := range []string{"Capable verification instructions", "section:model"} {
		if strings.Contains(content, unwanted) {
			t.Fatalf("installed skill must remove %q:\n%s", unwanted, content)
		}
	}
}

func TestInstallSkillsFromEmbeddedSDDVerify_RendersModelSpecificSections(t *testing.T) {
	skillsFS, err := fs.Sub(jarvis.SkillsFS, "embed/skills")
	if err != nil {
		t.Fatalf("open embedded skills FS: %v", err)
	}

	minimumVerifierContract := []string{
		"## Activation Contract",
		"## Hard Rules",
		"## Status Handling and Blockers",
		"## Runtime Evidence Policy",
		"## Skipped Dimensions",
		"## Final Verdict Constraints",
		"## Output Contract",
		"Execute relevant tests; static analysis alone is never verification.",
		"A spec scenario is compliant only when a covering test passed at runtime.",
		"If runtime tests cannot be run, report runtime evidence as skipped and do not claim full PASS for behavior that was not executed.",
		"A documented manual verification path is not evidence by itself.",
		"Manual or runtime verification counts as `PASS` only when it was executed and the report records the command or manual action, result, timestamp or session, and operator/evidence source.",
		"Unresolved CRITICAL verification finding exists",
	}
	forbiddenVerifierDrift := []string{
		"Do NOT run tests unless `strict_tdd` is active and the test runner is explicitly provided.",
		"project explicitly documents an accepted manual verification path",
	}

	tests := []struct {
		name     string
		model    string
		want     []string
		unwanted []string
	}{
		{
			name:  "capable model keeps full verifier guidance",
			model: "sonnet",
			want: []string{
				"The orchestrator should provide structured status from `jarvis sdd status <change> --json`",
				"Any unchecked implementation task is CRITICAL and blocks archive readiness.",
				"Capable Model Execution Strategy",
			},
			unwanted: []string{"Return Minimal Report", "section:model"},
		},
		{
			name:  "small model keeps minimal verifier guidance",
			model: "haiku",
			want: []string{
				"You are a VERIFY sub-agent. Your job: check implemented changes match spec acceptance criteria.",
				"Keep the report concise, but preserve the neutral contract above",
				`"next": "ready-for-archive|sdd-apply|missing-evidence-required"`,
			},
			unwanted: []string{"Graceful Artifact Handling", "section:model"},
		},
		{
			name:  "unknown model keeps neutral-only rendered skill",
			model: "vendor/custom-model",
			want: []string{
				"Synced from https://raw.githubusercontent.com/Gentleman-Programming/gentle-ai/v1.40.2",
			},
			unwanted: []string{"Graceful Artifact Handling", "Return Minimal Report", "section:model"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dest := t.TempDir()
			cfg := &config.AppConfig{SDD: config.SDDConfig{PhaseModels: map[string]config.PhaseModelSelection{
				"sdd-verify": {OpenCode: tt.model},
			}}}
			if providerID, modelID, ok := strings.Cut(tt.model, "/"); ok {
				cfg.SDD.OpenCodePhaseModels = map[string]config.OpenCodeModelAssignment{
					"sdd-verify": {ProviderID: providerID, ModelID: modelID},
				}
			}
			sectionClass, err := skillModelSectionClassForPlatform(sddruntime.PlatformOpenCode, cfg.PhaseModelsForState())
			if err != nil {
				t.Fatalf("resolve model section class: %v", err)
			}

			if err := installSkillsFromFSWithModelSections(dest, skillsFS, []string{"sdd-verify"}, sectionClass); err != nil {
				t.Fatalf("installSkillsFromFSWithModelSections: %v", err)
			}

			installed, err := os.ReadFile(filepath.Join(dest, "sdd-verify", "SKILL.md"))
			if err != nil {
				t.Fatalf("read installed skill: %v", err)
			}
			content := string(installed)
			for _, want := range append(minimumVerifierContract, tt.want...) {
				if !strings.Contains(content, want) {
					t.Fatalf("installed sdd-verify missing %q for model %q:\n%s", want, tt.model, content)
				}
			}
			for _, unwanted := range append(forbiddenVerifierDrift, tt.unwanted...) {
				if strings.Contains(content, unwanted) {
					t.Fatalf("installed sdd-verify must not contain %q for model %q:\n%s", unwanted, tt.model, content)
				}
			}
		})
	}
}

func TestInstallSkillsFromEmbeddedSDDApply_PreservesJarvisStatusAndWorkspaceGuards(t *testing.T) {
	skillsFS, err := fs.Sub(jarvis.SkillsFS, "embed/skills")
	if err != nil {
		t.Fatalf("open embedded skills FS: %v", err)
	}

	dest := t.TempDir()
	cfg := &config.AppConfig{SDD: config.SDDConfig{PhaseModels: map[string]config.PhaseModelSelection{
		"sdd-apply": {OpenCode: "sonnet"},
	}}}
	sectionClass, err := skillModelSectionClassForPlatform(sddruntime.PlatformOpenCode, cfg.PhaseModelsForState())
	if err != nil {
		t.Fatalf("resolve model section class: %v", err)
	}

	if err := installSkillsFromFSWithModelSections(dest, skillsFS, []string{"sdd-apply"}, sectionClass); err != nil {
		t.Fatalf("installSkillsFromFSWithModelSections: %v", err)
	}

	installed, err := os.ReadFile(filepath.Join(dest, "sdd-apply", "SKILL.md"))
	if err != nil {
		t.Fatalf("read installed skill: %v", err)
	}
	content := string(installed)

	requiredSnippets := []string{
		"Synced from https://raw.githubusercontent.com/Gentleman-Programming/gentle-ai/v1.40.2",
		"jarvis sdd status <change> --json",
		"schema: `jarvis.sdd-status`",
		"actionContext",
		"actionContext.allowedEditRoots",
		"contextFiles",
		"artifactPaths",
		"allowedEditRoots",
		"workspace-planning",
		"blockedReasons",
		"applyState",
		"applyState.hasProgress",
		"applyState.complete",
		"phaseInstructions",
		"If `jarvis sdd status <change> --json` is unavailable, STOP before editing unless the maintainer explicitly approves manual recovery mode in the current conversation.",
		"Manual recovery mode does not make missing status safe by default; report missing status dimensions: blockers, dependencies, workspace-planning, artifact context, and allowed edit roots.",
		"If `actionContext.allowedEditRoots` is missing or empty, STOP before editing.",
		"If a needed edit is outside every `actionContext.allowedEditRoots` entry, STOP",
		"Generated artifacts are output, never sources of truth",
		"When prior `apply-progress = partial` exists, merge/reconcile it with current task state",
		"do not jump to `sdd-verify` until apply progress and task checkboxes agree.",
	}
	for _, want := range requiredSnippets {
		if !strings.Contains(content, want) {
			t.Fatalf("installed sdd-apply missing %q:\n%s", want, content)
		}
	}

	forbiddenSnippets := []string{
		"mcp__engram__",
		"Artifact store mode (`engram | openspec | hybrid | none`)",
		"~/.claude/skills",
		"~/.config/opencode/skills",
		"If `applyState` says apply is blocked",
		"If the command is unavailable, build the equivalent status from the artifacts before editing.",
		"If status is unavailable and no explicit `actionContext.allowedEditRoots` is available, STOP before editing.",
		"section:model",
	}
	for _, unwanted := range forbiddenSnippets {
		if strings.Contains(content, unwanted) {
			t.Fatalf("installed sdd-apply must not contain %q:\n%s", unwanted, content)
		}
	}
}

func TestInstallSkillsFromEmbeddedSDDArchive_PreservesJarvisArchiveSafetyGuards(t *testing.T) {
	skillsFS, err := fs.Sub(jarvis.SkillsFS, "embed/skills")
	if err != nil {
		t.Fatalf("open embedded skills FS: %v", err)
	}

	dest := t.TempDir()
	if err := installSkillsFromFS(dest, skillsFS, []string{"sdd-archive"}); err != nil {
		t.Fatalf("installSkillsFromFS: %v", err)
	}

	installed, err := os.ReadFile(filepath.Join(dest, "sdd-archive", "SKILL.md"))
	if err != nil {
		t.Fatalf("read installed skill: %v", err)
	}
	content := string(installed)

	requiredSnippets := []string{
		"Synced from https://raw.githubusercontent.com/Gentleman-Programming/gentle-ai/v1.40.2",
		"jarvis sdd status <change> --json",
		"schema: `jarvis.sdd-status`",
		"blockedReasons",
		"taskProgress",
		"applyState",
		"artifacts[\"verify-report\"]",
		"artifactPaths[\"verify-report\"]",
		"contextFiles[\"verify-report\"]",
		"verify-report artifact content",
		"actionContext",
		"phaseInstructions",
		"If verify-report evidence is missing, failing, stale, or does not cover the current artifacts, STOP",
		"Unresolved CRITICAL verification findings always block archive",
		"Any incomplete task checkbox or `taskProgress` entry blocks archive",
		"Stale checkboxes are not archive-ready by themselves",
		"When prior `apply-progress = partial` exists, STOP until current tasks, apply-progress, and verify-report have been reconciled and re-verified",
		"Partial, missing, or stale artifacts block archive until they are reconciled and re-verified",
		"For `none` mode, return a closure summary only; do not persist an archive report",
		"Generated artifacts are output, never sources of truth",
	}
	for _, want := range requiredSnippets {
		if !strings.Contains(content, want) {
			t.Fatalf("installed sdd-archive missing %q:\n%s", want, content)
		}
	}

	forbiddenSnippets := []string{
		"mcp__engram__",
		"Artifact store mode (`engram | openspec | hybrid | none`)",
		"Engram archive report",
		"verifyReport",
		"intentional archive override",
		"intentional-with-warnings",
		"non-critical partial artifact archive",
		"non-critical partial artifacts",
		"explicit approves a non-critical partial",
		"explicitly approves a non-critical partial",
		"Exceptional stale-checkbox reconciliation is allowed",
		"archive-time reconciliation",
		"~/.claude/skills",
		"~/.config/opencode/skills",
		"section:model",
	}
	for _, unwanted := range forbiddenSnippets {
		if strings.Contains(content, unwanted) {
			t.Fatalf("installed sdd-archive must not contain %q:\n%s", unwanted, content)
		}
	}
}

func TestInstallSkillsFromEmbeddedSharedPersistenceContract_PreservesJarvisHiveModeContract(t *testing.T) {
	skillsFS, err := fs.Sub(jarvis.SkillsFS, "embed/skills")
	if err != nil {
		t.Fatalf("open embedded skills FS: %v", err)
	}

	dest := t.TempDir()
	if err := installSkillsFromFS(dest, skillsFS, []string{"sdd-apply"}); err != nil {
		t.Fatalf("installSkillsFromFS: %v", err)
	}

	installed, err := os.ReadFile(filepath.Join(dest, "_shared", "persistence-contract.md"))
	if err != nil {
		t.Fatalf("read installed shared persistence contract: %v", err)
	}
	content := string(installed)

	requiredSnippets := []string{
		"Artifact store mode (`hive | openspec | hybrid | none`)",
		"## Mode Resolution",
		"## State Persistence Across Phases",
		"## Sub-Agent Response Ordering",
		"the final output MUST be text, never only a tool result",
		"## Skill Registry Handoff",
		"Use topic keys in the format `{domain}/{identifier}` or `{domain}/{change}/{phase}`",
		"Topic keys group related artifact saves; they are not artifact identity, recency, or version guarantees.",
		"If Hive search returns multiple artifacts for the same topic and no explicit recency/version metadata, treat the result as ambiguous.",
		"Ask the orchestrator/user or use a provided artifact reference before proceeding.",
		"Phase agents persist their own phase artifact according to the resolved mode.",
		"The orchestrator may pass state or artifact references to phase agents, but this contract does not require per-transition DAG-state persistence unless runtime status explicitly implements it.",
		"- Explore: `sdd/{change-name}/explore` or `openspec/changes/{change-name}/explore.md`",
		"Explore artifact uses `explore` for both the Hive topic key and the OpenSpec file path.",
		"Do not treat Jarvis product Hive, Hive API, or Hive ↔ Hive API synchronization as SDD artifact persistence.",
	}
	for _, want := range requiredSnippets {
		if !strings.Contains(content, want) {
			t.Fatalf("installed shared persistence contract missing %q:\n%s", want, content)
		}
	}

	forbiddenSnippets := []string{
		"Engram",
		"engram",
		"mcp__engram__",
		"`engram | openspec | hybrid | none`",
		"upsert",
		"overwrite",
		"latest returned observation",
		"latest-by-topic",
		"authoritative version",
		"`sdd/{change-name}/exploration`",
		"openspec/changes/{change-name}/exploration.md",
		"The orchestrator persists DAG state after each phase transition",
		"Both backends stay in sync",
	}
	for _, unwanted := range forbiddenSnippets {
		if strings.Contains(content, unwanted) {
			t.Fatalf("installed shared persistence contract must not contain %q:\n%s", unwanted, content)
		}
	}
}

func TestInstallSkillsFromEmbeddedSharedDocs_AvoidUnsupportedLatestGuarantees(t *testing.T) {
	skillsFS, err := fs.Sub(jarvis.SkillsFS, "embed/skills")
	if err != nil {
		t.Fatalf("open embedded skills FS: %v", err)
	}

	dest := t.TempDir()
	if err := installSkillsFromFS(dest, skillsFS, []string{"sdd-apply"}); err != nil {
		t.Fatalf("installSkillsFromFS: %v", err)
	}

	for _, relPath := range []string{
		filepath.Join("_shared", "sdd-phase-common.md"),
		filepath.Join("_shared", "hive-convention.md"),
	} {
		installed, err := os.ReadFile(filepath.Join(dest, relPath))
		if err != nil {
			t.Fatalf("read installed shared doc %s: %v", relPath, err)
		}
		content := string(installed)

		for _, forbidden := range []string{
			"mem_search returns the most recent",
			"retrieval surfaces the most recent",
			"most recent, which is the authoritative version",
			"authoritative version",
			"latest returned observation",
			"latest-by-topic",
		} {
			if strings.Contains(content, forbidden) {
				t.Fatalf("installed shared doc %s must not promise unsupported Hive latest/authoritative retrieval with %q:\n%s", relPath, forbidden, content)
			}
		}

		for _, required := range []string{
			"Search results are previews, not source material.",
			"If Hive search returns multiple candidate artifacts for the same topic and no explicit artifact reference is available, treat the result as ambiguous.",
		} {
			if !strings.Contains(content, required) {
				t.Fatalf("installed shared doc %s missing ambiguity-safe Hive retrieval wording %q:\n%s", relPath, required, content)
			}
		}
	}
}

func TestInstallSkillsFromEmbeddedSkillDocs_AvoidUnsupportedMostRecentRetrievalClaims(t *testing.T) {
	skillsFS, err := fs.Sub(jarvis.SkillsFS, "embed/skills")
	if err != nil {
		t.Fatalf("open embedded skills FS: %v", err)
	}

	dest := t.TempDir()
	if err := installSkillsFromFS(dest, skillsFS, []string{"sdd-apply", "hive"}); err != nil {
		t.Fatalf("installSkillsFromFS: %v", err)
	}

	for _, relPath := range []string{
		filepath.Join("sdd-apply", "SKILL.md"),
		filepath.Join("hive", "SKILL.md"),
	} {
		installed, err := os.ReadFile(filepath.Join(dest, relPath))
		if err != nil {
			t.Fatalf("read installed skill doc %s: %v", relPath, err)
		}
		content := string(installed)

		for _, forbidden := range []string{
			"use the most recent on retrieval",
			"retrieval uses the most recent",
			"retrieve the most recent version",
			"phases retrieve the most recent version",
			"most recent, which is the authoritative version",
			"authoritative version",
		} {
			if strings.Contains(content, forbidden) {
				t.Fatalf("installed skill doc %s must not promise unsupported most-recent retrieval with %q:\n%s", relPath, forbidden, content)
			}
		}

		for _, required := range []string{
			"multiple candidates",
			"explicit",
			"ambiguous",
		} {
			if !strings.Contains(content, required) {
				t.Fatalf("installed skill doc %s missing ambiguity-safe retrieval guidance %q:\n%s", relPath, required, content)
			}
		}
	}
}

type brokenReadFS struct{}

func (brokenReadFS) Open(name string) (fs.File, error) {
	switch name {
	case ".", "selected-skill":
		return fstest.MapFS{
			"selected-skill/SKILL.md": {},
		}.Open(name)
	case "selected-skill/SKILL.md":
		return nil, fmt.Errorf("boom reading %s", name)
	}

	return nil, fmt.Errorf("boom reading %s", name)
}

// assertFileContent reads the file at path and asserts its content equals expected.
func assertFileContent(t *testing.T, path, expected string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Errorf("expected file at %s, got error: %v", path, err)
		return
	}
	if string(data) != expected {
		t.Errorf("file %s content mismatch:\n  got:  %q\n  want: %q", path, string(data), expected)
	}
}

// assertFileExists checks that a file exists at path.
func assertFileExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected file at %s to exist, got: %v", path, err)
	}
}

func assertInstalledFiles(t *testing.T, dest string, want map[string]string) {
	t.Helper()
	for relPath, content := range want {
		assertFileContent(t, filepath.Join(dest, filepath.FromSlash(relPath)), content)
	}
}

func assertDirectoryAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected %s to be absent, got err=%v", path, err)
	}
}

func assertInstalledRelativePaths(t *testing.T, dest string, want []string) {
	t.Helper()

	var got []string
	err := filepath.WalkDir(dest, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(dest, path)
		if err != nil {
			return err
		}
		got = append(got, filepath.ToSlash(relPath))
		return nil
	})
	if err != nil {
		t.Fatalf("walk installed files: %v", err)
	}

	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("installed files mismatch\n got: %v\nwant: %v", got, want)
	}
}

// TestInstallOrchestrator_CreatesFile verifies that installOrchestrator creates
// the orchestrator file at the destination path with correct content.
func TestInstallOrchestrator_CreatesFile(t *testing.T) {
	dest := t.TempDir()
	destFile := filepath.Join(dest, "sdd-orchestrator.md")

	err := installOrchestrator(destFile, []byte("# SDD Orchestrator\nContent here"))
	if err != nil {
		t.Fatalf("installOrchestrator: %v", err)
	}

	assertFileContent(t, destFile, "# SDD Orchestrator\nContent here")
}

// TestInstallOrchestrator_ReturnsErrorOnMissingFile verifies that installOrchestrator
// returns an error when the orchestrator file is missing from the embedded FS.
func TestInstallOrchestrator_ReturnsErrorOnMissingFile(t *testing.T) {
	t.Skip("file-read behavior moved to caller; installOrchestrator now writes provided rendered content")
}

// TestInstallOrchestrator_Idempotent verifies that calling installOrchestrator twice
// produces no error and does not duplicate content.
func TestInstallOrchestrator_Idempotent(t *testing.T) {
	dest := t.TempDir()
	destFile := filepath.Join(dest, "sdd-orchestrator.md")

	// First call.
	if err := installOrchestrator(destFile, []byte("# Orchestrator")); err != nil {
		t.Fatalf("first installOrchestrator: %v", err)
	}

	// Second call (idempotency check).
	if err := installOrchestrator(destFile, []byte("# Orchestrator")); err != nil {
		t.Fatalf("second installOrchestrator: %v", err)
	}

	// Content must be exactly what was written, not appended.
	assertFileContent(t, destFile, "# Orchestrator")
}

func TestInstallOrchestrator_WritesRenderedContent(t *testing.T) {
	dest := t.TempDir()
	destFile := filepath.Join(dest, "sdd-orchestrator.md")

	rendered := []byte("| sdd-apply | opus |\n")
	if err := installOrchestrator(destFile, rendered); err != nil {
		t.Fatalf("installOrchestrator: %v", err)
	}

	assertFileContent(t, destFile, "| sdd-apply | opus |\n")
}
