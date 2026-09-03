# Design: Convergent Zoho Skills Pack

## Decision summary

Implement the Zoho Skills Pack as one explicit, catalog-derived value in `internal/skills`, and pass its concrete lexicographically sorted member IDs through the existing setup and sync paths. No caller keeps a Zoho membership list.

Fresh setup reduces the pack answer to concrete IDs before installation and persistence. Sync makes one copied, in-memory `state.State`, expands it before instruction rendering or planning, and uses that same pointer through planner, runner, backup coverage, and final verification. The original durable state is updated only after all agents converge and `verifyApplied` passes.

The successful state transaction re-reads `state.yaml` while holding `state.WithLock`, rechecks the `zoho-deluge` enrollment anchor, preserves unrelated concurrent changes, and returns only the IDs that this transaction actually made durable. Those IDs are then rendered by the existing sync report in lexicographic order.

No state schema, generic pack framework, installer abstraction, sync prompt, sync flag, or `config.Save()` call is added.

## Current boundaries that drive the design

| Boundary | Current behavior | Design consequence |
| --- | --- | --- |
| `internal/skills/registry.go` | `ListSkills` derives IDs from embedded `SKILL.md` directories. | The embedded catalog remains the source of current pack membership. |
| `internal/skills/interactive.go` | A hard-coded map marks only `zoho-deluge` and `zoho-crm` interactive. | Zoho classification must use the shared prefix rule, not a member list. |
| `internal/tui/skills_selection.go` | The Zoho prompt hard-codes two IDs; other optional skills auto-select. | Build the one Zoho prompt from the catalog-derived pack value. |
| `internal/tui/steps.go` and `nontui.go` | TUI and non-TUI independently turn the selection map into persisted IDs; non-TUI also has a map-order persistence path. | Both paths must call one reducer that produces concrete desired IDs. |
| `cmd/jarvis/cmd_sync.go` | `replayInput` creates the shared `sync.ReplayInput`; planner and runner consume it. | Expand a copied state before constructing `ReplayInput`, then use that one expanded pointer everywhere. |
| `internal/sync/plan.go` and `runner.go` | `PlanInputFor` and `NewRunner` read `ReplayInput.State`; the plan's `Tracked` paths drive backup and verification. | No planner, installer, backup, or verifier special case is needed for Zoho. |
| `internal/sync/backup.go` | Bookkeeping currently runs before `verifyApplied` returns, and the already-current path skips bookkeeping. | Move bookkeeping after verification and support verified no-op expansion commits. |
| `internal/sync/bookkeeping.go` | Bookkeeping already locks, re-reads, mutates the latest state, and atomically saves it. | Extend this transaction rather than introducing another persistence path. |
| `internal/state/state.go` | `State.Skills` is the concrete desired-state and ownership list; `Save` preserves supplied IDs. | Keep schema version 1 and store no pack marker. |

## 1. Catalog-derived Zoho contract

Add `internal/skills/zoho_pack.go` with a deliberately Zoho-specific API:

```go
const ZohoLegacyAnchorID = "zoho-deluge"

type ZohoPack struct {
    memberIDs []string
}

func NewZohoPack(catalog []Skill) ZohoPack
func (p ZohoPack) MemberIDs() []string
func (p ZohoPack) Selected(recorded []string) bool
func (p ZohoPack) Expand(recorded []string) (desired, missing []string, eligible bool)
func (p ZohoPack) ApplySelection(recorded []string, selected bool) []string
```

The implementation uses one private `zoho-` prefix predicate. `NewZohoPack` filters actual catalog entries through that predicate, deduplicates IDs, and applies `sort.Strings`. `MemberIDs` returns a defensive copy so callers cannot alter the contract.

`IsInteractive(id string)` remains the shared classifier used by setup and sync ownership, but delegates every `zoho-*` decision to the same private prefix predicate. Its static map retains only the non-Zoho interactive IDs (`phpunit-testing`, `laravel-architecture`, and `go-testing`). Because both current callers invoke it only for IDs already present in their catalog, prefix classification remains catalog-constrained while automatically covering future embedded Zoho skills.

`Selected` returns true only when the recorded list contains the exact `ZohoLegacyAnchorID`. This is sufficient without a schema marker:

- the released legacy selected state contains `zoho-deluge`;
- every fresh pack selection under this design contains every current member, including `zoho-deluge`;
- an isolated `zoho-*` ID other than the anchor is not enrollment evidence;
- pack deselection removes the anchor together with every other recorded Zoho ID.

