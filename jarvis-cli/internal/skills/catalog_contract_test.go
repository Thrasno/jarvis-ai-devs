package skills

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"testing"

	jarvis "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/project"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddstatus"
)

const sharedPhaseCommonPath = "embed/skills/_shared/sdd-phase-common.md"
const sharedHiveConventionPath = "embed/skills/_shared/hive-convention.md"
const sharedPersistenceContractPath = "embed/skills/_shared/persistence-contract.md"
const sharedOpenSpecConventionPath = "embed/skills/_shared/openspec-convention.md"

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
		"mem_search returns the most recent",
		"authoritative version",
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

func TestCatalogContract_SharedHiveRetrievalDocsAvoidUnsupportedLatestGuarantees(t *testing.T) {
	t.Parallel()

	for _, assetPath := range []string{sharedPhaseCommonPath, sharedHiveConventionPath} {
		t.Run(assetPath, func(t *testing.T) {
			content := readEmbeddedSkillAsset(t, assetPath)
			for _, forbidden := range []string{
				"mem_search returns the most recent",
				"retrieval surfaces the most recent",
				"most recent, which is the authoritative version",
				"authoritative version",
				"latest returned observation",
				"latest-by-topic",
			} {
				if strings.Contains(content, forbidden) {
					t.Fatalf("expected %s not to promise unsupported Hive latest/authoritative retrieval with %q", assetPath, forbidden)
				}
			}

			for _, required := range []string{
				"Search results are previews, not source material.",
				"If Hive search returns multiple candidate artifacts for the same topic and no explicit artifact reference is available, treat the result as ambiguous.",
			} {
				if !strings.Contains(content, required) {
					t.Fatalf("expected %s to contain ambiguity-safe Hive retrieval wording %q", assetPath, required)
				}
			}
		})
	}
}

