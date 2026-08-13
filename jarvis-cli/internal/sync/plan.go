// This file holds the rendering half of the read-only planner: it turns the
// desired-state manifest plus the currently installed binary's embedded assets
// into the artifacts a later stage applies. It reads; it never writes.
package sync

import (
	"errors"
	"io/fs"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/reconcile"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/state"
)

// ErrNoConfiguredAgents reports a manifest that records no agents. The planner
// never recovers by scanning the filesystem for installed agents: an agent file
// on disk is not evidence that this machine's installation configured it, so
// the only honest answer is to block and name the recovery command.
var ErrNoConfiguredAgents = errors.New(
	"state.yaml records no configured agents, so there is nothing to replay; run `jarvis` to reinstall and record this machine's configuration",
)

// Proof is the closed sum of the two ways Jarvis proves it owns an artifact.
// Nothing outside this package can add a third: the unexported method keeps the
// set closed, so every consumer switch stays exhaustive.
type Proof interface{ isProof() }

// MarkerProof is ownership proven by a versioned provenance marker bound to the
// current manifest. It is produced through reconcile's planner, whose Classify
// rule stays exactly as strict as it is: names, paths and conventions are not
// evidence.
type MarkerProof struct{ Provenance reconcile.Provenance }

func (MarkerProof) isProof() {}

// isProof makes IdentityProof, declared in ownership.go, the second and last
// member of the sum. It covers artifacts that carry no marker, such as skill
// directories, where catalog and manifest membership is the only evidence.
func (IdentityProof) isProof() {}

// PlannedArtifact is one desired target: what it is, where it belongs, the
// exact bytes the installed binary renders for it, and why Jarvis may write it.
type PlannedArtifact struct {
	Identity string
	Location string
	Bytes    []byte
	Proof    Proof
}

// Plan is the read-only result of planning a sync run.
type Plan struct {
	Artifacts []PlannedArtifact
}

// PlanInput carries everything the planner is allowed to read. Every target is
// rendered from Templates, the assets embedded in the running binary, so a plan
// always reflects the installed version rather than whatever a previous version
// left on disk.
type PlanInput struct {
	Root      string
	State     *state.State
	Templates fs.FS
	Layer1    string
	Layer2    string
}

// BuildPlan renders the desired targets recorded by the last installation.
func BuildPlan(in PlanInput) (Plan, error) {
	if in.State == nil || len(in.State.InstalledAgents) == 0 {
		return Plan{}, ErrNoConfiguredAgents
	}
	return Plan{}, nil
}
