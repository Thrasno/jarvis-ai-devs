# Proposal: Bounded, Verifiable Apply Progress

## Intent

Replace the unbounded cumulative `apply-progress` artifact with `jarvis.sdd-apply-progress/v2`: a bounded canonical snapshot that references ordered, immutable evidence batches. This removes the current continuation deadlock at Hive's 50,000-rune limit while preserving every required TDD and task-completion record, detecting stale writers, and making apply, status, verify, and archive fail closed on incomplete or inconsistent progress.

The outcome is a durable continuation protocol, not merely a smaller document. No backend may silently truncate evidence, select an implicit latest batch, or report a successful checkpoint that was not durably committed.

## Product Outcomes

- Long-running apply sessions can checkpoint and continue without rewriting all prior evidence.
- Users and automation receive a structured continuation result when a write cannot proceed within protocol bounds.
- Every completed task is traceable to exact ordered evidence batches whose identities and hashes are validated before lifecycle progression.
- Concurrent or stale agents cannot overwrite a newer canonical snapshot.
- OpenSpec, Hive, and hybrid storage expose equivalent valid, invalid, continuation, conflict, and legacy states.
- Existing cumulative artifacts remain readable and can be upgraded without losing evidence.

## Scope

### In Scope

- Define the `jarvis.sdd-apply-progress/v2` snapshot and immutable evidence-batch contracts.
- Define deterministic serialization, hashing, stable task coverage, generation/revision behavior, and bounded payload rules.
- Add a dedicated Hive guarded apply-progress write primitive with transactional compare-and-swap, idempotency, and durable receipts; keep it separate from `mem_save`.
- Resolve a snapshot and its exact referenced batches in Hive, OpenSpec, and hybrid modes.
- Lazily validate v2 progress in apply, status, verify, and archive before trusting completion or routing to a later phase.
- Return typed invalid, conflict, capacity, and continuation outcomes with actionable recovery data.
- Migrate legacy cumulative apply progress on the next mutating apply operation while preserving read compatibility before migration.
- Update embedded apply/orchestrator source assets and their generated-asset contract tests only after the storage and reader capabilities exist.
- Preserve immutable evidence through archive and backend synchronization.

### Non-Goals

- Raising or removing Hive's general 50,000-rune `mem_save` limit.
- Changing `mem_save` from insert semantics or overloading it with revision/CAS behavior.
- Reusing delete/restore mutation guards for apply progress.
- Creating a general-purpose lease, transaction, or compare-and-swap API for arbitrary memories.
- Redesigning general Hive ↔ Hive API memory synchronization or `jarvis sync`.
- Automatically choosing one backend as authoritative when hybrid stores disagree.
- Silently compressing, summarizing, dropping, reordering, or repairing required evidence.
- Changing SDD task meaning, verification policy, archive eligibility, or unrelated generated agent configuration.
- Implementing code as part of this proposal phase.

## Proposed Contract Decisions

These choices settle proposal-level ambiguity; specs and design must formalize exact field schemas and state transitions without reopening the approved product behavior.

### Bounded Documents

- Both the serialized canonical snapshot and each serialized evidence batch have a protocol maximum of **40,000 Unicode runes**, measured exactly as Hive measures its 50,000-rune limit. The 10,000-rune reserve covers storage envelopes and future compatible metadata.
- Writers measure the final serialized document before any backend write. A document over the protocol maximum is rejected consistently in OpenSpec, Hive, and hybrid modes.
- Evidence is split only at complete evidence-entry boundaries. A writer persists a valid current batch and advances the snapshot before returning `continuation_required`; it never cuts or rewrites an entry to fit.
- If one indivisible evidence entry or the snapshot reference set cannot fit, the operation returns `evidence_item_too_large` or `snapshot_capacity_exhausted` and does not advance completion. This proposal does not weaken evidence requirements to bypass capacity.

### Identity, Serialization, and Integrity

