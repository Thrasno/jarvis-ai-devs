# Bounded Apply Progress Specification

## Purpose

Define verifiable, bounded apply progress that preserves task evidence across OpenSpec, Hive, and hybrid storage.

## Requirements

### Requirement: Bounded Canonical Evidence

The system MUST represent v2 progress as a `jarvis.sdd-apply-progress/v2` snapshot referencing ordered, immutable `jarvis.sdd-apply-evidence/v2` batches. Each final serialized snapshot and batch MUST be at most 40,000 Unicode runes. The snapshot MUST bind a generation epoch, a revision CAS coordinate, task-manifest digest, ordered batch IDs/hashes, and reconciled task coverage; ordinary successors retain their generation and increment revision exactly once. Batch hashes and the snapshot digest MUST use the defined canonical UTF-8 JSON and SHA-256 rules. A partial continuation MUST serialize `stream_sha256`, `next_entry_index`, and `next_entry_id` as an exact all-or-nothing group, including an index of `0`; a terminal complete snapshot MUST omit all three, including the stream hash. Historical pre-continuation v2 lineage reconstruction MAY accept the exact legacy transition that increments both generation and revision only when both predecessor and successor omit the entire continuation identity and the successor appends immutable evidence; any successor involving continuation identity MUST retain generation and increment only revision. A newly written partial snapshot with the whole group absent is an unbound exhausted stream and MAY start a later independent stream through the normal CAS path. Only the historical v2 wire shape that explicitly serialized the all-zero continuation group requires guarded upgrade before resume. A guarded historical-continuation successor MUST retain its generation; increment its revision exactly once; set `previous_digest` to the base digest; reseal its digest; preserve its batches, coverage, status, task-manifest digest, and immutable evidence; and add only the continuation identity group plus those CAS-envelope fields. `imported` evidence is internal legacy provenance only: ordinary caller-supplied checkpoint entries MUST NOT use it, and it does not claim a TDD lifecycle event.

Evidence supplied to a checkpoint MUST be an ordered sequence of whole `EvidenceEntry` values. The system MUST retain each committed entry exactly once and in input order, and MUST NOT split, mutate, summarize, drop, or reorder an entry.

#### Scenario: Evidence exceeds one Hive document

- GIVEN 162,725 runes of complete ordered evidence entries for a change
- WHEN successive checkpoints process the entries
- THEN every snapshot and batch MUST remain within 40,000 runes
- AND every input entry MUST be retained exactly once and in the original order across the resulting batches

#### Scenario: Evidence integrity is invalid

- GIVEN a snapshot with a missing, corrupt, reordered, duplicate, mismatched, or unreferenced batch
- WHEN progress is resolved
- THEN the result MUST be an actionable typed invalid state
- AND no completion marker alone MUST establish task completion

### Requirement: Authoritative Validated Progress Status

The bounded canonical snapshot MUST carry a lifecycle status of `partial` or `complete`. `jarvis sdd status` MUST report that status only after validating the authoritative snapshot, its exact referenced evidence batches, task coverage, and task-manifest digest; it MUST fail closed rather than report either status from an invalid or incomplete topology.

#### Scenario: Status reports validated partial progress

- GIVEN a canonical snapshot declares `partial` and its referenced batches, coverage, and task-manifest digest validate
- WHEN `jarvis sdd status` resolves progress
- THEN it MUST report `partial` as the authoritative status

#### Scenario: Status rejects an invalid declared status

- GIVEN a canonical snapshot declares `partial` or `complete` but its referenced evidence or coverage is invalid
- WHEN `jarvis sdd status` resolves progress
- THEN it MUST fail closed with the typed validation reason
- AND it MUST NOT report the declared lifecycle status as authoritative

### Requirement: Dedicated Guarded Hive Advance