`ApplySelection` has explicit slice semantics:

1. Preserve all non-Zoho IDs and their relative order.
2. If deselected, remove every recorded ID with the `zoho-` prefix.
3. If selected, union recorded Zoho ownership IDs with current catalog members, deduplicate them, sort the complete Zoho subset lexicographically, and append that subset after the preserved non-Zoho IDs.

Retaining catalog-absent Zoho IDs while selected preserves the existing ownership-proof rule in `State.Skills`; deselection is the only operation that intentionally removes all recorded Zoho IDs.

`Expand` first calls `Selected`. If ineligible, it returns an unchanged copy and no missing IDs. If eligible, it returns `ApplySelection(recorded, true)` plus only current catalog member IDs absent from the input. `missing` is lexicographic and is the candidate set for success-time persistence.

### Invariants

- V0 `MemberIDs()` is exactly `zoho-analytics`, `zoho-books`, `zoho-creator`, `zoho-crm`, `zoho-deluge`, `zoho-people`, `zoho-projects`.
- Adding an embedded `zoho-expense` catalog entry changes membership without another code list.
- Prefix and anchor are the only hard-coded Zoho identifiers. The anchor is eligibility evidence, not a membership list.
- Reapplying either selection state is idempotent and duplicate-free.

## 2. One setup reduction for TUI and non-TUI

### Prompt construction

`buildSkillSelectionPlan` derives one `ZohoPack` from the supplied catalog. It prepends one prompt with the existing label and description and `SkillIDs: pack.MemberIDs()` when the pack is non-empty. The remaining static prompt templates cover only PHP and Go.

The initial selection loop calls the shared `skills.IsInteractive` classifier. Consequently, every current or future catalog `zoho-*` entry is excluded from optional auto-selection and controlled only by the one pack prompt.

The Zoho prompt defaults on only when `pack.Selected(existingSelected)` is true. Generic PHP and Go prompts retain their current “any member previously selected” default. This prevents an arbitrary orphaned Zoho ID from silently enrolling the pack.

`updateSkills` needs no new Zoho branch: it already toggles every ID carried by one prompt and leaves all other map entries untouched. Direct `Model.Update` tests will expand the prompt fixture to all members.

### Concrete desired IDs

Add one package-local reducer in `internal/tui/skills_selection.go`, used by both setup routes:

```go
func selectedSkillIDs(
    catalog []skills.Skill,
    recorded []string,
    selected map[string]bool,
) []string
```

The reducer:

1. starts from recorded catalog-absent non-Zoho IDs so unrelated ownership evidence survives;
2. retains or removes current non-Zoho catalog IDs according to the existing selected/core rules, preserving previously retained relative order and appending newly selected IDs in catalog order;
3. derives the pack answer from the selected current IDs;
4. calls `ZohoPack.ApplySelection` exactly once to remove all recorded Zoho IDs or append the concrete selected set in lexicographic order.

For a fresh selected setup, the result therefore contains every current concrete member in lexicographic Zoho order. For fresh unselected setup it contains none. For reconfiguration, changing the pack answer changes only the Zoho subset; unrelated desired IDs survive.

The TUI `buildSelectedIDs(Model)` becomes a small wrapper over this reducer. Non-TUI computes this value once after all skill questions and reuses it for `configureWizardAgents` and `manifest.Skills`. Its current unordered `selectedSet` persistence loop is removed. Skill information rendering may continue to traverse the catalog because it uses the same selection map and has no persistence-order contract.

Neither route deletes installed files on deselection. Existing installer behavior remains authoritative.

## 3. One expanded in-memory sync manifest

`replayInput` continues to be the command seam, but it loads the embedded catalog once and prepares replay in this order:

1. Build `ZohoPack` from the catalog.
2. Call `pack.Expand(manifest.Skills)`.
3. Make a shallow `state.State` value copy and replace only its `Skills` slice with the returned copied/expanded slice. The loaded durable object is never mutated.
4. Build `config.SkillInfo` from the catalog against the copied state's `Skills`.
5. Return `sync.ReplayInput` whose `State` points to that copied state, together with the pack and lexicographic candidate IDs needed by bookkeeping.

The copied state is the single desired view for the run:

