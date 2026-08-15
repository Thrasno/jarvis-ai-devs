package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/state"
)

// TestRunConfigSet_RecordsThePresetInTheManifest proves the AppConfig writes
// `jarvis config set preset` used to perform were redundant: ~/.jarvis/state.yaml
// is the only store that holds the persona, and config.yaml must not carry it.
func TestRunConfigSet_RecordsThePresetInTheManifest(t *testing.T) {
	home := isolateTestHome(t)

	if err := runConfigSet("preset", "argentino"); err != nil {
		t.Fatalf("runConfigSet: %v", err)
	}

	manifest, err := state.Load()
	if err != nil {
		t.Fatalf("load state.yaml: %v", err)
	}
	if manifest.Persona != "argentino" {
		t.Errorf("state.yaml persona = %q, want argentino", manifest.Persona)
	}
	if manifest.PersonaSource != state.PersonaSourceBuiltin {
		t.Errorf("state.yaml persona_source = %q, want builtin", manifest.PersonaSource)
	}

	data, err := os.ReadFile(filepath.Join(home, ".jarvis", "config.yaml"))
	if err != nil {
		t.Fatalf("read config.yaml: %v", err)
	}
	for _, key := range state.ReplayConfigKeys() {
		if strings.Contains(string(data), key+":") {
			t.Errorf("config.yaml still carries replay key %q:\n%s", key, data)
		}
	}
}

// TestManifestAgentIDs_PreservesTheRecordedOrder covers what the view used to
// read off the config: the agents the last installation
// configured, in the order it configured them.
func TestManifestAgentIDs_PreservesTheRecordedOrder(t *testing.T) {
	manifest := state.New()
	manifest.InstalledAgents = []state.Agent{
		{ID: "opencode", InstructionsPath: "/i/opencode", ConfigPath: "/c/opencode"},
		{ID: "claude", InstructionsPath: "/i/claude", ConfigPath: "/c/claude"},
	}

	got := manifestAgentIDs(manifest)
	if len(got) != 2 || got[0] != "opencode" || got[1] != "claude" {
		t.Errorf("manifestAgentIDs = %v, want the recorded order", got)
	}
	if manifestAgentIDs(nil) != nil {
		t.Error("manifestAgentIDs(nil) must report no agents")
	}
}

// An agent record is never removed by a run that simply failed to detect the
// agent, which is correct: detection is presence-based and says nothing about
// ownership. But that left no exit at all -- a user who deletes an agent's
// configuration keeps its record forever, and `jarvis sync` rebuilds the managed
// files it names on every run. The exit has to exist and has to be explicit.
func TestRunConfigForgetAgent_RemovesTheRecordExplicitly(t *testing.T) {
	isolateTestHome(t)
	if err := state.Update(func(st *state.State) {
		st.InstalledAgents = []state.Agent{{ID: "claude"}, {ID: "opencode"}}
	}); err != nil {
		t.Fatalf("seed the manifest: %v", err)
	}

	if err := runConfigForgetAgent("opencode"); err != nil {
		t.Fatalf("runConfigForgetAgent: %v", err)
	}

	manifest, err := state.Load()
	if err != nil {
		t.Fatalf("load state.yaml: %v", err)
	}
	got := manifestAgentIDs(manifest)
	if len(got) != 1 || got[0] != "claude" {
		t.Fatalf("installed agents = %v, want only claude: the removal must take effect", got)
	}
}

// Removal is explicit, so an agent the manifest does not record is a mistake
// worth reporting rather than a silent no-op that leaves the user guessing.
func TestRunConfigForgetAgent_ReportsAnAgentItDoesNotRecord(t *testing.T) {
	isolateTestHome(t)
	if err := state.Update(func(st *state.State) {
		st.InstalledAgents = []state.Agent{{ID: "claude"}}
	}); err != nil {
		t.Fatalf("seed the manifest: %v", err)
	}

	err := runConfigForgetAgent("opencode")
	if err == nil {
		t.Fatal("runConfigForgetAgent = nil; an agent that is not recorded must be reported")
	}
	if !strings.Contains(err.Error(), "opencode") {
		t.Fatalf("error = %v; it must name the agent", err)
	}

	manifest, loadErr := state.Load()
	if loadErr != nil {
		t.Fatalf("load state.yaml: %v", loadErr)
	}
	if got := manifestAgentIDs(manifest); len(got) != 1 || got[0] != "claude" {
		t.Fatalf("installed agents = %v, want claude's record untouched", got)
	}
}

// TestRunConfigSet_MigratesBeforeWritingTheManifest covers the upgrade path.
//
// A machine upgrading into this version still has its persona, skills and agents
// in config.yaml and no manifest. Writing the manifest here without migrating
// first would create one carrying only the new persona, and state.Migrate would
// then find a manifest already in place and never carry the skills or the agents
// across -- silently dropping every skill the user had chosen.
func TestRunConfigSet_MigratesBeforeWritingTheManifest(t *testing.T) {
	home := isolateTestHome(t)
	writeCfg(t, home, `schema_version: 2
api_url: https://hivemem.dev
persona_preset: neutra
persona_preset_source: builtin
selected_skills:
  - go-testing
  - work-unit-commits
configured_agents:
  - claude
install:
  completed: true
  agents:
    claude:
      configured: true
      instructions_path: /home/u/.claude/CLAUDE.md
      config_path: /home/u/.claude/settings.json
`)

	if err := runConfigSet("preset", "argentino"); err != nil {
		t.Fatalf("runConfigSet: %v", err)
	}

	manifest, err := state.Load()
	if err != nil {
		t.Fatalf("load state.yaml: %v", err)
	}
	if manifest.Persona != "argentino" {
		t.Errorf("persona = %q, want the newly set value", manifest.Persona)
	}
	if len(manifest.Skills) != 2 {
		t.Errorf("skills = %v, want both values carried over from config.yaml", manifest.Skills)
	}
	if len(manifest.InstalledAgents) != 1 || manifest.InstalledAgents[0].ID != "claude" {
		t.Errorf("agents = %+v, want the record carried over from config.yaml", manifest.InstalledAgents)
	}
}
