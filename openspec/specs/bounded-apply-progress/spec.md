# Bounded Apply Progress Specification

## Purpose

Define verifiable, bounded apply progress that preserves task evidence across OpenSpec, Hive, and hybrid storage.

## Requirements

### Requirement: Bounded Canonical Evidence

The system MUST represent v2 progress as a `jarvis.sdd-apply-progress/v2` snapshot referencing ordered, immutable `jarvis.sdd-apply-evidence/v2` batches. Each final serialized snapshot and batch MUST be at most 40,000 Unicode runes. The snapshot MUST bind generation, guarded revision, task-manifest digest, ordered batch IDs/hashes, and reconciled task coverage; batch hashes and the snapshot digest MUST use the defined canonical UTF-8 JSON and SHA-256 rules.

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

Hive MUST expose a public apply-progress advance operation distinct from `mem_save`, delete/restore guards, and general memory CAS. It MUST atomically validate immutable same-ID batches, compare expected revision and generation, advance the snapshot, and retain an idempotency receipt. A successful response MUST return committed state and receipt; a stale request MUST return a typed conflict with current state and commit nothing.

The advance operation and its existing HTTP and MCP representations MUST remain low-level compatibility operations: they MUST validate and atomically persist a caller-proposed batch and snapshot, but MUST NOT select a prefix, split evidence, calculate capacity, create a checkpoint plan, or expose a planning-specific daemon endpoint.

#### Scenario: Stale writer is rejected

- GIVEN a writer advances progress after another writer committed a newer generation
- WHEN the stale writer submits its expected state
- THEN the public operation MUST return a conflict
- AND the newer snapshot MUST remain unchanged

#### Scenario: Outcome is unknown after transport loss

- GIVEN a request loses its response after reaching the public boundary
- WHEN the caller reads back or retries the same request ID and byte-identical canonical advance payload
- THEN it MUST receive the original committed result or safely complete it once
- AND reuse of that request ID with different content MUST fail closed

### Requirement: Checkpoint Prefix Planning and Recovery

`jarvis sdd progress checkpoint` MUST be the sibling CLI writer that plans and performs bounded checkpoints. It MUST use a pure planning step over the ordered whole `EvidenceEntry` input, the final canonical serialization of the candidate batch, and the final canonical serialization of the successor snapshot. The planner MUST select the largest contiguous input prefix for which both final documents fit the protocol limit; it MUST NOT perform storage writes or infer an implicit latest batch while planning.

The checkpoint command MUST expose exactly these four checkpoint outcomes: `committed`, `continuation_required`, `evidence_item_too_large`, and `snapshot_capacity_exhausted`. `committed` MUST durably commit the planned prefix and successor snapshot when all supplied entries fit. `continuation_required` MUST durably commit the largest fitting proper prefix and successor snapshot before returning. `evidence_item_too_large` MUST be returned when the next indivisible entry cannot fit in an otherwise valid candidate batch, and `snapshot_capacity_exhausted` MUST be returned when the successor snapshot reference set cannot fit; either capacity outcome MUST commit no batch, snapshot, completion, or receipt for that checkpoint attempt.

A `continuation_required` outcome MUST include recovery data containing `next_entry_index`, `next_entry_id`, the committed/current revision, generation, and snapshot digest, and the durable receipt. A resumed checkpoint MUST fail closed before planning or writing unless its supplied ordered input, cursor index, and entry identity agree with that recovery data and committed/current state. A request ID MUST be reused only for a byte-identical retry of the same advance payload; every successor-prefix checkpoint MUST use a new request ID. Any request-ID reuse, cursor, or entry-identity mismatch MUST fail closed and MUST commit nothing.

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

#### Scenario: Single entry or successor snapshot cannot fit

- GIVEN the next whole evidence entry cannot fit in a valid candidate batch or the successor snapshot cannot fit its reference set
- WHEN the checkpoint command plans the checkpoint
- THEN it MUST return `evidence_item_too_large` or `snapshot_capacity_exhausted` respectively
- AND it MUST commit nothing

### Requirement: Structured Checkpoint and Lifecycle Blocking

A writer MUST split evidence only at complete entry boundaries and validate final serialized documents before writing. It MUST return only the machine-readable `committed`, `continuation_required`, `evidence_item_too_large`, or `snapshot_capacity_exhausted` checkpoint outcomes as applicable; it MUST NOT truncate, summarize, or falsely claim persistence. Apply, status, verify, and archive MUST lazily validate referenced evidence, task coverage, and task-manifest digest and MUST fail closed on invalid, incomplete, conflict, capacity, or divergence states.

#### Scenario: Lifecycle reads invalid progress

- GIVEN referenced evidence is incomplete or the task manifest has changed
- WHEN apply, status, verify, or archive evaluates progress
- THEN the operation MUST block later lifecycle routing
- AND it MUST return the typed validation reason

### Requirement: Equivalent Backend Publication and Retention

OpenSpec, Hive, and hybrid modes MUST expose equivalent valid, invalid, continuation, conflict, and legacy semantics. OpenSpec snapshot publication MUST be atomic against expected generation and digest. Hybrid MUST validate both stores independently and return `backend_diverged` for a missing, invalid, or different side; it MUST NOT apply Hive precedence or repair automatically. Archive MUST retain resolvable snapshot-to-batch topology, and orphaned batches MUST NOT count as evidence.

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

Read-only status, verify, and archive MUST conservatively read legacy cumulative progress without mutation and MUST fail closed for malformed or ambiguous completion. The next mutating apply MUST upgrade unambiguous legacy evidence to bounded v2 batches and initial generation through the guarded path; legacy data MUST remain authoritative until that commit succeeds.

#### Scenario: Legacy next mutation upgrade fails

- GIVEN a legacy artifact cannot be fully converted or committed
- WHEN apply attempts its next mutation
- THEN it MUST return a structured migration failure
- AND it MUST preserve the legacy artifact without claiming v2 completion

#### Scenario: Non-goal boundaries remain intact

- GIVEN v2 progress is written or read
- WHEN the operation uses Hive or hybrid storage
- THEN general `mem_save`, general Hive synchronization, and `jarvis sync` MUST remain unchanged
- AND the system MUST NOT silently drop, reorder, compress, or repair evidence
