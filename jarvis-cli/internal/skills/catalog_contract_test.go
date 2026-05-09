package skills

import (
	"fmt"
	"io/fs"
	"os"
	"strings"
	"testing"

	jarvis "github.com/Thrasno/jarvis-dev/jarvis-cli"
)

const sharedPhaseCommonPath = "embed/skills/_shared/sdd-phase-common.md"

func TestCatalogContract_SharedSDDPhaseCommonMatchesJarvisAdaptedUpstreamShape(t *testing.T) {
	content := readEmbeddedSkillAsset(t, sharedPhaseCommonPath)

	if got := strings.Count(content, "\n") + 1; got < 100 {
		t.Fatalf("expected upstream-sized shared protocol (>=100 lines), got %d", got)
	}

	requiredSnippets := []string{
		"# SDD Phase — Common Protocol",
		"Executor boundary: every SDD phase agent is an EXECUTOR, not an orchestrator.",
		"## A. Skill Loading",
		"## B. Artifact Retrieval (Hive Mode)",
		"mcp__hive__mem_search",
		"mcp__hive__mem_get_observation",
		"## C. Artifact Persistence",
		"### Hive mode",
		"capture_prompt: false",
		"## D. Return Envelope",
		"status`: `success`, `partial`, or `blocked`",
		"## E. Review Workload Guard",
		"Feature Branch Chain",
	}

	for _, snippet := range requiredSnippets {
		if !strings.Contains(content, snippet) {
			t.Fatalf("expected %s to contain %q", sharedPhaseCommonPath, snippet)
		}
	}
}

func TestCatalogContract_SharedSDDPhaseCommonCarriesVerifiedSourceAndNoLegacyEngramDrift(t *testing.T) {
	content := readEmbeddedSkillAsset(t, sharedPhaseCommonPath)

	if !strings.Contains(content, "Gentleman-Programming/gentle-ai/v1.26.5/internal/assets/skills/_shared/sdd-phase-common.md") {
		t.Fatalf("expected %s to record the verified upstream source", sharedPhaseCommonPath)
	}
	if !strings.Contains(content, "5f73974b39ae2b9b525ef465b3642030c5f2ce6c") {
		t.Fatalf("expected %s to record the verified upstream commit", sharedPhaseCommonPath)
	}

	forbiddenSnippets := []string{
		"Artifact Retrieval (Engram Mode)",
		"mcp__engram__",
		"engram-convention.md",
		"`engram | openspec | hybrid | none`",
	}

	for _, snippet := range forbiddenSnippets {
		if strings.Contains(content, snippet) {
			t.Fatalf("expected %s not to contain legacy drift %q", sharedPhaseCommonPath, snippet)
		}
	}

	if !strings.Contains(content, "`hive | openspec | hybrid | none`") {
		t.Fatalf("expected %s to document Jarvis runtime store modes", sharedPhaseCommonPath)
	}
}

