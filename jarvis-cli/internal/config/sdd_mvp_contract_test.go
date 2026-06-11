package config

import (
	"strings"
	"testing"
)

func TestMVPContract_Orchestrator_UsesSupportedArtifactStoreModesOnly(t *testing.T) {
	orchestrator := strings.ToLower(readFileForMVP(t, "embed/orchestrator/sdd-orchestrator.md"))

	for _, mode := range []string{"hive", "openspec", "hybrid"} {
		if !strings.Contains(orchestrator, mode) {
			t.Fatalf("orchestrator must document %q artifact store mode", mode)
		}
	}

	if strings.Contains(orchestrator, "artifact store") && !strings.Contains(orchestrator, "none") {
		t.Fatalf("orchestrator artifact-store section must still document none fallback behavior")
	}
}

func TestMVPContract_Layer1_IsBehaviorOnly(t *testing.T) {
	layer1 := strings.ToLower(readFileForMVP(t, "internal/config/layer1.md"))

	for _, forbidden := range []string{"## expertise", "## philosophy", "## pair programmer", "## personality"} {
		if strings.Contains(layer1, forbidden) {
			t.Fatalf("layer1 must stay behavior-only; found forbidden section %q", forbidden)
		}
	}

	if !strings.Contains(layer1, "canonical source") || !strings.Contains(layer1, "orchestrator") {
		t.Fatalf("layer1 must defer runtime authority to orchestrator canonical source")
	}
}

func TestMVPContract_HiveProtocol_DocumentsSDDBoundary(t *testing.T) {
	protocol := strings.ToLower(readFileForMVP(t, "embed/hive-protocol.md"))

	if !strings.Contains(protocol, "sdd/") || !strings.Contains(protocol, "artifact") {
		t.Fatalf("hive protocol must explicitly describe reserved sdd artifact topic namespace")
	}
	if !strings.Contains(protocol, "general") || !strings.Contains(protocol, "must not") {
		t.Fatalf("hive protocol must state that general memory must not overwrite reserved sdd artifact topics")
	}
}

func TestMVPContract_HiveProtocol_UsesActivityBasedReminderNotAgentTimer(t *testing.T) {
	protocol := strings.ToLower(readFileForMVP(t, "embed/hive-protocol.md"))

	for _, required := range []string{
		"automatic mcp nudge",
		"5 tool calls",
		"no agent-side 15-minute timer",
		"noisy timers",
	} {
		if !strings.Contains(protocol, required) {
			t.Fatalf("hive protocol reminder guidance must include %q", required)
		}
	}
}

func readFileForMVP(t *testing.T, rel string) string {
	t.Helper()
	return readConfigTestFile(t, rel)
}
