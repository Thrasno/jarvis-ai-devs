package state

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
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
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no POSIX permission bits: Chmod only toggles the read-only attribute and Perm always reads 0666 or 0444")
	}

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

// TestNormalizedPhaseModels_ReproducesTheNormalizationConfigLoadApplied covers
// the read every phase-model consumer now performs. The manifest stores what was
// written -- migration copies config.yaml verbatim -- while consumers look
// phases up by the SDD contract's names, so a hand-edited `Apply:` key must
// still match `apply`, exactly as config.Load's normalization made it.
func TestNormalizedPhaseModels_ReproducesTheNormalizationConfigLoadApplied(t *testing.T) {
	manifest := New()
	manifest.PhaseModels.Aliases[" Apply "] = PhaseModelSelection{OpenCode: " Sonnet ", Claude: " OPUS "}
	manifest.PhaseModels.OpenCode["  VERIFY"] = OpenCodeModelAssignment{ProviderID: " openai ", ModelID: " gpt ", Effort: " high "}
	manifest.PhaseModels.Claude["Spec  "] = ClaudeModelAssignment{Model: " haiku ", Effort: " max "}
	manifest.PhaseModels.Aliases["   "] = PhaseModelSelection{OpenCode: "dropped"}

	got := manifest.NormalizedPhaseModels()

	if sel := got.Aliases["apply"]; sel.OpenCode != "sonnet" || sel.Claude != "opus" {
		t.Errorf("aliases[apply] = %+v, want lowercased and trimmed values", sel)
	}
	if assignment := got.OpenCode["verify"]; assignment.ProviderID != "openai" || assignment.ModelID != "gpt" || assignment.Effort != "high" {
		t.Errorf("opencode[verify] = %+v, want trimmed values", assignment)
	}
	if assignment := got.Claude["spec"]; assignment.Model != "haiku" || assignment.Effort != "max" {
		t.Errorf("claude[spec] = %+v, want trimmed values", assignment)
	}
	if len(got.Aliases) != 1 {
		t.Errorf("aliases = %#v, want the unnamed phase dropped", got.Aliases)
	}

	if empty := (*State)(nil).NormalizedPhaseModels(); empty.Aliases == nil || empty.OpenCode == nil || empty.Claude == nil {
		t.Error("a nil manifest must still return usable empty maps")
	}
}

// TestResolvedPersona_AppliesTheDefaultsAnUnpopulatedManifestFallsBackTo pins
// the persona defaults down in the one place that now owns them. config.Load
// used to apply exactly this chain to the AppConfig fields the manifest
// replaced: an unrecorded persona reads as the built-in default, and any source
// that is not exactly "user" reads as "builtin".
func TestResolvedPersona_AppliesTheDefaultsAnUnpopulatedManifestFallsBackTo(t *testing.T) {
	tests := []struct {
		name       string
		manifest   *State
		wantSlug   string
		wantSource PersonaSource
	}{
		{name: "nil manifest", manifest: nil, wantSlug: DefaultPersona, wantSource: PersonaSourceBuiltin},
		{name: "empty manifest", manifest: New(), wantSlug: DefaultPersona, wantSource: PersonaSourceBuiltin},
		{
			name:       "recorded user persona",
			manifest:   &State{Persona: " neutra ", PersonaSource: " USER "},
			wantSlug:   "neutra",
			wantSource: PersonaSourceUser,
		},
		{
			name:       "unrecognized source reads as builtin",
			manifest:   &State{Persona: "neutra", PersonaSource: "nonsense"},
			wantSlug:   "neutra",
			wantSource: PersonaSourceBuiltin,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			slug, source := tt.manifest.ResolvedPersona()
			if slug != tt.wantSlug || source != tt.wantSource {
				t.Errorf("ResolvedPersona() = (%q, %q), want (%q, %q)", slug, source, tt.wantSlug, tt.wantSource)
			}
		})
	}
}

// TestResolvedScope_FallsBackThroughTheStoredCloudLink pins the scope default.
// config.Load defaulted an absent or unrecognized scope to local+cloud when the
// machine already had a stored cloud link and to local-only otherwise. The cloud
// link lives in config.yaml, which this package does not read, so it arrives as
// an argument.
func TestResolvedScope_FallsBackThroughTheStoredCloudLink(t *testing.T) {
	tests := []struct {
		name         string
		manifest     *State
		hasCloudLink bool
		want         Scope
	}{
		{name: "recorded scope wins over the cloud link", manifest: &State{Scope: ScopeLocalOnly}, hasCloudLink: true, want: ScopeLocalOnly},
		{name: "recorded local+cloud", manifest: &State{Scope: ScopeLocalCloud}, want: ScopeLocalCloud},
		{name: "unset with a cloud link", manifest: New(), hasCloudLink: true, want: ScopeLocalCloud},
		{name: "unset without a cloud link", manifest: New(), want: ScopeLocalOnly},
		{name: "unrecognized with a cloud link", manifest: &State{Scope: "nonsense"}, hasCloudLink: true, want: ScopeLocalCloud},
		{name: "nil manifest", manifest: nil, want: ScopeLocalOnly},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.manifest.ResolvedScope(tt.hasCloudLink); got != tt.want {
				t.Errorf("ResolvedScope(%t) = %q, want %q", tt.hasCloudLink, got, tt.want)
			}
		})
	}
}

