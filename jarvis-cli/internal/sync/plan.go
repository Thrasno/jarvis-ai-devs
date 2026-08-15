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
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/persona"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/reconcile"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
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
	// Mode is the permission Jarvis asserts, not whatever the file carries today.
	Mode fs.FileMode
}

// Plan is the read-only result of planning a sync run.
type Plan struct {
	Artifacts []PlannedArtifact
	// Tracked is the single list of absolute paths this run is responsible for:
	// one producer, two consumers (the snapshot/diff that measures idempotency,
	// and the pre-apply backup). A second list built anywhere else would let
	// backup coverage and measurement drift apart in silence.
	Tracked []TrackedPath
}

// Managed artifact names living beside an agent's instruction file, the only
// per-agent location the manifest records.
const (
	statuslineScriptName = "statusline-command.sh"
	skillsDirName        = "skills"
)

// agentPlatforms binds a manifest agent ID to the runtime platform whose model
// assignments decide how that agent's skill files render.
var agentPlatforms = map[string]sddruntime.Platform{
	"claude":   sddruntime.PlatformClaude,
	"opencode": sddruntime.PlatformOpenCode,
}

// trackedPaths lists what this run is responsible for, and what each of those
// paths should contain.
//
// Every entry carries a desired digest, because a path list alone cannot answer
// whether a run has anything to do. Skills and the statusline are therefore
// rendered here rather than derived from the manifest: the skills walk is the
// installer's own, so one skill contributes every file it actually installs
// instead of a guessed SKILL.md, and the statusline carries the embedded
// script's bytes.
func trackedPaths(in PlanInput, artifacts []PlannedArtifact, ownerByLocation map[string]string) ([]TrackedPath, error) {
	tracked := make([]TrackedPath, 0, len(artifacts))
	for _, artifact := range artifacts {
		tracked = append(tracked, TrackedPath{
			Agent:    ownerByLocation[artifact.Location],
			Identity: filepath.ToSlash(filepath.Clean(artifact.Location)),
			Path:     filepath.Join(in.Root, filepath.FromSlash(artifact.Location)),
			Mode:     artifact.Mode,
			Desired:  digestOf(artifact.Bytes),
		})
	}
	for _, configured := range in.State.InstalledAgents {
		location, err := managedLocation(in.Root, configured.InstructionsPath)
		if err != nil {
			continue
		}
		dir := filepath.Dir(filepath.Join(in.Root, filepath.FromSlash(location)))
		skillFiles, err := renderSkills(in, configured.ID)
		if err != nil {
			return nil, err
		}
		for _, file := range skillFiles {
			tracked = append(tracked, TrackedPath{
				Agent:    configured.ID,
				Identity: filepath.ToSlash(filepath.Join(filepath.Dir(location), skillsDirName, filepath.FromSlash(file.RelPath))),
				Path:     filepath.Join(dir, skillsDirName, filepath.FromSlash(file.RelPath)),
				Mode:     ManagedFileMode,
				Desired:  digestOf(file.Bytes),
			})
		}
		if configured.ID == "claude" {
			settingsPath := filepath.Join(dir, "settings.json")
			if configured.ConfigPath != "" {
				configLocation, configErr := managedLocation(in.Root, configured.ConfigPath)
				if configErr != nil {
					return nil, fmt.Errorf("agent %q config_path %q: %w", configured.ID, configured.ConfigPath, configErr)
				}
				settingsPath = filepath.Join(in.Root, filepath.FromSlash(configLocation))
			}
			managed := map[string]any{}
			if in.State.Statusline.ShouldManage() {
				managed["statusLine"] = map[string]any{"type": "command", "command": "bash ~/.claude/statusline-command.sh"}
			}
			if in.Profile != nil {
				styleName := persona.OutputStyleName(in.Profile.Name)
				managed["outputStyle"] = styleName
				content := []byte(persona.RenderOutputStyle(in.Profile))
				tracked = append(tracked, TrackedPath{Agent: configured.ID, Identity: filepath.ToSlash(filepath.Join(filepath.Dir(location), "output-styles", styleName+".md")), Path: filepath.Join(dir, "output-styles", styleName+".md"), Mode: ManagedFileMode, Desired: digestOf(content)})
			}
			if len(managed) > 0 {
				settingsIdentity, identityErr := managedLocation(in.Root, settingsPath)
				if identityErr != nil {
					return nil, fmt.Errorf("derive stable identity for %s: %w", settingsPath, identityErr)
				}
				tracked = append(tracked, TrackedPath{Agent: configured.ID, Identity: filepath.ToSlash(filepath.Clean(settingsIdentity)), Path: settingsPath, Mode: ManagedFileMode, Semantic: &ManagedJSON{Fragments: managed}})
			}
		}
		// Undecided and decided-against consent both mean "do not touch", so an
		// unauthorized statusline is not a path this run is responsible for.
		if configured.ID != "claude" || !in.State.Statusline.ShouldManage() {
			continue
		}
		if in.HooksFS == nil {
			return nil, errors.New("state.yaml records a managed statusline, for which this Jarvis version embeds no hooks source")
		}
		script, err := fs.ReadFile(in.HooksFS, agent.StatuslineScriptSource)
		if err != nil {
			return nil, fmt.Errorf("read embedded statusline script: %w", err)
		}
		tracked = append(tracked, TrackedPath{
			Agent:    configured.ID,
			Identity: filepath.ToSlash(filepath.Join(filepath.Dir(location), statuslineScriptName)),
			Path:     filepath.Join(dir, statuslineScriptName),
			Mode:     ManagedExecutableMode,
			Desired:  digestOf(script),
		})
	}
	return tracked, nil
}

// renderSkills reuses the installer's walk and model-section rendering, so the
// content sync compares against is the content the installer writes.
func renderSkills(in PlanInput, agentID string) ([]agent.RenderedSkillFile, error) {
	if len(in.State.Skills) == 0 {
		return nil, nil
	}
	if in.SkillsFS == nil {
		return nil, fmt.Errorf("state.yaml records %d skills, for which this Jarvis version embeds no skills source", len(in.State.Skills))
	}
	// Always non-nil, and deliberately so. RenderSkillFilesForPlatform renders
	// verbatim for a nil models, which is right for InstallSkills but never for
	// the planner: the installer this plan is compared against always renders
	// with the manifest's assignments, so a verbatim plan would digest content no
	// run ever writes and report drift forever. The manifest is the authority and
	// it always has assignments, even empty ones.
	models := in.State.PhaseModels
	files, err := agent.RenderSkillFilesForPlatform(in.SkillsFS, in.State.Skills, agentPlatforms[agentID], &models)
	if err != nil {
		return nil, fmt.Errorf("render skills for agent %q: %w", agentID, err)
	}
	return files, nil
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
	// SkillsFS is the embedded skill tree, rooted the way the installer expects
	// it (a sub-FS of embed/skills), and HooksFS is the root FS carrying the
	// embedded statusline script. Both are read to render desired content, never
	// to discover what a previous version installed.
	SkillsFS fs.FS
	HooksFS  fs.FS
	Profile  *persona.Profile
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
	// The owner is recorded here, where the manifest entry is in hand, rather
	// than recovered later by parsing an identity or matching a path prefix.
	ownerByLocation := make(map[string]string, len(in.State.InstalledAgents))
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
		ownerByLocation[location] = configured.ID
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
			Mode:  ManagedFileMode,
		})
	}
	if plan.Tracked, err = trackedPaths(in, plan.Artifacts, ownerByLocation); err != nil {
		return Plan{}, err
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
