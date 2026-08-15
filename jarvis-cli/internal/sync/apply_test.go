package sync

import (
	"errors"
	"reflect"
	"testing"
)

// recordingRunner captures which component the applier asked for, in call
// order, and which single agent each call was scoped to. Failures are keyed by
// agent so one agent can be broken while its sibling stays healthy.
type recordingRunner struct {
	calls    []string
	scopedTo []string
	failAt   map[string]error
}

func (r *recordingRunner) record(id string, target AgentTarget) error {
	r.calls = append(r.calls, id)
	r.scopedTo = append(r.scopedTo, target.ID)
	return r.failAt[target.ID+"/"+id]
}

func (r *recordingRunner) ApplyModels(t AgentTarget) error { return r.record(ComponentModels, t) }
func (r *recordingRunner) ApplySkills(t AgentTarget) error { return r.record(ComponentSkills, t) }
func (r *recordingRunner) ApplyMCPs(t AgentTarget) error   { return r.record(ComponentMCPs, t) }
func (r *recordingRunner) ApplyRuntimeAssets(t AgentTarget) error {
	return r.record(ComponentRuntimeAssets, t)
}
func (r *recordingRunner) ApplyPersonaInstructions(t AgentTarget) error {
	return r.record(ComponentPersonaInstructions, t)
}
func (r *recordingRunner) ApplyStatusline(t AgentTarget) error {
	return r.record(ComponentStatusline, t)
}

// orderedComponentIDs is the contract this package promises. It is spelled out
// literally, not derived from the production slice, so reordering the applier
// fails this test instead of silently rewriting the expectation.
var orderedComponentIDs = []string{
	"models",
	"skills",
	"orchestrator-agents-hooks",
	"mcps",
	"persona-instructions",
	"statusline",
}

func TestApply_LocksTheComponentOrderWithPersonaAfterContentInjectors(t *testing.T) {
	runner := &recordingRunner{}

	report := Apply(ApplyInput{Runner: runner, Targets: []AgentTarget{{ID: "claude"}}})

	if !reflect.DeepEqual(runner.calls, orderedComponentIDs) {
		t.Fatalf("component order = %v, want %v", runner.calls, orderedComponentIDs)
	}
	if !report.Converged() {
		t.Fatalf("report should claim convergence when every component succeeds: %+v", report)
	}
	if report.ExitCode() != 0 {
		t.Fatalf("exit code = %d, want 0", report.ExitCode())
	}
}

// "Nothing ran" is not "everything converged". A run with no targets applies no
// component and produces no per-agent evidence, so the only honest answer is a
// non-zero exit: vacuous truth over an empty agent list would let a caller that
// lost its targets report a healthy sync.
//
// The empty list does not arrive here from production today, because BuildPlan
// refuses a manifest with no configured agents (plan.go:26) long before an
// ApplyInput is built. This pins the type's own contract, which is what the
// caller in cmd_sync.go relies on rather than re-deriving.
func TestApply_WithNoTargetsRefusesToClaimConvergence(t *testing.T) {
	runner := &recordingRunner{}

	report := Apply(ApplyInput{Runner: runner, Targets: nil})

	if len(report.Agents) != 0 {
		t.Fatalf("report.Agents = %+v, want no per-agent results", report.Agents)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("components ran without a target: %v", runner.calls)
	}
	if report.Converged() {
		t.Fatal("a run with no target converged nothing; it must not claim convergence")
	}
	if report.ExitCode() != 1 {
		t.Fatalf("exit code = %d, want 1", report.ExitCode())
	}
}

