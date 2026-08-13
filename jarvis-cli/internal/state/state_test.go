package state

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// isolateHome sets HOME to a fresh temp dir and registers cleanup.
// This is mandatory to prevent tests from touching the real ~/.jarvis.
func isolateHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}

// fullState returns a manifest with every replay field populated.
func fullState() *State {
	return &State{
		SchemaVersion: currentSchemaVersion,
		InstalledAgents: []Agent{
			{ID: "claude", InstructionsPath: "/home/u/.claude/CLAUDE.md", ConfigPath: "/home/u/.claude/settings.json"},
			{ID: "opencode", InstructionsPath: "/home/u/.config/opencode/AGENTS.md"},
		},
		SelectionConfigured: true,
		Skills:              []string{"go-testing", "work-unit-commits"},
		Persona:             "argentino",
		PersonaSource:       PersonaSourceBuiltin,
		Statusline:          StatuslineState{Decided: true, Enabled: true},
		PhaseModels: PhaseModels{
			Aliases:  map[string]PhaseModelSelection{"apply": {OpenCode: "sonnet", Claude: "opus"}},
			OpenCode: map[string]OpenCodeModelAssignment{"apply": {ProviderID: "anthropic", ModelID: "claude-sonnet-4", Effort: "high"}},
			Claude:   map[string]ClaudeModelAssignment{"apply": {Model: "opus", Effort: "high"}},
		},
		Scope:              ScopeLocalCloud,
		ManagedAssetDigest: "sha256:abc123",
	}
}

func TestSaveLoad_RoundTripsEveryReplayField(t *testing.T) {
	isolateHome(t)

	want := fullState()
	if err := Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got.SchemaVersion != currentSchemaVersion {
		t.Errorf("SchemaVersion = %d, want %d", got.SchemaVersion, currentSchemaVersion)
	}
	if len(got.InstalledAgents) != 2 {
		t.Fatalf("InstalledAgents = %#v, want 2 entries", got.InstalledAgents)
	}
	if got.InstalledAgents[0].ID != "claude" || got.InstalledAgents[1].ID != "opencode" {
		t.Errorf("InstalledAgents order/IDs = %#v, want claude then opencode", got.InstalledAgents)
	}
	if got.InstalledAgents[0].InstructionsPath != "/home/u/.claude/CLAUDE.md" {
		t.Errorf("claude InstructionsPath = %q", got.InstalledAgents[0].InstructionsPath)
	}
	if got.InstalledAgents[0].ConfigPath != "/home/u/.claude/settings.json" {
		t.Errorf("claude ConfigPath = %q", got.InstalledAgents[0].ConfigPath)
	}
	if !got.SelectionConfigured {
		t.Error("SelectionConfigured = false, want true")
	}
	if strings.Join(got.Skills, ",") != "go-testing,work-unit-commits" {
		t.Errorf("Skills = %#v", got.Skills)
	}
	if got.Persona != "argentino" || got.PersonaSource != PersonaSourceBuiltin {
		t.Errorf("Persona = %q/%q, want argentino/builtin", got.Persona, got.PersonaSource)
	}
	if !got.Statusline.Decided || !got.Statusline.Enabled {
		t.Errorf("Statusline = %#v, want decided+enabled", got.Statusline)
	}
	if got.PhaseModels.Aliases["apply"].Claude != "opus" {
		t.Errorf("PhaseModels.Aliases = %#v", got.PhaseModels.Aliases)
	}
	if got.PhaseModels.OpenCode["apply"].ModelID != "claude-sonnet-4" {
		t.Errorf("PhaseModels.OpenCode = %#v", got.PhaseModels.OpenCode)
	}
	if got.PhaseModels.Claude["apply"].Effort != "high" {
		t.Errorf("PhaseModels.Claude = %#v", got.PhaseModels.Claude)
	}
	if got.Scope != ScopeLocalCloud {
		t.Errorf("Scope = %q, want %q", got.Scope, ScopeLocalCloud)
	}
	if got.ManagedAssetDigest != "sha256:abc123" {
		t.Errorf("ManagedAssetDigest = %q", got.ManagedAssetDigest)
	}
}

func TestSave_WritesDocumentedTopLevelKeys(t *testing.T) {
	home := isolateHome(t)

	if err := Save(fullState()); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(home, ".jarvis", "state.yaml"))
	if err != nil {
		t.Fatalf("read state.yaml: %v", err)
	}
	raw := map[string]any{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal state.yaml: %v", err)
	}

	for _, key := range []string{
		"schema_version",
		"installed_agents",
		"selection_configured",
		"skills",
		"persona",
		"persona_source",
		"statusline_decided",
		"statusline_enabled",
		"phase_models",
		"scope",
		"managed_asset_digest",
	} {
		if _, ok := raw[key]; !ok {
			t.Errorf("state.yaml is missing top-level key %q; got keys %v", key, sortedKeys(raw))
		}
	}
}

