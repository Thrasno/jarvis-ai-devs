package tui

import (
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/persona"
)

// A manifest the cockpit could not read is not an empty manifest. Both of these
// actions write files -- persona rewrites every agent's instructions and output
// styles, login writes sync credentials -- before the state.Update inside them
// refuses. That order leaves the machine's rendered files disagreeing with the
// manifest `jarvis sync` replays from, and reports failure anyway. The guard has
// to fire before the first file is written.
func TestCockpitPersona_RefusesBeforeWritingWhenTheManifestCouldNotBeRead(t *testing.T) {
	runner := &fakeCockpitRunner{personaSummary: "persona argentino applied to 1 agent"}
	m := newCockpitHandlerTestModel(runner)
	m.manifestErr = errors.New("load the desired-state manifest: state.yaml has incompatible schema_version 99, want 1")
	m.Presets = []persona.ProfileOption{{Name: "argentino", DisplayName: "Argentino"}}
	m.presetCur = 0
	m = selectCockpitAction(t, m, CockpitActionPersona)
	m = sendCockpitKey(m, tea.KeyEnter)

	if len(runner.calls) != 0 {
		t.Fatalf("persona wrote agent files over an unreadable manifest: %v", runner.calls)
	}
	assertViewContains(t, m.View(), "Persona error", "schema_version", "Nothing was written")
}

func TestCockpitLogin_RefusesBeforeWritingWhenTheManifestCouldNotBeRead(t *testing.T) {
	runner := &fakeCockpitRunner{loginEmail: "resolved@example.com"}
	m := newCockpitHandlerTestModel(runner)
	m.manifestErr = errors.New("load the desired-state manifest: state.yaml has incompatible schema_version 99, want 1")
	m = selectCockpitAction(t, m, CockpitActionHiveCloudLogin)
	m.Email = ""
	m.Password = ""
	m = typeCockpitText(m, "input@example.com")
	m = sendCockpitKey(m, tea.KeyEnter)
	m = typeCockpitText(m, "secret")
	m = sendCockpitKey(m, tea.KeyEnter)

	if len(runner.calls) != 0 {
		t.Fatalf("login wrote sync credentials over an unreadable manifest: %v", runner.calls)
	}
	assertViewContains(t, m.View(), "Hive Cloud Login error", "schema_version", "Nothing was written")
}
