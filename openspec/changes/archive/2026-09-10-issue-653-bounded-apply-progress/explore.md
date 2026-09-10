## Exploration: issue-653-bounded-apply-progress

### Current State

`apply-progress` is a single, cumulative artifact. The embedded apply executor requires a continuation to read the prior artifact, merge every completed task and its TDD evidence, then save one combined replacement. In Hive, however, `mem_save` rejects content over 50,000 Unicode runes and always inserts a new memory row; it cannot replace an artifact or compare a revision. The reported 162,725-rune progress artifact therefore cannot be saved, causing an apply continuation to lose its required durable checkpoint and deadlock.

Hive's SDD reader selects the latest active memory for each logical artifact topic by `(created_at DESC, id DESC)`. The CLI status reader considers apply progress complete based only on exact `status: complete`/`status: partial` markers, with a legacy fallback to task checkbox completion. OpenSpec reads one `apply-progress.md`; hybrid merges both sources with Hive content taking priority. Neither path currently understands a manifest/snapshot, evidence batches, hash validation, generation, or evidence task coverage.

### Affected Areas

- `hive-daemon/internal/mcp/tools.go` and `hive-daemon/internal/mcp/server.go` — `mem_save` exposes only insert semantics and enforces the 50,000-rune limit; `MemoryStore` will need a distinct guarded write capability, not an overloaded save.
- `hive-daemon/internal/db/memory.go` and `hive-daemon/internal/db/db.go` — current persistence assigns a new `sync_id` for every save; the only guarded mutation is delete/restore. A new transactional CAS/revision write path and durable receipt/revision data are required.
- `hive-daemon/internal/db/sdd.go`, `hive-daemon/internal/governance/sdd.go`, and `hive-daemon/internal/httpapi/server.go` — current Hive projection returns one opaque content value per artifact and must expose/retrieve the v2 snapshot plus referenced immutable evidence deterministically.
- `jarvis-cli/internal/hiveclient/client.go` — Hive client types/routes must carry the new guarded write and/or resolved apply-progress representation.
- `jarvis-cli/internal/sddstatus/source.go` and `jarvis-cli/internal/sddstatus/status.go` — apply/status/verify/archive routing must lazily validate v2 invariants and fail closed rather than trusting a marker alone.
- `jarvis-cli/embed/skills/sdd-apply/SKILL.md` and `jarvis-cli/embed/orchestrator/sdd-orchestrator.md` — current continuation instructions mandate an ever-growing merged artifact; they must instead append bounded immutable evidence and advance a canonical snapshot through the new guarded write path.
- Associated `*_test.go` files in the above packages — parser/validation, SQLite concurrency, Hive API, OpenSpec, hybrid parity, migration, and generated-asset contract coverage need focused tests.

### Product Decisions Already Fixed

The approved issue fixes these decisions; proposal/design/specs must treat them as constraints rather than reopen them:

1. `jarvis.sdd-apply-progress/v2` is a bounded canonical snapshot plus immutable evidence batches, not a cumulative evidence document.
2. The canonical snapshot records the exact ordered evidence batch IDs and hashes, generation, and task coverage. These are authoritative integrity inputs, not display-only metadata.
3. Apply, status, verify, and archive perform lazy validation. Invalid, missing, reordered, duplicated, mismatched, or incomplete evidence must fail closed.
4. Oversized continuation is structured: return a machine-readable continuation outcome. Never silently truncate, summarize away required evidence, or claim a save succeeded.
5. Legacy cumulative progress must migrate to v2 without losing completion evidence.
6. OpenSpec, Hive, and hybrid modes must have equivalent observable validity and routing behavior. Hybrid must not conceal disagreement through its current Hive-wins merge rule.
7. Hive has no usable CAS/revision/lease through `mem_save`; `SaveMemory` always inserts. Existing guarded mutation is only delete/restore. Stale-writer protection is net-new write-path work and must remain its own work unit.

### Approaches

1. **Keep one progress artifact and compress/truncate it**
   - Pros: small apparent code change.
   - Cons: violates the approved no-truncation, exact-evidence, and legacy-preservation requirements; cannot prevent the observed limit failure.
   - Effort: Low, but rejected.

2. **Store batches but make the latest batch implicitly authoritative**
   - Pros: avoids a large cumulative write.
   - Cons: cannot prove ordered coverage or detect missing/reordered batches; races can select an unintended writer; fails the canonical-snapshot requirement.
   - Effort: Medium, but rejected.

