# Design: `jarvis sync` replays the last installation's desired state

## Technical Approach

A new `internal/state` store loads `~/.jarvis/state.yaml` fail-closed. A new `internal/sync` package turns that manifest plus the embedded assets into an ordered plan, then runs snapshot → backup → apply → diff → verify → bookkeeping. Rendering is never reimplemented: the shared apply logic is lifted out of `internal/tui` into `internal/agentapply` and called by both the wizard and sync.

## Architecture Decisions

### Decision: Component order is models → skills → orchestrator/agents/hooks → MCPs → persona+instructions → statusline

**Choice**: persona and the instruction write run **last**, not first.

**Alternatives considered**: mirroring `gentle-ai`'s "persona before content-injecting components" (`internal/cli/sync.go:321-333`), as the proposal's Approach table assumed.

**Rationale**: verified — jarvis has the inverse hazard, not the same one.

| Evidence | Consequence |
|---|---|
| The instruction file has exactly one writer, `WriteInstructions` (`agent/claude.go:327`, `agent/opencode.go:424`). Skills land in separate files (`agent/install.go:83`). | No component injects prose after persona, so gentle-ai's "persona strips what was just written" collision cannot occur. |
| `WriteInstructions` re-applies the Hive protocol and orchestrator import in the same pass (`claude.go:359-365`). | Persona is self-healing; no later injector is needed. |
| Its no-sentinel branch **replaces** the file (`claude.go:350-355`, gated by `ValidateSentinels`, `sentinel.go:64-94`). | Persona is a destructive *last* writer. Any component writing to CLAUDE.md after it would be discarded on the next run. |
| `WriteInstructions(layer1, layer2, skills)` renders the Skills section from the installed set; skill bodies render from model assignments (`claude.go:532`, `install.go:76-82`). | Data dependency forces models → skills → instructions. |
| Production already does this: `configureWizardAgents` applies agents then `applyWizardProfile` (`tui/agent_setup.go:245-277`). | Sync inherits a proven order instead of inventing one. |

**Test that locks it**: the applier takes an ordered `[]component` with stable IDs; a recorder fake captures IDs, and `TestSyncComponentOrder` asserts the exact slice. A companion regression test starts from a CLAUDE.md with no sentinels and asserts the post-run file has valid sentinels, every installed skill in the Skills section, the Hive protocol block, and the orchestrator import.

### Decision: extract the apply pipeline into `internal/agentapply`

**Choice**: move the body of `configureWizardAgent` (`tui/agent_setup.go:77-150`) and `reconcileWizardMCPs` (`:169-202`) into `internal/agentapply`; `internal/tui` and `internal/sync` become two callers differing only in the statusline decision function.

**Alternatives considered**: importing `internal/tui` from sync (inverts layering, drags Bubbletea in); duplicating the pipeline in sync (two divergent orders).

**Rationale**: sync must reuse, not re-render. Unchanged seams: `agent.BuildProductionReconcileRequest` / `NewFileCompensationStore` (`production_bridge.go:438,77`), `agent.MergeJSON`, `opencode_managed_store.go`, `config.RenderCLAUDEMd` / `RenderAGENTSMd`, `PatchFile`.

### Decision: statusline decision derived from the tri-state, never `nil`

**Choice**:

```go
switch {
case !st.Statusline.Decided, !st.Statusline.Enabled:
    // never call InstallStatusline
default:
    err = sl.InstallStatusline(jarvis.HooksFS, func() bool { return true })
}
```

**Alternatives considered**: passing `nil` (panics — `confirm()` is invoked unguarded at `claude.go:868` whenever the script exists, i.e. every upgrade); changing `InstallStatusline`'s signature (breaks the wizard).

**Rationale**: "not decided" and "decided-disabled" both mean *do not touch*, so the call is skipped entirely. Decided+enabled always converges: absent script → fresh write (D2 reinstall), existing script → `true` → overwrite with embedded bytes. Because `InstallStatusline` unconditionally rewrites (`claude.go:882-885`), the idempotency diff MUST compare content and mode, never mtime.

### Decision: two ownership proofs, one planner

**Choice**: the planner emits `PlannedArtifact{Identity, Location, Bytes, Proof}` where `Proof` is a closed two-constructor sum: `MarkerProof` (routed through `BuildProductionReconcileRequest`, keeping `reconcile.Classify` and `Provenance` exactly as-is) and `IdentityProof` (membership in embedded catalog ∪ `manifest.Skills`, implemented in `internal/sync/ownership.go`).

**Alternatives considered**: adding markers to skills and the statusline (explicit non-goal); loosening `reconcile.Classify` (`reconcile.go:105-118`) to accept path/name evidence (destroys its guarantee); implementing `InventoryProvider` (`agent/inventory.go:10`, zero implementations).

**Rationale**: the enum keeps both paths first-class without weakening either. Sync bypasses `InventoryProvider` and populates `RenderedManagedOutput.Existing` itself, the way install already does (`production_bridge.go:460-463`); implementing a dead interface would add a second unproven observation path. Skill deletion rule: delete iff the directory name is in `manifest.Skills` **and** absent from the embedded catalog; anything in neither list is untouchable.

### Decision: per-agent failure isolation (D1)

