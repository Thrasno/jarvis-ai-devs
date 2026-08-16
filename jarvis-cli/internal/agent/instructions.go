// This file owns how a managed instruction file is composed, and it is the only
// place in the tree that owns it.
//
// It exists because the rule was previously written twice. Each writer assembled
// its own file -- render, then the protocol injection, and for Claude the
// orchestrator import -- while the replay planner rendered the template alone and
// digested that as the desired content. The two descriptions of the same file
// therefore disagreed by exactly the injected blocks, which made every managed
// instruction file fail replay's post-apply verification: `jarvis sync` measured
// a file it had just written correctly, found it did not match, and exited
// non-zero telling the user to run `jarvis sync` to repair. Forever, on every
// machine.
//
// It survived a long review chain because every test in internal/sync drives a
// fake writer: the planner was checked against the template renderer, the
// applier against a stub, and nothing compared the planner's answer with the
// bytes a real writer produces. That is the reason this function has to stay
// shared rather than merely correct. A second site that assembles an instruction
// file, however carefully, re-creates the same class of defect the moment either
// copy grows a step.
package agent

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/config"
)

// ErrNoInstructionTemplate reports an agent this binary cannot compose a managed
// instruction file for. It is a sentinel so a caller can recognize the class and
// name its own recovery: replay meets it through a manifest that records an
// agent the running binary no longer embeds.
var ErrNoInstructionTemplate = errors.New("this Jarvis version embeds no instruction template for the agent")

// instructionComposer is one agent's slice of the composition: the template that
// renders its file, and the injections that belong to it alone.
type instructionComposer struct {
	render func(fsys fs.FS, layer1, layer2, expertise string, skills []config.SkillInfo) (string, error)
	// finalize applies the injections unique to this agent, after the shared
	// ones. A nil finalize means the agent has none, which is not a special case
	// to test for at the call site.
	finalize func(content string) string
}

// instructionComposers keys the per-agent differences by manifest agent ID, the
// way every other agent-keyed table in this tree does. Keeping the difference
// here is what lets both the writers and the planner ask one question and get a
// complete answer: no caller re-branches on which agent it is holding.
var instructionComposers = map[string]instructionComposer{
	"claude": {
		render: config.RenderCLAUDEMd,
		// Claude alone carries the orchestrator @import. The legacy prose link is
		// cleaned up first, exactly as the marker-based protocol block is.
		finalize: func(content string) string {
			return InjectOrchestratorImport(CleanupOldOrchestratorLink(content))
		},
	},
	"opencode": {render: config.RenderAGENTSMd},
}

// managedRegion is one marker-delimited block Jarvis owns inside an instruction
// file. The pair is exactly the markers the injections above write, so a region
// cannot drift from the block it describes without the constants moving too.
type managedRegion struct{ start, end string }

// sharedManagedRegions are the blocks every managed instruction file carries,
// in the order ComposeInstructions produces them.
var sharedManagedRegions = []managedRegion{
	{start: Layer1Start, end: Layer1End},
	{start: Layer2Start, end: Layer2End},
	{start: HiveProtocolStart, end: HiveProtocolEnd},
}

// managedRegionsByAgent keys the per-agent region set the same way
// instructionComposers keys the composition, because it is describing the same
// per-agent difference: Claude's finalize step injects the orchestrator @import,
// so Claude's file has a fourth managed block and OpenCode's does not.
var managedRegionsByAgent = map[string][]managedRegion{
	"claude":   append(append([]managedRegion(nil), sharedManagedRegions...), managedRegion{start: OrchestratorImportStart, end: OrchestratorImportEnd}),
	"opencode": sharedManagedRegions,
}

// regionSeparator joins the extracted regions. The regions of one agent are a
// fixed list in a fixed order, so position identifies a region and the join only
// has to keep two adjacent blocks from running together.
const regionSeparator = "\n"

