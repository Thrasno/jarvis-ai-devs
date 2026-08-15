package agent

import (
	"errors"
	"strings"
	"testing"

	jarvis "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/config"
)

func composeFor(t *testing.T, agentID string, existing []byte) string {
	t.Helper()
	content, err := ComposeInstructions(agentID, jarvis.TemplatesFS, existing, "layer one", "layer two",
		[]config.SkillInfo{{Name: "sdd-apply", Description: "implements tasks", Trigger: "apply"}})
	if err != nil {
		t.Fatalf("ComposeInstructions(%s): %v", agentID, err)
	}
	return content
}

// The composition is complete: what comes back is the whole file, injections
// included, and not a template render some caller is expected to finish. A
// caller that had to add a step would be a second place that knows the assembly,
// which is the arrangement this function exists to remove.
//
// The per-agent difference is asserted from both sides. Claude carries the
// orchestrator @import and OpenCode does not, so an assertion that only checked
// Claude would pass for a function that injected the import into every agent.
func TestComposeInstructions_ReturnsTheCompleteFileForEachAgent(t *testing.T) {
	claude := composeFor(t, "claude", nil)
	openCode := composeFor(t, "opencode", nil)

	for id, content := range map[string]string{"claude": claude, "opencode": openCode} {
		for _, want := range []string{Layer1Start, "layer one", Layer2Start, "layer two", HiveProtocolStart, HiveProtocolEnd} {
			if !strings.Contains(content, want) {
				t.Errorf("%s instructions are missing %q", id, want)
			}
		}
	}
	if !strings.Contains(claude, OrchestratorImportStart) {
		t.Error("Claude instructions must carry the orchestrator @import block")
	}
	if strings.Contains(openCode, OrchestratorImportStart) {
		t.Error("OpenCode instructions must not carry the orchestrator @import block")
	}
}

// Idempotence is the property replay depends on, so it is asserted rather than
// assumed: composing an already-composed file must reproduce it byte for byte.
// Without it a machine could never measure as converged -- every run would
// rewrite its own instruction file with different bytes and report drift -- and
// the planner's digest would describe only the first write.
func TestComposeInstructions_RecomposingItsOwnOutputChangesNothing(t *testing.T) {
	for _, id := range []string{"claude", "opencode"} {
		t.Run(id, func(t *testing.T) {
			once := composeFor(t, id, nil)
			twice := composeFor(t, id, []byte(once))

			if once != twice {
				t.Fatalf("recomposing changed the file: %d bytes became %d", len(once), len(twice))
			}
			// A file that grew a second protocol block would still differ above, but
			// only by length; naming the duplication says which way it broke.
			if got := strings.Count(twice, HiveProtocolStart); got != 1 {
				t.Fatalf("the recomposed file carries %d Hive protocol blocks, want 1", got)
			}
		})
	}
}

// The three ways the existing file decides the base content. existing is data,
// not a mode switch: the same call handles all three, so no caller chooses a
// branch and no caller can choose a different one from its sibling.
func TestComposeInstructions_DerivesTheBaseFromWhatTheFileHoldsToday(t *testing.T) {
	const userProse = "# my own notes\nkeep this line\n"

	fresh := composeFor(t, "claude", nil)
	if !strings.Contains(fresh, "layer one") {
		t.Fatal("an absent file must be rendered fresh from the template")
	}

	// Sentinels present: the blocks are patched and everything outside them
	// survives, which is what makes a user's own prose safe.
	sentinelled := userProse + fresh
	patched := composeFor(t, "claude", []byte(sentinelled))
	if !strings.Contains(patched, "keep this line") {
		t.Error("content outside the sentinel blocks must survive a patch")
	}

	// No sentinels: foreign content is replaced rather than patched, because
	// there is no Jarvis block in it to patch.
	replaced := composeFor(t, "claude", []byte(userProse))
	if strings.Contains(replaced, "keep this line") {
		t.Error("a file carrying no Jarvis sentinels must be replaced, not merged")
	}
	if !strings.Contains(replaced, "layer one") {
		t.Error("the replacement must be a freshly rendered Jarvis file")
	}
}

// An agent this binary embeds no template for is refused through a sentinel, so
// a caller can recognize the class and name its own recovery. Replay meets this
// through a manifest recording an agent a later version dropped.
func TestComposeInstructions_RefusesAnAgentItEmbedsNoTemplateFor(t *testing.T) {
	content, err := ComposeInstructions("cursor", jarvis.TemplatesFS, nil, "l1", "l2", nil)

	if !errors.Is(err, ErrNoInstructionTemplate) {
		t.Fatalf("error = %v, want ErrNoInstructionTemplate", err)
	}
	if content != "" {
		t.Fatalf("a refused composition returned %d bytes, want none", len(content))
	}
	if !strings.Contains(err.Error(), "cursor") {
		t.Errorf("error %q does not name the agent it refused", err)
	}
}

// The agent ID is keyed the way every other agent-keyed table in this tree keys
// it. A manifest that records "Claude" must not be refused as an agent this
// binary has no template for.
func TestComposeInstructions_KeysTheAgentIdentifierLikeEverySiteThatResolvesOne(t *testing.T) {
	canonical := composeFor(t, "claude", nil)

	for _, spelling := range []string{"CLAUDE", "  Claude\t"} {
		if got := composeFor(t, spelling, nil); got != canonical {
			t.Errorf("ComposeInstructions(%q) composed a different file than %q", spelling, "claude")
		}
	}
}
