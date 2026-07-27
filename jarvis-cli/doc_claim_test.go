package jarvis_test

import (
	"strings"
	"testing"

	jarvis "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"
)

// falseRegistrationClaims are phrases that assert — incorrectly — that calling
// mem_context is what registers a project with Hive. Registration is actually a
// side effect of the SessionStart hook and of self-healing writes (mem_save /
// mem_session_summary with a directory), never of mem_context. These phrases
// must never reappear in the shipped memory-protocol docs.
var falseRegistrationClaims = []string{
	"registers the current project",
	"register the current project",
	"mem_context` registers",
	"mem_context registers",
}

// TestHiveProtocol_NoFalseRegistrationClaim guards the embedded hive-protocol.md
// against reintroducing the false claim that mem_context registers the project
// (spec: Documentation Reflects Actual Registration Behavior).
func TestHiveProtocol_NoFalseRegistrationClaim(t *testing.T) {
	assertNoFalseRegistrationClaim(t, "embed/hive-protocol.md", jarvis.HiveProtocol)

	// The doc must positively describe the real registration mechanism so the
	// fix is present, not merely the false claim removed.
	if !strings.Contains(jarvis.HiveProtocol, "SessionStart hook") {
		t.Error("hive-protocol.md must describe the SessionStart hook as the registration mechanism")
	}
}

// TestHiveSkill_NoFalseRegistrationClaim guards the embedded hive SKILL.md the
// same way.
func TestHiveSkill_NoFalseRegistrationClaim(t *testing.T) {
	data, err := jarvis.SkillsFS.ReadFile("embed/skills/hive/SKILL.md")
	if err != nil {
		t.Fatalf("embed/skills/hive/SKILL.md not readable: %v", err)
	}
	assertNoFalseRegistrationClaim(t, "embed/skills/hive/SKILL.md", string(data))
}

func TestHiveContracts_DoNotPressureSessionClosure(t *testing.T) {
	skill, err := jarvis.SkillsFS.ReadFile("embed/skills/hive/SKILL.md")
	if err != nil {
		t.Fatalf("embed/skills/hive/SKILL.md not readable: %v", err)
	}

	for name, content := range map[string]string{
		"embed/hive-protocol.md":     jarvis.HiveProtocol,
		"embed/skills/hive/SKILL.md": string(skill),
	} {
		lower := strings.ToLower(content)
		for _, required := range []string{
			"never recommend, suggest, vote for, or pressure the user to end a session",
			"session length",
			"time since the last memory save",
			"only when the user actually ends the session",
			"after compaction",
		} {
			if !strings.Contains(lower, required) {
				t.Errorf("%s must contain session-close invariant %q", name, required)
			}
		}
	}
}

func assertNoFalseRegistrationClaim(t *testing.T, name, content string) {
	t.Helper()
	lower := strings.ToLower(content)
	for _, claim := range falseRegistrationClaims {
		if strings.Contains(lower, strings.ToLower(claim)) {
			t.Errorf("%s must not claim mem_context registers the project (found %q); "+
				"registration happens via the SessionStart hook and self-healing writes", name, claim)
		}
	}
}