// A dropped skill ID is the only ownership proof that lets a later replay delete
// that skill's directory. Filtering the list against the current embedded catalog
// on write would destroy that proof, so the writer must never filter.
func TestSave_RetainsSkillIDsAbsentFromCurrentCatalog(t *testing.T) {
	isolateHome(t)

	st := fullState()
	st.Skills = []string{"go-testing", "retired-skill-no-longer-in-catalog"}
	if err := Save(st); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if strings.Join(got.Skills, ",") != "go-testing,retired-skill-no-longer-in-catalog" {
		t.Fatalf("Skills = %#v, want the catalog-absent ID retained", got.Skills)
	}
}

func TestSave_WritesOwnerOnlyPermissions(t *testing.T) {
	home := isolateHome(t)

	if err := Save(fullState()); err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Stat(filepath.Join(home, ".jarvis", "state.yaml"))
	if err != nil {
		t.Fatalf("stat state.yaml: %v", err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("state.yaml mode = %v, want 0600", got)
	}
}

func TestStatuslineResolution(t *testing.T) {
	tests := []struct {
		name       string
		statusline StatuslineState
		manage     bool
	}{
		{
			name:       "never asked leaves the statusline untouched",
			statusline: StatuslineState{Decided: false, Enabled: false},
			manage:     false,
		},
		{
			name:       "enabled without a decision is still never asked",
			statusline: StatuslineState{Decided: false, Enabled: true},
			manage:     false,
		},
		{
			name:       "decided disabled leaves the statusline untouched",
			statusline: StatuslineState{Decided: true, Enabled: false},
			manage:     false,
		},
		{
			name:       "decided enabled authorizes the statusline",
			statusline: StatuslineState{Decided: true, Enabled: true},
			manage:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.statusline.ShouldManage(); got != tt.manage {
				t.Errorf("ShouldManage() = %v, want %v", got, tt.manage)
			}
		})
	}
}

func TestLoad_MissingManifestIsAcceptableOnAFreshMachine(t *testing.T) {
	isolateHome(t)

	got, err := Load()
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Load error = %v, want ErrNotFound", err)
	}
	if got != nil {
		t.Fatalf("Load returned %#v, want nil state alongside ErrNotFound", got)
	}
}

func TestLoad_FailsClosed(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{
			name:    "corrupt yaml",
			content: "schema_version: 1\n  installed_agents: [\n",
			wantErr: "parse",
		},
		{
			name:    "whitespace only file",
			content: "   \n\t\n",
			wantErr: "empty",
		},
		{
			name:    "missing schema version",
			content: "skills: [go-testing]\n",
			wantErr: "schema_version",
		},
		{
			name:    "incompatible future schema version",
			content: "schema_version: 99\nskills: []\n",
			wantErr: "schema_version",
		},
		{
			name:    "whitespace only persona",
			content: "schema_version: 1\npersona: \"   \"\n",
			wantErr: "persona",
		},
		{
			name:    "whitespace only skill id",
			content: "schema_version: 1\nskills:\n  - \"  \"\n",
			wantErr: "skills",
		},
		{
			name:    "whitespace only agent id",
			content: "schema_version: 1\ninstalled_agents:\n  - id: \"  \"\n",
			wantErr: "installed_agents",
		},
		{
			name:    "unrecognized scope",
			content: "schema_version: 1\nscope: galactic\n",
			wantErr: "scope",
		},
		{
			name:    "unrecognized persona source",
			content: "schema_version: 1\npersona: argentino\npersona_source: telepathy\n",
			wantErr: "persona_source",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := isolateHome(t)
			path := filepath.Join(home, ".jarvis", "state.yaml")
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			if err := os.WriteFile(path, []byte(tt.content), 0600); err != nil {
				t.Fatalf("write state.yaml: %v", err)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read back: %v", err)
			}

			got, err := Load()
			if err == nil {
				t.Fatalf("Load succeeded with %#v, want a fail-closed error", got)
			}
			if got != nil {
				t.Errorf("Load returned %#v alongside an error, want nil", got)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not mention %q", err, tt.wantErr)
			}

			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read after: %v", err)
			}
			if string(before) != string(after) {
				t.Errorf("Load mutated state.yaml; before=%q after=%q", before, after)
			}
		})
	}
}

func TestLoad_ReadErrorAborts(t *testing.T) {
	home := isolateHome(t)

	// A directory where the manifest is expected is a read error, not a missing file.
	path := filepath.Join(home, ".jarvis", "state.yaml")
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	got, err := Load()
	if err == nil {
		t.Fatalf("Load succeeded with %#v, want a read error", got)
	}
	if errors.Is(err, ErrNotFound) {
		t.Fatal("read error was reported as ErrNotFound; a fresh machine and an unreadable manifest must differ")
	}
}

func TestValidateForReplay_BlocksOnMissingAgentsList(t *testing.T) {
	st := fullState()
	st.InstalledAgents = nil

	err := st.ValidateForReplay()
	if err == nil {
		t.Fatal("ValidateForReplay succeeded with no agents, want a blocking error")
	}
	if !strings.Contains(err.Error(), "installed_agents") {
		t.Errorf("error %q does not name the missing agents list", err)
	}

	if err := fullState().ValidateForReplay(); err != nil {
		t.Fatalf("ValidateForReplay on a complete manifest: %v", err)
	}
}

func sortedKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
