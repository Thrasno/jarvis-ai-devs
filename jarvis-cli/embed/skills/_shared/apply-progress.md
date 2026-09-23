# Apply-Progress Contract

`apply-progress.md` is the canonical OpenSpec snapshot location. Read it only with its exact referenced immutable evidence batches under `apply-evidence/`; do not infer state from a cumulative artifact.

## Snapshot Capacity Preflight

A successful `jarvis sdd progress checkpoint` response can include a structured `warning` before the snapshot reaches its 40,000-rune hard limit. The warning reports `document`, used `runes`, `threshold`, `limit`, and `remaining` runes, plus guidance.

The snapshot has a finite immutable-reference ceiling: each nonterminal checkpoint adds one batch reference and references are never compacted. Before binding a new stream that needs a continuation, checkpoint simulates its full maximal-batch continuation. If future snapshot references or coverage would overflow capacity, it returns `stream_preflight_required` with no write; consolidate evidence or evolve the snapshot before binding that stream. At 256 prior references, an undersized single-entry nonterminal checkpoint is rejected with `checkpoint_consolidation_required` only before a new stream is bound, when the base has no continuation identity. The caller may then accumulate or combine pending evidence without changing committed stream authority. An active frozen continuation never returns this outcome; its ordered stream cannot change. A full-stream terminal checkpoint remains eligible.

Treat the warning as a preflight signal: avoid starting new tiny evidence streams, checkpoint after a meaningful bounded evidence group, and consolidate evidence before the hard snapshot ceiling. Warning-sized successful outcomes carry the same exact `capacity` object (`current_runes`, `projected_runes`, `ceiling_runes`) as snapshot-capacity refusal. For `snapshot_capacity_exhausted`, stop with apply `StatusPartial` without archive, list task IDs absent from coverage, and carry them into a new ordinary SDD change. The warning is advisory; capacity, stream-preflight, and consolidation outcomes are fail-closed and commit nothing. A partial snapshot with full task coverage is valid only with its continuation cursor; without one, it is an exhausted incremental stream awaiting a future stream and has incomplete coverage.

## Continuation Lifecycle

A partial continuation binds `stream_sha256`, `next_entry_index`, and `next_entry_id` as one atomic group. Each successor preserves immutable batch references and advances the cursor by the exact number of appended entries. A terminal snapshot omits the group and recomputes the ordered stream digest before acceptance. Historical explicit-zero continuations require the guarded upgrade path before a new stream is bound.

## Archive Validation

Archive validates the canonical v2 `apply-progress.md` snapshot with exactly its referenced immutable evidence batches before moving the topology. It validates batch hashes, ordered references, task coverage, and any atomic continuation group. Archive never repairs or recomputes evidence. A superseded predecessor is not archive-ready, even when its old tasks were complete: Hive, OpenSpec, and hybrid require `done` progress. Revalidate that requirement under the archive lock before any move. Historical completed v2 changes remain archiveable.

The current supersession gate is local only; there is no public successor command yet. After all predecessor and successor prechecks, nonzero credited tasks require typing the exact successor name; zero credited tasks skip the prompt. Seal the predecessor against its original task manifest. A fresh successor starts at generation 1/revision 1 with no transferred credit. Never use archive to close the superseded predecessor.

## Strict Verification

Strict verification reads the canonical v2 `apply-progress.md` snapshot and exactly the immutable evidence batches referenced by that snapshot. It rejects missing, malformed, unreferenced, or hash-mismatched evidence.

- In Hive, fetch the snapshot with `sdd_apply_progress_get`, iterate `snapshot.batches` through `sdd_apply_evidence_get` for each referenced `batch_id`, derive the authoritative task manifest from Hive `tasks`, and run full `ValidateProgress` with those ordered canonical documents. Reject attribution, digest, reference-order, or coverage mismatches.
- In OpenSpec, read `apply-progress.md` and only the referenced `apply-evidence/<batch-id>.json` files.
- For a partial snapshot, require a valid atomic `stream_sha256`, `next_entry_index`, and `next_entry_id` continuation group. Verify that each successor preserves prior immutable batch references and advances the cursor by its appended entries before evaluating coverage.
- An exhausted partial stream has no continuation group and may begin a later independent stream while preserving prior immutable batch references.
- Do not infer completion from legacy standalone markers or unreferenced local artifacts.
