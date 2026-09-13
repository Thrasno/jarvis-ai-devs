# Archive Report: issue-653-bounded-apply-progress

status: PASS
review_fix_validation: PASS
archive_date: 2026-09-10
archived_path: openspec/changes/archive/2026-09-10-issue-653-bounded-apply-progress
canonical_spec_path: openspec/specs/bounded-apply-progress/spec.md
artifact_store: hybrid (OpenSpec + Engram)

## Archive Result

The bounded apply-progress change is archived with final review-fix verification complete. This report records archive state and validation evidence; it does not assert a commit, push, pull request, or merge.

## Artifacts Read

- proposal.md
- specs/bounded-apply-progress/spec.md
- design.md
- tasks.md
- apply-progress.md
- verify-report.md
- openspec/config.yaml
- Engram mirrors: proposal #7142, design #7144, spec #7143, tasks #7145, apply-progress #7146, verify-report #7216

## Validation Evidence

- Explicit change selection: `issue-653-bounded-apply-progress`.
- Final verification: PASS. The exact successful command record is preserved in `verify-report.md` and ends with `OVERALL: PASS`.
- Implementation tasks: 50/50 implementation-owned rows checked. The remaining unchecked review row is parent-owned and non-blocking.
- Apply progress: `status: complete` for the archived implementation record.
- Canonical sync: PASS. The canonical and archived domain specifications were byte-compared successfully.
- Forbidden-term scan: PASS; no prohibited tree/proof terminology was found.
- Archive topology: PASS. The archived change retains its reports, canonical delta specification, progress evidence, and receipts as a resolvable unit.

## Shipping Contract Recorded by the Archive

- Checkpoint planning is flat and operates on complete ordered entries only.
- The implementation exposes all six checkpoint outcomes: `committed`, `continuation_required`, `evidence_item_too_large`, `snapshot_capacity_exhausted`, `stream_preflight_required`, and `checkpoint_consolidation_required`.
- Capacity refusal is early and performs no write. Warning `current_runes` is derived from the persisted head.
- When capacity prevents completion, the change closes as partial without archive; uncovered tasks move to a new ordinary SDD change.
- Low-level advance compatibility remains intact. HTTP and MCP retain progress, evidence, and receipt retrieval endpoints.
- Apply, verify, and archive share the same bounded-progress guidance, and deterministic fixtures preserve daemon HTTP and CLI consumer contracts.

## Requirement Operations

- ADDED: none (full-domain initialization; seven requirements copied as the domain specification).
- MODIFIED: none.
- REMOVED: none.

## Legacy Apply-Progress History

The archived cumulative `apply-progress.md` remains immutable legacy history. It cannot be converted deterministically into canonical v2 batches without fabrication or loss: it contains multiple superseded attempts and status narratives rather than one frozen task manifest and a complete ordered stream of typed entries with stable entry IDs, commands, exit codes, outcomes, files, batch IDs, receipts, cursors, and stream hash.

A future, separately reviewed migration utility may accept one frozen legacy artifact plus a verified task manifest, emit `imported` entries with a source SHA-256 and explicit losslessness report, and refuse mixed-attempt cumulative history. This archive does not mutate that artifact.

## Status and Action Context

- Action context: repository-local.
- Workspace: repository workspace (path intentionally redacted from this archived report).
- Requested delivery constraints were honored: no commit, push, or pull-request action is recorded here.

## Engram Archive State

Archive report and mirror state are recorded under topic `sdd/issue-653-bounded-apply-progress/archive-report`, with the source observation IDs listed above.

## Post-Review Correction Appendix (Unit 11)

This appendix records post-review correction evidence only. It preserves the historical archive result above and does not modify the canonical verification report, task record, or specification.

### Correction Units and Commits

1. `4f7d2804` — `fix(sdd): block lifecycle on invalid tasks`
2. `c3516e10` — `fix(hive): upgrade historical zero-generation heads`
3. `34f14bd6` — `fix(sdd): align canonical verify report contract`
4. `9b1d5fe5` — `fix(sdd): trust durable hybrid acknowledgements`
5. `b8637332` — `perf(hive): cache apply progress lineage batches`
6. `f05084bb` — `fix(hive): centralize immutable sync policy`
7. `96c09cb7` — `fix(hive): skip rejected immutable remote creates`
8. `232e4bb0` — `fix(hive): accept exact reserved remote replays`
9. `67e35bd3` — `refactor(sdd): remove dead progress contracts`
10. `907b6ae8` — `fix(hive): protect immutable progress receipts`