// D1: sync deliberately differs from the wizard, which early-returns on the
// first agent failure (internal/tui/agent_setup.go:255-259). The wizard is
// interactive and the user can retry; sync is not, so stranding a healthy agent
// unsynced is the worse outcome.
func TestApply_ContinuesWithTheNextAgentAfterAFailure(t *testing.T) {
	boom := errors.New("managed MCP write refused")
	runner := &recordingRunner{failAt: map[string]error{"claude/" + ComponentMCPs: boom}}

	report := Apply(ApplyInput{Runner: runner, Targets: []AgentTarget{
		{ID: "claude"},
		{ID: "opencode"},
	}})

	if len(report.Agents) != 2 {
		t.Fatalf("report must name every agent, got %d: %+v", len(report.Agents), report.Agents)
	}

	failed := report.Agents[0]
	if failed.Agent != "claude" || failed.Converged {
		t.Fatalf("claude must be reported as not converged, got %+v", failed)
	}
	if failed.FailedAt != ComponentMCPs {
		t.Fatalf("FailedAt = %q, want %q", failed.FailedAt, ComponentMCPs)
	}
	if !errors.Is(failed.Err, boom) {
		t.Fatalf("failure cause = %v, want it to wrap %v", failed.Err, boom)
	}

	healthy := report.Agents[1]
	if healthy.Agent != "opencode" || !healthy.Converged || healthy.Err != nil {
		t.Fatalf("opencode must converge independently, got %+v", healthy)
	}

	if report.Converged() {
		t.Fatal("report must not claim global convergence when an agent failed")
	}
	if report.ExitCode() == 0 {
		t.Fatal("exit code must be non-zero when an agent failed")
	}

	// The failed agent stops at its failing component; the healthy agent still
	// runs the whole order afterwards. Nothing re-enters the failed agent, so no
	// cross-agent rollback can occur through this seam.
	wantCalls := append([]string{
		ComponentModels, ComponentSkills, ComponentRuntimeAssets, ComponentMCPs,
	}, orderedComponentIDs...)
	if !reflect.DeepEqual(runner.calls, wantCalls) {
		t.Fatalf("calls = %v, want %v", runner.calls, wantCalls)
	}
}

// What each agent actually executed is recorded, not left to be inferred.
//
// It survives where the changed-path diff does not: a failure between the
// applier and the closing measurement leaves Changed nil for the whole run, and
// then this list is the only thing that says how far each agent got. Reading it
// back out of FailedAt and the locked order would ask a reader to reconstruct
// what the run already knew.
func TestApply_RecordsTheComponentsEachAgentCompleted(t *testing.T) {
	boom := errors.New("managed MCP write refused")
	runner := &recordingRunner{failAt: map[string]error{"claude/" + ComponentMCPs: boom}}

	report := Apply(ApplyInput{Runner: runner, Targets: []AgentTarget{{ID: "claude"}, {ID: "opencode"}}})

	wantFailed := []string{ComponentModels, ComponentSkills, ComponentRuntimeAssets}
	if !reflect.DeepEqual(report.Agents[0].Completed, wantFailed) {
		t.Fatalf("completed for the failed agent = %v, want %v; the failing component is not one of them",
			report.Agents[0].Completed, wantFailed)
	}
	if !reflect.DeepEqual(report.Agents[1].Completed, orderedComponentIDs) {
		t.Fatalf("completed for the healthy agent = %v, want the whole order %v",
			report.Agents[1].Completed, orderedComponentIDs)
	}
}

// Each component call carries exactly one agent, so a ReconcileInstallRequest
// built from a target can never span agents and reconcile's compensation can
// never roll back a sibling's marker-backed artifacts.
func TestApply_ScopesEveryComponentCallToASingleAgent(t *testing.T) {
	runner := &recordingRunner{}

	Apply(ApplyInput{Runner: runner, Targets: []AgentTarget{{ID: "claude"}, {ID: "opencode"}}})

	want := make([]string, 0, 2*len(orderedComponentIDs))
	for range orderedComponentIDs {
		want = append(want, "claude")
	}
	for range orderedComponentIDs {
		want = append(want, "opencode")
	}
	if !reflect.DeepEqual(runner.scopedTo, want) {
		t.Fatalf("call scopes = %v, want %v", runner.scopedTo, want)
	}
}
