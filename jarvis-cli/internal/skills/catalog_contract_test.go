package skills

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	jarvis "github.com/Thrasno/jarvis-dev/jarvis-cli"
	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/project"
	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/sddruntime"
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
			path:      "embed/skills/sdd-workflow/SKILL.md",
			required:  []string{"tasks → apply → verify → archive", "sdd/{change-name}/verify-report", "sdd/{change-name}/archive-report"},
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
			forbidden: []string{"engram-convention.md", "sdd-qa", "qa-signoff", "~/.claude/skills", "~/.config/opencode/skills"},
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

func TestCatalogContract_EmbeddedProductAssetsAvoidLocalRuntimeSkillPaths(t *testing.T) {
	t.Parallel()

	testCases := []string{
		"embed/orchestrator/sdd-orchestrator.md",
		"embed/skills/skill-registry/SKILL.md",
		"embed/skills/judgment-day/SKILL.md",
		"embed/skills/sdd-apply/strict-tdd.md",
	}

	for _, path := range testCases {
		t.Run(path, func(t *testing.T) {
			content := readLocalOrEmbeddedAsset(t, path)

			for _, forbidden := range []string{"~/", "engram", "Engram"} {
				if strings.Contains(content, forbidden) {
					t.Fatalf("expected %s not to contain stale runtime wording %q", path, forbidden)
				}
			}
		})
	}
}

func TestCatalogContract_EmbeddedDocsUseJarvisRegistryAsCanonicalPath(t *testing.T) {
	t.Parallel()

	requiredCanonicalReferences := []string{
		"embed/skills/skill-registry/SKILL.md",
		"embed/skills/_shared/skill-resolver.md",
		"embed/skills/_shared/sdd-phase-common.md",
		"embed/skills/sdd-init/SKILL.md",
		"embed/skills/sdd-init/references/init-details.md",
		"embed/orchestrator/sdd-orchestrator.md",
	}
	for _, filePath := range requiredCanonicalReferences {
		content := readLocalOrEmbeddedAsset(t, filePath)
		if !strings.Contains(content, ".jarvis/skill-registry.md") {
			t.Fatalf("expected %s to prefer .jarvis/skill-registry.md", filePath)
		}
	}

	resolver := readLocalOrEmbeddedAsset(t, "embed/skills/_shared/skill-resolver.md")
	if !strings.Contains(resolver, ".atl/skill-registry.md") || !strings.Contains(strings.ToLower(resolver), "legacy read fallback") {
		t.Fatal("expected shared skill resolver to document .atl/skill-registry.md only as a legacy read fallback")
	}

	checked := 0
	err := fs.WalkDir(jarvis.SkillsFS, "embed", func(filePath string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() || !strings.HasSuffix(filePath, ".md") {
			return walkErr
		}

		content := readEmbeddedSkillAsset(t, filePath)
		if !strings.Contains(content, ".atl/skill-registry.md") {
			return nil
		}

		checked++
		for _, line := range strings.Split(content, "\n") {
			if !strings.Contains(line, ".atl/skill-registry.md") {
				continue
			}
			lowerLine := strings.ToLower(line)
			if !strings.Contains(lowerLine, "legacy") || !strings.Contains(lowerLine, "fallback") {
				t.Fatalf("expected %s to mention .atl/skill-registry.md only as explicit legacy fallback, got line %q", filePath, line)
			}
		}

		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir(embed): %v", err)
	}
	if checked == 0 {
		t.Fatal("expected at least one explicit legacy .atl/skill-registry.md fallback reference to be covered")
	}
}

func TestCatalogContract_EmbeddedProtocolsUsePathInjectedSkillResolution(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		path      string
		required  []string
		forbidden []string
	}{
		{
			path: "embed/skills/_shared/skill-resolver.md",
			required: []string{
				"## Skills to load before work",
				"exact `SKILL.md` paths",
				"`.jarvis/skills/<skill>/SKILL.md`",
				"paths-injected",
				".atl/skill-registry.md` only as a legacy read fallback",
			},
			forbidden: []string{"Project Standards (auto-resolved)", "Copy matching compact rule blocks"},
		},
		{
			path: "embed/skills/_shared/sdd-phase-common.md",
			required: []string{
				"## Skills to load before work",
				"read those exact `SKILL.md` files before task-specific work",
				"paths-injected",
			},
			forbidden: []string{"Project Standards (auto-resolved)", "Do NOT read any SKILL.md files", "`injected`"},
		},
		{
			path: "embed/orchestrator/sdd-orchestrator.md",
			required: []string{
				"## Skills to load before work",
				"exact `SKILL.md` paths",
				"`.jarvis/skills/<skill>/SKILL.md`",
				"paths-injected",
			},
			forbidden: []string{"Project Standards (auto-resolved)", "injects compact rules"},
		},
		{
			path: "embed/skills/judgment-day/SKILL.md",
			required: []string{
				"## Skills to load before work",
				"exact `SKILL.md` paths",
				"paths-injected",
			},
			forbidden: []string{"Project Standards", "compact rules"},
		},
		{
			path: "embed/skills/judgment-day/references/prompts-and-formats.md",
			required: []string{
				"## Skills to load before work",
				"/absolute/or/repo-resolved/path/to/",
				"Skill Resolution: {paths-injected|fallback-registry|fallback-path|none}",
			},
			forbidden: []string{"Project Standards (auto-resolved)", "{injected|fallback-registry"},
		},
		{
			path: "internal/config/layer1.md",
			required: []string{
				"## Skills to load before work",
				"orchestrator resolves skills from the registry",
				"passes exact `SKILL.md` paths",
				"Sub-agents read those exact files before task-specific work",
				"skill_resolution: paths-injected",
			},
			forbidden: []string{"Project Standards (auto-resolved)", "cache compact rules", "Sub-agents do NOT read SKILL.md files"},
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
					t.Fatalf("expected %s not to contain legacy path-injection drift %q", tc.path, snippet)
				}
			}
		})
	}
}

