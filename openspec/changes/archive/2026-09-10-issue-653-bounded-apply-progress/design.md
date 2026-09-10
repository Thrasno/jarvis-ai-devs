# Design: Bounded, Verifiable Apply Progress

## Technical Approach

Add shared protocol code in `hivederive/applyprogress`; both daemon and CLI use identical canonicalization/validation. Agents checkpoint through `jarvis sdd progress advance` (OpenSpec/hybrid coordinator) or MCP/HTTP (Hive). Every lifecycle read resolves one snapshot and exactly its referenced immutable batches before routing.

## Architecture Decisions

| Choice | Tradeoff / rationale |
| --- | --- |
| Dedicated v2 package and write path; never call/change `SaveMemory`/`mem_save` | Avoids duplicate serializers and preserves insert semantics. |
| Store Hive documents as memory rows plus additive head/receipt tables | Existing Hive↔Hive API opaque-memory sync carries data unchanged; sync only rematerializes heads and rejects forks. |
| Lock + atomic rename for OpenSpec | A change-local exclusive lock makes expected-state check and replacement one operation across processes. |
| Fail-closed hybrid coordinator | No distributed transaction exists; durable receipts permit same-request repair without choosing a winner. |

## Contracts

`Snapshot{Schema,Project,Change,Generation,Revision,PreviousDigest,TaskManifestSHA256,Status,Coverage,Batches,Digest}`; `Coverage{TaskID,BatchID,EntryID}`; `BatchRef{BatchID,SHA256}`. `Batch{Schema,Project,Change,BatchID,Entries,SHA256}`; `EvidenceEntry{EntryID,TaskIDs,CompletesTaskIDs,Kind,Summary,Command,ExitCode,Outcome,Files}`. Slices are non-nil; no maps or optional fields. Status is `partial|complete`; kind is `red|green|triangulate|refactor|verification|delivery`; outcome is `pass|fail|not_run`. IDs match `[A-Za-z0-9][A-Za-z0-9._-]{0,63}`; batches are `apb-` + 32 lowercase random hex; entry IDs are unique within a batch.

New tasks carry their hierarchical token (`1.1`) as ID. Legacy IDs are `legacy-` plus the first 32 hex SHA-256 digits of `hierarchical-path + "\n" + normalized-text`. Normalization is UTF-8 NFC, CRLF/CR→LF, trim, then collapse every Unicode whitespace run to one ASCII space. Manifest digest is SHA-256 of canonical JSON `[{"id":...,"text":...}]` in task order.

Canonical JSON uses typed struct field order above, UTF-8, JSON integer/string rules, HTML escaping disabled, no whitespace/BOM/final LF; unknown/duplicate fields, invalid UTF-8, trailing values, and non-canonical input fail. Batch SHA excludes only `sha256`; snapshot digest excludes only `digest` (there are no receipt fields). Final JSON including its hash/digest is checked with `utf8.RuneCountInString <= 40000` before any write.

Validation order: canonical decode/re-encode; schema/identity/limits; hashes; current manifest digest; unique ordered refs; fetch only referenced batches; verify batch identity/bytes/hash; reconcile entries in ref/entry order. Every attributed task must exist; each completed task appears once in `CompletesTaskIDs`; `Coverage` must equal manifest-ordered completion triples; `complete` requires full coverage, `partial` forbids it. Extra/orphan batches are ignored; missing, duplicate, reordered/forked, corrupt, or edited-task states return typed reasons.

## Data Flow and Persistence

```text
executor → CLI coordinator → validate/split → OpenSpec and/or Hive CAS
                                      ↓                 ↓
status/apply/verify/archive ← resolve + lazy validate ← snapshot+batches
```

SQLite migration in existing `hive-daemon/internal/db/db.go` adds `sdd_apply_heads(project,change_name,snapshot_memory_id,generation,revision,digest,PRIMARY KEY(project,change_name))` and `sdd_apply_receipts(request_id PRIMARY KEY,project,change_name,payload_sha256,response_json,created_at)`. New `internal/db/apply_progress.go` performs one transaction: receipt lookup; expected revision/generation/digest check; byte-identical same-ID batch reuse; memory rows/journal events; head update; receipt. Generation and revision must each be expected+1; retrying identical request returns the stored response; changed payload returns `request_id_conflict`; stale state commits nothing. Pulled v2 rows rebuild only a unique validated digest chain; forks stay invalid.