```text
expanded State
  ├─ SkillInfo / instruction rendering
  ├─ sync.PlanInputFor → BuildPlan → Plan.Tracked
  │    ├─ opening/final snapshots
  │    ├─ backup targets
  │    └─ final verification
  ├─ sync.NewRunner → ConfigureAgent skill IDs
  └─ sync.TargetsFor
```

No downstream component re-expands or reloads the selection. Newly selected pack files therefore enter existing instruction rendering, planning, tracked backup, installer, mode enforcement, and verification behavior automatically.

`runSync` passes the resulting expansion candidate to `sync.Bookkeeping` and renders the report against `input.State`, because that is the exact desired view the run replayed. It never calls `config.Save()`.

## 4. Post-verification persistence transaction

### Bookkeeping input and output

Extend `sync.Bookkeeping` with one explicit optional Zoho payload rather than a generic migration mechanism:

```go
type ZohoExpansion struct {
    Pack         skills.ZohoPack
    CandidateIDs []string
}

type Bookkeeping struct {
    ManagedAssetDigest string
    ZohoExpansion      *ZohoExpansion
    Lock               func(func() error) error
}
```

Change its internal record operation to receive whether tracked files changed and whether the report fully converged, and return the newly durable IDs:

```go
func (b *Bookkeeping) record(changed, converged bool) ([]string, error)
```

`RunResult` gains:

```go
Verified      bool
AddedSkillIDs []string
```

`Verified` distinguishes a post-verification persistence failure from a verification failure. `AddedSkillIDs` remains empty until `state.Save` has returned success.

### Exact success boundary

Refactor `sync.Run` to use the following order:

1. Validate backup and target protection.
2. Take the opening snapshot.
3. If already matched, use that opening snapshot as the final snapshot and produce `convergedWithoutApplying`; otherwise take backup, apply, enforce modes, take the closing snapshot, and attribute changes.
4. Call `verifyApplied` against the final snapshot in both paths.
5. If verification fails, return immediately with `Verified == false`; do not enter bookkeeping.
6. Set `Verified = true`.
7. Only when `Report.Converged()` is true, call bookkeeping. This makes successful outcomes require every configured agent plus final filesystem verification.
8. Set `AddedSkillIDs` only from a successful bookkeeping return.

The already-current path must perform steps 4–8. This matters when the newly desired pack files already exist with correct bytes: no apply or backup is necessary, but the missing IDs may safely become durable after the opening snapshot proves final convergence.

Moving all bookkeeping after `verifyApplied` deliberately replaces the current “bookkeeping and verification errors are joined” behavior. A verification failure cannot attempt state persistence. A later bookkeeping failure returns a non-zero run after verification has passed, leaves `AddedSkillIDs` empty, and is reported as a state-persistence failure rather than mislabeled as verification failure.

### Locked merge algorithm

`Bookkeeping.record` skips the lock when neither a changed digest nor a non-empty Zoho candidate can alter state. Otherwise, under `Lock` (default `state.WithLock`) it:

1. calls `state.Load()` inside the critical section;
2. starts from that freshly read state, never the replay copy;
3. updates `ManagedAssetDigest` only for a completely converged, verified run that changed tracked output;
4. considers Zoho expansion only when the run was initially eligible, had candidate IDs, and fully converged;
5. rechecks `ZohoExpansion.Pack.Selected(latest.Skills)` against the fresh state;
6. if the anchor is absent, skips every Zoho mutation, which prevents resurrection after concurrent pack deselection;
7. if the anchor remains, computes `nextSkills = Pack.ApplySelection(latest.Skills, true)` and computes added IDs as current pack members absent from `latest.Skills` but present in `nextSkills`;
8. saves once only if either digest or skills changed;
9. returns added IDs only after that save succeeds.

Because the merge starts from `latest`, concurrent unrelated additions, removals, agent records, phase models, and other fields survive. A concurrent complete deselection removes the anchor, so the successful replay may leave installed Zoho copies on disk but cannot restore desired-state enrollment. That outcome is consistent with non-destructive deselection.

The lock remains fail-fast and non-reentrant. `state.Save(latest)` is called directly inside `state.WithLock`; `state.Update` and `config.Save` are forbidden there because both would acquire the same lock again.

## 5. Reporting contract

`renderSyncReport` appends one line per `RunResult.AddedSkillIDs` entry, using a stable form such as:

```text
zoho skill added to desired state: zoho-analytics
```

