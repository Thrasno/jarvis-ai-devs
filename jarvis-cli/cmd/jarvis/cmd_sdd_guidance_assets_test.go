package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	jarvis "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"
)

func TestSDDGuidanceAssets_DescribeBoundStoreSafety(t *testing.T) {
	assets := map[string]string{
		"persistence contract": mustReadSDDGuidanceAsset(t, jarvis.SkillsFS, "embed/skills/_shared/persistence-contract.md"),
		"phase common":         mustReadSDDGuidanceAsset(t, jarvis.SkillsFS, "embed/skills/_shared/sdd-phase-common.md"),
		"orchestrator":         mustReadSDDGuidanceAsset(t, jarvis.OrchestratorFS, "embed/orchestrator/sdd-orchestrator.md"),
		"apply":                mustReadSDDGuidanceAsset(t, jarvis.SkillsFS, "embed/skills/sdd-apply/SKILL.md"),
		"archive":              mustReadSDDGuidanceAsset(t, jarvis.SkillsFS, "embed/skills/sdd-archive/SKILL.md"),
		"user guide":           mustReadSDDUserGuide(t),
	}

	required := map[string][]string{
		"persistence contract": {
			"one immutable authoritative binding per `project/change`",
			"`none` is never persisted",
			"Hive stores its binding in SQLite",
			"OpenSpec stores its binding in `openspec/changes/{change-name}/state.yaml`",
		},
		"phase common": {
			"Persisted binding wins even when `JARVIS_SDD_STORE_MODE` changes or is invalid",
			"resolve or adopt the binding before selecting a backend",
			"Outage or protocol failure is not absence",
		},
		"orchestrator": {
			"`jarvis sdd status` may adopt a binding",
			"not purely read-only recovery",
			"hybrid divergence fails closed",
		},
		"apply": {
			"preserve the existing hybrid receipt",
			"replay or revalidate the exact request against both backends through their idempotent contracts before accepting acknowledgements",
			"A complete receipt returns without replay",
			"Never change the payload, identity, or authority",
			"never rewrite confirmed progress",
		},
		"archive": {
			"archive-report must be complete and executor-written",
			"byte-identical and non-blank archive reports",
			"Do not introduce a typed closure API, closure state, or archive receipt",
		},
		"user guide": {
			"binding and provenance in `jarvis sdd status`",
			"distinguish absence from unavailable",
			"`jarvis doctor` cannot determine an effective per-change binding",
			"replay or revalidate the exact request against both backends through their idempotent contracts before accepting acknowledgements",
			"A complete receipt returns without replay",
			"never change the payload, identity, or authority",
			"never rewrite confirmed progress",
		},
	}
	forbidden := map[string][]string{
		"apply":                {"Replay only the receipt-recorded missing side"},
		"user guide":           {"replays only the receipt-recorded missing side"},
		"persistence contract": {"Hive first, filesystem fallback", "Read priority: Hive first; fall back to filesystem"},
		"orchestrator":         {"`/sdd-status` is read-only recovery", "read-only status handled directly by the orchestrator"},
	}

	for name, snippets := range required {
		for _, snippet := range snippets {
			if !strings.Contains(strings.ToLower(assets[name]), strings.ToLower(snippet)) {
				t.Errorf("%s is missing binding-safety guidance %q", name, snippet)
			}
		}
	}
	for name, snippets := range forbidden {
		for _, snippet := range snippets {
			if strings.Contains(strings.ToLower(assets[name]), strings.ToLower(snippet)) {
				t.Errorf("%s retains prohibited operational guidance %q", name, snippet)
			}
		}
	}
}

