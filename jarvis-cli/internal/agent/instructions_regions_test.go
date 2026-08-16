package agent

import (
	"strings"
	"testing"

	jarvis "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"
)

// The regions are the file, minus whatever the user wrote around them. A
// composed file passed straight back must therefore lose its prose and keep
// every managed block, or the two halves of replay would still be comparing a
// document Jarvis only partly owns.
func TestExtractManagedRegions_KeepsEveryManagedBlockAndDropsEverythingElse(t *testing.T) {
	for _, id := range []string{"claude", "opencode"} {
		t.Run(id, func(t *testing.T) {
			composed := composeFor(t, id, nil)

			regions := ExtractManagedRegions(id, composed)

			for _, want := range []string{
				Layer1Start, "layer one", Layer1End,
				Layer2Start, "layer two", Layer2End,
				HiveProtocolStart, HiveProtocolEnd,
			} {
				if !strings.Contains(regions, want) {
					t.Errorf("%s regions are missing %q", id, want)
				}
			}
			if len(regions) >= len(composed) {
				t.Errorf("%s regions (%d bytes) must be a strict subset of the file (%d bytes)", id, len(regions), len(composed))
			}
		})
	}
}

// Claude alone carries the orchestrator @import, so the region set is asserted
// from both sides: an extraction that always looked for the block would report
// it absent for OpenCode, and one that never looked would stop noticing a
// tampered import on Claude.
func TestExtractManagedRegions_CoversTheOrchestratorImportForClaudeAlone(t *testing.T) {
	claude := ExtractManagedRegions("claude", composeFor(t, "claude", nil))
	openCode := ExtractManagedRegions("opencode", composeFor(t, "opencode", nil))

	if !strings.Contains(claude, OrchestratorImportStart) {
		t.Error("Claude regions must cover the orchestrator @import block")
	}
	if strings.Contains(openCode, OrchestratorImportStart) {
		t.Error("OpenCode regions must not cover an orchestrator @import block")
	}
}

// The property the fix rests on: content the user owns cannot move the regions.
// Prose before, between and after the blocks is exactly what a user adds to
// their own CLAUDE.md, and it is exactly what used to make every run measure a
// mismatch.
func TestExtractManagedRegions_IgnoresContentTheUserAddedOutsideTheMarkers(t *testing.T) {
	for _, id := range []string{"claude", "opencode"} {
		t.Run(id, func(t *testing.T) {
			composed := composeFor(t, id, nil)
			edited := "# My own notes\n\nRemember the standup.\n\n" + composed + "\n## More of my prose\n"

			if got, want := ExtractManagedRegions(id, edited), ExtractManagedRegions(id, composed); got != want {
				t.Errorf("prose outside the markers changed the managed regions:\n got %q\nwant %q", got, want)
			}
		})
	}
}

// The other direction, which must not be traded away for the fix: an edit
// inside a managed block is Jarvis's own content being tampered with, and the
// regions have to show it.
func TestExtractManagedRegions_ShowsAnEditInsideAManagedBlock(t *testing.T) {
	composed := composeFor(t, "opencode", nil)
	tampered := strings.Replace(composed, "layer two", "layer two, edited by hand", 1)
	if tampered == composed {
		t.Fatal("the fixture did not tamper with the Layer2 block")
	}

	if ExtractManagedRegions("opencode", tampered) == ExtractManagedRegions("opencode", composed) {
		t.Error("an edit inside a managed block must change the managed regions")
	}
}

// A truncated file is still a broken file. Losing a whole block, which is what
// a partial write or a user deleting a section leaves behind, must not measure
// as converged.
func TestExtractManagedRegions_ReportsAMissingOrTruncatedBlock(t *testing.T) {
	composed := composeFor(t, "opencode", nil)
	protocolStart := strings.Index(composed, HiveProtocolStart)
	if protocolStart == -1 {
		t.Fatal("the composed fixture carries no Hive protocol block")
	}

	for name, broken := range map[string]string{
		"empty file":      "",
		"truncated":       composed[:protocolStart],
		"markers deleted": strings.ReplaceAll(composed, Layer1Start, ""),
	} {
		t.Run(name, func(t *testing.T) {
			if ExtractManagedRegions("opencode", broken) == ExtractManagedRegions("opencode", composed) {
				t.Error("a file missing a managed block must not measure as the composed one")
			}
		})
	}
}

// An agent this binary embeds no composer for has no known region set, so there
// is nothing to narrow the comparison to. Falling back to the whole file keeps
// the strictest answer rather than inventing a lenient one.
func TestExtractManagedRegions_FallsBackToTheWholeFileForAnUnknownAgent(t *testing.T) {
	const content = "anything at all\n"
	if got := ExtractManagedRegions("cursor", content); got != content {
		t.Errorf("ExtractManagedRegions for an unknown agent = %q, want the whole content", got)
	}
}

// The agent identifier is keyed exactly the way every other site that resolves
// one keys it, so a manifest recording "Claude " is not silently demoted to the
// unknown-agent fallback.
func TestExtractManagedRegions_KeysTheAgentIdentifierLikeEverySiteThatResolvesOne(t *testing.T) {
	composed := composeFor(t, "claude", nil)

	if ExtractManagedRegions(" Claude ", composed) != ExtractManagedRegions("claude", composed) {
		t.Error("the agent identifier must be trimmed and lowercased like every other lookup")
	}
}

// The whole point of extracting from the composed content rather than from the
// template: what the planner digests and what the snapshot digests are produced
// by one function, so a composed file recomposed over a user's edited copy
// still measures identical.
func TestExtractManagedRegions_MatchesAcrossComposingOverAUserEditedFile(t *testing.T) {
	for _, id := range []string{"claude", "opencode"} {
		t.Run(id, func(t *testing.T) {
			fresh := composeFor(t, id, nil)
			userEdited := fresh + "\n<!-- my own trailing note -->\n"
			rewritten := composeFor(t, id, []byte(userEdited))

			if got, want := ExtractManagedRegions(id, rewritten), ExtractManagedRegions(id, fresh); got != want {
				t.Errorf("the regions of a rewritten user file differ from a fresh compose:\n got %q\nwant %q", got, want)
			}
			if !strings.Contains(rewritten, "my own trailing note") {
				t.Error("the writer dropped the user's own content, which is the promise this fix must not break")
			}
		})
	}
}

// Guards the fixture the tests above lean on: composeFor renders from the real
// embedded templates, so an empty template set would make every comparison
// above trivially true.
func TestExtractManagedRegions_UsesTheRealEmbeddedTemplates(t *testing.T) {
	if jarvis.HiveProtocol == "" {
		t.Fatal("the embedded Hive protocol is empty, so the region fixtures prove nothing")
	}
}