- Snapshot and batch documents identify the schema as `jarvis.sdd-apply-progress/v2` and `jarvis.sdd-apply-evidence/v2` respectively.
- Batch IDs are opaque, collision-resistant identifiers generated from cryptographic randomness and encoded as lowercase `apb-` plus 32 hexadecimal characters. Ordering comes only from the snapshot's reference list, never lexical ID order or creation time.
- Batch hashes use SHA-256 over canonical UTF-8 JSON for the complete batch envelope except its own hash field. Canonical JSON uses fixed typed fields, fixed field order, LF line endings, no insignificant whitespace, and no unordered maps.
- The snapshot contains its generation, guarded revision, task-manifest digest, lifecycle status, ordered task coverage, and the exact ordered `{batch_id, sha256}` list. A canonical snapshot digest, computed with the same rules while excluding its own digest and backend receipt fields, is used for cross-backend comparison.
- Hashes cover evidence content and its task attribution. Snapshot metadata is protected by the snapshot digest and guarded write, not folded into each batch hash.
- Duplicate batch IDs, duplicate task completion, an unexpected order, an unreferenced batch treated as evidence, a missing batch, a hash mismatch, or task coverage inconsistent with the task manifest is invalid.

### Task Identity and Coverage

- V2 uses stable task IDs carried by the task artifact. The task-manifest digest covers ordered task IDs and normalized task text so task edits are detectable.
- For legacy task artifacts without explicit IDs, migration derives IDs deterministically from the hierarchical task path plus normalized task text and records the source task-manifest digest.
- Every evidence entry names one or more task IDs. Snapshot coverage is the ordered reconciliation of referenced evidence against the current task manifest; a completion marker alone is never sufficient.
- A post-checkpoint task edit causes a typed `task_manifest_mismatch`. It requires explicit task/progress reconciliation on a later apply and cannot be silently interpreted as completed work.

### Guarded Hive Write and Wire Semantics

- Hive exposes a dedicated apply-progress advance operation through storage, daemon governance, HTTP API/MCP, and the CLI client. It is not routed through `SaveMemory`/`mem_save`.
- One Hive transaction stores any new immutable batches, verifies existing same-ID batches are byte-identical, compares the caller's expected revision and generation, advances the canonical snapshot, and stores an idempotency receipt.
- Requests carry project/change identity, `expected_revision`, `expected_generation`, a collision-resistant `request_id`, the proposed batches, and the proposed snapshot.
- Success returns the committed revision, generation, snapshot digest, and durable receipt. Retrying the same request ID with the same canonical payload returns the original success; reusing it with different content fails closed.
- A stale comparison returns a typed conflict (`HTTP 409` at the HTTP boundary and the equivalent structured MCP result) containing the current revision, generation, and snapshot digest, without committing any proposed change.
- Capacity, validation, conflict, and backend errors are distinct machine-readable outcomes. Transport loss is recovered by retrying the same request ID before attempting a new generation.

### Legacy Migration

- Status, verify, and archive may read legacy cumulative progress through a conservative compatibility parser; malformed or ambiguous legacy completion fails closed.
- Read-only commands never mutate or migrate progress.
- The next mutating apply operation performs an explicit in-flow upgrade: parse legacy tasks/evidence, create bounded immutable batches, validate complete coverage, then commit the initial v2 snapshot as generation 1 through the guarded backend path.
- The legacy source remains intact until the v2 snapshot and all referenced batches are durably committed. A failed upgrade leaves legacy state authoritative and returns a structured migration failure.
- No standalone migration command or eager/background migration is introduced in this change.

### OpenSpec and Hybrid Semantics

- OpenSpec stores the canonical snapshot at `apply-progress.md` and immutable batches beneath a dedicated change-local apply-evidence directory. Snapshot replacement uses an atomic temporary-file rename and an expected generation/digest check.
- Hive stores the equivalent logical documents through its dedicated guarded path. General memory insertion remains unchanged.
- Hybrid readers load and validate each configured backend independently. They consider progress equivalent only when schema version, generation, task-manifest digest, snapshot digest, ordered batch IDs/hashes, and validated task coverage agree.
- A missing side, one valid and one invalid side, or two valid but different sides returns `backend_diverged`; the existing Hive-wins merge rule does not apply to v2 progress.
- Hybrid writes append immutable batches to both stores before advancing either snapshot. Because there is no distributed transaction, a durable continuation receipt records per-backend outcomes. Retry with the same request completes the missing side; readers remain fail closed until both snapshots agree. No automatic winner or destructive repair is allowed.
- Archive validation occurs before moving/marking the complete topology. OpenSpec moves the snapshot and evidence directory together; Hive retains resolvable snapshot-to-batch references under the archived change scope. Orphaned batches may remain harmlessly stored but never count as evidence.