func TestSDDGuidanceAssets_PreflightBindsStoreBeforeStatus(t *testing.T) {
	const bindCommand = "jarvis sdd bind --change <change> --mode <hive|openspec|hybrid>"
	orchestrator := mustReadSDDGuidanceAsset(t, jarvis.OrchestratorFS, "embed/orchestrator/sdd-orchestrator.md")

	dispatcher := sddGuidanceSection(t, orchestrator, "#### Binding Resolution and Diagnostics", "### SDD Entry Routing")
	bind, status := strings.Index(dispatcher, "\n"+bindCommand), strings.Index(dispatcher, "\njarvis sdd status <change> --json")
	if bind == -1 || status == -1 || bind > status {
		t.Errorf("native dispatcher must list %q before `jarvis sdd status` (bind at %d, status at %d)", bindCommand, bind, status)
	}

	preflight := sddGuidanceSection(t, orchestrator, "### SDD Session Preflight (HARD GATE)", "### Review Workload Guard")
	binding := sddGuidanceSection(t, preflight, "#### Preflight Store Binding", "")
	if strings.Index(preflight, "#### Preflight Store Binding") < strings.Index(preflight, "Map answers to canonical values") {
		t.Error("the preflight store binding must follow the mapping of the preflight answers")
	}

	assets := map[string]string{
		"orchestrator":           orchestrator,
		"orchestrator preflight": binding,
		"persistence contract":   mustReadSDDGuidanceAsset(t, jarvis.SkillsFS, "embed/skills/_shared/persistence-contract.md"),
		"phase common":           mustReadSDDGuidanceAsset(t, jarvis.SkillsFS, "embed/skills/_shared/sdd-phase-common.md"),
		"user guide":             mustReadSDDUserGuide(t),
	}
	required := map[string][]string{
		"orchestrator preflight": {
			bindCommand,
			"right after the preflight artifact-store choice",
			"before any `jarvis sdd status` or `jarvis sdd continue` call, phase launch, or artifact write",
			"`initial:preflight`",
			"idempotent replay",
			"a non-zero exit stops the sdd path for that change before status",
			"`JARVIS_SDD_STORE_MODE=none`",
		},
		"persistence contract": {
			"the preflight choice reaches the CLI through `" + bindCommand + "`",
			"before any `jarvis sdd status` call or artifact write",
			"`initial:preflight`",
			"`JARVIS_SDD_STORE_MODE=none`",
		},
		"phase common": {
			bindCommand,
			"before your own first `jarvis sdd status` call or artifact write",
			"`storeBinding.persisted: true`",
			"`JARVIS_SDD_STORE_MODE=none`",
		},
		"user guide": {
			bindCommand,
			"`initial:preflight`",
		},
	}
	for name, snippets := range required {
		for _, snippet := range snippets {
			if !strings.Contains(strings.ToLower(assets[name]), strings.ToLower(snippet)) {
				t.Errorf("%s is missing preflight binding guidance %q", name, snippet)
			}
		}
	}

	// The environment variable no longer carries the preflight choice.
	forbidden := map[string]string{
		"persistence contract": "`JARVIS_SDD_STORE_MODE` or the explicit preflight choice selects the initial mode",
		"orchestrator":         "the environment selects only an unbound change",
		"user guide":           "the environment variable only selects an unbound change",
	}
	for name, snippet := range forbidden {
		if strings.Contains(assets[name], snippet) {
			t.Errorf("%s still routes the preflight choice through the environment: %q", name, snippet)
		}
	}
}

// sddGuidanceSection returns the text from start up to end, or to the end of
// content when end is empty.
func sddGuidanceSection(t *testing.T, content, start, end string) string {
	t.Helper()
	from := strings.Index(content, start)
	if from == -1 {
		t.Fatalf("guidance section %q not found", start)
	}
	section := content[from:]
	if end == "" {
		return section
	}
	to := strings.Index(section, end)
	if to == -1 {
		t.Fatalf("guidance section %q has no end marker %q", start, end)
	}
	return section[:to]
}

func mustReadSDDGuidanceAsset(t *testing.T, fs interface{ ReadFile(string) ([]byte, error) }, path string) string {
	t.Helper()
	content, err := fs.ReadFile(path)
	if err != nil {
		t.Fatalf("read embedded guidance asset %s: %v", path, err)
	}
	return string(content)
}

func mustReadSDDUserGuide(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	for dir := cwd; ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			content, err := os.ReadFile(filepath.Join(filepath.Dir(dir), "docs", "sdd-user-guide.md"))
			if err != nil {
				t.Fatalf("read docs/sdd-user-guide.md: %v", err)
			}
			return string(content)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not locate jarvis-cli module root")
		}
	}
}