Hive MUST expose a public apply-progress advance operation distinct from `mem_save`, delete/restore guards, and general memory CAS. It MUST atomically validate immutable same-ID batches, compare expected generation/revision/digest, advance the snapshot, and retain an idempotency receipt. Request IDs are globally unique within one daemon, so a changed payload cannot reuse an ID across projects or changes. Each receipt retains one bounded response payload; Hive has no time-based receipt GC today, while OpenSpec receipts move with the archived topology. A successful response MUST return committed state and receipt; a stale request MUST return a typed conflict with current state and commit nothing.

The advance operation and its existing HTTP and MCP representations MUST remain low-level compatibility operations: they MUST validate and atomically persist a caller-proposed batch and snapshot, but MUST NOT plan or select an evidence prefix, split evidence, create a checkpoint plan, or expose a planning-specific daemon endpoint. They MAY validate the final serialized capacity of a caller-proposed batch or snapshot; that defensive validation MUST NOT plan, split, or classify checkpoint capacity outcomes.

Each low-level request payload identity MUST include `legacy_source_sha256` when a legacy migration uses or replays `imported` evidence. An appended batch containing `imported` evidence MUST be rejected unless it is the initial authorized legacy migration and that binding equals the SHA-256 of the exact authoritative legacy `apply-progress` bytes being migrated. A v2 head or successor MUST NOT append `imported` evidence. Missing, malformed, or mismatched provenance MUST fail before writes with typed `legacy_migration` validation. Exact receipt replay of the originally authorized payload remains permitted; changing the binding is a changed payload and MUST fail closed. Automatic OpenSpec, Hive, and Hybrid migration MUST set the binding internally only after comparing their authoritative source bytes. Normal `checkpoint` JSON MUST NOT expose the field; only low-level compatibility/recovery JSON may carry it.

#### Scenario: Stale writer is rejected

- GIVEN a writer advances progress after another writer committed a newer revision in the same generation epoch
- WHEN the stale writer submits its expected state
- THEN the public operation MUST return a conflict with the current generation, revision, and digest
- AND recovery MUST state that generation is a stable epoch and retry with the authoritative generation plus the current revision and digest to prepare the next revision
- AND the newer snapshot MUST remain unchanged

#### Scenario: Outcome is unknown after transport loss

- GIVEN a request loses its response after reaching the public boundary
- WHEN the caller reads back or retries the same request ID and byte-identical canonical advance payload
- THEN it MUST receive the original committed result or safely complete it once
- AND reuse of that request ID with different content MUST fail closed

#### Scenario: Imported evidence has authoritative provenance

- GIVEN an initial legacy migration proposes an appended batch containing `imported` evidence
- WHEN the OpenSpec, Hive, or Hybrid authority compares the exact legacy `apply-progress` bytes
- THEN it MUST set and validate the matching `legacy_source_sha256` before any write
- AND missing or mismatched provenance MUST return typed `legacy_migration` validation without persisting a batch, snapshot, or receipt
- AND an exact replay of the bound request MAY return the original committed result while a changed source binding MUST fail as a changed payload

### Requirement: Checkpoint Prefix Planning and Recovery

`jarvis sdd progress checkpoint` MUST be the sibling CLI writer that plans and performs bounded checkpoints. It MUST use a pure planning step over the ordered whole `EvidenceEntry` input, the final canonical serialization of the candidate batch, and the final canonical serialization of the successor snapshot. The planner MUST select the largest contiguous input prefix for which both final documents fit the protocol limit; it MUST NOT perform storage writes or infer an implicit latest batch while planning.

