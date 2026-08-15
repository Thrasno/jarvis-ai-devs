package state

import (
	"errors"
	"os"
	"reflect"
	"testing"
)

// LoadWithoutMigrating is what a read-only observation reads. It must answer
// with the same desired state a migration would record, without creating
// state.yaml or rewriting config.yaml: observing is not installing.
func TestLoadWithoutMigrating_DerivesTheLegacyConfigWithoutWritingAnything(t *testing.T) {
	home := isolateHome(t)
	configPath := writeConfig(t, home, configV2Fixture)
	before, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config.yaml: %v", err)
	}

	derived, err := LoadWithoutMigrating()
	if err != nil {
		t.Fatalf("LoadWithoutMigrating on a pre-migration machine: %v", err)
	}

	statePath, err := Path()
	if err != nil {
		t.Fatalf("state path: %v", err)
	}
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Errorf("state.yaml must not be created by a read, stat err = %v", err)
	}
	after, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("re-read config.yaml: %v", err)
	}
	if string(after) != string(before) {
		t.Errorf("config.yaml was rewritten by a read:\n%s", after)
	}

	// The answer must match what the machine would replay after migrating.
	if _, err := Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	migrated, err := Load()
	if err != nil {
		t.Fatalf("Load after migrating: %v", err)
	}
	if !reflect.DeepEqual(derived.NormalizedPhaseModels(), migrated.NormalizedPhaseModels()) {
		t.Errorf("phase models read-only = %+v, after migrating = %+v", derived.NormalizedPhaseModels(), migrated.NormalizedPhaseModels())
	}
	if derived.Persona != migrated.Persona || derived.PersonaSource != migrated.PersonaSource {
		t.Errorf("persona read-only = (%q, %q), after migrating = (%q, %q)", derived.Persona, derived.PersonaSource, migrated.Persona, migrated.PersonaSource)
	}
	if derived.Scope != migrated.Scope {
		t.Errorf("scope read-only = %q, after migrating = %q", derived.Scope, migrated.Scope)
	}
}

// A machine that already migrated reads the manifest, not the legacy config.
func TestLoadWithoutMigrating_PrefersAnExistingManifest(t *testing.T) {
	home := isolateHome(t)
	writeConfig(t, home, configV2Fixture)
	recorded := New()
	recorded.Persona = "recorded-by-the-manifest"
	if err := Save(recorded); err != nil {
		t.Fatalf("save manifest: %v", err)
	}

	got, err := LoadWithoutMigrating()
	if err != nil {
		t.Fatalf("LoadWithoutMigrating: %v", err)
	}
	if got.Persona != "recorded-by-the-manifest" {
		t.Errorf("persona = %q, want the manifest's value", got.Persona)
	}
}

// With neither store on disk there is nothing to read, and that is the same
// "no manifest yet" answer Load reports.
func TestLoadWithoutMigrating_ReportsNotFoundWithNeitherStore(t *testing.T) {
	isolateHome(t)

	if _, err := LoadWithoutMigrating(); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