OpenSpec topology is `apply-progress.md`, `apply-evidence/<batch-id>.json`, and `.apply-progress-receipts/<request-id>.json`. Under exclusive `apply-progress.lock`, write/fsync immutable batches with create-only equality checks, persist receipt, recheck expected state, then write/fsync temp snapshot and rename; fsync directory. A crash leaves the old snapshot authoritative. Hybrid persists a receipt before publication, records `pending|committed|failed` per backend after each step, writes batches both sides before snapshots, and retries only missing outcomes with the same request. Until equal validated digests, readers return `backend_diverged`.

HTTP `GET /sdd/changes/{change}/apply-progress` resolves; `POST .../advance` advances. MCP tools are `sdd_apply_progress_get/advance`. Success/idempotent is 200; conflict/request reuse 409 with current generation/revision/digest; capacity 413; validation/migration 422; unavailable 503. All boundaries return `outcome`, `code`, committed/current state, receipt, and recovery; `continuation_required` is a successful committed checkpoint with the next unpersisted entry. Client preserves these typed outcomes rather than flattening `APIError`.

Legacy reads remain marker/checkbox-compatible but ambiguous input is invalid. First mutation converts complete entries at boundaries and commits generation/revision 1; source remains until success. Archive validates first, then moves the whole OpenSpec change (including evidence/receipts) atomically by directory rename; Hive leaves resolvable rows/head and marks archive only through normal SDD artifacts.

## File Changes

| Paths | State/action |
| --- | --- |
| `hivederive/applyprogress/{model,canonical,validate,legacy}.go` + tests | proposed new: protocol owner |
| `hive-daemon/internal/db/{db.go,sdd.go}`; `internal/{governance/sdd.go,mcp/server.go,mcp/tools.go,httpapi/server.go}` + tests | existing modify |
| `hive-daemon/internal/db/apply_progress.go` + tests | proposed new: cohesive CAS/retrieval |
| `jarvis-cli/internal/hiveclient/client.go`, `internal/sddstatus/{source,status}.go`, `cmd/jarvis/cmd_sdd.go` + tests | existing modify |
| `jarvis-cli/internal/sddprogress/{store,openspec,hybrid}.go` + tests; `cmd/jarvis/cmd_sdd_progress.go` | proposed new |
| `jarvis-cli/embed/skills/{sdd-apply,sdd-archive}/SKILL.md`, `embed/orchestrator/sdd-orchestrator.md`, `internal/skills/catalog_contract_test.go` | existing source assets/tests; no generated machine files |

## Strict TDD, Rollout, and Work Units

Each unit starts with table-driven/pure tests or `t.TempDir`/SQLite/`httptest` concurrency tests, then minimum code, triangulation, refactor, focused module tests, module suite, and vet. Order/forecast: (1) contract 300–400; (2) **cohesive CAS** 450–600; (3) retrieval/lifecycle 350–500; (4) OpenSpec/hybrid/legacy/archive 450–650; (5) assets 200–300 lines. Total 1,750–2,450: high risk, chained PRs recommended, decision required before apply under `ask-on-risk`; CAS remains one work unit even above 400.

Roll out 1→2→3→4→5. Roll back assets first, then disable writers; retain readers, migrations, rows, receipts, and legacy compatibility. Never downgrade a committed v2 change to legacy writes; restore writers before cleanup. No destructive down-migration or evidence GC is included.

## Final-Verification Completion Amendment

This amendment is authoritative only for the checkpoint-planning gap found by final verification. Everything above remains the implemented architectural and historical baseline: v2 documents, canonicalization, validation, legacy migration, backend resolution, OpenSpec durability, Hive CAS, hybrid receipts, lifecycle blocking, archive behavior, and the low-level public boundaries are preserved.

### Approved Minimal Completion