The transaction already returns IDs in lexicographic order, and the renderer preserves that order without sorting a second time. It never derives additions from the plan, changed paths, the pre-run manifest, or candidate IDs.

Reporting rules are therefore:

- legacy expansion reports every V0 member newly persisted, excluding an already durable `zoho-deluge`;
- future expansion reports every newly durable future member;
- a concurrent writer that already persisted an ID prevents this run from reporting it;
- concurrent deselection, planning failure, backup failure, partial apply, mode/snapshot failure, final verification failure, lock failure, or save failure reports no successful addition;
- a second unchanged sync reports no addition;
- `Verified == true` plus a returned bookkeeping error renders `verification: passed` followed by `state persistence: failed: ...`; pre-verification failures retain `verification: failed`.

Existing flag rejection, non-interactive behavior, changed-path reporting, backup reporting, agent outcomes, and Hive notice remain unchanged.

## 6. Failure, retry, and rollback behavior

| Failure point | Durable Zoho state | Filesystem/report behavior |
| --- | --- | --- |
| Catalog/replay preparation or planning | Unchanged | No apply; no added-ID report. |
| Opening measurement or backup | Unchanged | Existing fail-closed behavior; no mutation after backup failure. |
| Partial/blocked application | Unchanged | Existing backup and per-agent evidence remain; no added-ID report. |
| Mode enforcement or closing measurement | Unchanged | Files may have changed and backup remains available; no durable expansion. |
| Final verification | Unchanged | Verification error names invalid outputs; bookkeeping is not entered. |
| Lock or atomic state save after verification | Unchanged by this transaction | Files may be converged, command exits non-zero, no added-ID report. |
| Concurrent pack deselection | No Zoho resurrection | Installed copies remain; later sync does not manage the pack. |
| Successful retry | Expanded once | A no-op filesystem run may commit after verification; only actually persisted IDs are reported. |

No automatic filesystem rollback is introduced. Existing backups remain the recovery mechanism, and rerunning sync is convergent. `ApplySelection`, expansion, and locked merge are set-like and deterministic, so repeated success produces no duplicate IDs, no state rewrite when nothing changed, and no added-ID output.

Code rollback removes pack-aware setup and expansion behavior but leaves already durable concrete IDs intact. They remain valid ownership evidence under the existing sync model; rollback performs no uninstall or state contraction.

## 7. Strict-TDD verification seams

Implementation must follow RED → GREEN → REFACTOR with narrow `jarvis-cli` package tests first.

| Seam | Focused behavior to prove |
| --- | --- |
| `internal/skills/zoho_pack_test.go` | Unsorted/duplicate synthetic catalog yields sorted unique membership; future `zoho-*` is included; non-Zoho is excluded; only `zoho-deluge` establishes enrollment; select/deselect preserves unrelated IDs; repeat application is idempotent. |
| `internal/skills/interactive_test.go` and `internal/sync/ownership_test.go` | A future catalog Zoho ID is interactive/skipped until pack-selected; non-Zoho interactive behavior is unchanged. |
| `internal/tui/model_test.go` | One prompt contains all V0 IDs in lexicographic order; direct `Model.Update` toggles the full pack and preserves unrelated map entries; an orphan non-anchor does not default the pack on. |
| `internal/tui/nontui_test.go` | TUI and non-TUI reducers produce identical concrete IDs; selected and deselected outcomes preserve unrelated recorded IDs and deterministic Zoho order. |
| `internal/sync/bookkeeping_test.go` | Verification failure never enters the lock; successful verified change enters afterward; fresh re-read preserves concurrent unrelated changes; concurrent anchor removal prevents resurrection; lock/save failure returns no additions; second commit is idempotent. Use `t.TempDir()`. |
| `internal/sync/backup_test.go` or existing run tests | An already-matching expanded plan still reaches post-verification bookkeeping without backup or apply. |
| `cmd/jarvis/cmd_sync_test.go` | Report prints only successful newly durable IDs in lexicographic order and distinguishes post-verification persistence failure. |
| `cmd/jarvis/cmd_sync_e2e_test.go` | Existing OpenCode-only fixture: a manifest with only `zoho-deluge` converges V0, persists all members, reports each missing ID once, and reports none on the second run. |

Existing installer, plan, idempotency, mode, and symlink tests remain the proof for managed-file safety. Add only a focused assertion that the expanded state causes pack files to appear in `Plan.Tracked`; do not duplicate the installer safety suite.