The checkpoint command MUST expose exactly these six checkpoint outcomes: `committed`, `continuation_required`, `evidence_item_too_large`, `snapshot_capacity_exhausted`, `stream_preflight_required`, and `checkpoint_consolidation_required`. A snapshot with full task coverage MUST be `complete` unless its all-or-nothing continuation cursor proves trailing evidence remains; a partial snapshot without continuation is an exhausted incremental stream awaiting a future stream and therefore MUST have incomplete coverage. `committed` MUST durably commit the planned prefix and successor snapshot when all supplied entries fit. `continuation_required` MUST durably commit the largest fitting proper prefix and successor snapshot before returning. `evidence_item_too_large` MUST be returned when the next indivisible entry cannot fit in an otherwise valid candidate batch, and `snapshot_capacity_exhausted` MUST be returned when the exact canonical successor snapshot cannot fit. A snapshot-capacity response MUST include `capacity.current_runes`, `capacity.projected_runes`, and `capacity.ceiling_runes`; `current_runes` is derived only from the persisted head. Either capacity outcome MUST commit no batch, snapshot, completion, or receipt for that checkpoint attempt. Its recovery is: stop with apply `StatusPartial` without archive, list task IDs absent from coverage, and carry them into a new ordinary SDD change.

A snapshot has a finite immutable-reference ceiling: every nonterminal checkpoint appends a batch reference and references are never compacted or removed. Before binding a new stream that needs a continuation, the planner MUST simulate its complete maximal-batch continuation without writes. If any future indivisible entry batch, snapshot reference, or coverage topology exceeds capacity, it MUST return typed `stream_preflight_required` with capacity detail and commit no batch, snapshot, completion, or receipt. Before that hard capacity is approached, the planner MUST return the typed `checkpoint_consolidation_required` outcome when an unbound partial successor, including an exhausted-but-incomplete terminal stream, projects canonical snapshot/reference pressure beyond the safe threshold; it MAY retain the 256-reference undersized-batch backstop. It MUST commit nothing and instruct the caller to accumulate or combine pending evidence into a larger batch without changing committed stream authority. An active frozen continuation MUST NEVER return `checkpoint_consolidation_required`; its ordered stream remains immutable. A truly complete terminal stream MUST remain eligible. A successful successor whose canonical snapshot crosses the conservative 80% capacity threshold or leaves 8,000 runes or fewer MUST include the same structured `capacity` object alongside its snapshot-capacity warning, with exact current, projected, and ceiling runes plus used runes, threshold, limit, and remaining runes. Its guidance MUST tell callers to avoid starting new tiny streams and consolidate evidence before the hard ceiling. A `continuation_required` outcome MUST include recovery data containing `next_entry_index`, `next_entry_id`, the committed/current generation epoch, revision, and snapshot digest, and the durable receipt. A resumed checkpoint MUST fail closed before planning or writing unless its supplied ordered input, cursor index, and entry identity agree with that recovery data and committed/current state. A request ID MUST be reused only for a byte-identical retry of the same advance payload; every successor-prefix checkpoint MUST use a new request ID. Any request-ID reuse, cursor, or entry-identity mismatch MUST fail closed and MUST commit nothing.

#### Scenario: Checkpoint needs continuation

- GIVEN complete ordered entries have a non-empty largest prefix whose final canonical batch and successor snapshot fit, while the next entry does not fit
- WHEN `jarvis sdd progress checkpoint` runs
- THEN it MUST durably advance the valid snapshot for that largest prefix and return `continuation_required`
- AND it MUST return `next_entry_index`, `next_entry_id`, committed/current state, and the durable receipt
- AND the successor checkpoint MUST use a new request ID

#### Scenario: Recovery cursor does not identify the next entry

- GIVEN a `continuation_required` recovery result
- WHEN a caller resumes with a different cursor index, entry ID, entry order, or committed/current state
- THEN the checkpoint command MUST fail closed before planning or writing
- AND it MUST commit nothing

#### Scenario: Checkpoint frequency guard preserves future capacity

- GIVEN a partial base without continuation identity has reached the conservative immutable-reference guard
- WHEN a caller submits an undersized single-entry nonterminal checkpoint batch before binding a new stream
- THEN the command MUST return `checkpoint_consolidation_required` with actionable reference and batch-size details
- AND it MUST commit no batch, snapshot, completion, or receipt
- AND a larger combined nonterminal retry that fits the canonical documents MUST remain eligible to commit

