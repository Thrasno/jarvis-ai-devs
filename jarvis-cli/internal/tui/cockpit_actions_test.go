package tui

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCockpitActions_ApprovedRegistryMembershipAndOrder(t *testing.T) {
	actions := CockpitActions()

	got := make([]struct {
		ID    CockpitActionID
		Label string
	}, 0, len(actions))
	for _, action := range actions {
		got = append(got, struct {
			ID    CockpitActionID
			Label string
		}{ID: action.ID, Label: action.Label})
	}

	want := []struct {
		ID    CockpitActionID
		Label string
	}{
		{ID: CockpitActionInstall, Label: "Install/Reconfigure"},
		{ID: CockpitActionPersona, Label: "Persona"},
		{ID: CockpitActionConfig, Label: "Config"},
		{ID: CockpitActionHiveCloudLogin, Label: "Hive Cloud Login"},
		{ID: CockpitActionDoctor, Label: "Doctor"},
		{ID: CockpitActionVerify, Label: "Verify"},
		{ID: CockpitActionBackup, Label: "Backup"},
		{ID: CockpitActionRestore, Label: "Restore"},
		{ID: CockpitActionReconcile, Label: "Reconcile"},
		{ID: CockpitActionUninstall, Label: "Uninstall"},
		{ID: CockpitActionExit, Label: "Exit"},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected cockpit registry membership/order:\n got: %#v\nwant: %#v", got, want)
	}

	for _, action := range actions {
		id := string(action.ID)
		label := strings.ToLower(action.Label)
		if strings.Contains(id, "sync") || strings.Contains(label, "sync") {
			t.Fatalf("cockpit registry must exclude sync, found action %#v", action)
		}
		if strings.Contains(id, "timeline") || strings.Contains(label, "timeline") {
			t.Fatalf("cockpit registry must exclude timeline, found action %#v", action)
		}
	}
}

func TestCockpitActions_DispatchContractIsStableAndExtensible(t *testing.T) {
	actions := CockpitActions()
	extended := append(actions, CockpitAction{
		ID:          CockpitActionID("future-action"),
		Label:       "Future Action",
		Description: "Future cockpit extension.",
		Kind:        CockpitActionKindReadOnly,
		Danger:      CockpitDangerNone,
	})

	existing, ok := CockpitActionByID(extended, CockpitActionDoctor)
	if !ok {
		t.Fatalf("expected doctor action to remain discoverable after extension")
	}
	if existing.Label != "Doctor" || existing.Kind != CockpitActionKindReadOnly || existing.Danger != CockpitDangerNone {
		t.Fatalf("doctor dispatch contract changed after extension: %#v", existing)
	}

	future, ok := CockpitActionByID(extended, CockpitActionID("future-action"))
	if !ok {
		t.Fatalf("expected future action to be discoverable through the same dispatch contract")
	}
	if future.Label != "Future Action" || future.Kind != CockpitActionKindReadOnly || future.Danger != CockpitDangerNone {
		t.Fatalf("future action dispatch contract not preserved: %#v", future)
	}
}

func TestCockpitLogo_LoadsEmbeddedTextAssetCopiedFromDesignSource(t *testing.T) {
	source, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "design", "nexus-logo-braille-64col.txt"))
	if err != nil {
		t.Fatalf("read design source logo: %v", err)
	}

	got := CockpitLogo()
	if got != string(source) {
		t.Fatalf("embedded cockpit logo must match design source asset")
	}
	if strings.Contains(got, "\x1b]") {
		t.Fatalf("cockpit logo must be plain text, got terminal image/control sequence")
	}
}
