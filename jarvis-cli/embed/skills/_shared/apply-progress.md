# Apply-Progress Contract

`apply-progress.md` is the canonical OpenSpec snapshot location. Read it only with its exact referenced immutable evidence batches under `apply-evidence/`; do not infer state from a cumulative artifact.

## Checkpoint Flow

The apply executor checkpoints once per apply batch: one `jarvis sdd progress checkpoint` request carries one `task_records` item per covered task, the base snapshot, and its expected generation/revision/digest. The CLI expands the records into ordered evidence entries, derives the request, batch, and stream identities, and returns `next: {action, instruction}`. Callers follow `next` rather than interpreting outcomes: `continue_stream` reruns from the returned cursor, `done` and `continue_tasks` end the batch, and `stop_*` actions halt with their typed recovery. Task checkboxes are marked only for coverage the committed snapshot names.

## Snapshot Capacity Preflight

A successful checkpoint can include a structured `warning` before the snapshot reaches its 40,000-rune hard limit. It reports `document`, used `runes`, `threshold`, `limit`, and `remaining` runes, and carries the same exact `capacity` object (`current_runes`, `projected_runes`, `ceiling_runes`) as snapshot-capacity refusal. Treat it as a signal to avoid new tiny evidence streams.

The snapshot has a finite immutable-reference ceiling: each nonterminal checkpoint adds one batch reference and references are never compacted. Capacity, stream-preflight, and consolidation outcomes are fail-closed and commit nothing. `stream_preflight_required` and `checkpoint_consolidation_required` map to `stop_consolidate`; the latter fires only before a new stream is bound, never for an active frozen continuation. `snapshot_capacity_exhausted` maps to `stop_new_change`: stop with apply `StatusPartial` without archive and carry the uncovered task IDs into a new ordinary SDD change. A partial snapshot with full task coverage is valid only with its continuation cursor; without one, it is an exhausted incremental stream awaiting a future stream and has incomplete coverage.

## Continuation Lifecycle

A partial continuation binds `stream_sha256`, `next_entry_index`, and `next_entry_id` as one atomic group. Each successor preserves immutable batch references and advances the cursor by the exact number of appended entries. A terminal snapshot omits the group and recomputes the ordered stream digest before acceptance. Historical explicit-zero continuations require the guarded upgrade path before a new stream is bound.

## Archive Validation

Archive validates the canonical v2 `apply-progress.md` snapshot with exactly its referenced immutable evidence batches before moving the topology. It validates batch hashes, ordered references, task coverage, and any atomic continuation group. Archive never repairs or recomputes evidence. A superseded predecessor is not archive-ready, even when its old tasks were complete: Hive, OpenSpec, and hybrid require `done` progress. Revalidate that requirement under the archive lock before any move. Historical completed v2 changes remain archiveable.

Use `jarvis sdd supersede --change <predecessor> --successor <successor> --actor <actor> --reason <reason>` to supersede a bound partial change. A new seal requires `--actor` and `--reason`; strict predecessor and successor preflight runs before consent. Signed credited tasks require typing the exact successor name; zero credit skips the prompt only after preflight. On retry, omit attribution flags or repeat their exact signed values, and keep the same successor. The seal retains the predecessor's original task manifest and immutable history; fresh successor progress starts at generation 1/revision 1 without transferred credit. Hybrid publication is not distributed-atomic: retry using the exact signed intent rather than choosing a backend winner or changing attribution. Never archive the superseded predecessor.

## Strict Verification

Strict verification reads the canonical v2 `apply-progress.md` snapshot and exactly the immutable evidence batches referenced by that snapshot. It rejects missing, malformed, unreferenced, or hash-mismatched evidence.

- In Hive, fetch the snapshot with `sdd_apply_progress_get`, iterate `snapshot.batches` through `sdd_apply_evidence_get` for each referenced `batch_id`, derive the authoritative task manifest from Hive `tasks`, and run full `ValidateProgress` with those ordered canonical documents. Reject attribution, digest, reference-order, or coverage mismatches.
- In OpenSpec, read `apply-progress.md` and only the referenced `apply-evidence/<batch-id>.json` files.
- For a partial snapshot, require a valid atomic `stream_sha256`, `next_entry_index`, and `next_entry_id` continuation group. Verify that each successor preserves prior immutable batch references and advances the cursor by its appended entries before evaluating coverage.
- An exhausted partial stream has no continuation group and may begin a later independent stream while preserving prior immutable batch references.
- Do not infer completion from legacy standalone markers or unreferenced local artifacts.