### TDD and Verification Highlights

- The correction units were regression-test-led, covering invalid task gating, historical head upgrades, durable hybrid acknowledgements, lineage-batch caching, immutable remote mutation handling, and receipt protection.
- An authorized receipt-trigger scope discovery identified that durable acknowledgement and immutable-receipt behavior required the bounded correction set; it was incorporated into the ten units above rather than treated as unrelated work.
- `/tmp/653-postreview-verification.log` records successful formatting, vet, standard test, and race-test gates for `hivederive`, `hive-daemon`, and `jarvis-cli`, plus Windows vet checks where applicable.
- The final gate is PASS: the corrected archived parity check successfully compared `openspec/changes/archive/2026-09-10-issue-653-bounded-apply-progress/specs/bounded-apply-progress/spec.md` with `openspec/specs/bounded-apply-progress/spec.md`.
- The log first recorded a stale active-spec `cmp` path after the change had been archived. That command exit was a verification-path correction, not a product failure; the corrected archived parity check passed.
- No commit, push, pull request, or release is asserted or recorded by this appendix.

### Ready-to-File Follow-up Issue Drafts (Not Created)

#### 1. Snapshot compaction and rollover design

**English**

Title: Design snapshot compaction and rollover after capacity exhaustion

Current snapshot-capacity exhaustion is intentionally safe and performs no write. Define and review a future compaction/rollover design that preserves bounded-progress evidence, ordering, and recovery guarantees. This work is explicitly out of #653.

Acceptance criteria:
- Specify safe compaction and rollover triggers, invariants, and recovery behavior.
- Preserve evidence integrity and ordered progress semantics across rollover.
- Demonstrate that exhaustion remains no-write until the new design is implemented and enabled.

**Español (España)**

Título: Diseñar la compactación y la rotación de instantáneas tras el agotamiento de capacidad

El agotamiento actual de capacidad de instantáneas es deliberadamente seguro y no realiza escrituras. Definir y revisar un diseño futuro de compactación/rotación que preserve las garantías de evidencia, orden y recuperación del progreso acotado. Este trabajo queda explícitamente fuera de #653.

Criterios de aceptación:
- Especificar activadores, invariantes y comportamiento de recuperación seguros para la compactación y la rotación.
- Preservar la integridad de la evidencia y la semántica de progreso ordenado entre rotaciones.
- Demostrar que el agotamiento sigue siendo de solo rechazo, sin escritura, hasta que el nuevo diseño se implemente y habilite.

#### 2. Durable quarantine and audit storage for rejected immutable remote mutations

**English**

Title: Add durable quarantine and audit storage for rejected immutable remote mutations

Rejected remote immutable mutations currently use sanitized logging and a cursor-safe skip, preserving sync progress without applying the rejected mutation. Design and implement durable quarantine/audit storage for those rejected inputs. This work is explicitly out of #653.

Acceptance criteria:
- Persist sanitized, reviewable rejection metadata without storing unsafe payload data.
- Retain cursor-safe skip behavior and prove continued sync progress.
- Define operator retrieval, retention, and access-control expectations for quarantine/audit records.

**Español (España)**

Título: Añadir almacenamiento duradero de cuarentena y auditoría para mutaciones remotas inmutables rechazadas

Las mutaciones remotas inmutables rechazadas utilizan actualmente un registro saneado y una omisión segura para el cursor, lo que preserva el progreso de sincronización sin aplicar la mutación rechazada. Diseñar e implementar almacenamiento duradero de cuarentena/auditoría para dichas entradas rechazadas. Este trabajo queda explícitamente fuera de #653.

Criterios de aceptación:
- Persistir metadatos de rechazo saneados y revisables sin almacenar datos de carga inseguros.
- Mantener el comportamiento de omisión segura para el cursor y demostrar que la sincronización continúa progresando.
- Definir las expectativas de consulta por parte de operadores, retención y control de acceso para los registros de cuarentena/auditoría.
