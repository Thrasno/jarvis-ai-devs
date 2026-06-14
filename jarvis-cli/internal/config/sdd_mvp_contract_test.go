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

func TestMVPContract_Orchestrator_HiveArtifactGuidanceAvoidsUnsupportedPersistenceGuarantees(t *testing.T) {
	orchestrator := strings.ToLower(readFileForMVP(t, "embed/orchestrator/sdd-orchestrator.md"))

	for _, required := range []string{
		"topic keys group related sdd artifact saves; they are not identity, recency, overwrite, or version guarantees.",
		"if hive search returns multiple candidate artifacts for the same topic and no explicit artifact reference is available, treat the result as ambiguous.",
	} {
		if !strings.Contains(orchestrator, required) {
			t.Fatalf("orchestrator hive artifact guidance must contain %q", required)
		}
	}

	for _, forbidden := range []string{
		"re-running a phase overwrites",
		"no history",
		"topic_key ensures upserts",
		"ensures upserts",
		"running again updates the same observation",
		"latest returned observation",
		"latest-by-topic",
		"authoritative version",
		"most recent, which is the authoritative version",
	} {
		if strings.Contains(orchestrator, forbidden) {
			t.Fatalf("orchestrator hive artifact guidance must not promise unsupported persistence behavior with %q", forbidden)
		}
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

func TestMVPContract_HiveProtocol_TopicKeyGuidanceAvoidsUnsupportedLatestRetrievalGuarantees(t *testing.T) {
	protocol := strings.ToLower(readFileForMVP(t, "embed/hive-protocol.md"))

	for _, required := range []string{
		"reuse the same `topic_key` to group related observations",
		"if search returns multiple candidates",
		"treat retrieval as ambiguous",
		"explicit observation id or artifact reference",
	} {
		if !strings.Contains(protocol, required) {
			t.Fatalf("hive protocol topic_key guidance must contain ambiguity-safe wording %q", required)
		}
	}

	for _, forbidden := range []string{
		"retrieval returns the most recent row",
		"retrieval returns the most recent",
		"most recent row",
		"latest returned observation",
		"latest-by-topic",
		"retrieve the most recent version",
		"phases retrieve the most recent version",
	} {
		if strings.Contains(protocol, forbidden) {
			t.Fatalf("hive protocol topic_key guidance must not promise unsupported latest retrieval with %q", forbidden)
		}
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