func TestCatalogContract_RegistryPromptAndProtocolStayPathInjected(t *testing.T) {
	registryPaths := project.CanonicalRegistryPaths()
	contract := sddruntime.DefaultPromptContract("opencode", "sdd-apply")
	sources, err := contract.OrderedRequiredSources()
	if err != nil {
		t.Fatalf("OrderedRequiredSources(): %v", err)
	}

	var registrySource sddruntime.PromptSource
	for _, source := range sources {
		if source.Layer == sddruntime.LayerRegistry {
			registrySource = source
			break
		}
	}
	if registrySource.ID == "" {
		t.Fatal("expected registry source in default prompt contract")
	}
	if registrySource.ID != sddruntime.RegistrySkillIndexSourceID {
		t.Fatalf("registry source ID = %q, want %q", registrySource.ID, sddruntime.RegistrySkillIndexSourceID)
	}
	if registrySource.Path != registryPaths.WritePath {
		t.Fatalf("registry source path = %q, want canonical registry path %q", registrySource.Path, registryPaths.WritePath)
	}
	layerName := string(registrySource.Layer)
	if strings.Contains(layerName, "rule") || !strings.Contains(layerName, "skill_index") {
		t.Fatalf("registry prompt layer must describe the skill-index contract, got %q", layerName)
	}

	skills, err := ListSkills(jarvis.SkillsFS)
	if err != nil {
		t.Fatalf("ListSkills(): %v", err)
	}
	rows := RegistryRows(skills)
	if len(rows) == 0 {
		t.Fatal("expected embedded skills to produce registry rows")
	}

	projectRows := make([]project.RegistrySkill, 0, len(rows))
	for _, row := range rows {
		projectRows = append(projectRows, project.RegistrySkill{
			ID:           row.ID,
			Name:         row.Name,
			Description:  row.Description,
			Trigger:      row.Trigger,
			Scope:        row.Scope,
			Path:         row.Path,
			CompactRules: row.CompactRules,
		})
	}

	dir := t.TempDir()
	if err := project.WriteRegistry(dir, "contract-project", project.StackGo, []string{"sdd-workflow", "hive", "go-testing"}, projectRows); err != nil {
		t.Fatalf("WriteRegistry(): %v", err)
	}
	content, err := os.ReadFile(filepath.Join(dir, registryPaths.WritePath))
	if err != nil {
		t.Fatalf("read generated registry: %v", err)
	}
	registry := string(content)

	required := []string{
		"Canonical registry path: `.jarvis/skill-registry.md`",
		"| Skill | Trigger / Description | Scope | Path |",
		"| SDD Apply | When implementing tasks — Implement tasks following specs and design; supports Strict TDD mode | core | `.jarvis/skills/sdd-apply/SKILL.md` |",
		"| Go Testing | When writing Go tests, using teatest, or adding test coverage — Go testing patterns including Bubbletea TUI testing | optional | `.jarvis/skills/go-testing/SKILL.md` |",
		"## Compact Rules (Transitional Metadata)",
		"Compact rules are compatibility metadata; the skill index path rows above are the primary instruction contract.",
	}
	for _, snippet := range required {
		if !strings.Contains(registry, snippet) {
			t.Fatalf("expected generated registry to contain %q, got:\n%s", snippet, registry)
		}
	}

	for _, forbidden := range []string{
		"| Skill | Trigger | Path | Type |",
		"| SDD Apply | When implementing tasks — Implement tasks following specs and design; supports Strict TDD mode | core | `sdd-apply/SKILL.md` |",
		"## Compact Rules\n",
		"Project Standards (auto-resolved)",
		"Sub-agents do NOT read SKILL.md files",
	} {
		if strings.Contains(registry, forbidden) {
			t.Fatalf("generated registry drifted back to legacy injection contract %q in:\n%s", forbidden, registry)
		}
	}
}