#### Scenario: New stream preflight prevents an unreachable terminal snapshot

- GIVEN an unbound base and a new ordered stream whose first maximal batch fits but whose future batch references or coverage cannot fit a successor snapshot
- WHEN the planner evaluates the new stream before binding its continuation cursor
- THEN it MUST return `stream_preflight_required` with snapshot capacity detail
- AND it MUST commit no batch, snapshot, completion, or receipt

#### Scenario: Single entry or successor snapshot cannot fit

- GIVEN the next whole evidence entry cannot fit in a valid candidate batch or the successor snapshot cannot fit its reference set
- WHEN the checkpoint command plans the checkpoint
- THEN it MUST return `evidence_item_too_large` or `snapshot_capacity_exhausted` respectively
- AND it MUST commit nothing

### Requirement: Structured Checkpoint and Lifecycle Blocking

A writer MUST split evidence only at complete entry boundaries and validate final serialized documents before writing. It MUST return only the machine-readable `committed`, `continuation_required`, `evidence_item_too_large`, `snapshot_capacity_exhausted`, `stream_preflight_required`, or `checkpoint_consolidation_required` checkpoint outcomes as applicable; it MUST NOT truncate, summarize, or falsely claim persistence. Apply, status, verify, and archive MUST lazily validate referenced evidence, task coverage, and task-manifest digest and MUST fail closed on invalid, incomplete, conflict, capacity, or divergence states.

#### Scenario: Lifecycle reads invalid progress

- GIVEN referenced evidence is incomplete or the task manifest has changed
- WHEN apply, status, verify, or archive evaluates progress
- THEN the operation MUST block later lifecycle routing
- AND it MUST return the typed validation reason

### Requirement: Equivalent Backend Publication and Retention

OpenSpec, Hive, and hybrid modes MUST expose equivalent valid, invalid, continuation, conflict, and legacy semantics. Before any OpenSpec or Hybrid publication, the proposed snapshot manifest MUST match locally authoritative `tasks.md` parsed by the shared parser; a mismatch commits nothing. Before any pure Hive checkpoint publication, the CLI MUST fetch the authoritative `tasks` artifact through Hive, parse it with that shared parser, and reject a missing, malformed, or caller-manifest-mismatched artifact before any write. OpenSpec snapshot publication MUST be atomic against expected generation and digest. Hybrid MUST validate both stores independently and return `backend_diverged` for a missing, invalid, or different side; it MUST NOT apply Hive precedence or repair automatically. Hybrid receipts MUST be canonical regular files and are recovery metadata, never sole success authority: before accepting either recorded committed side, Hybrid MUST reconcile that backend's current guarded head and exact request receipt/payload through the existing idempotent advance path. Remote synchronization MAY insert a validated immutable snapshot or evidence CREATE idempotently to reconstruct a missing peer head, but it MUST never overwrite an existing immutable topic and MUST no-op UPDATE, DELETE, and RESTORE. OpenSpec status and archived-topology validation MUST discover non-empty canonical delta specifications recursively at `specs/<domain>/spec.md`; archive moves MUST preserve every such delta specification. OpenSpec current reads and archive MUST prove receipt lineage: every canonical receipt snapshot MUST be the current head or a uniquely validated ancestor in one append-only chain. A pending successor, fork, orphan, torn `.json` receipt, or retained staging file MUST fail closed with typed `publication_interrupted`; only an exact low-level advance or checkpoint recovery with the original request ID and byte-identical canonical payload remains allowed. A checkpoint retry MUST replan its candidate and invoke that guarded advance recovery only when its durable receipt binds the identical request; a changed payload or request ID MUST remain blocked. OpenSpec staging is deterministic and binds both destination and bytes: only the byte-identical exact retry may finalize its own stage. Anonymous, malformed, changed, directory, or symlink staging residue MUST remain fail closed and read-only status MUST not remove or mutate it. OpenSpec MUST reject a symlink in every existing component of its root, authoritative progress/task/evidence/receipt paths, staging paths, archive source, destination, and destination parent; writers MAY create a missing final file only after validating all existing parents as regular directories. Archive MUST enforce `ActionContext.AllowedEditRoots` for both source and destination, reject a destination that resolves to its source root, and while holding its lock re-read and validate regular-file lifecycle artifacts (proposal, design, specs, tasks, verify report, archive report), PASS readiness, canonical progress, receipts, evidence, and dependencies immediately before rename. Archived receipts MUST use the canonical OpenSpec shape containing a valid payload digest and a verified snapshot with matching project and change identity; torn `.json` receipts or retained staging files MUST block archive and archived hybrid acceptance. Archive MUST retain resolvable snapshot-to-batch topology, and orphaned batches MUST NOT count as evidence.