## Component and Ecosystem Boundaries

| Component | Responsibility and impact |
| --- | --- |
| SDD apply/orchestrator sources | Emit bounded batches, request guarded snapshot advances, preserve receipts, and route structured continuation/conflict recovery. They do not implement storage safety themselves. |
| `jarvis-cli/internal/sddstatus` | Parse and lazily validate progress, reconcile task coverage, expose fail-closed lifecycle routing, and report hybrid disagreement. |
| `jarvis-cli/internal/hiveclient` | Carry typed guarded-write/read models and preserve daemon conflict/idempotency outcomes without translating them into generic `mem_save`. |
| Hive daemon MCP/HTTP/governance | Authenticate/route the dedicated operation, enforce document limits and validation, and expose deterministic snapshot-plus-batch retrieval. |
| Hive SQLite persistence | Own atomic immutable-batch insertion, snapshot CAS, revision/generation advancement, and durable idempotency receipts. |
| Hive ↔ Hive API sync | Replicate committed v2 records using existing local/shared separation; missing or reordered replicated data remains invalid until complete. This change does not redefine general sync or claim shared completion from a local receipt. |
| OpenSpec filesystem | Store the same logical topology with atomic guarded snapshot replacement and retain batches with the change. |
| Hybrid mode | Compare independently validated states and expose divergence; it does not apply Hive precedence to v2. |
| Verify/archive | Consume validated evidence only and retain exact references through lifecycle transitions. |
| Installer/generated configuration | Only source assets and render/contract expectations may change. Generated developer-machine files are never edited directly. |
| Todoist backlog | No behavior change; issue/project workflow remains outside apply-progress persistence. |

## Affected Areas

- `hive-daemon/internal/db/` — v2 records, revisions, receipts, transactions, retrieval, and concurrency tests.
- `hive-daemon/internal/governance/`, `internal/mcp/`, and `internal/httpapi/` — guarded operation and typed responses.
- `jarvis-cli/internal/hiveclient/` — client wire models and conflict/idempotency handling.
- `jarvis-cli/internal/sddstatus/` — contract parsing, validation, legacy compatibility, backend parity, and lifecycle routing.
- OpenSpec source adapters — snapshot/batch persistence, expected-state checks, archive retention, and crash recovery.
- `jarvis-cli/embed/skills/sdd-apply/SKILL.md` and `jarvis-cli/embed/orchestrator/sdd-orchestrator.md` — continuation behavior after capabilities are available.
- Source-asset, parser, database, API/MCP, filesystem, hybrid, migration, and lifecycle test suites.

## Explicit Work Units

Each unit follows strict TDD (RED → GREEN → TRIANGULATE/REFACTOR), keeps tests with behavior, and is independently revertible. These units are intended as future commit/PR boundaries rather than file-type splits.

1. **V2 contract and deterministic validation core** — Add typed snapshot/batch models, 40,000-rune checks, canonical JSON/SHA-256 rules, stable task coverage, structured errors, and conservative legacy conversion. Include table-driven parser, hash, ordering, coverage, capacity, and migration tests. Forecast: 250–350 changed lines.
2. **Hive guarded snapshot advance** — Add dedicated schema/storage transaction, immutable batch enforcement, generation/revision CAS, durable request receipts, HTTP/MCP/client contract, and concurrent/stale/idempotent writer tests. Do not mix with general status routing or `mem_save`. Forecast: 350–500 lines; this is the mandatory separate net-new CAS work unit.
3. **Hive retrieval and lifecycle enforcement** — Resolve exact referenced batches and make apply/status/verify/archive consume validated state and typed continuation/conflict failures. Include missing, reordered, duplicated, corrupt, delayed-sync, and archive tests. Forecast: 300–450 lines.
4. **OpenSpec persistence, migration, and hybrid parity** — Add file topology and atomic expected-state replacement, next-apply legacy upgrade, independent dual-backend validation, divergence/receipt recovery, and archive retention. Forecast: 300–450 lines.
5. **Executor/orchestrator behavior** — Update source-of-truth assets to append batches, advance snapshots, and recover from continuation/conflict/migration outcomes; verify generated output contracts without editing generated local files. Forecast: 200–300 lines.

Dependency order: **1 → 2 → 3 and 4 → 5**. Units 3 and 4 may be separate implementation lanes only after unit 1 is stable. Unit 2 must remain cohesive and must not be split across unrelated PRs.