func TestCatalogContract_SharedPersistenceContractUsesJarvisHiveModeContract(t *testing.T) {
	content := readEmbeddedSkillAsset(t, sharedPersistenceContractPath)

	requiredSnippets := []string{
		"Gentleman-Programming/gentle-ai/v1.40.2/internal/assets/skills/_shared/persistence-contract.md",
		"660917927b4821f5e540dc8fa501d6bee723222c",
		"Artifact store mode (`hive | openspec | hybrid | none`)",
		"## Mode Resolution",
		"## Mode Roles",
		"### Mode Comparison",
		"## Behavior Per Mode",
		"## State Persistence Across Phases",
		"`sdd/{change-name}/state`",
		"`sdd/{change-name}/{artifact-type}`",
		"## Sub-Agent Response Ordering",
		"the final output MUST be text, never only a tool result",
		"## Skill Registry Handoff",
		"## Detail Level",
		"detail_level`: `concise | standard | deep`",
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
	for _, snippet := range requiredSnippets {
		if !strings.Contains(content, snippet) {
			t.Fatalf("expected %s to contain %q", sharedPersistenceContractPath, snippet)
		}
	}

	forbiddenSnippets := []string{
		"Engram",
		"engram",
		"mcp__engram__",
		"`engram | openspec | hybrid | none`",
		"upsert",
		"upserts",
		"overwrite",
		"overwrites",
		"no revision history is kept",
		"latest returned observation",
		"latest-by-topic",
		"authoritative version",
		"`sdd/{change-name}/exploration`",
		"openspec/changes/{change-name}/exploration.md",
		"The orchestrator persists DAG state after each phase transition",
		"Both backends stay in sync",
	}
	for _, snippet := range forbiddenSnippets {
		if strings.Contains(content, snippet) {
			t.Fatalf("expected %s not to contain unsupported or legacy persistence wording %q", sharedPersistenceContractPath, snippet)
		}
	}
}

func TestCatalogContract_SDDExploreHiveTopicAgreesAcrossSharedContractAndPhaseSkills(t *testing.T) {
	t.Parallel()

	sharedContract := readEmbeddedSkillAsset(t, sharedPersistenceContractPath)
	exploreSkill := readEmbeddedSkillAsset(t, "embed/skills/sdd-explore/SKILL.md")
	proposeSkill := readEmbeddedSkillAsset(t, "embed/skills/sdd-propose/SKILL.md")

	for assetPath, content := range map[string]string{
		sharedPersistenceContractPath:       sharedContract,
		"embed/skills/sdd-explore/SKILL.md": exploreSkill,
		"embed/skills/sdd-propose/SKILL.md": proposeSkill,
	} {
		if !strings.Contains(content, "sdd/{change-name}/explore") {
			t.Fatalf("expected %s to use stable Explore Hive topic key sdd/{change-name}/explore", assetPath)
		}
		if strings.Contains(content, "sdd/{change-name}/exploration") {
			t.Fatalf("expected %s not to rename the Explore Hive topic key to sdd/{change-name}/exploration", assetPath)
		}
	}

	if !strings.Contains(sharedContract, "openspec/changes/{change-name}/explore.md") {
		t.Fatal("expected shared persistence contract to use OpenSpec explore.md file path convention")
	}
}

func TestCatalogContract_SDDExploreOpenSpecPathAgreesWithRuntimeSourceConvention(t *testing.T) {
	t.Parallel()

	changeName := "my-change"
	projectRoot := t.TempDir()
	changeDir := filepath.Join(projectRoot, "openspec", "changes", changeName)
	if err := os.MkdirAll(changeDir, 0o755); err != nil {
		t.Fatalf("create change dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(changeDir, "explore.md"), []byte("# Exploration\n"), 0o644); err != nil {
		t.Fatalf("write runtime explore artifact: %v", err)
	}

	artifacts, _, err := sddstatus.NewOpenSpecSource(projectRoot).FetchArtifacts(context.Background(), changeName)
	if err != nil {
		t.Fatalf("fetch OpenSpec artifacts: %v", err)
	}
	if got := artifacts[sddstatus.ArtifactExplore]; got != sddstatus.ArtifactDone {
		t.Fatalf("runtime OpenSpec source explore artifact = %q, want %q", got, sddstatus.ArtifactDone)
	}

	sharedContract := readEmbeddedSkillAsset(t, sharedPersistenceContractPath)
	if !strings.Contains(sharedContract, "openspec/changes/{change-name}/explore.md") {
		t.Fatalf("expected %s to document runtime OpenSpec explore path explore.md", sharedPersistenceContractPath)
	}
	if strings.Contains(sharedContract, "openspec/changes/{change-name}/exploration.md") {
		t.Fatalf("expected %s not to document stale OpenSpec exploration.md path", sharedPersistenceContractPath)
	}
}

func TestCatalogContract_SDDExploreOpenSpecPathAgreesAcrossSharedDocsAndPhaseSkill(t *testing.T) {
	t.Parallel()

	assets := map[string]string{
		sharedPersistenceContractPath:       readEmbeddedSkillAsset(t, sharedPersistenceContractPath),
		sharedOpenSpecConventionPath:        readEmbeddedSkillAsset(t, sharedOpenSpecConventionPath),
		"embed/skills/sdd-explore/SKILL.md": readEmbeddedSkillAsset(t, "embed/skills/sdd-explore/SKILL.md"),
	}

	for assetPath, content := range assets {
		if !strings.Contains(content, "openspec/changes/{change-name}/explore.md") {
			t.Fatalf("expected %s to document runtime OpenSpec Explore path openspec/changes/{change-name}/explore.md", assetPath)
		}
		if strings.Contains(content, "exploration.md") {
			t.Fatalf("expected %s not to document stale OpenSpec Explore path exploration.md", assetPath)
		}
	}
}

func TestCatalogContract_OpenSpecConventionDocumentsRemovedAndRenamedDeltaSemantics(t *testing.T) {
	t.Parallel()

	content := readEmbeddedSkillAsset(t, sharedOpenSpecConventionPath)

	requiredSnippets := []string{
		"## Delta Spec Sections",
		"## ADDED Requirements",
		"## MODIFIED Requirements",
		"## REMOVED Requirements",
		"## RENAMED Requirements",
		"REMOVED requirements MUST include non-empty, non-placeholder Reason and Migration evidence.",
		"`Migration: None` is allowed only when the delta explicitly justifies why no replacement or user/operator migration is needed.",
		"(Reason: {why this requirement is being removed})",
		"(Migration: {what replaces this behavior, or `None` with justification})",
		"(Old name: {Existing Requirement Name})",
		"(New name: {New Requirement Name})",
		"RENAMED requirements MUST include explicit old and new requirement names.",
	}

	for _, snippet := range requiredSnippets {
		if !strings.Contains(content, snippet) {
			t.Fatalf("expected %s to document OpenSpec delta semantic %q", sharedOpenSpecConventionPath, snippet)
		}
	}
}

func TestCatalogContract_SDDSpecDocumentsRemovedMigrationAndRenamedDeltas(t *testing.T) {
	t.Parallel()

	content := readEmbeddedSkillAsset(t, "embed/skills/sdd-spec/SKILL.md")

	requiredSnippets := []string{
		"what's being ADDED, MODIFIED, REMOVED, or RENAMED from the system's behavior",
		"## RENAMED Requirements",
		"(Old name: {Existing Requirement Name})",
		"(New name: {New Requirement Name})",
		"(Migration: {what replaces this behavior, or `None` with justification})",
		"| {domain} | Delta/New | {N added, M modified, K removed, R renamed} | {total scenarios} |",
		"If existing specs exist, write DELTA specs (ADDED/MODIFIED/REMOVED/RENAMED sections)",
		"Before writing a REMOVED requirement, verify Reason and Migration are both non-empty and not placeholder text.",
		"Use `Migration: None` only with an explicit justification explaining why no replacement or migration is needed.",
		"Do not write REMOVED requirements with empty Reason, empty Migration, placeholder evidence, or unjustified `Migration: None`.",
		"RENAMED requirements MUST include explicit old and new requirement names",
	}

	for _, snippet := range requiredSnippets {
		if !strings.Contains(content, snippet) {
			t.Fatalf("expected sdd-spec source to document OpenSpec removed/renamed semantic %q", snippet)
		}
	}
}

func TestCatalogContract_SDDArchiveDocumentsRenamedMergeAndRemovedGuards(t *testing.T) {
	t.Parallel()

	content := readEmbeddedSkillAsset(t, "embed/skills/sdd-archive/SKILL.md")

	requiredSnippets := []string{
		"RENAMED Requirements → Rename the matching requirement in main spec using the explicit old/new names",
		"If a RENAMED requirement omits Old name or New name, STOP before renaming it",
		"Before deleting any REMOVED requirement, confirm the delta includes both `Reason:` and `Migration:` with non-empty, non-placeholder evidence",
		"`Migration: None` is valid only when it includes a justification",
		"If Reason or Migration is empty, placeholder text, or unjustified `None`, STOP before deleting it",
		"For RENAMED requirements, preserve the requirement body and scenarios unless the delta also modifies them",
		"| {domain} | Created/Updated | {N added, M modified, K removed, R renamed requirements} |",
	}

	for _, snippet := range requiredSnippets {
		if !strings.Contains(content, snippet) {
			t.Fatalf("expected sdd-archive source to document OpenSpec archive semantic %q", snippet)
		}
	}
}

func TestCatalogContract_EmbeddedSkillDocsDoNotKeepStaleExploreOpenSpecFilename(t *testing.T) {
	t.Parallel()

	err := fs.WalkDir(jarvis.SkillsFS, "embed/skills", func(filePath string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() || !strings.HasSuffix(filePath, ".md") {
			return walkErr
		}

		content := readEmbeddedSkillAsset(t, filePath)
		if strings.Contains(content, "exploration.md") {
			t.Fatalf("%s keeps stale OpenSpec Explore filename exploration.md; use explore.md unless explicitly scoped to legacy migration", filePath)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir(embed/skills): %v", err)
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
				"Chain strategy: <stacked-to-main|feature-branch-chain|size:exception|pending>",
				"Return envelope per **Section D** from `skills/_shared/sdd-phase-common.md`.",
			},
		},
		{
			name: "sdd-apply",
			path: "embed/skills/sdd-apply/SKILL.md",
			required: []string{
				"disable-model-invocation: true",
				"user-invocable: false",
				"jarvis sdd status <change> --json",
				"schema: `jarvis.sdd-status`",
				"allowedEditRoots",
				"workspace-planning",
				"Read Previous Apply-Progress (if exists)",
				"apply-progress = partial",
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

			upstreamVersion := "v1.26.5"
			if tc.name == "sdd-verify" || tc.name == "sdd-apply" || tc.name == "sdd-archive" {
				upstreamVersion = "v1.40.2"
			}
			sourceStamp := fmt.Sprintf("Synced from https://raw.githubusercontent.com/Gentleman-Programming/gentle-ai/%s/internal/assets/skills/%s/SKILL.md", upstreamVersion, tc.name)
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

func TestCatalogContract_SDDVerifySourceUsesJarvisAdaptedModelSections(t *testing.T) {
	content := readEmbeddedSkillAsset(t, "embed/skills/sdd-verify/SKILL.md")

	requiredSnippets := []string{
		"Gentleman-Programming/gentle-ai/v1.40.2/internal/assets/skills/sdd-verify/SKILL.md",
		"660917927b4821f5e540dc8fa501d6bee723222c",
		"<!-- section:model-capable -->",
		"<!-- /section:model-capable -->",
		"<!-- section:model-small -->",
		"<!-- /section:model-small -->",
		"jarvis sdd status <change> --json",
		"schema: `jarvis.sdd-status`",
		"contextFiles",
		"artifactPaths",
		"allowedEditRoots",
		"workspace-planning",
		"apply-progress missing or partial",
		"unchecked implementation task is CRITICAL",
		"## Status Handling and Blockers",
		"## Runtime Evidence Policy",
		"If runtime tests cannot be run, report runtime evidence as skipped",
		"A documented manual verification path is not evidence by itself.",
		"Manual or runtime verification counts as `PASS` only when it was executed and the report records the command or manual action, result, timestamp or session, and operator/evidence source.",
		"## Skipped Dimensions",
		"## Final Verdict Constraints",
		"Generated artifacts are output, never sources of truth",
		"mcp__hive__mem_save",
	}
	for _, snippet := range requiredSnippets {
		if !strings.Contains(content, snippet) {
			t.Fatalf("expected sdd-verify source to contain %q", snippet)
		}
	}

	forbiddenSnippets := []string{
		"mcp__engram__",
		"Artifact Retrieval (Engram Mode)",
		"Persist `verify-report` according to mode: Engram",
		"Do NOT run tests unless `strict_tdd` is active",
		"project explicitly documents an accepted manual verification path",
	}
	for _, snippet := range forbiddenSnippets {
		if strings.Contains(content, snippet) {
			t.Fatalf("expected sdd-verify source not to contain %q", snippet)
		}
	}
}

func TestCatalogContract_SDDVerifyStrictTDDSourcePreservesQualityVerificationRules(t *testing.T) {
	t.Parallel()

	content := readEmbeddedSkillAsset(t, "embed/skills/sdd-verify/strict-tdd-verify.md")

	requiredSnippets := []string{
		"Gentleman-Programming/gentle-ai/v1.40.2/internal/assets/skills/sdd-verify/strict-tdd-verify.md",
		"660917927b4821f5e540dc8fa501d6bee723222c",
		"## Step 5a: TDD Compliance Check",
		"RED evidence must include an executed focused failing command",
		"GREEN evidence must be re-run during verify",
		"REFACTOR evidence must include a post-refactor passing command or an explicit no-refactor rationale",
		"### Test Layer Distribution",
		"For each spec scenario, note which test layer covers it",
		"### Changed File Coverage",
		"Go coverage example: `go test ./... -coverprofile=/tmp/opencode/jarvis-sdd-verify.coverprofile`",
		"### Assertion Quality",
		"| File | Line | Assertion | Issue | Severity |",
		"Tautologies",
		"Ghost loops",
		"Smoke-test-only",
		"Mock/assertion ratio",
		"Behavior coverage vs implementation-only tests",
		"Mock/Fake Hygiene",
		"Skipped Dimensions and Uncertainty",
		"Coverage analysis skipped — no coverage tool detected",
		"Missing optional tooling is not a failure, but skipped TDD evidence is still a finding.",
	}

	for _, want := range requiredSnippets {
		if !strings.Contains(content, want) {
			t.Fatalf("expected sdd-verify strict-tdd source to contain quality rule %q", want)
		}
	}
}

func TestCatalogContract_SDDVerifyStrictTDDSourceDocumentsJarvisPolicyAdditions(t *testing.T) {
	t.Parallel()

	content := readEmbeddedSkillAsset(t, "embed/skills/sdd-verify/strict-tdd-verify.md")

	requiredSnippets := []string{
		"Upstream-derived from Gentle AI v1.40.2",
		"Jarvis-specific verification policy additions beyond runtime wording",
		"Future parity runs MUST NOT overwrite Jarvis-specific verification policy without maintainer approval.",
	}

	for _, want := range requiredSnippets {
		if !strings.Contains(content, want) {
			t.Fatalf("expected sdd-verify strict-tdd source provenance to contain %q", want)
		}
	}
}

func TestCatalogContract_SDDVerifyStrictTDDSourceFlagsOverIntegratedCoverageGaps(t *testing.T) {
	t.Parallel()

	content := readEmbeddedSkillAsset(t, "embed/skills/sdd-verify/strict-tdd-verify.md")

	requiredSnippets := []string{
		"### Coverage Allocation Audit",
		"over-integrated",
		"E2E-heavy",
		"deterministic lower-layer tests",
		"Flag WARNING when behavior is covered only by E2E or broad integration tests but deterministic unit or lower-layer integration tests should cover it",
		"Do not accept expensive E2E coverage as a substitute for cheaper deterministic coverage of pure logic, parsing, mapping, validation, command construction, or artifact rendering.",
	}

	for _, want := range requiredSnippets {
		if !strings.Contains(content, want) {
			t.Fatalf("expected sdd-verify strict-tdd source to contain coverage allocation guard %q", want)
		}
	}
}

func TestCatalogContract_SDDVerifyStrictTDDSourceWritesCoverageProfilesOutsideRepo(t *testing.T) {
	t.Parallel()

	content := readEmbeddedSkillAsset(t, "embed/skills/sdd-verify/strict-tdd-verify.md")

	if !strings.Contains(content, "/tmp/opencode/") {
		t.Fatal("expected coverage examples to write generated profiles outside the repository under /tmp/opencode")
	}

	for _, forbidden := range []string{
		"-coverprofile=coverage.out",
		"-coverprofile ./coverage.out",
		"-coverprofile=./coverage.out",
		"Go coverage example: `go test ./... -coverprofile=coverage.out`",
	} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("expected sdd-verify strict-tdd source not to recommend repo-local coverage output %q", forbidden)
		}
	}

	for _, line := range strings.Split(content, "\n") {
		if !strings.Contains(line, "coverprofile") {
			continue
		}
		if !regexp.MustCompile(`-coverprofile[= ]/tmp/opencode/[^\s` + "`" + `]+`).MatchString(line) {
			t.Fatalf("expected coverprofile guidance to use /tmp/opencode, got line %q", line)
		}
	}
}

func TestCatalogContract_SDDVerifyStrictTDDSourceRejectsWeakTDDPassLoopholes(t *testing.T) {
	t.Parallel()

	content := readEmbeddedSkillAsset(t, "embed/skills/sdd-verify/strict-tdd-verify.md")

	forbiddenSnippets := []string{
		"verify adequate cases or a valid single-scenario reason",
		"If \"➖ Single\" → verify spec truly has only one scenario",
		"Skip verification, trust the report",
		"subjective quality; skip strict verification",
		"Coverage and quality metrics are informational, not blocking.",
		"Test layer distribution is informational — SUGGESTION level only",
		"assertions that never call production code",
		"type-only assertions without value assertions",
		"empty collection assertions are enough",
		"manual verification path is evidence by itself",
		"RED would fail",
		"would fail if run",
	}

	for _, unwanted := range forbiddenSnippets {
		if strings.Contains(content, unwanted) {
			t.Fatalf("expected sdd-verify strict-tdd source not to contain weak TDD loophole %q", unwanted)
		}
	}
}

func TestCatalogContract_SDDApplySourceUsesJarvisAdaptedStatusGuards(t *testing.T) {
	content := readEmbeddedSkillAsset(t, "embed/skills/sdd-apply/SKILL.md")

	requiredSnippets := []string{
		"Gentleman-Programming/gentle-ai/v1.40.2/internal/assets/skills/sdd-apply/SKILL.md",
		"660917927b4821f5e540dc8fa501d6bee723222c",
		"delegate_only: true",
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
		"Read context from `contextFiles` and `artifactPaths` before reading implementation files.",
		"Generated artifacts are output, never sources of truth",
		"mcp__hive__mem_save",
		"Artifact store mode (`hive | openspec | hybrid | none`)",
		"When prior `apply-progress = partial` exists, merge/reconcile it with current task state",
		"do not jump to `sdd-verify` until apply progress and task checkboxes agree.",
	}
	for _, snippet := range requiredSnippets {
		if !strings.Contains(content, snippet) {
			t.Fatalf("expected sdd-apply source to contain %q", snippet)
		}
	}

	forbiddenSnippets := []string{
		"mcp__engram__",
		"Artifact store mode (`engram | openspec | hybrid | none`)",
		"mem_update(id: {tasks-observation-id}",
		"~/.claude/skills",
		"~/.config/opencode/skills",
		"If `applyState` says apply is blocked",
		"If the command is unavailable, build the equivalent status from the artifacts before editing.",
		"If status is unavailable and no explicit `actionContext.allowedEditRoots` is available, STOP before editing.",
	}
	for _, snippet := range forbiddenSnippets {
		if strings.Contains(content, snippet) {
			t.Fatalf("expected sdd-apply source not to contain %q", snippet)
		}
	}
}

func TestCatalogContract_SDDArchiveSourceUsesJarvisAdaptedArchiveSafetyGuards(t *testing.T) {
	content := readEmbeddedSkillAsset(t, "embed/skills/sdd-archive/SKILL.md")

	requiredSnippets := []string{
		"Gentleman-Programming/gentle-ai/v1.40.2/internal/assets/skills/sdd-archive/SKILL.md",
		"660917927b4821f5e540dc8fa501d6bee723222c",
		"delegate_only: true",
		"## Status and Archive Safety Gate",
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
		"Generated artifacts are output, never sources of truth",
		"Partial, missing, or stale artifacts block archive until they are reconciled and re-verified",
		"For `none` mode, return a closure summary only; do not persist an archive report",
		"mcp__hive__mem_save",
		"Artifact store mode (`hive | openspec | hybrid | none`)",
	}
	for _, snippet := range requiredSnippets {
		if !strings.Contains(content, snippet) {
			t.Fatalf("expected sdd-archive source to contain %q", snippet)
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
		"R1",
		"R2",
		"R3",
		"R4",
		"sdd-qa",
	}
	for _, snippet := range forbiddenSnippets {
		if strings.Contains(content, snippet) {
			t.Fatalf("expected sdd-archive source not to contain %q", snippet)
		}
	}
}

func TestCatalogContract_SDDArchiveStatusFieldsMatchChangeStatusJSONContract(t *testing.T) {
	content := readEmbeddedSkillAsset(t, "embed/skills/sdd-archive/SKILL.md")
	statusJSONNames := jsonFieldNames(reflect.TypeOf(sddstatus.ChangeStatus{}))

	for _, fieldName := range []string{
		"artifacts",
		"artifactPaths",
		"contextFiles",
		"blockedReasons",
		"taskProgress",
		"applyState",
		"actionContext",
		"phaseInstructions",
	} {
		if !statusJSONNames[fieldName] {
			t.Fatalf("ChangeStatus JSON contract missing %q", fieldName)
		}
		if !strings.Contains(content, fieldName) {
			t.Fatalf("expected sdd-archive to reference real status JSON field %q", fieldName)
		}
	}

	if statusJSONNames["verifyReport"] {
		t.Fatal("ChangeStatus JSON contract unexpectedly exposes top-level verifyReport")
	}
	if strings.Contains(content, "verifyReport") {
		t.Fatal("sdd-archive must not reference nonexistent top-level verifyReport; use artifacts/artifactPaths/contextFiles verify-report evidence")
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
			required:  []string{"proposal -> specs --> tasks -> apply -> verify -> archive", "Artifact store is collected by `SDD Session Preflight`"},
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
	if _, err := fs.Stat(jarvis.SkillsFS, "embed/skills/sdd-workflow/SKILL.md"); err == nil {
		t.Fatal("expected embedded sdd-workflow skill to be deleted")
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

func TestCatalogContract_SDDApplyStrictTDDSourcePreservesQualityRules(t *testing.T) {
	t.Parallel()

	content := readEmbeddedSkillAsset(t, "embed/skills/sdd-apply/strict-tdd.md")

	requiredSnippets := []string{
		"## Assertion Quality Rules (MANDATORY)",
		"### Banned Assertion Patterns (NEVER write these)",
		"NEVER write trivial assertions",
		"# GHOST LOOP",
		"loop iterates 0 times",
		"assert the collection is non-empty FIRST",
		"TRIANGULATE (MANDATORY for most tasks)",
		"MINIMUM: at least 2 test cases per behavior",
		"NON-EMPTY/NON-TRIVIAL",
		"Failure Evidence Requirements",
		"RED evidence must include one of these",
		"executed focused failing test command",
		"Go compile-time failure from a missing symbol",
		"If infrastructure blocks the focused RED command, STOP",
		"### Smoke Test Rule",
		"Renders without crash",
		"does NOT count toward TDD coverage",
		"### Mock/Fake Hygiene Rules",
		"If you need more mocks than assertions",
		"Extract-Before-Mock Rule",
		"### Behavior-First Test Rule",
		"Tests must assert **behavior visible to the user or caller**",
		"BEFORE touching production code",
		"approval tests",
		"if got != want",
	}

	for _, want := range requiredSnippets {
		if !strings.Contains(content, want) {
			t.Fatalf("expected sdd-apply strict-tdd source to contain %q", want)
		}
	}
}

func TestCatalogContract_SDDApplyStrictTDDSourceRejectsWeakTestLoopholes(t *testing.T) {
	t.Parallel()

	content := readEmbeddedSkillAsset(t, "embed/skills/sdd-apply/strict-tdd.md")

	forbiddenSnippets := []string{
		"TRIANGULATE (when behavior has multiple cases)",
		"If NO (single-case behavior, e.g., config setup, simple mapping):",
		"Skip triangulation, proceed to REFACTOR",
		"➖ Single if spec has only one scenario",
		"\"➖ Single\" if spec has only one scenario",
		"skip triangulation because the spec has only one scenario",
		"skip triangulation when the spec has only one scenario",
		"documented unimplemented-behavior failure when execution is not possible yet",
		"RED would fail",
		"would fail if run",
		"would fail when run",
		"would fail once executed",
		"report as \"Blocked\" and continue to next task",
		"expect(true).toBe(true)              # ✅",
		"assert 1 == 1                        # ✅",
		"Renders without crash\" is a unit test",
		"type-only assertions are enough",
		"empty collection assertions are enough",
	}

	for _, unwanted := range forbiddenSnippets {
		if strings.Contains(content, unwanted) {
			t.Fatalf("expected sdd-apply strict-tdd source not to contain weak-test loophole %q", unwanted)
		}
	}
}

func TestCatalogContract_SDDApplyStrictTDDSourceLimitsTriangulationSkipToStructuralOneOutputTasks(t *testing.T) {
	t.Parallel()

	content := readEmbeddedSkillAsset(t, "embed/skills/sdd-apply/strict-tdd.md")

	requiredSnippets := []string{
		"Skip triangulation ONLY when ALL of these are true:",
		"The task is purely structural",
		"There is literally ONE possible output",
		"no branching, no logic",
		"A single spec scenario is NOT a triangulation skip reason",
		"only structural one-output work may skip triangulation",
		"Triangulation skipped: {reason}",
	}

	for _, want := range requiredSnippets {
		if !strings.Contains(content, want) {
			t.Fatalf("expected sdd-apply strict-tdd source to contain triangulation skip guard %q", want)
		}
	}
}

func TestCatalogContract_SDDApplyStrictTDDSourceRequiresExecutedREDEvidence(t *testing.T) {
	t.Parallel()

	content := readEmbeddedSkillAsset(t, "embed/skills/sdd-apply/strict-tdd.md")

	requiredSnippets := []string{
		"Strict TDD requires proof that RED happened before GREEN.",
		"RED evidence MUST include the executed focused failing test command",
		"the failing assertion, compile error, or behavior mismatch output",
		"Do not document RED as \"would fail\"",
		"it is not RED until the focused command was executed and failed",
		"If infrastructure blocks the focused RED command, STOP",
		"do NOT implement or move to another task",
		"Do not proceed to GREEN with hypothetical RED evidence.",
	}

	for _, want := range requiredSnippets {
		if !strings.Contains(content, want) {
			t.Fatalf("expected sdd-apply strict-tdd source to require executed RED evidence %q", want)
		}
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
	if err := project.WriteRegistry(dir, "contract-project", project.StackGo, []string{"hive", "go-testing"}, projectRows); err != nil {
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

func TestCatalogContract_SkillRegistryHivePersistenceDoesNotPromiseTopicKeyUpserts(t *testing.T) {
	t.Parallel()

	content := readEmbeddedSkillAsset(t, "embed/skills/skill-registry/SKILL.md")

	for _, required := range []string{
		"If Hive is available, also save to Hive",
		"`topic_key` groups registry saves for retrieval; it is not an identity, recency, or upsert guarantee.",
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("expected skill-registry source to contain Hive-safe persistence wording %q", required)
		}
	}

	for _, forbidden := range []string{
		"topic_key ensures upserts",
		"ensures upserts",
		"running again updates the same observation",
		"updates the same observation",
	} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("expected skill-registry source not to promise unsupported topic_key upsert semantics with %q", forbidden)
		}
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
		"explore.md":                       true,
		"proposal.md":                      true,
		"design.md":                        true,
		"tasks.md":                         true,
		"verify-report.md":                 true,
		"spec.md":                          true,
		"openspec/config.yaml":             true,
		"openspec/config.md":               true,
		"openspec/changes/{change-name}/explore.md":       true,
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
			if !hasYAMLFrontmatter(content) {
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

func TestCatalogContract_SkillImproverIsPackagedWithSafetyContract(t *testing.T) {
	t.Parallel()

	const skillPath = "embed/skills/skill-improver/SKILL.md"
	content := readEmbeddedSkillAsset(t, skillPath)
	lowerContent := strings.ToLower(content)

	if _, err := fs.Stat(jarvis.SkillsFS, skillPath); err != nil {
		t.Fatalf("expected skill-improver path %s to be embedded: %v", skillPath, err)
	}
	if !hasYAMLFrontmatter(content) {
		t.Fatalf("expected %s to include YAML frontmatter", skillPath)
	}

	requiredSnippets := []string{
		"name: skill-improver",
		"Trigger: improve skills, audit skills, refactor skills, skill quality",
		"license: Apache-2.0",
		"Packaged for Jarvis skill registry and path-injected loading",
		"## Activation Contract",
		"## Hard Safety Rules",
		"Audit existing skills against the style guide",
		"explicit user approval before modifying any skill file",
		"Default to audit-only mode",
		"Style Guide Checks",
		".jarvis/skill-registry.md",
		".jarvis/skills/<skill>/SKILL.md",
	}
	for _, snippet := range requiredSnippets {
		if !strings.Contains(content, snippet) {
			t.Fatalf("expected skill-improver to contain %q", snippet)
		}
	}

	for _, forbidden := range []string{
		"rewrite skills without approval",
		"modify skill files without approval",
		"automatically mutate user skills",
		"auto-mutate user skills",
		"autonomously rewrite",
		"~/.claude/skills",
		"~/.config/opencode/skills",
	} {
		if strings.Contains(lowerContent, forbidden) {
			t.Fatalf("expected skill-improver not to allow unsafe or local-runtime wording %q", forbidden)
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

func TestCatalogContract_GentleAIParityRunbookProtectsSourceTemplates(t *testing.T) {
	t.Parallel()

	content := readWorkspaceAsset(t, "docs/maintenance/gentle-ai-skill-parity.md")
	scopeRows := markdownTableRows(t, markdownSection(t, content, "Scope boundaries"))
	checklist := markdownSection(t, content, "Verification checklist")

	if !strings.Contains(strings.ToLower(content), "maintainer-only") {
		t.Fatal("expected parity runbook to identify the workflow as maintainer-only")
	}

	requiredScopeRules := []struct {
		area  string
		terms []string
	}{
		{"Workflow audience", []string{"Maintainers only", "public CLI", "doctor", "install", "automatic sync"}},
		{"Editable source", []string{"Jarvis source-of-truth files", "jarvis-cli/embed/**", "jarvis-cli/internal/agent/**", "jarvis-cli/internal/persona/**", "jarvis-cli/internal/sddruntime/**"}},
		{"Forbidden edit targets", []string{"~/.claude/**", "~/.config/opencode/**", "generated registries", "installed `.jarvis/skills/**` copies", "team environments"}},
		{"Upstream updates", []string{"No blind upstream sync", "recorded decision", "rationale"}},
		{"Skill content updates", []string{"Out of scope", "chained PR slices"}},
	}
	for _, rule := range requiredScopeRules {
		row := requireMarkdownTableRow(t, scopeRows, rule.area)
		requireAllTerms(t, row, rule.terms...)
	}

	requireAllTerms(t, checklist,
		"Accepted changes", "Jarvis source templates/assets only",
		"Generated artifacts", "team environments", "untouched",
	)
}

func TestCatalogContract_GentleAIParityRunReportTemplateCapturesRequiredDecisions(t *testing.T) {
	t.Parallel()

	content := readWorkspaceAsset(t, "docs/maintenance/skill-parity-run-report-template.md")
	metadataRows := markdownTableRows(t, markdownSection(t, content, "Run metadata"))
	guardrail := markdownSection(t, content, "Source-of-truth guardrail")
	differenceRows := markdownTableRows(t, markdownSection(t, content, "Difference log"))
	approvalPlan := markdownSection(t, content, "Approval and implementation plan")

	for _, field := range []string{
		"Run ID",
		"Gentle AI selected reference",
		"Gentle AI retrieval date",
		"Jarvis commit or branch",
		"Last adopted Gentle AI version",
		"Maintainer",
		"Report location",
		"Reference availability",
	} {
		requireMarkdownTableRow(t, metadataRows, field)
	}

	requireAllTerms(t, guardrail,
		"Jarvis source templates/assets", "not generated user-machine configs",
		"~/.claude/**", "~/.config/opencode/**", "generated registries", "installed `.jarvis/skills/**` copies", "team environments",
	)

	differenceHeader := differenceRows[0]
	requireAllTerms(t, differenceHeader, "Decision", "Rationale", "Owner", "Follow-up")
	requireAllTerms(t, approvalPlan, "Maintainer approval", "before skill content or workflow semantics changed")

	if !strings.Contains(content, "## Inventory summary") {
		t.Fatal("expected run report template to include inventory summary section")
	}

	for _, required := range []string{
		"Hive/current Jarvis artifact store",
		"maintenance PR",
		"committed report",
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("expected run report template persistence guidance to contain %q", required)
		}
	}

	for _, forbidden := range []string{
		"Store completed reports in Engram",
		"completed reports in Engram",
		"reports are stored in Engram",
	} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("run report template must not direct completed maintenance reports to legacy Engram storage with %q", forbidden)
		}
	}

	for _, category := range []string{"apply", "adapt", "ignore", "investigate"} {
		if !strings.Contains(content, "`"+category+"`") {
			t.Fatalf("expected run report template to document decision category %q", category)
		}
	}
}

func TestCatalogContract_GentleAIParityRunbookDocumentsInventoryDecisions(t *testing.T) {
	t.Parallel()

	content := readWorkspaceAsset(t, "docs/maintenance/gentle-ai-skill-parity.md")
	inventoryRows := markdownTableRows(t, markdownSection(t, content, "Inventory rules"))
	checklist := markdownSection(t, content, "Verification checklist")

	for _, skillID := range []string{"go-testing", "skill-creator", "skill-improver", "skill-registry"} {
		row := requireMarkdownTableRow(t, inventoryRows, "`"+skillID+"`")
		requireAllTerms(t, row, "In scope", "Adopted Gentle AI skill", "stamp metadata is absent")
	}

	requireAllTerms(t, requireMarkdownTableRow(t, inventoryRows, "Stamped Gentle AI skill files"), "In scope", "selected upstream path", "reference")
	requireAllTerms(t, requireMarkdownTableRow(t, inventoryRows, "Stamped `_shared` or reference files"), "In scope", "shared contracts")
	requireAllTerms(t, requireMarkdownTableRow(t, inventoryRows, "Adopted unstamped skills"), "In scope", "intentionally adopted")
	requireAllTerms(t, requireMarkdownTableRow(t, inventoryRows, "`hive`"), "Adapted equivalent", "Engram", "compare intent")
	requireAllTerms(t, requireMarkdownTableRow(t, inventoryRows, "`qa-checklist`"), "Out of Gentle AI parity", "Jarvis-local")
	requireAllTerms(t, requireMarkdownTableRow(t, inventoryRows, "`sdd-workflow`"), "Retired/removed", "orchestrator", "shared SDD contracts", "phase skills")
	requireAllTerms(t, requireMarkdownTableRow(t, inventoryRows, "Ambiguous local files"), "Investigate", "Do not silently include", "exclude", "edit")
	requireAllTerms(t, checklist,
		"inventory includes all adopted Gentle AI skills", "adopted unstamped skills",
		"`hive`, `qa-checklist`, and `sdd-workflow`", "scope table",
	)
}

func TestCatalogContract_GentleAISourceStampsAreParseableAndNetworkFree(t *testing.T) {
	t.Parallel()

	checked := 0
	err := fs.WalkDir(jarvis.SkillsFS, "embed/skills", func(filePath string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() || !strings.HasSuffix(filePath, ".md") {
			return walkErr
		}

		content := readEmbeddedSkillAsset(t, filePath)
		if !strings.Contains(content, "Synced from https://raw.githubusercontent.com/Gentleman-Programming/gentle-ai/") {
			return nil
		}

		checked++
		stamp, ok := parseGentleAISourceStamp(content)
		if !ok {
			t.Fatalf("expected %s to have a parseable Gentle AI source stamp", filePath)
		}
		if stamp.Repository != "Gentleman-Programming/gentle-ai" {
			t.Fatalf("%s source stamp repository = %q, want Gentleman-Programming/gentle-ai", filePath, stamp.Repository)
		}
		if !strings.HasPrefix(stamp.UpstreamPath, "internal/assets/skills/") {
			t.Fatalf("%s source stamp upstream path = %q, want internal/assets/skills/...", filePath, stamp.UpstreamPath)
		}
		if stamp.Reference == "" {
			t.Fatalf("%s source stamp must include a tag, commit, or reference segment", filePath)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir(embed/skills): %v", err)
	}
	if checked == 0 {
		t.Fatal("expected at least one Gentle AI source stamp to be covered")
	}
}

func TestCatalogContract_GentleAIParityInventoryClassifiesStampedAndAmbiguousSources(t *testing.T) {
	t.Parallel()

	runbook := readWorkspaceAsset(t, "docs/maintenance/gentle-ai-skill-parity.md")
	stampedContent := readEmbeddedSkillAsset(t, "embed/skills/sdd-apply/SKILL.md")
	unstampedContent := "# Unstamped local fixture\n\nThis fixture intentionally has no Gentle AI source stamp.\n"

	if got := classifyGentleAIParityCandidate(runbook, "embed/skills/sdd-apply/SKILL.md", stampedContent); got != "in-scope" {
		t.Fatalf("stamped adopted skill classified as %q, want in-scope", got)
	}
	if got := classifyGentleAIParityCandidate(runbook, "embed/skills/go-testing/SKILL.md", unstampedContent); got != "in-scope" {
		t.Fatalf("documented adopted unstamped skill classified as %q, want in-scope", got)
	}
	if got := classifyGentleAIParityCandidate(runbook, "embed/skills/hive/SKILL.md", unstampedContent); got != "adapted-equivalent" {
		t.Fatalf("hive classified as %q, want adapted-equivalent", got)
	}
	if got := classifyGentleAIParityCandidate(runbook, "embed/skills/qa-checklist/SKILL.md", unstampedContent); got != "out-of-parity" {
		t.Fatalf("qa-checklist classified as %q, want out-of-parity", got)
	}
	if got := classifyGentleAIParityCandidate(runbook, "embed/skills/local-only/SKILL.md", unstampedContent); got != "investigate" {
		t.Fatalf("ambiguous unstamped skill classified as %q, want investigate", got)
	}
}

type gentleAISourceStamp struct {
	Repository   string
	Reference    string
	UpstreamPath string
}

func parseGentleAISourceStamp(content string) (gentleAISourceStamp, bool) {
	pattern := regexp.MustCompile(`Synced from https://raw\.githubusercontent\.com/(Gentleman-Programming/gentle-ai)/([^/\s)]+)/([^\s)]+\.md)`)
	match := pattern.FindStringSubmatch(content)
	if match == nil {
		return gentleAISourceStamp{}, false
	}

	return gentleAISourceStamp{
		Repository:   match[1],
		Reference:    match[2],
		UpstreamPath: match[3],
	}, true
}

func classifyGentleAIParityCandidate(runbook, filePath, content string) string {
	skillID := embeddedSkillID(filePath)

	if _, ok := parseGentleAISourceStamp(content); ok && strings.Contains(runbook, "| Stamped Gentle AI skill files | In scope |") {
		return "in-scope"
	}
	if strings.Contains(runbook, fmt.Sprintf("| `%s` | In scope |", skillID)) {
		return "in-scope"
	}
	if strings.Contains(runbook, fmt.Sprintf("| `%s` | Adapted equivalent |", skillID)) {
		return "adapted-equivalent"
	}
	if strings.Contains(runbook, fmt.Sprintf("| `%s` | Out of Gentle AI parity |", skillID)) {
		return "out-of-parity"
	}
	if strings.Contains(runbook, fmt.Sprintf("| `%s` | Retired/removed |", skillID)) {
		return "retired"
	}
	if strings.Contains(runbook, "| Ambiguous local files | Investigate |") {
		return "investigate"
	}
	return "unknown"
}

func embeddedSkillID(filePath string) string {
	relativePath := strings.TrimPrefix(filePath, "embed/skills/")
	parts := strings.Split(relativePath, "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

func markdownSection(t *testing.T, content, heading string) string {
	t.Helper()

	sectionStart := strings.Index(content, "## "+heading)
	if sectionStart == -1 {
		t.Fatalf("expected markdown heading %q", heading)
	}

	section := content[sectionStart:]
	nextHeading := strings.Index(section[len("## "+heading):], "\n## ")
	if nextHeading == -1 {
		return section
	}
	return section[:len("## "+heading)+nextHeading]
}

func markdownTableRows(t *testing.T, section string) []string {
	t.Helper()

	var rows []string
	for _, line := range strings.Split(section, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "|") || !strings.HasSuffix(trimmed, "|") {
			continue
		}
		if isMarkdownTableSeparator(trimmed) {
			continue
		}
		rows = append(rows, trimmed)
	}

	if len(rows) == 0 {
		t.Fatal("expected markdown section to contain a table")
	}
	return rows
}

func isMarkdownTableSeparator(row string) bool {
	for _, cell := range markdownTableCells(row) {
		if strings.Trim(cell, "-:") != "" {
			return false
		}
	}
	return true
}

func requireMarkdownTableRow(t *testing.T, rows []string, firstCell string) string {
	t.Helper()

	for _, row := range rows {
		cells := markdownTableCells(row)
		if len(cells) == 0 {
			continue
		}
		if cells[0] == firstCell {
			return strings.Join(cells, " | ")
		}
	}

	t.Fatalf("expected markdown table row with first cell %q", firstCell)
	return ""
}

func markdownTableCells(row string) []string {
	trimmed := strings.Trim(row, "|")
	parts := strings.Split(trimmed, "|")
	for i, part := range parts {
		parts[i] = strings.TrimSpace(part)
	}
	return parts
}

func requireAllTerms(t *testing.T, content string, terms ...string) {
	t.Helper()

	for _, term := range terms {
		if !strings.Contains(content, term) {
			t.Fatalf("expected content to contain %q in %q", term, content)
		}
	}
}

func jsonFieldNames(structType reflect.Type) map[string]bool {
	fields := make(map[string]bool, structType.NumField())
	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)
		jsonTag := field.Tag.Get("json")
		name, _, _ := strings.Cut(jsonTag, ",")
		if name == "" || name == "-" {
			continue
		}
		fields[name] = true
	}
	return fields
}

func readEmbeddedSkillAsset(t *testing.T, path string) string {
	t.Helper()

	content, err := jarvis.SkillsFS.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", path, err)
	}

	return string(content)
}

func hasYAMLFrontmatter(content string) bool {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	return strings.HasPrefix(normalized, "---\n") && strings.Contains(normalized, "\n---\n")
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

func readWorkspaceAsset(t *testing.T, path string) string {
	t.Helper()

	content, err := os.ReadFile(filepath.Join(repositoryRoot(t), "..", filepath.FromSlash(path)))
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