| Boundary | Completion decision |
| --- | --- |
| Shared protocol | Add one pure entry-boundary planner in `hivederive/applyprogress`. It owns fitting complete entries into one new immutable batch and its next snapshot; it performs no I/O, locking, ID generation, retry, or backend selection. |
| High-level writer | Add sibling CLI command `jarvis sdd progress checkpoint`. It accepts ordered complete entries plus a stable base snapshot/cursor and wraps the planner's result in the existing `sddprogress.AdvanceRequest`. |
| Existing low-level writer | Keep `jarvis sdd progress advance` unchanged as the expert/internal command for submitting an already planned batch and snapshot. It remains a defensive guarded-write boundary, not a planner. |
| Backends | Reuse the existing `OpenSpec.Advance`, Hive client `AdvanceApplyProgress`, and `Hybrid.Advance` paths, including their CAS, immutable-batch checks, durable receipts, missing-side repair, and legacy behavior. No second persistence path is introduced. |
| Daemon surfaces | Keep the existing HTTP GET/POST and MCP get/advance endpoints, request/response schemas, status mapping, and generic defensive `capacity` code unchanged. There is no HTTP or MCP checkpoint endpoint. |

The earlier statement that executors checkpoint through `advance` remains true as historical low-level behavior. After this amendment is implemented, normal executor guidance moves to `checkpoint`; `advance` remains available and is what `checkpoint` delegates to after planning.

### Checkpoint Contract

`checkpoint` uses a canonical request file, parallel to `advance`, containing project/change identity, current tasks, a nullable base snapshot for generation zero, expected generation/revision/digest, a collision-resistant `request_id`, a fresh random `batch_id`, the complete ordered entry stream, and a cursor (`entry_index`, `entry_id`). The index is zero-based in that stable stream; the ID must equal the entry at the index. IDs are supplied and retained in the request file so a process restart can reproduce the same payload.

A successful response includes the existing committed state coordinates and the committed snapshot. When more entries remain it also includes both:

- `next_entry_index`: zero-based index of the first uncommitted entry in the submitted stream.
- `next_entry_id`: that entry's exact `entry_id`, used as a guard against a changed or reordered stream.

The next checkpoint starts from that returned snapshot/state and cursor, with a new request ID and new batch ID. A retry of the same attempted prefix uses the original base snapshot, cursor, request ID, batch ID, and entries unchanged.

The checkpoint writer has exactly four domain outcomes:

| Outcome | Durable effect | Required response |
| --- | --- | --- |
| `committed` | All entries from the input cursor were committed by `Advance`. | Committed generation, revision, digest, and snapshot; no continuation cursor. |
| `continuation_required` | A non-empty maximal prefix was committed by `Advance`; no later entry was claimed. | Committed state/snapshot plus `next_entry_index` and `next_entry_id`. |
| `evidence_item_too_large` | No write. The first pending indivisible entry cannot fit in an otherwise empty canonical batch. | Offending entry coordinate and capacity detail. |
| `snapshot_capacity_exhausted` | No write. No non-empty next prefix can produce a canonical snapshot within 40,000 runes because its reference/coverage set is full. | Current state and recovery that protocol evolution or explicit reconciliation is required. |

These are checkpoint-writer outcomes, not replacements for existing operational errors. Validation failures, stale CAS conflicts, request-ID conflicts, backend divergence, transport failures, and migration failures retain the existing typed error/recovery contracts. The low-level `advance` command and HTTP/MCP boundaries may continue to return generic defensive `capacity`; they do not inspect an evidence stream and must not duplicate planner logic.

### Pure Planner and Boundary Algorithm

Conceptually, the planner receives `PlanInput{BaseSnapshot, Tasks, Entries, EntryIndex, EntryID, BatchID}` and returns a sealed batch, a sealed next snapshot, the exclusive end index, and a planned checkpoint disposition. `hivederive` does not import CLI packages; the coordinator converts that result into the already implemented `sddprogress.AdvanceRequest` with the caller's request ID and expected coordinates.

The planner:

1. Validates the base snapshot, task-manifest digest, cursor pair, IDs, entry values, and ordering without reading a backend.
2. Starts one new batch at the cursor and adds whole entries in order. It never slices, truncates, summarizes, or rewrites an entry.
3. After each addition, seals the candidate batch with existing canonical JSON/SHA-256 code, appends its reference, updates manifest-ordered coverage, increments generation/revision by one, and seals the candidate snapshot.
4. Retains the largest non-empty prefix for which both final canonical documents are at most 40,000 Unicode runes.
5. If the first entry cannot seal in an otherwise empty batch, plans `evidence_item_too_large`. If that entry can form a batch but no non-empty candidate snapshot can seal, plans `snapshot_capacity_exhausted`.
6. Otherwise returns one batch and one next snapshot. The coordinator emits `committed` only after `Advance` confirms the whole remaining stream was committed, or `continuation_required` only after `Advance` confirms the planned prefix was durably committed.

One checkpoint attempt intentionally creates at most one batch and one snapshot generation. This keeps one request ID aligned with one immutable payload and one existing receipt. It also makes every continuation an explicit, independently guarded commit rather than a hidden multi-commit loop.

### Data Flow

```text
stable entries + base snapshot + cursor + request/batch IDs
                         |
                         v
          hivederive pure boundary planner
          | capacity terminal (no write)
          | sealed one-batch/one-snapshot plan
                         v
       CLI checkpoint coordinator builds existing AdvanceRequest
                         v
     configured OpenSpec.Advance / Hive Advance / Hybrid.Advance
          | CAS + immutable batches + durable receipt
          | hybrid same-request missing-side recovery
                         v
 committed ----------------------> final coordinates/snapshot
 continuation_required ----------> final coordinates/snapshot
                                   + next_entry_index + next_entry_id
```

Before planning a new prefix, the coordinator resolves the configured backend's validated current snapshot and checks it against the supplied expected coordinates/base. OpenSpec uses the existing change-local topology, Hive uses the existing GET result, and hybrid accepts only equal independently validated snapshots. On an initial retry, a current snapshot equal to the already planned candidate is not treated as permission to replan; the coordinator resubmits the exact original `AdvanceRequest` so the existing receipt can return the original success.

### Races, Retries, and Failures

- **Lost response or uncertain publication:** replay the exact prefix with the same request ID, batch ID, base snapshot, cursor, and entries. Existing OpenSpec/Hive/hybrid receipts make that replay idempotent. Never change content under that request ID.
- **Continuation:** only after a committed `continuation_required`, use the returned snapshot and generation/revision/digest plus the returned next-entry pair. Generate a new request ID and batch ID for the next prefix.
- **Concurrent writer before commit:** existing CAS rejects the stale candidate and commits nothing. Return the existing conflict/current-state recovery. Re-resolve and replan from the new authority under a new request ID; do not mutate or recycle the stale request ID.
- **Hybrid partial publication:** return the existing divergence/recovery failure and retry the same request. Do not advance the cursor or create a new request ID until both sides record that prefix committed.
- **Planner capacity result:** call no backend and create no receipt, batch, snapshot, or completion coverage. A prefix already committed by an earlier call remains authoritative.
- **Unexpected low-level capacity:** treat generic `capacity` from `advance` as a defensive protocol/infrastructure failure and preserve its existing recovery envelope. It should be unreachable for a valid planner result and is not guessed into one of the two high-level capacity outcomes.
- **Crash after a committed prefix:** resolution and the durable receipt establish whether that exact request committed. The caller resumes only from returned/read-back current coordinates; it never assumes the next index advanced merely because planning succeeded.

### Required 162,725-Rune Scenario

The executable acceptance fixture contains exactly 162,725 Unicode runes of evidence distributed across multiple complete entries; no individual entry is oversized. Starting at index zero, the test repeatedly invokes `checkpoint` against the same stable stream:

1. Every non-final call returns `continuation_required`, commits a non-empty prefix, and identifies the first uncommitted entry by matching index and ID.
2. Replaying one uncertain call with its same request ID and payload returns the same committed state and adds no duplicate batch, coverage, or generation.
3. Each subsequent prefix uses a new request ID/batch ID and exactly the generation, revision, digest, snapshot, and cursor returned by the prior call.
4. The final call returns `committed`; resolving progress yields every original entry exactly once and in order, with every canonical batch and snapshot at or below 40,000 runes.
5. Concatenated recovered evidence is exactly 162,725 runes. No truncation, summary, hidden unreferenced batch, or premature task completion is accepted.

The test must exercise repeated durable checkpoint commits, not merely call the pure planner or prebuild several batches for one low-level `advance` request.