## Review Workload and Delivery Gate

- Forecast: **1,400–2,050 changed lines**.
- 400-line budget risk: **High**.
- Chained PRs recommended: **Yes**.
- Delivery strategy: **ask-on-risk**.
- Decision needed before apply: **Yes**.

Planning may continue through specs, design, and tasks. Before implementation, the parent must pause for a human-selected chain strategy or explicit `size:exception`. This proposal does not grant either approval. Some honest units, especially the cohesive CAS unit, may exceed 400 lines; tasks must report the smallest reviewable slices rather than code-golfing tests or safety behavior.

## Risks and Mitigations

| Risk | Impact | Mitigation |
| --- | --- | --- |
| Snapshot or batch still exceeds Hive's limit | Continuation remains blocked | Enforce a shared 40,000-rune protocol limit on final serialization and return typed capacity outcomes before writes. |
| CAS exists only in SQLite | Agents remain vulnerable to stale writes | Carry expected state, receipts, and conflict responses through MCP, HTTP, and client boundaries. |
| Partial hybrid commit creates split brain | Lifecycle may trust incomplete progress | Validate both sides independently, return `backend_diverged`, and recover only by idempotent same-request completion. |
| Legacy parser overstates completion | Invalid work could verify/archive | Require unambiguous task/evidence coverage and fail closed; preserve source until successful v2 commit. |
| Sync delay or reordering hides evidence | Shared status may be overstated | Validate exact snapshot references; distinguish local commit from shared availability. |
| Canonicalization differs by backend | False hash mismatch or concealed corruption | Use one shared typed canonical serializer and cross-backend golden vectors. |
| Task edits invalidate prior evidence | Apply may deadlock or misattribute work | Bind snapshots to task-manifest digest and require explicit reconciliation. |
| Prompt rollout precedes capability rollout | Agents call unsupported operations | Deliver executor/orchestrator changes last and guard them with capability/contract tests. |
| Immutable batches accumulate | Storage grows over time | Retain required evidence through archive; defer general retention/GC policy and ensure orphaned batches are non-authoritative. |

## Rollback Plan

- Roll back executor/orchestrator emission first so no new v2 writes are initiated.
- Keep v2 readers and legacy compatibility available while any v2 data exists; never revert to treating a v2 snapshot as opaque legacy text.
- Disable the guarded-write route if needed without deleting committed snapshots, batches, revisions, or receipts.
- Resume legacy cumulative writes only for changes that never committed v2. A change with committed v2 remains read-only/fail-closed until the v2 path is restored.
- OpenSpec and Hive v2 data are additive and non-destructive; rollback does not rewrite or discard immutable evidence.
- Any schema migration must be backward-safe for older binaries (ignored additive tables/records) or accompanied by a compatibility gate defined in design.

## Success Criteria

- [ ] Apply progress can exceed 50,000 total runes across batches while every individual snapshot/batch remains within the 40,000-rune protocol budget.
- [ ] A continuation durably commits all complete evidence entries and returns a machine-readable next action; no path silently truncates or falsely reports persistence.
- [ ] Snapshot generation, guarded revision, task-manifest digest, ordered batch identities/hashes, and task coverage are deterministic and validated.
- [ ] Missing, reordered, duplicated, corrupt, mismatched, or incomplete evidence blocks apply/status/verify/archive with a typed actionable state.
- [ ] Concurrent stale writers are rejected through the public daemon boundary; idempotent retry after transport loss cannot duplicate or fork progress.
- [ ] Legacy cumulative progress is conservatively readable and upgrades on the next mutating apply without loss or read-triggered mutation.
- [ ] OpenSpec, Hive, and hybrid modes produce equivalent valid/invalid/conflict/continuation behavior, and hybrid disagreement is never hidden by Hive precedence.
- [ ] Archive retains a resolvable, validated snapshot-to-evidence topology.
- [ ] `mem_save`, general Hive synchronization, `jarvis sync`, delete/restore guards, unrelated SDD semantics, and generated local configuration remain unchanged.
- [ ] Focused tests cover canonicalization, Unicode limits, migration, SQLite concurrency, API/MCP contracts, OpenSpec atomicity, hybrid crash recovery, lifecycle blocking, and source-asset generation; relevant `go test` and `go vet` checks pass during implementation.