func TestCatalogContract_SkillRegistryDocsAreIndexFirstAndPathFirst(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		path      string
		required  []string
		forbidden []string
	}{
		{
			path: "embed/skills/skill-registry/SKILL.md",
			required: []string{
				"index-first",
				"path-first",
				"| Skill | Trigger / Description | Scope | Path |",
				".jarvis/skills/<skill>/SKILL.md",
				"## Skills to load before work",
				"Compact rules are transitional compatibility metadata",
			},
			forbidden: []string{"Project Standards (auto-resolved)", "Sub-agents do NOT read", "compact rules are the MOST IMPORTANT"},
		},
		{
			path: "embed/skills/sdd-init/references/init-details.md",
			required: []string{
				"index-first and path-first",
				"Skill, Trigger / Description, Scope, and Path",
				".jarvis/skills/<skill>/SKILL.md",
				"exact `SKILL.md` path",
			},
			forbidden: []string{"compact rules as 5-15 actionable lines", "Sub-agents do NOT read"},
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
					t.Fatalf("expected %s not to contain registry contract drift %q", tc.path, snippet)
				}
			}
		})
	}
}

func TestCatalogContract_SkillRegistryDoesNotIgnoreJarvisDirectory(t *testing.T) {
	t.Parallel()

	content := readLocalOrEmbeddedAsset(t, "embed/skills/skill-registry/SKILL.md")
	lowerContent := strings.ToLower(content)

	for _, forbidden := range []string{
		"add `.jarvis/` to the project's `.gitignore`",
		"add .jarvis/ to the project's .gitignore",
		"ignore `.jarvis/`",
		"ignore .jarvis/",
	} {
		if strings.Contains(lowerContent, forbidden) {
			t.Fatalf("skill-registry must not tell agents to ignore .jarvis/: found %q", forbidden)
		}
	}

	if !strings.Contains(content, ".jarvis/skill-registry.md`) is intended to be committed/shared") &&
		!strings.Contains(content, ".jarvis/skill-registry.md` is intended to be committed/shared") {
		t.Fatal("expected skill-registry to state .jarvis/skill-registry.md is committed/shared")
	}
}