The command-level OpenCode fixture is only a wiring test for setup/state/sync/report behavior. This change must not add nested-reference assertions, inspect Zoho skill content, invoke Claude's native runtime, or claim Claude Code/OpenCode end-to-end parity. Those remain issue #547.

## 8. Minimal file-change plan

### Production

| File | Change |
| --- | --- |
| `jarvis-cli/internal/skills/zoho_pack.go` | New explicit pack value, prefix/anchor contract, ordering, selection, and expansion. |
| `jarvis-cli/internal/skills/interactive.go` | Remove hard-coded Zoho IDs and delegate Zoho classification to the shared prefix predicate. |
| `jarvis-cli/internal/tui/skills_selection.go` | Derive the single prompt and shared concrete-ID reducer from `ZohoPack`. |
| `jarvis-cli/internal/tui/steps.go` | Route TUI selected-ID construction through the reducer. |
| `jarvis-cli/internal/tui/nontui.go` | Reuse one reduced ID list for apply and persistence; remove unordered map iteration. |
| `jarvis-cli/cmd/jarvis/cmd_sync.go` | Prepare the copied expanded state, pass Zoho bookkeeping intent, and render newly durable IDs. |
| `jarvis-cli/internal/sync/backup.go` | Move bookkeeping after final verification, cover verified no-op runs, and return verification/addition facts. |
| `jarvis-cli/internal/sync/bookkeeping.go` | Add the locked Zoho merge and return actual durable additions. |

No production change is expected in `state/state.go`, `sync/plan.go`, `sync/runner.go`, `sync/verify.go`, installer code, embedded skills, or generated user-machine files.

### Tests

Prefer focused edits to the existing tests named above plus one new `zoho_pack_test.go`. Do not add a broad new harness.

The strict success, failure, concurrency, and report cases will likely push the complete change beyond 400 reviewed lines even though production code remains small. Preserve the following natural review boundary if the task forecast confirms that risk:

1. catalog contract plus TUI/non-TUI convergence;
2. sync expansion, post-verification transaction, concurrency, and reporting.

Each slice should stay independently testable and target fewer than 400 changed lines. The tasks phase must provide the actual forecast and trigger the configured `ask-on-risk` decision before apply; this design does not pre-authorize a chained delivery strategy.

## 9. Alternatives and tradeoffs

### Chosen: explicit `ZohoPack` plus post-verification bookkeeping

- **Benefits:** one membership rule; no schema; deterministic IDs; existing planner/runner safety is reused; one locked save provides accurate reporting and concurrency safety.
- **Cost:** `sync.Run` must expose verification state and reorder bookkeeping, so focused regression tests are required.

### Alternative: commit expansion in `cmd_sync.go` after `sync.Run`

- **Benefit:** fewer changes to the sync runner.
- **Rejected tradeoff:** the command would have to reinterpret `RunResult`, convergence, no-op verification, lock semantics, and reporting outside the existing bookkeeping boundary. That creates two state-write paths and makes it easier to persist after only apparent success.

### Alternative: keep the seven V0 IDs in prompt and sync lists

- **Benefit:** smallest immediate diff.
- **Rejected tradeoff:** violates future catalog expansion and repeats the drift that caused the issue.

### Alternative: add a generic skill-pack framework or pack marker to schema

- **Benefit:** could support hypothetical future packs explicitly.
- **Rejected tradeoff:** expands architecture and migration surface for one approved pack, exceeds the review budget more easily, and is unnecessary because the released anchor plus concrete IDs fully represents selection.

### Alternative: persist before apply and roll state back on failure

- **Benefit:** planner can read durable expanded state directly.
- **Rejected tradeoff:** rollback adds another failure mode, cannot safely undo concurrent edits, and violates the requirement that failed convergence leave prior desired state durable.

## Review checklist

- One prefix-based, catalog-derived Zoho contract; no member list outside tests/specification.
- One pack prompt in TUI and non-TUI; every catalog Zoho member is excluded from auto-selection.
- Fresh selected/deselected state uses concrete IDs and preserves unrelated ownership.
- One copied expanded state feeds rendering, plan, runner, backup, and verification.
- No Zoho persistence occurs before all agents converge and final verification passes.
- Locked re-read preserves unrelated changes and rechecks the anchor before saving.
- Added-ID output comes only from successful durable mutation and is lexicographic.
- No `config.Save()` in sync, no schema change, no uninstall, and no #547 scope.
