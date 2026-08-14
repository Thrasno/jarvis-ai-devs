// This file holds the rendering half of the read-only planner: it turns the
// desired-state manifest plus the currently installed binary's embedded assets
// into the artifacts a later stage applies. It reads; it never writes.
package sync

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/agent"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/config"
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
//
// It applies only to artifacts that actually carry a marker on disk, which today
// means the OpenCode global document. Managed instruction files carry none, so
// they are proven by manifest membership instead and never by this.
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
	Skills    []config.SkillInfo
}

// renderInstructions renders one agent's managed instruction file.
type renderInstructions func(fsys fs.FS, layer1, layer2, expertise string, skills []config.SkillInfo) (string, error)

// instructionTemplates binds a manifest agent ID to the embedded template that
// renders its instruction file. An ID absent from this map has no embedded
// target in the installed binary, so planning fails closed rather than guessing.
var instructionTemplates = map[string]renderInstructions{
	"claude":   config.RenderCLAUDEMd,
	"opencode": config.RenderAGENTSMd,
}

// BuildPlan renders the desired targets recorded by the last installation.
//
// Every target comes from in.Templates, the assets embedded in the running
// binary, so the plan always describes the installed version. Nothing already on
// disk contributes content, and nothing is written.
func BuildPlan(in PlanInput) (Plan, error) {
	if in.State == nil || len(in.State.InstalledAgents) == 0 {
		return Plan{}, ErrNoConfiguredAgents
	}

	outputs := make([]agent.RenderedManagedOutput, 0, len(in.State.InstalledAgents))
	for _, configured := range in.State.InstalledAgents {
		render, embedded := instructionTemplates[configured.ID]
		if !embedded {
			return Plan{}, fmt.Errorf("state.yaml records agent %q, for which this Jarvis version embeds no instruction template", configured.ID)
		}
		content, err := render(in.Templates, in.Layer1, in.Layer2, "", in.Skills)
		if err != nil {
			return Plan{}, fmt.Errorf("render %s instructions: %w", configured.ID, err)
		}
		location, err := managedLocation(in.Root, configured.InstructionsPath)
		if err != nil {
			return Plan{}, fmt.Errorf("agent %q instructions_path %q: %w", configured.ID, configured.InstructionsPath, err)
		}
		outputs = append(outputs, agent.RenderedManagedOutput{
			Identity: "jarvis-instructions-" + configured.ID,
			Location: location,
			Bytes:    []byte(content),
		})
	}

	// The bridge is used for what it can actually prove here: it rejects a
	// location outside the managed root, refuses a duplicate managed identity,
	// and keeps the OpenCode global document off this path. It is not used as an
	// ownership oracle, because for these targets it cannot be one.
	request, err := agent.BuildProductionReconcileRequest(agent.ProductionReconcileInput{
		Root:            in.Root,
		RenderedOutputs: outputs,
	})
	if err != nil {
		return Plan{}, fmt.Errorf("plan managed instruction targets: %w", err)
	}
	if request.StorePlan.Blocked() {
		return Plan{}, errors.New("the recorded instruction targets do not authorize a managed write; run `jarvis` to reinstall")
	}

	bytesByLocation := make(map[string][]byte, len(outputs))
	for _, output := range outputs {
		bytesByLocation[output.Location] = output.Bytes
	}
	plan := Plan{Artifacts: make([]PlannedArtifact, 0, len(request.StorePlan.Operations))}
	for _, operation := range request.StorePlan.Operations {
		plan.Artifacts = append(plan.Artifacts, PlannedArtifact{
			Identity: operation.Identity,
			Location: operation.Location,
			Bytes:    bytesByLocation[operation.Location],
			// Manifest membership, not a marker. Instruction files carry no
			// provenance marker on disk, so reconcile never observed one and the
			// operation's Provenance is the marker this run would write, not
			// evidence about what is already there. Claiming MarkerProof here
			// would assert a check that never ran; the honest proof is that the
			// manifest lists this agent, which is exactly the rule ApplyInstructions
			// enforces before any write.
			Proof: IdentityProof{Source: IdentitySourceManifest},
		})
	}
	return plan, nil
}

// managedLocation projects a manifest-recorded path onto a root-relative
// managed location. A path outside the managed root is refused, never clamped.
func managedLocation(root, path string) (string, error) {
	if !filepath.IsAbs(path) {
		return filepath.ToSlash(filepath.Clean(path)), nil
	}
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("is outside the managed root")
	}
	return filepath.ToSlash(relative), nil
}
