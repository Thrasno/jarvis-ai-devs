package agent

import (
	"path/filepath"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/state"
)

func isolateTestHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	setTestHome(t, home)
	return home
}

func setTestHome(t *testing.T, home string) {
	t.Helper()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))
}

// stubRuntimePhaseModels stands in for the desired-state manifest the runtime
// verification falls back to when no pending assignments are passed.
// ~/.jarvis/state.yaml owns those assignments.
func stubRuntimePhaseModels(t *testing.T, models state.PhaseModels) {
	t.Helper()
	previous := loadDesiredPhaseModels
	loadDesiredPhaseModels = func() (state.PhaseModels, error) { return models, nil }
	t.Cleanup(func() { loadDesiredPhaseModels = previous })
}

// phaseModelsPtr wraps assignments in the pending-assignment pointer the runtime
// observers take. A nil pointer still means "no pending assignments", so tests
// keep the nil/non-nil distinction they rely on.
func phaseModelsPtr(models state.PhaseModels) *state.PhaseModels {
	return &models
}

// defaultRuntimePhaseModels is a manifest that records no assignment at all, so
// every phase resolves to its contract default.
func defaultRuntimePhaseModels() state.PhaseModels {
	return state.New().PhaseModels
}