3. **Use a bounded v2 snapshot with immutable batches and a separate CAS write primitive**
   - Pros: preserves complete evidence, bounds each write, enables exact validation and stale-writer rejection, and supports equivalent filesystem/Hive representations.
   - Cons: spans daemon storage/MCP/API, CLI status parsing, migration, and generated SDD instructions.
   - Effort: High; recommended.

### Recommendation

Adopt approach 3 through cohesive, independently reviewable work units. Define a shared v2 document model and deterministic validator first, then introduce the new Hive CAS primitive as an isolated persistence/API work unit. Make all readers validate the snapshot lazily and return a structured invalid/continuation state before changing executor prompts to emit batches. Do not change `mem_save` semantics or repurpose delete/restore guards.

Recommended work units (each includes its own RED/GREEN/REFACTOR tests):

1. **V2 contract and validation core** — define bounded snapshot and immutable-batch schemas, canonical serialization/hash rules, task-coverage reconciliation, typed validation/continuation errors, and legacy-to-v2 conversion. Target pure Go code plus tests; roughly 250–350 changed lines.
2. **Hive guarded snapshot write primitive (separate CAS work unit)** — add revision/generation storage, transactional compare-and-swap, idempotency/receipt semantics, MCP/API/client contract, and stale-writer/concurrent-writer tests. Do not route it through `mem_save`. Roughly 350–500 lines; likely at or above the review budget.
3. **Hive retrieval and lazy status enforcement** — resolve snapshot plus exact batches, validate on apply/status/verify/archive paths, and make invalid/oversized states block safely with actionable structured data. Roughly 300–450 lines.
4. **OpenSpec and hybrid parity** — persist the same snapshot/batch topology in the change folder, validate both backends independently, detect disagreement rather than silently preferring Hive, and migrate legacy file/Hive artifacts. Roughly 300–450 lines.
5. **Executor/orchestrator source assets and contract tests** — replace cumulative merge instructions with bounded batch append/CAS-advance/continuation recovery and update generated-asset tests. Roughly 200–300 lines.

The dependency order is 1 → 2 → 3/4 → 5. Units 3 and 4 may become parallel implementation slices only after the contract is stable; do not split the CAS primitive across unrelated PRs.

### Review Workload

Forecast: approximately 1,400–2,050 changed lines across five work units, with high integration and migration risk. This exceeds the 400-line budget. Under `ask-on-risk`, planning may proceed, but tasks must set `400-line budget risk: High`, `Chained PRs recommended: Yes`, and `Decision needed before apply: Yes`; implementation must pause for a human-selected chain strategy or explicit `size:exception`. The smallest credible first slice is the CAS primitive or the pure contract core, not a mixed cross-layer change.

### Genuine Unresolved Decisions

- The exact bounded payload budget for snapshots and batches (including Unicode/rune accounting and serialization overhead) must leave headroom under Hive's 50,000-rune hard limit.
- The canonical batch ID format, hash algorithm, canonical byte serialization, and whether hash coverage includes snapshot metadata need a single formal definition.
- The precise v2 task identity/coverage mapping is not yet defined: stable task IDs versus ordered task entries, and how task edits after a batch are detected/migrated.
- The public CAS wire contract needs a conflict response and idempotency model (expected generation/revision, request ID, returned current state) that callers can recover from safely.
- Legacy migration timing and write policy need definition: read-only interpretation until next apply, automatic upgrade on next guarded write, or explicit migration command; it must not mutate either backend silently.
- Hybrid disagreement behavior needs a user-visible structured state and repair direction, including partial persistence/crash recovery when only one backend receives a v2 update.
- Archive retention must specify whether batches remain in the active artifact topology until the entire change folder/topic set moves, and how archive validation references them.

### Risks

- A CAS design that is only local SQLite-safe but not represented through the MCP/API boundary will not protect agent writers.
- Current Hive latest-row projection and hybrid Hive-wins precedence can mask split-brain or stale data unless readers validate the exact snapshot-referenced batch set.
- Prompt changes without the write primitive would direct agents to an operation Hive cannot safely perform.
- Migration can accidentally reinterpret legacy free-form progress as complete; compatibility must remain fail-closed and preserve evidence.
- The strict-TDD project policy means all behavioral work needs RED/GREEN/REFACTOR evidence and focused module tests; no tests were run during this exploration.

### Ready for Proposal

Yes. The approved issue supplies the core product decisions. The proposal should lock the unresolved wire/format/migration choices above, state the cross-component non-goals, and preserve the separate CAS work unit and review-budget gate.