**Choice**: the apply loop iterates agents; a failure stops that agent's remaining components, records the cause, and continues with the next agent. No global rollback; exit code non-zero. **Constraint**: each `ReconcileInstallRequest` is scoped to a single agent, so `reconcile`'s transactional compensation rolls back only that agent's marker-backed artifacts and never a sibling's.

**Alternatives considered**: early return (current wizard behavior, `agent_setup.go:255-259`) — strands a healthy agent unsynced.

## Data Flow

    state.yaml ──→ Loader ──→ Planner ──→ ordered []component
      (fail-closed)             │              │
    embedded assets ────────────┘              ▼
                                        Snapshot(paths)
                                               ▼
                                        Backup(same paths)
                                               ▼
                    per agent: models→skills→orchestrator/hooks→MCPs→persona→statusline
                                               ▼
                                        Diff ──→ changed? ──→ verify + bookkeeping (locked)
                                               └─ zero ──→ write nothing

## File Changes

| File | Action | Description |
|---|---|---|
| `internal/state/state.go` | Create | `state.yaml` schema, tri-state statusline, load/validate |
| `internal/state/migrate.go` | Create | one-way move of replay fields out of `config.yaml`, before any early return |
| `internal/sync/plan.go` | Create | manifest + assets → `[]PlannedArtifact`, ordered components |
| `internal/sync/ownership.go` | Create | `IdentityProof` membership check, skill deletion rule |
| `internal/sync/apply.go` | Create | ordered applier, per-agent isolation, report |
| `internal/sync/snapshot.go` | Create | content+mode snapshot/diff over the plan's own path list |
| `internal/agentapply/apply.go` | Create | pipeline extracted from `tui/agent_setup.go:77-202` |
| `internal/tui/agent_setup.go` | Modify | delegate to `internal/agentapply` |
| `internal/config/config.go` | Modify | `schema_version: 3`, replay fields removed |
| `cmd/jarvis/cmd_sync.go` | Modify | no-op replaced by the replay command |
| `internal/lifecycle/backup.go` | Modify | accept an explicit target list from sync's plan |
| `docs/` | Modify | sync behavior and upgrade notes |

## Interfaces / Contracts

```go
type StatuslineState struct{ Decided, Enabled bool }

type Proof interface{ isProof() }
type MarkerProof struct{ Provenance reconcile.Provenance }
type IdentityProof struct{ Source IdentitySource } // catalog | manifest

type PlannedArtifact struct {
    Identity, Location string
    Bytes              []byte
    Proof              Proof
}

type AgentResult struct {
    Agent      string
    Converged  bool
    FailedAt   string // component ID
    Err        error
    Changed    []string
}
```

## Testing Strategy

| Layer | What to Test | Approach |
|---|---|---|
| Unit | Order contract | Recorder fake; assert exact ordered component IDs |
| Unit | Statusline tri-state | Table-driven over the four `(Decided, Enabled)` × script-present combinations; assert `confirm` is never `nil` and never called when undecided |
| Unit | Ownership | Table-driven: catalog-only, manifest-only, both, neither → install / delete / untouched |
| Unit | Migration | v2 fixture → v3; assert replay fields absent from `config.yaml` and present in `state.yaml` |
| Integration | Zero-diff second run | `t.TempDir()` home; run sync twice; assert second run's diff is empty and no file mtime/bookkeeping write occurs |
| Integration | Sentinel-loss recovery | CLAUDE.md without sentinels → full order → assert sentinels, skills, protocol, orchestrator import |
| Integration | D1 partial failure | Injected failure on agent B; assert agent A converged, B reported, non-zero exit, no cross-agent rollback |
| Integration | Fail-closed | Missing/invalid state, agent-less manifest → error, zero writes |

All filesystem tests use `t.TempDir()`; every contract lands test-first (RED) per strict TDD.

## Threat Matrix

| Boundary | Applicability | Design response | Planned RED tests |
|---|---|---|---|
| Documentation-like paths | **Applicable** — `statusline-command.sh` is written mode 0755 (`claude.go:885`); skills are non-executable Markdown (`install.go:83`, 0644) | Executable mode is asserted, not inherited: statusline stays 0755, every skill/instruction artifact stays 0644; the diff compares mode | Assert 0755 on statusline and 0644 on skills/instructions after sync, including the reinstall-after-delete path |
| Git repository selection | N/A — sync is machine-scoped; no `git -C`, no worktree, no project scope (F2) | — | — |
| Commit state | N/A — sync performs no VCS operation | — | — |
| Push state | N/A — sync performs no VCS operation | — | — |
| PR commands | N/A — sync performs no PR automation | — | — |

Native Claude MCP management shells out (`claude mcp add/remove`, `claude.go:229-249`), but sync reuses the existing executor verbatim with fixed identities and no user-composed arguments; no new argument-composition boundary is introduced.

## Migration / Rollout

One-way `config.yaml` v2 → v3 field move, executed before any early return; the notice is emitted only after a durable write. Recovery is the pre-apply backup (`~/.jarvis/backups/<id>.tar.gz`, checksum-validated). Slices 2-5 are additive to a command that is a no-op today.

## Open Questions

- [ ] None blocking. The proposal's Approach line "persona before content-injecting components" is superseded by the evidence above; `sdd-tasks` must carry the inverted order.