func TestCatalogContract_SDDCoreSkillsMatchJarvisAdaptedUpstreamContract(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		path      string
		required  []string
		forbidden []string
	}{
		{
			name: "sdd-init",
			path: "embed/skills/sdd-init/SKILL.md",
			required: []string{
				"disable-model-invocation: true",
				"user-invocable: false",
				"Artifact store modes supported by Jarvis skills: `hive | openspec | hybrid | none`.",
				"`../_shared/hive-convention.md`",
				"capture_prompt: false",
			},
		},
		{
			name: "sdd-explore",
			path: "embed/skills/sdd-explore/SKILL.md",
			required: []string{
				"disable-model-invocation: true",
				"user-invocable: false",
				"Execution and Persistence Contract",
				"Return envelope per **Section D** from `skills/_shared/sdd-phase-common.md`.",
			},
		},
		{
			name: "sdd-propose",
			path: "embed/skills/sdd-propose/SKILL.md",
			required: []string{
				"disable-model-invocation: true",
				"user-invocable: false",
				"## Capabilities",
				"Return envelope per **Section D** from `skills/_shared/sdd-phase-common.md`.",
			},
		},
		{
			name: "sdd-spec",
			path: "embed/skills/sdd-spec/SKILL.md",
			required: []string{
				"disable-model-invocation: true",
				"user-invocable: false",
				"MODIFIED requirements MUST be the FULL block",
				"Return envelope per **Section D** from `skills/_shared/sdd-phase-common.md`.",
			},
		},
		{
			name: "sdd-design",
			path: "embed/skills/sdd-design/SKILL.md",
			required: []string{
				"disable-model-invocation: true",
				"user-invocable: false",
				"## Migration / Rollout",
				"Return envelope per **Section D** from `skills/_shared/sdd-phase-common.md`.",
			},
		},
		{
			name: "sdd-tasks",
			path: "embed/skills/sdd-tasks/SKILL.md",
			required: []string{
				"disable-model-invocation: true",
				"user-invocable: false",
				"## Review Workload Forecast",
				"Chain strategy: <stacked-to-main|feature-branch-chain|size-exception|pending>",
				"Return envelope per **Section D** from `skills/_shared/sdd-phase-common.md`.",
			},
		},
		{
			name: "sdd-apply",
			path: "embed/skills/sdd-apply/SKILL.md",
			required: []string{
				"disable-model-invocation: true",
				"user-invocable: false",
				"Read Previous Apply-Progress (if exists)",
				"There is no silent fallback.",
				"Return envelope per **Section D** from `skills/_shared/sdd-phase-common.md`.",
			},
			forbidden: []string{"Ready for sdd-qa", "next_recommended: sdd-qa"},
		},
		{
			name: "sdd-verify",
			path: "embed/skills/sdd-verify/SKILL.md",
			required: []string{
				"disable-model-invocation: true",
				"user-invocable: false",
				"Activation Contract",
				"strict-tdd-verify.md",
				"Return the Section D envelope from `../_shared/sdd-phase-common.md`.",
			},
		},
		{
			name: "sdd-archive",
			path: "embed/skills/sdd-archive/SKILL.md",
			required: []string{
				"disable-model-invocation: true",
				"user-invocable: false",
				"topic_key: `sdd/{change-name}/archive-report`",
				"Return envelope per **Section D** from `skills/_shared/sdd-phase-common.md`.",
			},
			forbidden: []string{"qa-signoff", "sdd-qa"},
		},
		{
			name: "sdd-onboard",
			path: "embed/skills/sdd-onboard/SKILL.md",
			required: []string{
				"disable-model-invocation: true",
				"user-invocable: false",
				"Artifact store mode (`hive | openspec | hybrid | none`)",
				"Return envelope per **Section D** from `skills/_shared/sdd-phase-common.md`.",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			content := readEmbeddedSkillAsset(t, tc.path)

			sourceStamp := fmt.Sprintf("Synced from https://raw.githubusercontent.com/Gentleman-Programming/gentle-ai/v1.26.5/internal/assets/skills/%s/SKILL.md", tc.name)
			if !strings.Contains(content, sourceStamp) {
				t.Fatalf("expected %s to contain source stamp %q", tc.path, sourceStamp)
			}

			if !strings.Contains(content, "adapted for Jarvis/Hive") {
				t.Fatalf("expected %s to record Jarvis/Hive adaptation", tc.path)
			}

			for _, snippet := range tc.required {
				if !strings.Contains(content, snippet) {
					t.Fatalf("expected %s to contain %q", tc.path, snippet)
				}
			}

			for _, snippet := range append([]string{
				"~/.claude/skills",
				"~/.config/opencode/skills",
				"mcp__engram__",
				"engram-convention.md",
				"Artifact store mode (`engram | openspec | hybrid | none`)",
			}, tc.forbidden...) {
				if strings.Contains(content, snippet) {
					t.Fatalf("expected %s not to contain %q", tc.path, snippet)
				}
			}
		})
	}
}

func TestCatalogContract_SDDFilesDoNotReferenceRetiredQAGates(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		path      string
		required  []string
		forbidden []string
	}{
		{
			path:     "embed/skills/sdd-workflow/SKILL.md",
			required: []string{"tasks → apply → verify → archive", "sdd/{change-name}/verify-report", "sdd/{change-name}/archive-report"},
			forbidden: []string{"sdd-qa", "qa-signoff", "qa-checklist"},
		},
		{
			path:      "embed/skills/hive/SKILL.md",
			required:  []string{"`sdd/{change}/verify-report`", "`sdd/{change}/archive-report`"},
			forbidden: []string{"qa-signoff", "qa-checklist", "sdd-qa"},
		},
		{
			path:      "embed/skills/_shared/hive-convention.md",
			required:  []string{"`sdd/{change}/verify-report`", "`sdd/{change}/archive-report`"},
			forbidden: []string{"qa-signoff", "qa-checklist", "sdd-qa"},
		},
		{
			path:      "embed/orchestrator/sdd-orchestrator.md",
			required:  []string{"proposal -> specs --> tasks -> apply -> verify -> archive", "`hive` — default when available; persistent memory across sessions"},
			forbidden: []string{"engram-convention.md", "sdd-qa", "qa-signoff"},
		},
		{
			path:      "internal/config/layer1.md",
			required:  []string{"SDD DAG: `proposal → specs → tasks → apply → verify → archive`", "Apply-progress continuity"},
			forbidden: []string{"sdd-qa", "qa-signoff"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.path, func(t *testing.T) {
			content := readLocalOrEmbeddedAsset(t, tc.path)

			for _, snippet := range tc.required {
				if !strings.Contains(content, snippet) {
					t.Fatalf("expected %s to contain %q", tc.path, snippet)
				}
			}

			for _, snippet := range tc.forbidden {
				if strings.Contains(content, snippet) {
					t.Fatalf("expected %s not to contain %q", tc.path, snippet)
				}
			}
		})
	}

	if _, err := fs.Stat(jarvis.SkillsFS, "embed/skills/sdd-qa/SKILL.md"); err == nil {
		t.Fatal("expected embedded sdd-qa skill to be deleted")
	}
}

func readEmbeddedSkillAsset(t *testing.T, path string) string {
	t.Helper()

	content, err := jarvis.SkillsFS.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", path, err)
	}

	return string(content)
}

func readLocalOrEmbeddedAsset(t *testing.T, path string) string {
	t.Helper()

	if strings.HasPrefix(path, "embed/skills/") {
		return readEmbeddedSkillAsset(t, path)
	}

	const repoRoot = "/home/andres/Desarrollo/Proyectos/jarvis-dev/jarvis-cli"
	content, err := os.ReadFile(repoRoot + "/" + path)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", path, err)
	}

	return string(content)
}
