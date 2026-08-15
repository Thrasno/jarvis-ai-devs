// This file holds the statusline component of replay.
//
// D2: the manifest is the sole authority for statusline intent. A recorded,
// enabled decision whose script is missing from disk is drift, not revocation.
// Treating the absent file as revoked consent would make sync infer intent from
// disk state and would leave the manifest lying about what this machine wants.
package sync

import (
	"fmt"
	"io/fs"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/agentapply"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/state"
)

// StatuslineComponent replays the statusline for one agent from the manifest's
// recorded tri-state. It reads that decision and never writes it: reinstalling
// a drifted artifact is not a reason to touch stored consent.
type StatuslineComponent struct {
	Resolve AgentResolver
	// HooksFS carries the embedded statusline script.
	HooksFS fs.FS
	// Consent is the manifest's recorded tri-state, read-only here. The
	// resolution itself lives in agentapply and is not re-decided in this package.
	Consent state.StatuslineState
}

// Apply converges this agent's statusline: an absent script is written fresh and
// a present one is overwritten with the embedded bytes.
func (c StatuslineComponent) Apply(target AgentTarget) error {
	// AgentResolver is a nilable func type, so an unwired one is a missing
	// dependency rather than a resolution failure. It fails closed into the same
	// refusal as an unknown ID: replay must not panic mid-pass.
	if c.Resolve == nil {
		return fmt.Errorf("replay statusline for %q: %w", target.ID, ErrUnknownAgent)
	}
	resolved, ok := c.Resolve(target.ID)
	if !ok {
		return fmt.Errorf("replay statusline for %q: %w", target.ID, ErrUnknownAgent)
	}
	decision := agentapply.StatuslineDecisionFromState(c.Consent)
	if err := agentapply.ApplyStatusline(resolved, c.HooksFS, decision); err != nil {
		return fmt.Errorf("replay statusline for %q: %w", target.ID, err)
	}
	return nil
}
