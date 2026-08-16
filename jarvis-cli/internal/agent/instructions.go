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
