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