// ExtractManagedRegions reduces an instruction file's content to the blocks
// Jarvis actually owns, in a fixed order.
//
// It exists because a managed instruction file is shared: ComposeInstructions
// patches the Jarvis blocks and deliberately preserves everything around them,
// which is the product's promise, while the replay planner composes from the
// installed binary's assets alone and reads nothing from disk, which is its
// standing contract. Both halves are right, and comparing the whole file made
// them disagree by exactly the user's own prose -- so a user with a note in
// their own CLAUDE.md saw `jarvis sync` report a managed output as invalid on
// every run, and repairing it never helped because the note was still there.
//
// So the comparison is narrowed rather than either half changed, and it is
// narrowed here, once: the planner reduces the content it composed and the
// snapshot reduces the bytes on disk through this same function. A second
// description of which blocks are managed would re-create the class of defect
// ComposeInstructions was written to remove.
//
// What it must keep is the other direction. An edit inside a managed block
// changes what comes back, so tampering with Jarvis's own sections is still
// measured and still repaired; a missing or truncated block is still a file
// that does not match.
//
// An agent this binary knows no region set for falls back to the whole content:
// the strictest available answer, never a lenient one.
func ExtractManagedRegions(agentID, content string) string {
	regions, known := managedRegionsByAgent[strings.ToLower(strings.TrimSpace(agentID))]
	if !known {
		return content
	}
	extracted := make([]string, 0, len(regions))
	for _, region := range regions {
		extracted = append(extracted, extractRegion(content, region))
	}
	return strings.Join(extracted, regionSeparator)
}

// extractRegion returns one block including its markers, or the empty string
// when the block is absent or malformed. Absence is a value rather than an
// error: a file missing a block simply does not match one that has it, which is
// the answer both callers want.
func extractRegion(content string, region managedRegion) string {
	start := strings.Index(content, region.start)
	if start == -1 {
		return ""
	}
	end := strings.Index(content[start:], region.end)
	if end == -1 {
		return ""
	}
	return content[start : start+end+len(region.end)]
}

// ComposeInstructions assembles the complete content of one agent's managed
// instruction file: the whole file, injections included, ready to be written or
// digested.
//
// existing is what the file holds today, and it is data rather than a mode
// switch: it is exactly what a writer just read, and it selects the same three
// outcomes the writers always had.
//
//   - absent or empty -> render fresh from the template
//   - Jarvis sentinels present -> patch in place, preserving content outside them
//   - no sentinels -> render fresh, replacing foreign content
//
// A nil existing therefore describes the file as this binary would render it
// from scratch, which is precisely what a planner wants: replay's plan is a
// statement about the installed version's assets, and nothing already on disk
// contributes content to it.
func ComposeInstructions(
	agentID string,
	templates fs.FS,
	existing []byte,
	layer1, layer2 string,
	skills []config.SkillInfo,
) (string, error) {
	composer, embedded := instructionComposers[strings.ToLower(strings.TrimSpace(agentID))]
	if !embedded {
		return "", fmt.Errorf("agent %q: %w", agentID, ErrNoInstructionTemplate)
	}

	renderFresh := func() (string, error) {
		content, err := composer.render(templates, layer1, layer2, "", skills)
		if err != nil {
			return "", fmt.Errorf("render %s instructions: %w", agentID, err)
		}
		return content, nil
	}

	var content string
	var err error
	switch existingContent := string(existing); {
	case len(existing) == 0:
		content, err = renderFresh()
	case ValidateSentinels(existingContent) == nil:
		if content, err = PatchFile(existingContent, layer1, layer2); err != nil {
			err = fmt.Errorf("patch %s instruction sentinels: %w", agentID, err)
		}
	default:
		content, err = renderFresh()
	}
	if err != nil {
		return "", err
	}

	// The shared tail, which every managed instruction file carries. Both
	// injections are idempotent and marker-based, so composing an already-composed
	// file reproduces it byte for byte -- which is what lets a replayed machine
	// measure as converged instead of rewriting itself forever.
	content = InjectProtocol(CleanupOldProtocol(content), getHiveProtocol())
	if composer.finalize != nil {
		content = composer.finalize(content)
	}
	return content, nil
}