// TestRecordsCompleteInstall_RequiresEveryPartTheManifestOwns covers the half of
// the reconfigure-readiness question this store can answer. config.yaml owns the
// other half.
func TestRecordsCompleteInstall_RequiresEveryPartTheManifestOwns(t *testing.T) {
	complete := func() *State {
		manifest := New()
		manifest.Persona = "neutra"
		manifest.Skills = []string{"go-testing"}
		manifest.InstalledAgents = []Agent{{ID: "claude", InstructionsPath: "/i", ConfigPath: "/c"}}
		return manifest
	}

	if !complete().RecordsCompleteInstall() {
		t.Fatal("a manifest with a persona, a skill and an agent records a complete install")
	}

	noPersona := complete()
	noPersona.Persona = ""
	noSkills := complete()
	noSkills.Skills = nil
	noAgents := complete()
	noAgents.InstalledAgents = nil

	for name, manifest := range map[string]*State{
		"nil":        nil,
		"empty":      New(),
		"no persona": noPersona,
		"no skills":  noSkills,
		"no agents":  noAgents,
	} {
		if manifest.RecordsCompleteInstall() {
			t.Errorf("%s must not record a complete install", name)
		}
	}
}

// TestRecordsAnyState_TellsADamagedInstallFromAFreshMachine covers the signal
// behind the recover status. A persona equal to the built-in default is not a
// recorded choice: it is what an unpopulated manifest reads as.
func TestRecordsAnyState_TellsADamagedInstallFromAFreshMachine(t *testing.T) {
	if (*State)(nil).RecordsAnyState() || New().RecordsAnyState() {
		t.Fatal("nothing recorded must not read as recorded state")
	}

	defaulted := New()
	defaulted.Persona = DefaultPersona
	if defaulted.RecordsAnyState() {
		t.Error("the built-in default persona is not a recorded choice")
	}

	chosen := New()
	chosen.Persona = "neutra"
	withSkills := New()
	withSkills.Skills = []string{"go-testing"}
	withAgents := New()
	withAgents.InstalledAgents = []Agent{{ID: "claude", InstructionsPath: "/i", ConfigPath: "/c"}}
	withSelection := New()
	withSelection.SelectionConfigured = true

	for name, manifest := range map[string]*State{
		"chosen persona":     chosen,
		"selected skills":    withSkills,
		"configured agent":   withAgents,
		"asked about agents": withSelection,
	} {
		if !manifest.RecordsAnyState() {
			t.Errorf("%s must read as recorded state", name)
		}
	}
}

// Two agents cannot own the same instruction file. Ownership downstream is a
// path-to-agent map, so the second record silently replaces the first: the loser
// can no longer write the file the manifest recorded for it, and the winner may
// write a file recorded for its sibling. Nothing further down can tell the two
// apart afterwards, so the collision is refused where the manifest is validated.
func TestValidate_RejectsTwoAgentsRecordingTheSameInstructionsPath(t *testing.T) {
	tests := []struct {
		name   string
		second string
	}{
		{name: "byte-identical paths", second: "/home/u/.claude/CLAUDE.md"},
		{name: "paths that differ only before cleaning", second: "/home/u/.claude/./skills/../CLAUDE.md"},
		{name: "paths that differ only by surrounding whitespace", second: "  /home/u/.claude/CLAUDE.md\t"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := fullState()
			st.InstalledAgents[1].InstructionsPath = tt.second

			err := st.Validate()
			if err == nil {
				t.Fatal("Validate accepted two agents owning one instruction file")
			}
			// Validate runs inside decode, so this refusal rejects a manifest that
			// loaded fine until now and blocks every command that reads it. It is
			// the one validation failure a user can reach without having just
			// written the file, so unlike its siblings it has to name the way out.
			for _, want := range []string{"instructions_path", "claude", "opencode", "run `jarvis` to reinstall"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not mention %q", err, want)
				}
			}
		})
	}

	// The rule rejects a collision, never a resemblance. The fixture's two paths
	// share nothing past the home directory, so re-validating it unchanged cannot
	// make that point; each pair below shares as much as two distinct paths can
	// and still names a different file.
	for _, tt := range []struct {
		name   string
		first  string
		second string
	}{
		{name: "the same directory", first: "/home/u/.claude/CLAUDE.md", second: "/home/u/.claude/AGENTS.md"},
		{name: "one agent's directory nested inside the other's", first: "/home/u/.claude/CLAUDE.md", second: "/home/u/.claude/nested/AGENTS.md"},
		{name: "one path a literal string prefix of the other", first: "/home/u/.claude/CLAUDE.md", second: "/home/u/.claude/CLAUDE.md.local"},
	} {
		t.Run("accepts two agents sharing "+tt.name, func(t *testing.T) {
			st := fullState()
			st.InstalledAgents[0].InstructionsPath = tt.first
			st.InstalledAgents[1].InstructionsPath = tt.second

			if err := st.Validate(); err != nil {
				t.Fatalf("Validate rejected %q and %q, which are distinct files: %v", tt.first, tt.second, err)
			}
		})
	}
}