func TestCatalogContract_EmbeddedSkillMarkdownReferencesResolve(t *testing.T) {
	t.Parallel()

	referencePattern := regexp.MustCompile(`\[[^\]]+\]\(([^)]+\.md)\)|` + "`" + `([^` + "`" + `]+\.md)` + "`")
	allowedGeneratedOrRuntimeReferences := map[string]bool{
		".atl/skill-registry.md":           true,
		"atl/skill-registry.md":            true,
		".jarvis/skill-registry.md":        true,
		"jarvis/skill-registry.md":         true,
		"AGENTS.md":                        true,
		"agents.md":                        true,
		"CLAUDE.md":                        true,
		"GEMINI.md":                        true,
		"copilot-instructions.md":          true,
		"SKILL.md":                         true,
		".github/PULL_REQUEST_TEMPLATE.md": true,
		"github/PULL_REQUEST_TEMPLATE.md":  true,
		"exploration.md":                   true,
		"proposal.md":                      true,
		"design.md":                        true,
		"tasks.md":                         true,
		"verify-report.md":                 true,
		"spec.md":                          true,
		"openspec/config.yaml":             true,
		"openspec/config.md":               true,
		"openspec/changes/{change-name}/proposal.md":      true,
		"openspec/changes/{change-name}/design.md":        true,
		"openspec/changes/{change-name}/tasks.md":         true,
		"openspec/changes/{change-name}/verify-report.md": true,
	}

	err := fs.WalkDir(jarvis.SkillsFS, "embed/skills", func(filePath string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() || !strings.HasSuffix(filePath, ".md") {
			return walkErr
		}

		content := readEmbeddedSkillAsset(t, filePath)
		matches := referencePattern.FindAllStringSubmatch(content, -1)
		for _, match := range matches {
			reference := strings.TrimSpace(firstNonEmpty(match[1], match[2]))
			reference = strings.Trim(reference, " ,;:")
			if reference == "" || shouldAllowGeneratedMarkdownReference(reference, allowedGeneratedOrRuntimeReferences) {
				continue
			}

			resolved, ok := resolveEmbeddedMarkdownReference(filePath, reference)
			if !ok {
				t.Fatalf("%s references non-local markdown %q without explicit allowlist entry", filePath, reference)
			}
			if _, statErr := fs.Stat(jarvis.SkillsFS, resolved); statErr != nil {
				t.Fatalf("%s references %q resolved to %s, but embedded file is missing: %v", filePath, reference, resolved, statErr)
			}
		}

		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir(embed/skills): %v", err)
	}
}

func TestCatalogContract_ComplementarySkillsMatchUpstreamContract(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		path     string
		required []string
	}{
		{
			name: "chained-pr",
			path: "embed/skills/chained-pr/SKILL.md",
			required: []string{
				"name: chained-pr",
				"PRs over 400 lines, stacked PRs, review slices",
				"## Activation Contract",
				"Feature Branch Chain",
				"[references/chaining-details.md](references/chaining-details.md)",
			},
		},
		{
			name: "work-unit-commits",
			path: "embed/skills/work-unit-commits/SKILL.md",
			required: []string{
				"name: work-unit-commits",
				"Plan commits as reviewable work units",
				"## Work Unit Checklist",
				"Keep tests with code",
				"## SDD Relationship",
			},
		},
		{
			name: "comment-writer",
			path: "embed/skills/comment-writer/SKILL.md",
			required: []string{
				"name: comment-writer",
				"Write warm, direct collaboration comments",
				"## Voice Rules",
				"Match thread language",
				"No em dashes",
			},
		},
		{
			name: "cognitive-doc-design",
			path: "embed/skills/cognitive-doc-design/SKILL.md",
			required: []string{
				"name: cognitive-doc-design",
				"Design docs that reduce cognitive load",
				"## Critical Patterns",
				"Progressive disclosure",
				"## PR and Review Docs",
			},
		},
		{
			name: "branch-pr",
			path: "embed/skills/branch-pr/SKILL.md",
			required: []string{
				"name: branch-pr",
				"Create Gentle AI pull requests with issue-first checks",
				"Synced from https://raw.githubusercontent.com/Gentleman-Programming/gentle-ai/v1.26.5/internal/assets/skills/branch-pr/SKILL.md",
				"adapted for Jarvis packaging",
				"Every PR MUST link an approved issue",
				"No `Co-Authored-By` trailers",
			},
		},
		{
			name: "issue-creation",
			path: "embed/skills/issue-creation/SKILL.md",
			required: []string{
				"name: issue-creation",
				"Create Gentle AI issues with issue-first checks",
				"Synced from https://raw.githubusercontent.com/Gentleman-Programming/gentle-ai/v1.26.5/internal/assets/skills/issue-creation/SKILL.md",
				"adapted for Jarvis packaging",
				"Every issue gets `status:needs-review` automatically",
				"A maintainer MUST add `status:approved`",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			content := readEmbeddedSkillAsset(t, tc.path)
			if _, err := fs.Stat(jarvis.SkillsFS, tc.path); err != nil {
				t.Fatalf("expected workflow skill path %s to be embedded: %v", tc.path, err)
			}
			if !strings.HasPrefix(content, "---\n") || !strings.Contains(content, "\n---\n") {
				t.Fatalf("expected %s to include YAML frontmatter", tc.path)
			}
			if !strings.Contains(content, "license: Apache-2.0") {
				t.Fatalf("expected %s to preserve Apache-2.0 frontmatter license", tc.path)
			}

			sourceStamp := fmt.Sprintf("Synced from https://raw.githubusercontent.com/Gentleman-Programming/gentle-ai/v1.26.5/internal/assets/skills/%s/SKILL.md", tc.name)
			if !strings.Contains(content, sourceStamp) {
				t.Fatalf("expected %s to contain source stamp %q", tc.path, sourceStamp)
			}

			for _, snippet := range tc.required {
				if !strings.Contains(content, snippet) {
					t.Fatalf("expected %s to contain %q", tc.path, snippet)
				}
			}
		})
	}
}