### Completion File Changes

| Paths | Minimal completion action |
| --- | --- |
| `hivederive/applyprogress/planner.go` and `planner_test.go` | New pure boundary planner, four writer dispositions, cursor validation, one-batch maximal-prefix fitting, and exact-rune tests. Existing model/canonical/validation behavior is reused, not copied. |
| `jarvis-cli/cmd/jarvis/cmd_sdd_progress.go` and tests | Add sibling `checkpoint` request/output adapter while leaving `advance` flags and behavior intact. |
| `jarvis-cli/internal/sddprogress/` and focused tests only where needed | Expose/reuse validated current-snapshot resolution and existing OpenSpec/Hive/hybrid `Advance`; do not add storage tables, files, receipts, or alternate write methods. |
| Canonical embedded apply/orchestrator assets and catalog contract tests | Switch normal checkpoint guidance to the high-level command and document cursor/request-ID rules; preserve low-level recovery guidance. Generated machine files remain untouched. |

No daemon, HTTP API, MCP, SQLite schema, Hive client wire model, general sync, lifecycle, archive, or legacy-format change is part of this completion.

### Compatibility and Non-Goals

- Existing callers of `jarvis sdd progress advance`, HTTP POST advance, MCP advance, and their generic defensive capacity behavior remain source- and wire-compatible.
- Existing v2 snapshots, batches, revisions, receipts, legacy upgrades, archives, and hybrid recovery remain valid; checkpoint only constructs inputs those paths already accept.
- No evidence schema v3, compression, alternate rune limit, entry fragmentation, automatic summarization, backend winner, distributed transaction, general CAS, or evidence garbage collection is introduced.
- No server-side planner and no duplicated planner in CLI, daemon, HTTP, MCP, OpenSpec, or hybrid code.
- No automatic unbounded loop commits an entire stream. One explicit checkpoint call equals at most one committed prefix.
- No prior implementation decision, completed task history, verification evidence, or rollback boundary is rewritten by this amendment.

### Four Bounded Implementation Units

Each unit is a strict RED → GREEN → TRIANGULATE/REFACTOR slice and has a hard limit of **400 changed lines (additions plus deletions)**. If a unit cannot remain within that limit, implementation pauses for a new delivery decision rather than inferring `size:exception`.

| Unit | Dependency and finish line | Focused verification and rollback | Forecast |
| --- | --- | --- | ---: |
| C1 — pure planner contract | Add planner types/algorithm in `hivederive/applyprogress`; prove whole-entry maximal prefixes, cursor checks, exact four dispositions, Unicode limits, and pure repeated planning over 162,725 runes. | `cd hivederive && go test ./applyprogress -count=1 && go vet ./...`; remove only planner files. | 300–390 |
| C2 — OpenSpec checkpoint command | Add sibling Cobra command, canonical request/output, current-snapshot/base checks, and one-prefix delegation to existing OpenSpec `Advance`; prove continuation coordinates, no-write capacity, same-request replay, stale CAS, and durable 162,725-rune repeated checkpoints. | Focused `cmd/jarvis` + `internal/sddprogress` tests, then CLI suite/vet; remove checkpoint command seam, retain all v2 data. | 320–395 |
| C3 — Hive and hybrid reuse | Route the same checkpoint coordinator through existing Hive and hybrid advancers/resolvers; prove exact-payload retry, both hybrid interruption directions, no committed-side rewrite, current-coordinate rebasing, and unchanged generic HTTP/MCP/advance contracts. | Focused CLI/hiveclient compatibility tests, then CLI suite/vet; revert only checkpoint routing, retain receipts/topology. | 260–380 |
| C4 — executor contract and regression closure | Update canonical embedded apply/orchestrator sources and catalog assertions to use `checkpoint`; add final cross-mode outcome/compatibility regressions without changing generated files or daemon endpoints. | Focused skill/catalog and command tests, full module tests/vet; roll back assets first while low-level `advance` remains operational. | 180–300 |

Rollout is **C1 → C2 → C3 → C4**. The old low-level path remains usable throughout. Rollback removes executor guidance first, then the high-level command/coordinator, while retaining the pure planner harmlessly if already released; no committed progress or receipt is deleted.

## Open Questions

None.
