package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A manifest that is absent because a migration failed is not the same machine
// as one that is absent because nothing was ever installed. Update creates a
// fresh manifest for the second, and must refuse the first: the fresh manifest
// it would write carries only the field this writer touched, and Migrate's
// regular-file gate then early-returns forever, stranding the persona, skills,
// agents, scope and phase models in a config.yaml nothing reads any more.
func TestUpdate_RefusesWhileConfigYamlStillCarriesUnmigratedState(t *testing.T) {
	home := isolateHome(t)
	writeConfig(t, home, configV2Fixture)

	err := Update(func(st *State) { st.Scope = ScopeLocalCloud })
	if err == nil {
		t.Fatal("Update wrote a manifest while config.yaml still carried unmigrated installation state")
	}
	if !strings.Contains(err.Error(), configFileName) {
		t.Errorf("error = %v; it must name %s so the user can see what still holds their state", err, configFileName)
	}

	statePath, pathErr := Path()
	if pathErr != nil {
		t.Fatalf("Path: %v", pathErr)
	}
	if _, statErr := os.Stat(statePath); !os.IsNotExist(statErr) {
		t.Fatalf("stat %s = %v; the refused write must leave no manifest behind", stateFileName, statErr)
	}
}

// The refusal is scoped to an unfinished migration. A machine whose config.yaml
// holds no replay field at all is a genuinely fresh one, and the first writer
// still gets its manifest.
func TestUpdate_StillCreatesTheManifestWhenConfigYamlCarriesNoReplayField(t *testing.T) {
	home := isolateHome(t)
	writeConfig(t, home, "schema_version: 3\napi_url: https://hivemem.dev\n")

	if err := Update(func(st *State) { st.Scope = ScopeLocalCloud }); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Scope != ScopeLocalCloud {
		t.Errorf("scope = %q, want %q", got.Scope, ScopeLocalCloud)
	}
}

// A machine with no config.yaml at all has nothing to migrate, so the first
// writer is never blocked on one.
func TestUpdate_StillCreatesTheManifestWithoutAConfigFile(t *testing.T) {
	home := isolateHome(t)
	if _, err := os.Stat(filepath.Join(home, ".jarvis", configFileName)); !os.IsNotExist(err) {
		t.Fatalf("stat %s = %v; this test needs a machine with no config.yaml", configFileName, err)
	}

	if err := Update(func(st *State) { st.Persona = "gentleman" }); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Persona != "gentleman" {
		t.Errorf("persona = %q, want gentleman", got.Persona)
	}
}

// Once the migration has actually moved the replay fields, the manifest exists
// and every writer works normally against it.
func TestUpdate_WritesNormallyAfterASuccessfulMigration(t *testing.T) {
	home := isolateHome(t)
	writeConfig(t, home, configV2Fixture)

	if _, err := Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if err := Update(func(st *State) { st.Scope = ScopeLocalOnly }); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Scope != ScopeLocalOnly {
		t.Errorf("scope = %q, want %q", got.Scope, ScopeLocalOnly)
	}
	if got.Persona != "gentleman" {
		t.Errorf("persona = %q; the migrated state must survive the later write", got.Persona)
	}
}

// The other way the manifest cannot be read is a manifest that is there and
// unreadable. Update must refuse that one too, and leave the bytes it could not
// understand exactly where they are for a human to look at.
func TestUpdate_RefusesWhenTheExistingManifestCannotBeRead(t *testing.T) {
	home := isolateHome(t)
	statePath := filepath.Join(home, ".jarvis", stateFileName)
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		t.Fatalf("mkdir .jarvis: %v", err)
	}
	const damaged = "schema_version: 3\npersona_source: telepathy\n"
	if err := os.WriteFile(statePath, []byte(damaged), 0o600); err != nil {
		t.Fatalf("write %s: %v", stateFileName, err)
	}

	if err := Update(func(st *State) { st.Persona = "gentleman" }); err == nil {
		t.Fatal("Update overwrote a manifest it could not read")
	}

	after, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read %s: %v", stateFileName, err)
	}
	if string(after) != damaged {
		t.Errorf("%s = %q; the refused write must leave the file untouched", stateFileName, after)
	}
}