func TestCatalogContract_ChainedPRReferencesAreEmbeddedRecursively(t *testing.T) {
	t.Parallel()

	const referencePath = "embed/skills/chained-pr/references/chaining-details.md"
	content := readEmbeddedSkillAsset(t, referencePath)

	requiredSnippets := []string{
		"Synced from https://raw.githubusercontent.com/Gentleman-Programming/gentle-ai/v1.26.5/internal/assets/skills/chained-pr/references/chaining-details.md",
		"# Chained PR Details",
		"## Feature Branch Chain",
		"## Chain Context Section",
		"📍 #NNN This PR",
	}

	for _, snippet := range requiredSnippets {
		if !strings.Contains(content, snippet) {
			t.Fatalf("expected %s to contain %q", referencePath, snippet)
		}
	}
}

func TestCatalogContract_SkillCreatorUsesCompactLLMFirstContract(t *testing.T) {
	t.Parallel()

	content := readEmbeddedSkillAsset(t, "embed/skills/skill-creator/SKILL.md")

	requiredSnippets := []string{
		"name: skill-creator",
		"description: \"Trigger: new skills, agent instructions, documenting AI usage patterns.",
		"## Activation Contract",
		"## Hard Rules",
		"## Decision Gates",
		"## Execution Steps",
		"## Output Contract",
		"[references/quality-loop.md](references/quality-loop.md)",
	}
	for _, snippet := range requiredSnippets {
		if !strings.Contains(content, snippet) {
			t.Fatalf("expected skill-creator to contain %q", snippet)
		}
	}

	for _, forbidden := range []string{"\n## Keywords", "description: >", "WebSearch", "Task", "sdd-qa", "qa-signoff"} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("expected compact skill-creator not to contain %q", forbidden)
		}
	}
}

func TestCatalogContract_QAChecklistOutputContractAndSDDBoundaries(t *testing.T) {
	t.Parallel()

	content := readEmbeddedSkillAsset(t, "embed/skills/qa-checklist/SKILL.md")
	lowerContent := strings.ToLower(content)

	requiredSnippets := []string{
		"name: qa-checklist",
		"batería de pruebas",
		"checklist de pruebas",
		"QA checklist",
		"test checklist",
		"## Output Contract",
		"Manual QA checklist",
		"Automated test recommendations",
		"Risks and edge cases",
		"Assumptions and questions",
		"not executed",
		"not verification evidence",
		"sdd-verify",
	}
	for _, snippet := range requiredSnippets {
		if !strings.Contains(content, snippet) {
			t.Fatalf("expected qa-checklist to contain %q", snippet)
		}
	}

	for _, forbidden := range []string{"sdd-qa", "qa-signoff", "qa phase", "final verification", "verification passed", "tests passed"} {
		if strings.Contains(lowerContent, forbidden) {
			t.Fatalf("expected qa-checklist not to imply QA-as-verification with %q", forbidden)
		}
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

	repoRoot := repositoryRoot(t)
	content, err := os.ReadFile(repoRoot + "/" + path)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", path, err)
	}

	return string(content)
}

func repositoryRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}

	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func shouldAllowGeneratedMarkdownReference(reference string, allowlist map[string]bool) bool {
	if strings.HasPrefix(reference, "http://") || strings.HasPrefix(reference, "https://") {
		return true
	}
	if allowlist[reference] || allowlist[path.Base(reference)] {
		return true
	}
	for _, generatedPrefix := range []string{"openspec/", "specs/", "changes/", "archive/", "path/to/", "{project-root}/"} {
		if strings.HasPrefix(reference, generatedPrefix) {
			return true
		}
	}
	return strings.Contains(reference, "{") || strings.Contains(reference, "...")
}

func resolveEmbeddedMarkdownReference(sourcePath, reference string) (string, bool) {
	cleanReference := path.Clean(reference)
	switch {
	case strings.HasPrefix(cleanReference, "../") || strings.HasPrefix(cleanReference, "./"):
		return path.Clean(path.Join(path.Dir(sourcePath), cleanReference)), true
	case strings.HasPrefix(cleanReference, "_shared/"):
		return "embed/skills/" + cleanReference, true
	case strings.HasPrefix(cleanReference, "skills/"):
		return "embed/" + cleanReference, true
	case strings.HasPrefix(cleanReference, "references/"):
		return path.Clean(path.Join(path.Dir(sourcePath), cleanReference)), true
	case !strings.Contains(cleanReference, "/") && strings.HasSuffix(cleanReference, ".md"):
		return path.Clean(path.Join(path.Dir(sourcePath), cleanReference)), true
	default:
		return "", false
	}
}