#### Scenario: Snapshot publication is interrupted

- GIVEN a batch write succeeds but snapshot publication is interrupted
- WHEN progress is read or retried
- THEN only the last valid snapshot MAY establish evidence
- AND the interrupted topology MUST remain fail closed until same-request recovery succeeds

#### Scenario: Hybrid stores disagree

- GIVEN OpenSpec and Hive contain different valid snapshots or one side is unavailable
- WHEN hybrid progress is resolved
- THEN it MUST return `backend_diverged`
- AND archive MUST NOT proceed

### Requirement: Conservative Legacy Upgrade

Read-only status, verify, and archive MUST conservatively read legacy cumulative progress without mutation and MUST fail closed for malformed or ambiguous completion. The next mutating checkpoint MUST automatically import unambiguous legacy evidence as bounded v2 `imported` entries at initial generation through the guarded path; an automatic partial import MUST run the same complete-stream topology and capacity preflight as explicit continuation upgrade before it writes a stream hash or cursor, and aggregate import capacity failure MUST return structured capacity details. Ordinary checkpoint planning MUST reject `imported` evidence; only the internal verified legacy-migration path may construct it. That internal path MUST use the deterministic hashed legacy task IDs derived from task path plus normalized text, MUST bind the exact source bytes with `legacy_source_sha256`, and callers cannot supply that field through checkpoint JSON. Legacy data MUST remain authoritative until that commit succeeds. A readable pre-continuation v2 snapshot is not legacy Markdown and requires its separate guarded recovery.

#### Scenario: Legacy next mutation upgrade fails

- GIVEN a legacy artifact cannot be fully converted or committed
- WHEN apply attempts its next mutation
- THEN it MUST return a structured migration failure
- AND it MUST preserve the legacy artifact without claiming v2 completion

#### Scenario: Pre-continuation v2 snapshot has an executable safe recovery

- GIVEN a readable historical v2 snapshot whose wire bytes explicitly serialize the all-zero stream and cursor fields
- WHEN `jarvis sdd progress checkpoint` attempts to resume it
- THEN it MUST return `legacy_upgrade_required` without writing
- AND `jarvis sdd progress upgrade-continuation --root <change-root> --request <request.json>` MAY create one guarded successor when the current snapshot is a partial historical explicit-zero continuation snapshot and reuses the blocked checkpoint request ID with the exact base, tasks, complete ordered entries, and stream SHA-256
- AND that successor MUST preserve the base generation, batches, coverage, status, task manifest, and evidence; increment only its revision; set `previous_digest`; reseal its digest; and add only the all-or-nothing continuation identity group
- AND appended evidence or any mutation to batches, coverage, status, or task manifest MUST be rejected

#### Scenario: Non-goal boundaries remain intact

- GIVEN v2 progress is written or read
- WHEN the operation uses Hive or hybrid storage
- THEN general `mem_save`, general Hive synchronization, and `jarvis sync` MUST remain unchanged
- AND the system MUST NOT silently drop, reorder, compress, or repair evidence
