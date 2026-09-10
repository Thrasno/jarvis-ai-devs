# Apply Progress: Bounded, Verifiable Apply Progress

status: complete

## PR 1 Protocol Foundation

- **Delivery boundary:** stacked-to-main PR 1 only; no Hive CAS, lifecycle routing, OpenSpec/hybrid persistence, or asset changes.
- **Status consumed:** explicit `issue-653-bounded-apply-progress` selection; `dependencies.apply: ready` for this slice; repo-local edits restricted to `hivederive/applyprogress/**` and these change artifacts.
- **Action-context warning:** none. The user did not authorize a PR 1 size exception.

## TDD Cycle Evidence

| Task | Layer | RED | GREEN | TRIANGULATE | REFACTOR |
| --- | --- | --- | --- | --- | --- |
| 1.1 | Unit | `cd hivederive && go test ./applyprogress -run TestSealBatchCanonicalAndBounded` failed as required: `undefined: Batch`, `EvidenceSchema`, and `SealBatch`. | Not retained | Not started | Not started |
| 1.2 | Unit | Covered by 1.1 initial RED only | Not retained | Not started | Not started |
| 1.3 | Unit | Not started | Not started | Not started | Not started |

## Size Gate

A provisional RED/GREEN slice for the model, canonical serializer, and one table-driven test measured 169 added lines before the required validation, legacy conversion, remaining table-driven tests, and normal Go formatting. Completing the specified four production files and three test files is conservatively estimated to exceed the 400-line limit. The provisional files were removed rather than code-golfing or leaving an incomplete protocol package.

## Completed Tasks

None. No task checkbox was updated.

## Files Changed

- `openspec/changes/issue-653-bounded-apply-progress/apply-progress.md` — created bounded blocked-progress record.

## Verification

- RED command executed: `cd hivederive && go test ./applyprogress -run TestSealBatchCanonicalAndBounded` (expected missing-symbol failure).
- No retained production code exists; GREEN, module tests, and vet were not run.

## Remaining Tasks

- [ ] 1.1 **RED:** Add table-driven failures in `hivederive/applyprogress/{canonical,validate,legacy}_test.go` for canonical bytes, 40,000-rune Unicode limits, IDs, hashes, order, coverage, and ambiguous legacy input. <!-- sdd-owner: implementation -->
- [ ] 1.2 **GREEN:** Create `hivederive/applyprogress/{model,canonical,validate,legacy}.go` with v2 structs, deterministic JSON/SHA-256, manifest IDs/digest, typed outcomes, validation, and conservative conversion. <!-- sdd-owner: implementation -->
- [ ] 1.3 **TRIANGULATE/REFACTOR:** Add malformed/duplicate/reordered/orphan and complete-vs-partial cases; simplify only after `cd hivederive && go test ./... && go vet ./...`. <!-- sdd-owner: implementation -->

## Required Decision

The prior delivery decision was resolved by the authorized PR 1A/PR 1B split. The existing PR 2 exception still does not apply.

## PR 1A Attempt 2 — Budget Stop

- **Status:** partial; no task checkbox updated.
- **Scope consumed:** PR 1A.1–1A.3 only. No PR 1B validation/legacy, Hive CAS, lifecycle routing, OpenSpec/hybrid persistence, or assets were retained.
- **TDD evidence:** table-driven RED compiled with missing `Batch`, `EvidenceSchema`, and `SealBatch`; GREEN passed focused package tests after model/canonical implementation; TRIANGULATE added canonical-input, invalid UTF-8/ID, exact 40,000-rune Unicode boundary, oversized snapshot/batch, tamper, and snapshot-stability cases.
- **Verification before budget gate:** `cd hivederive && go test ./applyprogress`, `go test ./...`, and `go vet ./...` passed; `gofmt -l` and `git diff --check` were clean.
- **Budget result:** the new model, canonicalization, and test files totalled 450 added lines before change-artifact updates, exceeding the authorized 400-line PR 1A budget. Per the delivery contract, the provisional source files were removed rather than code-golfed or retained as an oversized slice.

### Remaining Tasks

- [ ] 1A.1 **RED:** Add table-driven failures in `hivederive/applyprogress/canonical_test.go` for typed v2 models, canonical bytes, deterministic SHA-256, valid IDs, and exact 40,000-rune Unicode boundaries. <!-- sdd-owner: implementation -->
- [ ] 1A.2 **GREEN:** Create `hivederive/applyprogress/{model,canonical}.go` with v2 structs, deterministic JSON/SHA-256 sealing, ID checks, and typed capacity outcomes. <!-- sdd-owner: implementation -->
- [ ] 1A.3 **TRIANGULATE/REFACTOR:** Cover non-canonical JSON, invalid UTF-8/IDs, oversized evidence/snapshot documents, and hash stability; simplify only after `cd hivederive && go test ./... && go vet ./...`. <!-- sdd-owner: implementation -->

### Next Decision

Authorize a smaller cohesive PR 1A subdivision or explicitly accept `size:exception`; no exception is inferred.

## PR 1A Budget Decision

The maintainer explicitly authorized `size:exception` up to 500 changed lines for PR 1A after the single honest split measured 450 lines. No additional production attempt ran under the superseded 400-line objective; it must be reset before implementation can be retained.

## PR 1A Attempt 3 — Retained Cohesive Slice

status: partial

- **Assigned boundary:** PR 1A tasks 1A.1–1A.3 only. PR 1B validation/legacy conversion and all later work remain unimplemented.
- **Status consumed:** explicit `issue-653-bounded-apply-progress`; both stores; apply ready for this slice; repo-local workspace with the five user-authorized edit surfaces. Verify, sync, and archive remain blocked parent lifecycle phases.
- **Action context:** allowed edit roots cover every changed path; no warnings.
- **Delivery / workload:** `stacked-to-main`, PR 1A. The maintainer explicitly authorized the PR 1A `size:exception` up to 500 changed lines. New Go source is **498 additions / 0 deletions** (three new package files), within that limit. No code-golfing or out-of-slice work.

### Completed Tasks and Persisted Checkboxes

- [x] 1A.1 **RED:** Add table-driven failures in `hivederive/applyprogress/canonical_test.go` for typed v2 models, canonical bytes, deterministic SHA-256, valid IDs, and exact 40,000-rune Unicode boundaries. <!-- sdd-owner: implementation -->
- [x] 1A.2 **GREEN:** Create `hivederive/applyprogress/{model,canonical}.go` with v2 structs, deterministic JSON/SHA-256 sealing, ID checks, and typed capacity outcomes. <!-- sdd-owner: implementation -->
- [x] 1A.3 **TRIANGULATE/REFACTOR:** Cover non-canonical JSON, invalid UTF-8/IDs, oversized evidence/snapshot documents, and hash stability; simplify only after `cd hivederive && go test ./... && go vet ./...`. <!-- sdd-owner: implementation -->

### TDD Cycle Evidence

| Task | Layer | RED | GREEN | TRIANGULATE | REFACTOR |
| --- | --- | --- | --- | --- | --- |
| 1A.1 | Unit | `go test ./applyprogress -run TestSealBatchCanonicalAndBounded` failed with undefined v2 model/sealing symbols before production code existed. | `go test ./applyprogress -run TestSealBatchCanonicalAndBounded` passed after the minimum typed model/sealer implementation. | Added Unicode exact-limit and invalid-ID table cases. | Scoped the package to canonical model/sealing primitives only; tests remain green. |
| 1A.2 | Unit | The 1A.1 table test was written and executed first against missing `Batch`, schema, capacity, and `SealBatch` symbols. | Focused package test passed after `model.go` and `canonical.go` were added. | Repeated sealing proves deterministic bytes and SHA-256 output. | Private payload envelopes prevent the self-hash/digest field from entering its own digest. |
| 1A.3 | Unit | Added integrity/bounds cases after the initial GREEN test. | `go test ./applyprogress` passed. | Covered non-canonical whitespace, invalid UTF-8, invalid IDs, batch tampering/hash mismatch, and exact 40,000/40,001-rune batch and snapshot limits. | Removed the unneeded snapshot decode entry point; retained only this PR's sealing/verification scope. |

### Files Changed

- `hivederive/applyprogress/model.go` — typed v2 snapshot/batch/evidence models, protocol constants, enums, and typed capacity errors.
- `hivederive/applyprogress/canonical.go` — deterministic no-whitespace, HTML-unescaped JSON; SHA-256 seal/verify; canonical batch decoding; basic schema/value/ID and final-rune checks.
- `hivederive/applyprogress/canonical_test.go` — table-driven canonicality, identity, tamper, hash-stability, and Unicode boundary tests.
- `openspec/changes/issue-653-bounded-apply-progress/tasks.md` — checked only implementation-owned 1A.1–1A.3 rows.
- `openspec/changes/issue-653-bounded-apply-progress/apply-progress.md` — appended this retained run without removing earlier failed-attempt evidence.

### Verification

- `cd hivederive && go test ./applyprogress -run TestSealBatchCanonicalAndBounded` — expected RED: failed on missing symbols.
- `cd hivederive && go test ./applyprogress -run TestSealBatchCanonicalAndBounded` — GREEN: passed.
- `cd hivederive && go test ./applyprogress` — passed after triangulation/refactor.
- `cd hivederive && go test ./...` — passed.
- `cd hivederive && go vet ./...` — passed.
- `gofmt -l hivederive/applyprogress/model.go hivederive/applyprogress/canonical.go hivederive/applyprogress/canonical_test.go` — clean.
- `git diff --check` and no-index checks for the three new Go files — clean.

### Deviations and Remaining Tasks

None — implementation matches the PR 1A design boundary. Full coverage validation, task normalization, legacy conversion, and lifecycle storage are explicitly deferred to PR 1B and later.

Exact unchecked implementation tasks remain in `tasks.md`, beginning with:

- [ ] 1B.1 **RED:** Add table-driven failures in `hivederive/applyprogress/{validate,legacy}_test.go` for manifest IDs/digest, order, coverage, duplicate/missing/corrupt evidence, and ambiguous legacy input. <!-- sdd-owner: implementation -->
- [ ] 1B.2 **GREEN:** Create `hivederive/applyprogress/{validate,legacy}.go` with task normalization/digests, typed validation outcomes, ordered coverage validation, and conservative conversion. <!-- sdd-owner: implementation -->
- [ ] 1B.3 **TRIANGULATE/REFACTOR:** Add malformed/reordered/orphan and complete-vs-partial cases; run `cd hivederive && go test ./... && go vet ./...`. <!-- sdd-owner: implementation -->

- [ ] 2.1 **RED:** In `hive-daemon/internal/db/apply_progress_test.go` and boundary tests, specify stale writers, identical retry, request-ID reuse conflict, batch collision, and transport recovery. <!-- sdd-owner: implementation -->
- [ ] 2.2 **GREEN:** Add head/receipt migration in `internal/db/db.go` and transactional `apply_progress.go`; wire typed GET/POST and MCP advance/get through `db/sdd.go`, governance, HTTP, MCP, and `jarvis-cli/internal/hiveclient/client.go`, never `mem_save`. <!-- sdd-owner: implementation -->
- [ ] 2.3 **TRIANGULATE/REFACTOR:** Exercise concurrent SQLite writers and HTTP/MCP 200/409/413/422/503 outcomes; run `cd hive-daemon && go test ./... && go vet ./...` and focused CLI client tests. <!-- sdd-owner: implementation -->
- [ ] 3.1 **RED:** Add `hive-daemon/internal/db/sdd_test.go` and `jarvis-cli/internal/sddstatus/{source,status}_test.go` cases for missing/corrupt batches, manifest mismatch, continuation/conflict, and blocked apply/status/verify/archive. <!-- sdd-owner: implementation -->
- [ ] 3.2 **GREEN:** Resolve only snapshot-referenced batches in daemon retrieval; update `source.go`, `status.go`, and `cmd/jarvis/cmd_sdd.go` to preserve typed fail-closed outcomes. <!-- sdd-owner: implementation -->
- [ ] 3.3 **TRIANGULATE/REFACTOR:** Cover delayed/forked synced rows and valid partial/complete routing; run `cd jarvis-cli && go test ./... && go vet ./...` plus daemon focused tests. <!-- sdd-owner: implementation -->
- [ ] 4.1 **RED:** Create `jarvis-cli/internal/sddprogress/{openspec,hybrid,store}_test.go` using `t.TempDir()` for atomic interruption, legacy-upgrade failure, divergence, same-request recovery, and archive retention. <!-- sdd-owner: implementation -->
- [ ] 4.2 **GREEN:** Implement lock/fsync/rename snapshot publication, immutable batches/receipts, next-mutation legacy upgrade, and independent hybrid comparison in `internal/sddprogress/` and `cmd_sdd_progress.go`. <!-- sdd-owner: implementation -->
- [ ] 4.3 **TRIANGULATE/REFACTOR:** Prove no side wins when stores differ and archive moves topology together; run `cd jarvis-cli && go test ./... && go vet ./...`. <!-- sdd-owner: implementation -->
- [ ] 5.1 **RED:** Extend `jarvis-cli/internal/skills/catalog_contract_test.go` to require batch append, guarded advance, receipt retry, and continuation/conflict/migration recovery guidance. <!-- sdd-owner: implementation -->
- [ ] 5.2 **GREEN:** Update source assets `embed/skills/{sdd-apply,sdd-archive}/SKILL.md` and `embed/orchestrator/sdd-orchestrator.md`; do not edit generated machine files. <!-- sdd-owner: implementation -->
- [ ] 5.3 **TRIANGULATE/REFACTOR:** Reject legacy cumulative-merge instructions and run `cd jarvis-cli && go test ./... && go vet ./...`; format changed Go with `gofmt`. <!-- sdd-owner: implementation -->

Deferred parent lifecycle action (unchanged):

- [ ] Start or reuse bounded review after each selected work unit; confirm tests, rollback boundary, and total diff against the 400-line budget. <!-- sdd-owner: parent -->

## PR 1A Candidate Correction — Budget-Blocked

status: partial

- **Scope:** the explicitly selected `issue-653-bounded-apply-progress` PR 1A candidate only. No PR 1B or later files were changed.
- **Status consumed/produced:** the native status was blocked solely by ambiguous selection; the user's explicit candidate selection resolves that ambiguity. `actionContext` remains `repo-local` with the repository as the allowed edit root and no warnings. This apply batch is now blocked by the maintainer-authorized **500-line** PR 1A maximum.
- **Findings disposition:** implemented an exact `DecodeCanonicalSnapshot`; it rejects BOM, whitespace, unknown/duplicate fields, and trailing values through canonical decoding. Snapshot digest-shaped values now require lowercase 64-hex. Batch and snapshot typed inputs validate UTF-8 before JSON serialization. Tests cover canonical snapshot round-trip/tampering, non-canonical snapshot JSON, malformed schema/status/kind/outcome/digests, and every string-typed field in the batch and snapshot models.

### Correction TDD Cycle Evidence

| Task | Layer | Safety net | RED | GREEN | TRIANGULATE | REFACTOR |
| --- | --- | --- | --- | --- | --- | --- |
| 1A.1–1A.3 correction | Unit | `cd hivederive && go test ./applyprogress` passed before changes. | `go test ./applyprogress -run '^TestDecodeCanonicalSnapshot$'` failed: `undefined: DecodeCanonicalSnapshot`. The expanded protocol tests then failed for BOM/unknown JSON, malformed digest shapes, and invalid UTF-8. | Added the exact snapshot decoder, canonical-decode error wrapping, digest-shape validation, and UTF-8 validation; focused tests passed. | Added table-driven canonical/tamper/malformed cases and reflection-driven coverage that poisons each typed string field individually; focused package tests passed. | Centralized UTF-8 validation with `validText`/`validTextSlice` and retained the existing exact-rune boundary tests with canonical digest-shaped inputs. |

### Verification

- `cd hivederive && go test ./applyprogress -run '^(TestDecodeCanonicalSnapshot|TestRejectsMalformedProtocolValues|TestRejectsInvalidUTF8TypedInput)$'` — passed.
- `cd hivederive && go test ./applyprogress` — passed.
- `cd hivederive && go test ./...` — passed.
- `cd hivederive && go vet ./...` — passed.
- `gofmt -l hivederive/applyprogress/{model,canonical,canonical_test}.go` — clean.
- `git diff --check` and no-index whitespace checks for all three Go files — clean.

### Size Gate and Task Reconciliation

- **Exact source count:** `107 + 292 + 297 = 696` lines across the three allowed Go files (all are untracked additions, so this is also 696 additions / 0 deletions).
- This is the smallest honest correction reached after factoring shared UTF-8 validation and shared snapshot-size test construction. It is **196 lines above** the explicit `size:exception` maximum of 500. Further compression would require code-golfing or weakening independently requested validation/test coverage.
- The implementation-owned 1A.1–1A.3 checkboxes remain visibly checked because every verifier finding is addressed and all requested hivederive verification commands passed; delivery is nevertheless blocked pending a maintainer decision on the 696-line cohesive correction.
- The native artifact authority for this session is OpenSpec. This correction evidence is merged into this authoritative `apply-progress.md`; no second apply-progress backend is named in the native status or permitted by the allowed filesystem surfaces.

### Remaining / Deferred

- No PR 1A implementation task remains technically incomplete; delivery remains blocked by the 500-line budget.
- All PR 1B–PR 5 implementation rows remain unchecked as listed above.
- Parent-owned lifecycle action remains unchanged.

## PR 1A Finalization — Parent-Authorized 850-Line Exception

status: partial

- **Status consumed:** explicit parent selection of `issue-653-bounded-apply-progress`; artifact store `both`; apply ready for PR 1A finalization. The parent handoff supersedes ambient ambiguous-change selection. `actionContext` is `repo-local`, the repository is the workspace root, all changed paths are within the supplied edit surfaces, and there are no warnings.
- **Delivery / workload:** `stacked-to-main`, PR 1A only. The maintainer explicitly authorized `size:exception` up to **850 changed implementation lines**. The retained package is **696 lines / 696 additions / 0 deletions**: `model.go` 107, `canonical.go` 292, and `canonical_test.go` 297. The allowed-surface working set before this finalization log was 935 physical lines including the two cumulative SDD artifacts (69 task lines and 170 prior progress lines); those audit artifacts are not production implementation lines. No PR 1B/later code was added.
- **Findings status:** pass for the assigned PR 1A canonical-model/sealing boundary. Exact canonical snapshot decoding rejects BOM, whitespace, unknown/duplicate fields, trailing values, and non-canonical JSON; SHA-256 digests require lowercase 64-hex syntax; typed inputs reject invalid UTF-8; and snapshot tamper, canonical round-trip, malformed protocol, and Unicode capacity cases pass. Full manifest/coverage/legacy validation remains deliberately deferred to PR 1B.

### Fresh Finalization Verification

- `cd hivederive && go test ./applyprogress` — passed.
- `cd hivederive && go test ./...` — passed.
- `cd hivederive && go vet ./...` — passed.
- `gofmt -l hivederive/applyprogress/model.go hivederive/applyprogress/canonical.go hivederive/applyprogress/canonical_test.go` — clean.
- `git diff --check` — clean; no-index whitespace checks for the untracked Go files — clean.

### Task Reconciliation and Next Work

- Persisted OpenSpec task rows **1A.1, 1A.2, and 1A.3 remain visibly `[x]`**. No unchecked row was changed, and the parent-owned lifecycle row remains byte-for-byte unchanged.
- Exact unchecked implementation rows remain the PR 1B–PR 5 rows already preserved above, beginning with `- [ ] 1B.1 **RED:** Add table-driven failures in \`hivederive/applyprogress/{validate,legacy}_test.go\` for manifest IDs/digest, order, coverage, duplicate/missing/corrupt evidence, and ambiguous legacy input. <!-- sdd-owner: implementation -->`.
- **PR boundary:** retain PR 1A only; PR 1B and later slices are excluded. Next action is parent-owned bounded review/lifecycle handling, not a new apply run.

## PR 1B — Validation and Conservative Legacy Conversion

status: partial

- **Status consumed:** explicit `issue-653-bounded-apply-progress`; `artifactStore: both`; PR 1B tasks 1B.1–1B.3 apply-ready. The repository is the allowed repo-local edit root; every edit stayed within the supplied surfaces. Verify, sync, and archive remain blocked parent lifecycle phases.
- **Delivery / workload:** `stacked-to-main`, current PR 1B, dependent on reviewed PR 1A. The implementation/test slice is **379 additions / 0 deletions** across the four PR 1B Go files, under the hard 400-line source budget. No CAS or later unit was touched.

### Completed Tasks and Persisted Checkboxes

- [x] 1B.1 **RED:** Table-driven manifest, ordered coverage, duplicate/missing/corrupt evidence, and ambiguous legacy cases were written before the validation/legacy production files existed.
- [x] 1B.2 **GREEN:** Added deterministic task normalization/manifest SHA-256, typed validation outcomes, canonical referenced-batch validation, exact coverage reconciliation, and conservative conversion.
- [x] 1B.3 **TRIANGULATE/REFACTOR:** Added reordered, orphan, complete/partial, malformed evidence, and path/ID collision cases; formatted and verified the cohesive package.

### TDD Cycle Evidence

| Task | Test file | Layer | Safety net | RED | GREEN | TRIANGULATE / REFACTOR |
| --- | --- | --- | --- | --- | --- | --- |
| 1B.1 | `validate_test.go`, `legacy_test.go` | Unit | `go test ./applyprogress` passed before edits | Focused test compilation failed on undefined `TaskManifest`, `ValidateProgress`, `LegacyProgress`, and typed outcomes. | Focused manifest/validation/legacy suite passed after minimum implementation. | Table cases cover normalization, digest changes, coverage order, missing/corrupt/duplicate evidence, and legacy ambiguity. |
| 1B.2 | `validate_test.go`, `legacy_test.go` | Unit | Same package baseline | RED test preceded both production files. | `go test ./applyprogress -run '^(TestTaskManifestNormalizesAndDerivesLegacyIDs | TestValidateProgress | TestConvertLegacy)$'` passed. | Replaced a batch only through PR 1A sealing helpers; no duplicate serializer was introduced. |
| 1B.3 | `validate_test.go`, `legacy_test.go` | Unit | Focused GREEN baseline | Added path-vs-ID legacy marker collision; it failed with a nil conversion result. | Added fail-closed collision detection; focused legacy test passed. | Added malformed batch, orphan, reordered coverage, and both complete/partial invariant cases; package test stayed green after `gofmt`. |

### Files Changed

- `hivederive/applyprogress/validate.go` — task normalization, deterministic manifest digest, typed document validation, and coverage reconciliation.
- `hivederive/applyprogress/legacy.go` — fail-closed normalized legacy conversion.
- `hivederive/applyprogress/validate_test.go` and `legacy_test.go` — table-driven unit coverage.
- `openspec/changes/issue-653-bounded-apply-progress/tasks.md` — checked exactly implementation-owned 1B.1–1B.3 rows.

### Verification and Remaining Work

- Passed: focused RED/GREEN/triangulation commands, `cd hivederive && go test ./applyprogress`, `go test ./...`, and `go vet ./...`.
- Passed: `gofmt -l` and tracked/untracked `git diff --check` checks.
- **Deviation:** none. PR 1A canonical `Seal*`/`Decode*` helpers are reused; `model.go` was unchanged.
- Exact unchecked implementation rows remain those in PR 2–PR 5 preserved above, beginning with `- [ ] 2.1 **RED:** In \`hive-daemon/internal/db/apply_progress_test.go\` and boundary tests, specify stale writers, identical retry, request-ID reuse conflict, batch collision, and transport recovery. <!-- sdd-owner: implementation -->`.
  - Deferred parent lifecycle action remains byte-for-byte unchanged: `- [ ] Start or reuse bounded review after each selected work unit; confirm tests, rollback boundary, and total diff against the 400-line budget. <!-- sdd-owner: parent -->`.

## PR 1B Correction — Manifest-Ordered Coverage and Immutable Legacy Identity

status: partial

- **Status consumed/produced:** The native OpenSpec status was blocked only because several changes were active. The user's explicit `issue-653-bounded-apply-progress` selection resolves that ambiguity for this PR 1B correction. `actionContext` remains `repo-local`, all writes are within the supplied edit surfaces, and there are no warnings. Apply remains partial because PR 2–5 are out of scope; verify/sync/archive are parent lifecycle phases.
- **Delivery/workload:** stacked-to-main PR 1B only; no PR 1A or later source was edited. The four PR 1B Go files total **395 physical lines** (`validate.go` 171, `validate_test.go` 138, `legacy.go` 43, `legacy_test.go` 43), within the hard 400-line objective.

### Findings Disposition

1. **Manifest order — fixed.** `ValidateProgress` records each completion by task ID, then constructs expected coverage by iterating the current normalized manifest. Reversed evidence with manifest-ordered coverage is valid; a snapshot that merely mirrors reversed evidence is rejected.
2. **Mutable legacy identity — fixed.** `ConvertLegacy` now keeps immutable path/ID identity indexes and a distinct `used` completion set. It rejects intrinsic path/ID ambiguity regardless of completion order and rejects malformed paths even for explicit IDs.
3. **Triangulation coverage — added.** Tests cover the exact ordered manifest digest and order sensitivity; duplicate refs/entry IDs; unknown/repeated attribution; completion without attribution/repeated completion; referenced hash, identity, project, and change mismatches; valid partial; corrupt orphan ignored; and malformed explicit-ID paths. Existing `canonical_test.go` already covers typed snapshot corruption.

### TDD Cycle Evidence

| Task | Test file | Layer | Safety net | RED | GREEN | TRIANGULATE | REFACTOR |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 1B.1–1B.3 correction | `hivederive/applyprogress/{validate,legacy}_test.go` | Unit | `cd hivederive && go test ./applyprogress` passed before changes. | Focused validation/legacy RED failed: reversed evidence was reconciled in evidence order and a collision after a prior completion converted successfully. | The same focused command passed after manifest-ordered reconciliation and separate identity/completion sets. | Added manifest, batch/ref, attribution, partial, orphan, and malformed-path table cases; focused suite passed. | Consolidated fixture batch replacement into a table-driven helper; package tests stayed green. |

### Verification

- `cd hivederive && go test ./applyprogress -run '^(TestValidateProgress|TestConvertLegacy)$'` — expected RED failure.
- `cd hivederive && go test ./applyprogress -run '^(TestValidateProgress|TestConvertLegacy)$'` — GREEN pass.
- `cd hivederive && go test ./applyprogress -run '^(TestTaskManifestNormalizesAndDerivesLegacyIDs|TestValidateProgress|TestConvertLegacy)$'` — triangulation pass.
- `cd hivederive && go test ./applyprogress` — pass.
- `cd hivederive && go test ./...` — pass.
- `cd hivederive && go vet ./...` — pass.
- `gofmt -d hivederive/applyprogress/{validate,legacy}{,_test}.go` — clean.
- `git diff --check` plus no-index `git diff --check` for all four untracked Go files — clean.

### Task Reconciliation / Remaining Work

- Persisted OpenSpec rows `1B.1`, `1B.2`, and `1B.3` remain visibly `[x]` after the passing correction. No parent-owned row was changed.
- Exact unchecked implementation rows remain:
  - [ ] 2.1 **RED:** In `hive-daemon/internal/db/apply_progress_test.go` and boundary tests, specify stale writers, identical retry, request-ID reuse conflict, batch collision, and transport recovery. <!-- sdd-owner: implementation -->
  - [ ] 2.2 **GREEN:** Add head/receipt migration in `internal/db/db.go` and transactional `apply_progress.go`; wire typed GET/POST and MCP advance/get through `db/sdd.go`, governance, HTTP, MCP, and `jarvis-cli/internal/hiveclient/client.go`, never `mem_save`. <!-- sdd-owner: implementation -->
  - [ ] 2.3 **TRIANGULATE/REFACTOR:** Exercise concurrent SQLite writers and HTTP/MCP 200/409/413/422/503 outcomes; run `cd hive-daemon && go test ./... && go vet ./...` and focused CLI client tests. <!-- sdd-owner: implementation -->
  - [ ] 3.1 **RED:** Add `hive-daemon/internal/db/sdd_test.go` and `jarvis-cli/internal/sddstatus/{source,status}_test.go` cases for missing/corrupt batches, manifest mismatch, continuation/conflict, and blocked apply/status/verify/archive. <!-- sdd-owner: implementation -->
  - [ ] 3.2 **GREEN:** Resolve only snapshot-referenced batches in daemon retrieval; update `source.go`, `status.go`, and `cmd/jarvis/cmd_sdd.go` to preserve typed fail-closed outcomes. <!-- sdd-owner: implementation -->
  - [ ] 3.3 **TRIANGULATE/REFACTOR:** Cover delayed/forked synced rows and valid partial/complete routing; run `cd jarvis-cli && go test ./... && go vet ./...` plus daemon focused tests. <!-- sdd-owner: implementation -->
  - [ ] 4.1 **RED:** Create `jarvis-cli/internal/sddprogress/{openspec,hybrid,store}_test.go` using `t.TempDir()` for atomic interruption, legacy-upgrade failure, divergence, same-request recovery, and archive retention. <!-- sdd-owner: implementation -->
  - [ ] 4.2 **GREEN:** Implement lock/fsync/rename snapshot publication, immutable batches/receipts, next-mutation legacy upgrade, and independent hybrid comparison in `internal/sddprogress/` and `cmd_sdd_progress.go`. <!-- sdd-owner: implementation -->
  - [ ] 4.3 **TRIANGULATE/REFACTOR:** Prove no side wins when stores differ and archive moves topology together; run `cd jarvis-cli && go test ./... && go vet ./...`. <!-- sdd-owner: implementation -->
  - [ ] 5.1 **RED:** Extend `jarvis-cli/internal/skills/catalog_contract_test.go` to require batch append, guarded advance, receipt retry, and continuation/conflict/migration recovery guidance. <!-- sdd-owner: implementation -->
  - [ ] 5.2 **GREEN:** Update source assets `embed/skills/{sdd-apply,sdd-archive}/SKILL.md` and `embed/orchestrator/sdd-orchestrator.md`; do not edit generated machine files. <!-- sdd-owner: implementation -->
  - [ ] 5.3 **TRIANGULATE/REFACTOR:** Reject legacy cumulative-merge instructions and run `cd jarvis-cli && go test ./... && go vet ./...`; format changed Go with `gofmt`. <!-- sdd-owner: implementation -->
- Deferred lifecycle action, unchanged: `- [ ] Start or reuse bounded review after each selected work unit; confirm tests, rollback boundary, and total diff against the 400-line budget. <!-- sdd-owner: parent -->`.

## PR 1B Finalization — Parent-Authorized 500-Line Exception

- **Status / scope:** Parent explicitly selected `issue-653-bounded-apply-progress`; `artifactStore: both`; PR 1B only. Ambient ambiguous selection is not used. All paths are within repo-local allowed roots; no action-context warning.
- **Authorization / audit:** Maintainer authorized `size:exception` up to **500 total changed lines**. The four Go files remain **395 lines**; the independently audited pre-finalization work-unit delta was **485 lines**. This concise record retains the slice within 500; no code changed.
- **Fresh verification:** `cd hivederive && go test ./applyprogress`, `go test ./...`, and `go vet ./...` passed; `gofmt -l` plus tracked/untracked `git diff --check` whitespace checks were clean.
- **Reconciliation / boundary:** 1B.1–1B.3 remain visibly `[x]` with prior strict-TDD evidence retained above; PR 2–PR 5 remain unchecked, the parent-owned row is unchanged, and the manifest-order and immutable-legacy-identity blockers remain closed. Return to parent lifecycle/bounded review; do not apply later units.

## PR 2 — Guarded Hive Advance (Blocked Reconciliation)

status: partial

- **Status consumed:** explicit `issue-653-bounded-apply-progress`; artifact store `both`; PR 2 tasks 2.1–2.3 apply-ready; repo-local allowed edit surfaces only. Delivery is `stacked-to-main`; the maintainer authorized the CAS-only `size:exception` up to 700 changed lines. Verify, sync, and archive remain parent-owned and blocked.
- **Implemented boundary:** additive `sdd_apply_heads`/`sdd_apply_receipts`; a dedicated transaction that does not call `SaveMemory`/`mem_save`; immutable batch reuse/collision detection; generation/revision/digest CAS; durable request receipts; typed governance, HTTP, MCP, and hiveclient paths. General memory writes are unchanged.
- **Blocking reconciliation:** `cd hive-daemon && go test ./...` fails only at `cmd/hive-daemon: TestDaemon_Starts_AndRegisters10Tools`: the real daemon correctly exposes the two required net-new MCP tools, so the existing test expects 10 but observes 12. Updating that assertion is outside the explicit allowed edit surfaces. No task checkbox was changed.

### TDD Cycle Evidence

| Task | Layer | RED | GREEN | TRIANGULATE / REFACTOR |
| --- | --- | --- | --- | --- |
| 2.1 | DB and client boundary | `go test ./internal/db -run '^TestAdvanceApplyProgress'` failed on missing advance APIs; `go test ./internal/hiveclient -run '^TestApplyProgressClient'` failed on missing client API. | Both focused suites pass after the minimum storage and transport wiring. | Added stale, same-request replay, request reuse, immutable collision, invalid no-commit, concurrent-writer, HTTP 200/readback, and client 409/recovery plus 413/422/503 mapping cases. |
| 2.2 | SQLite / daemon boundary | Covered by the initial missing-symbol RED. | DB, governance, HTTP, MCP, and client focused packages pass. | Kept the guarded path separate from `SaveMemory`; no lifecycle routing was added. |
| 2.3 | Integration boundary | Focused boundary tests preceded the corresponding implementation. | Focused DB/governance/HTTP/MCP/CLI tests pass. | Full daemon suite exposes the stale tool-count assertion blocker above; `go vet ./...` passes. |

### Verification

- Passed: focused DB, governance, HTTP, MCP, and hiveclient tests.
- Passed: `cd hive-daemon && go vet ./...`; `cd jarvis-cli && go test ./internal/hiveclient && go vet ./internal/hiveclient`; `gofmt`; `git diff --check`.
- Blocked: `cd hive-daemon && go test ./...` only by the out-of-scope stale `10 tools` assertion.

### Remaining Tasks

- [ ] 2.1 **RED:** In `hive-daemon/internal/db/apply_progress_test.go` and boundary tests, specify stale writers, identical retry, request-ID reuse conflict, batch collision, and transport recovery. <!-- sdd-owner: implementation -->
- [ ] 2.2 **GREEN:** Add head/receipt migration in `internal/db/db.go` and transactional `apply_progress.go`; wire typed GET/POST and MCP advance/get through `db/sdd.go`, governance, HTTP, MCP, and `jarvis-cli/internal/hiveclient/client.go`, never `mem_save`. <!-- sdd-owner: implementation -->
- [ ] 2.3 **TRIANGULATE/REFACTOR:** Exercise concurrent SQLite writers and HTTP/MCP 200/409/413/422/503 outcomes; run `cd hive-daemon && go test ./... && go vet ./...` and focused CLI client tests. <!-- sdd-owner: implementation -->

- **Workload / PR boundary:** PR 2 only, `stacked-to-main`; source working-set count is 651 added lines before this progress record (within the authorized 700 CAS-only exception). The smallest honest reconciliation requires allowing `hive-daemon/cmd/hive-daemon/main_test.go` to change its MCP count expectation from 10 to 12, then rerunning the full suite.

## PR 2 — Tool-Registration Reconciliation Complete

status: partial

- **Status consumed/produced:** The native status's only blocker was ambiguous selection; the parent's explicit `issue-653-bounded-apply-progress` PR 2 instruction resolves it. `actionContext.mode` is `repo-local`, every edit is inside the supplied roots, and there are no warnings. PR 2 is complete; PR 3–5 and the parent-owned lifecycle action remain deferred.
- **Scope and deviation:** Changed only `hive-daemon/cmd/hive-daemon/main_test.go` plus this change's task/progress artifacts. The startup test is now `TestDaemon_Starts_AndRegistersRequiredTools`; it asserts the exact 12-tool public MCP contract, including `sdd_apply_progress_advance` and `sdd_apply_progress_get`. No production behavior or later-slice file changed.
- **Completed / persisted checkboxes:** OpenSpec task rows **2.1**, **2.2**, and **2.3** are visibly `[x]`. The parent-owned row remains byte-for-byte unchanged.
- **Workload / PR boundary:** stacked-to-main PR 2 only. The complete PR 2 Go delta is **680 additions / 5 deletions**, including the startup-contract update and untracked DB files, within the authorized **700-line CAS-only exception**.

### TDD Cycle Evidence

| Task | Test file / layer | RED | GREEN | TRIANGULATE / REFACTOR |
| --- | --- | --- | --- | --- |
| 2.1–2.3 reconciliation | `hive-daemon/cmd/hive-daemon/main_test.go` / daemon startup integration | Existing `TestDaemon_Starts_AndRegisters10Tools` failed as required: daemon returned 12 tools while the stale assertion expected 10. | Updated only the test's semantic name and required MCP tool set; `TestDaemon_Starts_AndRegistersRequiredTools` passed. | Exact-name assertions cover both new SDD tools as well as missing/unexpected tools; no production refactor was needed. |

### Verification

- `cd hive-daemon && go test ./cmd/hive-daemon -run '^TestDaemon_Starts_AndRegisters10Tools$' -count=1` — expected RED (12 actual tools vs stale 10 expectation).
- `cd hive-daemon && go test ./cmd/hive-daemon -run '^TestDaemon_Starts_AndRegistersRequiredTools$' -count=1` — passed.
- `cd hive-daemon && go test ./internal/db ./internal/governance ./internal/httpapi ./internal/mcp ./cmd/hive-daemon -count=1` — passed.
- `cd jarvis-cli && go test ./internal/hiveclient -count=1 && go vet ./internal/hiveclient` — passed.
- `cd hive-daemon && go test ./... && go vet ./...` — passed.
- `gofmt -w` then `gofmt -l` on the startup test — clean; `git diff --check` — clean.

### Remaining Tasks / Deferred Lifecycle

- [ ] 3.1 **RED:** Add `hive-daemon/internal/db/sdd_test.go` and `jarvis-cli/internal/sddstatus/{source,status}_test.go` cases for missing/corrupt batches, manifest mismatch, continuation/conflict, and blocked apply/status/verify/archive. <!-- sdd-owner: implementation -->
- [ ] 3.2 **GREEN:** Resolve only snapshot-referenced batches in daemon retrieval; update `source.go`, `status.go`, and `cmd/jarvis/cmd_sdd.go` to preserve typed fail-closed outcomes. <!-- sdd-owner: implementation -->
- [ ] 3.3 **TRIANGULATE/REFACTOR:** Cover delayed/forked synced rows and valid partial/complete routing; run `cd jarvis-cli && go test ./... && go vet ./...` plus daemon focused tests. <!-- sdd-owner: implementation -->
- [ ] 4.1 **RED:** Create `jarvis-cli/internal/sddprogress/{openspec,hybrid,store}_test.go` using `t.TempDir()` for atomic interruption, legacy-upgrade failure, divergence, same-request recovery, and archive retention. <!-- sdd-owner: implementation -->
- [ ] 4.2 **GREEN:** Implement lock/fsync/rename snapshot publication, immutable batches/receipts, next-mutation legacy upgrade, and independent hybrid comparison in `internal/sddprogress/` and `cmd_sdd_progress.go`. <!-- sdd-owner: implementation -->
- [ ] 4.3 **TRIANGULATE/REFACTOR:** Prove no side wins when stores differ and archive moves topology together; run `cd jarvis-cli && go test ./... && go vet ./...`. <!-- sdd-owner: implementation -->
- [ ] 5.1 **RED:** Extend `jarvis-cli/internal/skills/catalog_contract_test.go` to require batch append, guarded advance, receipt retry, and continuation/conflict/migration recovery guidance. <!-- sdd-owner: implementation -->
- [ ] 5.2 **GREEN:** Update source assets `embed/skills/{sdd-apply,sdd-archive}/SKILL.md` and `embed/orchestrator/sdd-orchestrator.md`; do not edit generated machine files. <!-- sdd-owner: implementation -->
- [ ] 5.3 **TRIANGULATE/REFACTOR:** Reject legacy cumulative-merge instructions and run `cd jarvis-cli && go test ./... && go vet ./...`; format changed Go with `gofmt`. <!-- sdd-owner: implementation -->

Deferred parent lifecycle action, unchanged:

- [ ] Start or reuse bounded review after each selected work unit; confirm tests, rollback boundary, and total diff against the 400-line budget. <!-- sdd-owner: parent -->

## PR 2 CAS Candidate Correction — Independent-Ready

status: partial

- **Status consumed:** the native status was otherwise blocked only by ambiguous selection; the user's explicit `issue-653-bounded-apply-progress` selection resolves it. `actionContext.mode` is `repo-local`; all changes stay inside supplied PR 2 surfaces; no warnings.
- **Delivery / workload:** stacked-to-main, PR 2 only, under the explicit 1,100-line CAS-only exception. Corrected Go delta: **977 additions / 5 deletions** (tracked plus PR 2 untracked DB files); no PR 3–5 code changed.
- **Task reconciliation:** implementation rows 2.1–2.3 were re-read and are visibly `[x]` after focused, full-daemon, client, vet, formatting, and whitespace verification. Parent-owned row remains unchanged.

### Finding Disposition

1. **Generation-2 CAS:** fixed — an existing head updates only with the expected generation/revision/digest predicate; tests commit generations 1 and 2, then reject a stale writer.
2. **Error semantics:** fixed — capacity remains discoverable via `%w`; real HTTP tests cover 413 capacity/body limit, 422 validation, 409 stale conflict, and 503 unavailable.
3. **Hive client envelopes:** fixed — GET and POST non-2xx replies return `ApplyProgressError` while preserving outcome, code, state, receipt, and recovery.
4. **Immutable topology:** fixed — an SQLite trigger blocks general delete/restore changes to v2 snapshot/evidence rows; active head resolution remains intact.
5. **Replay ordering:** fixed — receipt lookup precedes project registration and writable-state checks; a committed replay succeeds after a project block.
6. **Identity and refs:** fixed — project/change are canonicalized before sealing; every snapshot ref must be supplied or an active canonical durable batch with matching identity/hash.
7. **MCP/concurrency:** fixed within the single-connection SQLite constraint — MCP tests cover invalid recovery, commit receipt, and stale conflict; DB writer test proves one commit/one conflict.

### TDD Cycle Evidence

| Task | Layer | RED | GREEN | TRIANGULATE / REFACTOR |
| --- | --- | --- | --- | --- |
| 2.1 | DB | Generation 2 initially failed on the unique head INSERT; canonical identity and immutable-delete assertions also failed. | Atomic head update, canonicalization, durable-ref validation, receipt ordering, and trigger protections pass. | Added stale, missing-ref, capacity-cause, block-after-receipt, delete/restore, and concurrent cases. |
| 2.2 | HTTP/client | New client envelope contract initially failed to compile without typed error/receipt fields; body-limit HTTP test returned 422 instead of 413. | Typed client errors and 413 mapping pass. | Added real 409/422/503 server envelopes and GET/POST recovery field assertions. |
| 2.3 | MCP | Invalid MCP replies lacked recovery. | Typed recovery replies pass. | Added committed receipt and stale-conflict MCP assertions. |

### Verification

- Focused DB, HTTP, MCP, governance, and hiveclient tests — passed.
- `cd hive-daemon && go test ./...` — passed.
- `cd hive-daemon && go vet ./...` — passed.
- `cd jarvis-cli && go test ./internal/hiveclient && go vet ./internal/hiveclient` — passed.
- `gofmt -l` on all allowed Go paths and `git diff --check` — clean.

### Remaining / Risks

- PR 3–5 implementation rows remain unchecked; parent bounded-review/lifecycle action remains deferred.
- SQLite tests use one connection for `:memory:`, so multi-process WAL contention remains an operational risk to exercise outside this PR 2 unit.

## PR 2 Corrective Rerun — Bounded Apply-Progress Findings

status: partial

- **Status consumed:** explicit parent selection `issue-653-bounded-apply-progress`; artifact store `both`; tasks 2.1–2.3 correction-ready; repo-local action context with only PR 2 and change-artifact surfaces allowed. PR 3–5 remain blocked/deferred.
- **Delivery / workload:** stacked-to-main PR 2 under the active 1,100-line exception. Exact PR 2 delta is **1,007 additions / 6 deletions = 1,013 changed lines**, within the hard maximum.

### Completed Tasks and Persisted Checkboxes

- [x] 2.1 — regression coverage now proves the immutable apply-progress trigger is recreated and operative after the legacy `memories` rebuild.
- [x] 2.2 — semantic governance validation maps to the typed HTTP validation envelope (422), and client conflict handling is narrow to stale CAS only.
- [x] 2.3 — MCP malformed/type-invalid argument recovery and real HTTP/client conflict envelopes are covered.

`tasks.md` was re-read after verification: implementation-owned rows 2.1, 2.2, and 2.3 are visibly `[x]`; the parent-owned lifecycle row was not changed.

### Finding Disposition

1. Recreated `protect_sdd_apply_progress_documents` during the legacy `memories` table rebuild and added it to schema-trigger validation; the regression advances a v2 snapshot after a legacy rebuild and confirms general delete is rejected.
2. Classified `governance.ErrProjectRequired`, `ErrSDDChangeRequired`, and `ErrSDDChangeInvalid` as validation (HTTP 422). Real daemon HTTP tests cover missing-project POST and GET requests.
3. The Hive client now returns a nil error only for the typed stale envelope (`409`, `outcome=conflict`, `code=stale`). `request_id_conflict` and `batch_collision` preserve their full envelope in `ApplyProgressError`.
4. Both MCP get and advance handlers now return concrete recovery guidance for malformed or type-invalid arguments.

### TDD Cycle Evidence

| Task | Test file / layer | Safety net | RED | GREEN | TRIANGULATE / REFACTOR |
| --- | --- | --- | --- | --- | --- |
| 2.1 | `hive-daemon/internal/db/apply_progress_test.go` / SQLite integration | Focused DB suite passed before edit. | Legacy rebuild regression failed because `protect_sdd_apply_progress_documents` was absent. | Recreated the trigger and added it to schema validation; focused test passed. | The test both locates the trigger and proves protected snapshot deletion fails. |
| 2.2 | `hive-daemon/internal/httpapi/sdd_test.go`, `jarvis-cli/internal/hiveclient/sdd_test.go` / HTTP-client integration | Focused HTTP/client suites passed before edit. | Missing-project semantic request returned 503; typed 409 request/batch conflicts returned nil client errors. | 422 mapping and stale-only recovery passed. | Added GET and POST missing-project cases plus both 409 conflict codes with state, receipt, and recovery assertions. |
| 2.3 | `hive-daemon/internal/mcp/tools_test.go` / MCP integration | Focused MCP suite passed before edit. | Type-invalid MCP arguments produced validation envelopes without a `recovery` string. | Both handler paths returned actionable recovery. | Covered get and advance alongside existing semantic-invalid, committed, and stale cases. |

### Verification

- RED: focused DB, HTTP, MCP, and hiveclient commands failed for the four reported behaviors before production changes.
- Passed: focused DB/HTTP/MCP/hiveclient correction tests and `cd hive-daemon && go test ./internal/db ./internal/governance ./internal/httpapi ./internal/mcp`.
- Passed: `cd hive-daemon && go test ./... && go vet ./...`.
- Passed: `cd jarvis-cli && go test ./internal/hiveclient && go vet ./internal/hiveclient`.
- Passed: `gofmt -l` on all changed Go files and `git diff --check`.

### Remaining Tasks / Lifecycle

- [ ] 3.1 **RED:** Add `hive-daemon/internal/db/sdd_test.go` and `jarvis-cli/internal/sddstatus/{source,status}_test.go` cases for missing/corrupt batches, manifest mismatch, continuation/conflict, and blocked apply/status/verify/archive. <!-- sdd-owner: implementation -->
- [ ] 3.2 **GREEN:** Resolve only snapshot-referenced batches in daemon retrieval; update `source.go`, `status.go`, and `cmd/jarvis/cmd_sdd.go` to preserve typed fail-closed outcomes. <!-- sdd-owner: implementation -->
- [ ] 3.3 **TRIANGULATE/REFACTOR:** Cover delayed/forked synced rows and valid partial/complete routing; run `cd jarvis-cli && go test ./... && go vet ./...` plus daemon focused tests. <!-- sdd-owner: implementation -->
- [ ] 4.1 **RED:** Create `jarvis-cli/internal/sddprogress/{openspec,hybrid,store}_test.go` using `t.TempDir()` for atomic interruption, legacy-upgrade failure, divergence, same-request recovery, and archive retention. <!-- sdd-owner: implementation -->
- [ ] 4.2 **GREEN:** Implement lock/fsync/rename snapshot publication, immutable batches/receipts, next-mutation legacy upgrade, and independent hybrid comparison in `internal/sddprogress/` and `cmd_sdd_progress.go`. <!-- sdd-owner: implementation -->
- [ ] 4.3 **TRIANGULATE/REFACTOR:** Prove no side wins when stores differ and archive moves topology together; run `cd jarvis-cli && go test ./... && go vet ./...`. <!-- sdd-owner: implementation -->
- [ ] 5.1 **RED:** Extend `jarvis-cli/internal/skills/catalog_contract_test.go` to require batch append, guarded advance, receipt retry, and continuation/conflict/migration recovery guidance. <!-- sdd-owner: implementation -->
- [ ] 5.2 **GREEN:** Update source assets `embed/skills/{sdd-apply,sdd-archive}/SKILL.md` and `embed/orchestrator/sdd-orchestrator.md`; do not edit generated machine files. <!-- sdd-owner: implementation -->
- [ ] 5.3 **TRIANGULATE/REFACTOR:** Reject legacy cumulative-merge instructions and run `cd jarvis-cli && go test ./... && go vet ./...`; format changed Go with `gofmt`. <!-- sdd-owner: implementation -->

Deferred parent lifecycle action, byte-for-byte unchanged in `tasks.md`:

- [ ] Start or reuse bounded review after each selected work unit; confirm tests, rollback boundary, and total diff against the 400-line budget. <!-- sdd-owner: parent -->

## PR 2 Finalization — Native Semantic Correction Approved

status: partial

- **Delivery / workload:** The maintainer expanded the cohesive CAS-only exception to **1,150 changed lines** after the mandatory correction. PR 2 totals **1,128 changed Go lines** (the independently verified 1,013-line candidate plus a 115-line correction), within that authorization.
- **Native finding:** Review found that canonical and batch-reference checks did not prevent a snapshot whose coverage contradicted its immutable evidence from becoming the durable head.
- **Correction:** Added manifest-free cross-document evidence validation before persistence. Coverage triples must resolve to matching completed evidence entries; complete snapshots require non-empty coverage and referenced batches. CAS and durable receipt replay semantics remain unchanged.
- **TDD evidence:** The new DB rejection tests failed before implementation because invalid snapshots committed. Focused `hivederive/applyprogress` and `hive-daemon/internal/db` suites passed after the correction, including existing stale-CAS, concurrent-writer, idempotent replay, and immutable-document cases.
- **Independent verification:** PASS. Correction delta was exactly **115 additions / 0 deletions**, below the native 180-line correction plan; `gofmt -d` was clean.
- **Native review:** lineage `review-090e30742f7ebe51` completed targeted validation, reached `approved`, and its exact acknowledgement burned the authority. Advisory warnings remain informational follow-up only.
  - **Boundary:** Rows 2.1–2.3 remain `[x]`; PR 3–5 remain unimplemented. No commit, branch, push, or PR was created.

## PR 3 — Hive Resolution and Lifecycle Blocking

status: partial

- **Status consumed:** explicit parent selection of `issue-653-bounded-apply-progress`; OpenSpec authoritative plus Engram progress mirror; repo-local roots cover all edits, no warnings. `stacked-to-main`, PR 3 only, hard 400-line limit.
- **Attempt:** `sha256:3e94e9c33059d1202d2868b89de0a8139f87b74a01565e2a02022d8d48077eef` is settled completed for 3.1–3.3; no receipt, review, or lifecycle gate was created.
- **Completed / persisted:** 3.1–3.3 visibly `[x]`; parent-owned lifecycle work unchanged.

### TDD Cycle Evidence

| Task | RED | GREEN | TRIANGULATE / REFACTOR |
| --- | --- | --- | --- |
| 3.1 | Missing/corrupt/forked retrieval and undefined typed outcomes failed. | Referenced-batch resolution and typed state mapping passed. | Added manifest mismatch and lifecycle cases. |
| 3.2 | Same focused REDs. | Daemon resolves exactly one canonical referenced row; CLI blocks typed outcomes. | Reused existing `ComputeStatus` command routing. |
| 3.3 | Delayed-orphan and partial/complete cases followed GREEN. | Focused DB/CLI tests passed. | Orphans ignored; duplicate referenced rows fail closed. |

- **Files:** `hive-daemon/internal/db/{apply_progress,sdd}{,_test}.go`; `jarvis-cli/internal/sddstatus/{source,status}{,_test}.go`; artifacts. `cmd_sdd.go` needs no change: it already consumes and prints `ComputeStatus` blocked reasons.
- **Verification:** `cd hive-daemon && go test ./internal/db -count=1`; `cd jarvis-cli && go test ./...`; `cd jarvis-cli && go vet ./...`; `gofmt -l` and `git diff --check` passed. An initial root-level Go invocation failed before test execution; retained commands used module directories.
- **Deviation:** none. Only ordered snapshot references are authoritative; delayed orphan rows are ignored.
- **Workload:** PR 3 stays below 400 changed lines; no size exception.
- **Remaining:**
  - [ ] 4.1 **RED:** Create `jarvis-cli/internal/sddprogress/{openspec,hybrid,store}_test.go` using `t.TempDir()` for atomic interruption, legacy-upgrade failure, divergence, same-request recovery, and archive retention. <!-- sdd-owner: implementation -->
  - [ ] 4.2 **GREEN:** Implement lock/fsync/rename snapshot publication, immutable batches/receipts, next-mutation legacy upgrade, and independent hybrid comparison in `internal/sddprogress/` and `cmd_sdd_progress.go`. <!-- sdd-owner: implementation -->
  - [ ] 4.3 **TRIANGULATE/REFACTOR:** Prove no side wins when stores differ and archive moves topology together; run `cd jarvis-cli && go test ./... && go vet ./...`. <!-- sdd-owner: implementation -->
  - [ ] 5.1 **RED:** Extend `jarvis-cli/internal/skills/catalog_contract_test.go` to require batch append, guarded advance, receipt retry, and continuation/conflict/migration recovery guidance. <!-- sdd-owner: implementation -->
  - [ ] 5.2 **GREEN:** Update source assets `embed/skills/{sdd-apply,sdd-archive}/SKILL.md` and `embed/orchestrator/sdd-orchestrator.md`; do not edit generated machine files. <!-- sdd-owner: implementation -->
  - [ ] 5.3 **TRIANGULATE/REFACTOR:** Reject legacy cumulative-merge instructions and run `cd jarvis-cli && go test ./... && go vet ./...`; format changed Go with `gofmt`. <!-- sdd-owner: implementation -->

## PR 3 Correction — Non-validating Inventory and Typed Hybrid Blocks

status: partial

- **Scope / status:** Parent selected `issue-653-bounded-apply-progress`; supplied repo-local PR 3 roots only. `stacked-to-main`, cumulative 400-line objective; no PR4/PR5 code or size exception.
- **Settlement context:** correction token `sha256:9085b9fbf550e7cb3411d8f16d3f469ae43746bcd46f9f87f566f3da82229f08`; a later settlement must use `--remediates-evidence-revision sha256:99db0779097d3446cfcfd10089e414706858c022f7a3a6ddbde082f37b07bdb8`. No receipt or review was created.
- **Correction:** `/artifacts` is inventory-only and does not validate v2 referenced batches; typed validation stays at get apply-progress. Hybrid preserves every typed `blocked:*` state.
- **Strict TDD:** safety nets passed; RED proved invalid topology made inventory fail generically and hybrid continuation/conflict/invalid/manifest mismatch became `partial`; GREEN plus table-driven triangulation and `gofmt` refactor passed.
- **Fresh verification:** focused DB/status tests and `go test ./... && go vet ./...` in both `hive-daemon` and `jarvis-cli` passed.
- **Task reconciliation:** 3.1–3.3 remain visibly `[x]` only after passing behavior/module verification; PR4–PR5 remain unchecked.

## PR 3 Finalization — Parent-Authorized 500-Line Audit Limit

status: partial

- **Decision:** The maintainer authorized an audited reset to **500 cumulative changed lines** after the runtime charged 468 lines across the failed and corrective attempts. The final PR 3 Go delta remains **359 additions / 12 deletions = 371 changed lines**.
- **Independent verification:** PASS. `/artifacts` remains inventory-only; dedicated apply-progress retrieval rejects invalid referenced topology; hybrid mode preserves continuation, conflict, invalid, and manifest-mismatch blocked states; valid partial/complete routing remains intact.
- **Verification:** full `go test ./...` and `go vet ./...` passed from both `hive-daemon` and `jarvis-cli`; `gofmt -d` and `git diff --check` were clean.
  - **Boundary:** Tasks 3.1–3.3 remain `[x]`; PR 4–5 remain untouched. This finalization records the reset objective only and adds no production code.

## PR 4A — Atomic OpenSpec Store Budget Stop

status: blocked

- **Status consumed:** parent explicitly selected `issue-653-bounded-apply-progress`; OpenSpec is authoritative, with Engram required as a progress mirror. Action context is repo-local; all provisional paths were within the supplied edit roots. Delivery boundary is `stacked-to-main`, PR 4A only, with a hard 400-line limit and no exception.
- **TDD evidence:** `cd jarvis-cli && go test ./internal/sddprogress -run '^TestOpenSpecAdvance' -count=1` first failed on missing `OpenSpec`, `AdvanceRequest`, and conflict symbols. A narrow GREEN run passed after provisional file-backed lock/receipt/batch/snapshot code and `t.TempDir()` interruption, collision, stale-CAS, and idempotent-retry tests. `cd jarvis-cli && go test ./cmd/jarvis -run '^TestRunSddProgressAdvance$' -count=1` similarly failed before the provisional command wiring and passed after it.
- **Budget stop / cleanup:** the smallest provisional RED/GREEN working set was 391 new Go lines across five files before the required `store_test.go` payload-conflict/crash-retry/old-snapshot triangulation and this required audit record. Completing 4A.1–4A.3 would exceed the hard 400 additions-plus-deletions limit. Per instruction, all provisional `internal/sddprogress` and `cmd_sdd_progress` source/test files were removed rather than retain an incomplete or oversized candidate.
- **Verification:** the one accidental root-level `go test` invocation failed before test execution because the root has no Go module; all actual Go commands were rerun from `jarvis-cli`. Focused GREEN tests above passed before cleanup. No full module test or vet was run because no implementation was retained.
- **Task reconciliation:** no PR 4A checkbox changed. Rows 4A.1–4A.3 remain visibly `[ ]`; the parent-owned lifecycle row remains unchanged.
- **Attempt settlement:** attempt `sha256:72f93bcb5963d046214f2f816c669aef1711f9cc62e1417eaa26537df5559671` must settle **failed**, with harness disposition `invalidated`, after this evidence is mirrored to Engram. No receipt, review, or delivery gate was created.
- **Remaining implementation tasks:**
  - [ ] 4A.1 **RED:** Add `t.TempDir()` failures in `jarvis-cli/internal/sddprogress/{openspec,store}_test.go` for create-only byte-identical batch reuse versus collision, expected generation/digest conflict, interruption after batch/receipt durability but before snapshot rename, and identical-request retry that publishes exactly once. <!-- sdd-owner: implementation -->
  - [ ] 4A.2 **GREEN:** Implement the OpenSpec-only store and `jarvis sdd progress advance` wiring in `jarvis-cli/internal/sddprogress/{store,openspec}.go` and `jarvis-cli/cmd/jarvis/cmd_sdd_progress.go`: change-local exclusive `apply-progress.lock`, create-only `apply-evidence/<batch-id>.json`, receipt persistence at `.apply-progress-receipts/<request-id>.json`, expected-state recheck, fsync/temp-rename snapshot publication, and directory fsync. <!-- sdd-owner: implementation -->
  - [ ] 4A.3 **TRIANGULATE/REFACTOR:** Add stale-state, request-ID payload-conflict, crash-retry, and old-snapshot-authority cases in `jarvis-cli/internal/sddprogress/{openspec,store}_test.go`; keep the PR 4A delta below 400 additions plus deletions and run `cd jarvis-cli && go test ./internal/sddprogress ./cmd/jarvis && go vet ./...`. <!-- sdd-owner: implementation -->

## PR 4A1 — Atomic OpenSpec File-Store Durability Primitives

status: partial

- **Status / boundary:** Parent explicitly selected `issue-653-bounded-apply-progress`; OpenSpec is authoritative with an Engram progress mirror. Repo-local allowed roots cover every edit; `stacked-to-main`, PR 4A1 only. PR 4A2, hybrid, legacy, archive, CLI, and assets were not changed.
- **Completion / remediation:** 4A1.1–4A1.3 are visibly `[x]`. Attempt `sha256:68a2cde333492189179be029c66d9d095a2626eac01cb1b304beca16c638ff51` is settled passed, remediating `sha256:98f77a4dcb5af1cf2407adc6109feb6f23b17207088b1e0e1336fb0cd1a37161` with fresh focused and module evidence. No receipt, review, or lifecycle gate was created.
- **Files:** Added `jarvis-cli/internal/sddprogress/{store,openspec}.go` and `openspec_test.go`; updated only `tasks.md` and this progress artifact.
- **Workload:** 352 added Go lines; 400-line cap retained with no exception. PR boundary is internal Go seam only.

### TDD Cycle Evidence

| Task | RED | GREEN / TRIANGULATE / REFACTOR |
| --- | --- | --- |
| 4A1.1 | New `t.TempDir()` tests failed on missing store symbols. | Collision, stale state, byte-identical reuse, and durable staged batch/receipt behavior pass. |
| 4A1.2 | Same behavior-first RED. | Lock, immutable writes, receipt, expected-state check, fsynced temp rename, and directory fsync pass. |
| 4A1.3 | Missing referenced-batch regression failed on its missing error. | Retry-after-interruption, changed-payload request conflict, and old-snapshot authority pass. |

### Verification

- `cd jarvis-cli && go test ./internal/sddprogress -count=1` — passed.
- `cd jarvis-cli && go test ./... && go vet ./...` — passed.
- `gofmt -l`, `git diff --check`, and no-index whitespace checks — clean.

### Remaining Implementation Tasks

- [ ] 4A2.1 **RED:** Add `jarvis-cli/cmd/jarvis/cmd_sdd_progress_test.go` end-to-end `t.TempDir()` cases that invoke the public `jarvis sdd progress advance` command path against PR 4A1: stale expected generation/digest returns the typed conflict/current state, and a retry with the same request ID after an injected pre-rename interruption commits one snapshot without duplicating evidence. <!-- sdd-owner: implementation -->
- [ ] 4A2.2 **GREEN:** Implement `jarvis-cli/cmd/jarvis/cmd_sdd_progress.go` as the public `jarvis sdd progress advance` adapter that constructs the typed OpenSpec request, calls the PR 4A1 store seam, and renders committed, conflict, and recovery outcomes without performing direct filesystem durability work. <!-- sdd-owner: implementation -->
- [ ] 4A2.3 **TRIANGULATE/REFACTOR:** In `jarvis-cli/cmd/jarvis/cmd_sdd_progress_test.go` and the reused `jarvis-cli/internal/sddprogress/{openspec,store}_test.go` harness, prove identical retry is idempotent, changed-payload request-ID reuse fails closed, and a crash-retry preserves old-snapshot authority until publication; keep the complete PR 4A2 delta below 400 additions plus deletions and run the stated focused/module tests and vet. <!-- sdd-owner: implementation -->
- [ ] 4B.1 **RED:** Add table-driven and `t.TempDir()` failures in `jarvis-cli/internal/sddprogress/{hybrid,openspec,store}_test.go` and the archive call-site test discovered at `jarvis-cli/cmd/jarvis/cmd_sdd.go` for valid-but-different/missing/invalid backend states, same-request recovery of a partial hybrid publication, failed next-mutation legacy upgrade that preserves its source, and archive retention of snapshot, evidence, and receipts. <!-- sdd-owner: implementation -->
- [ ] 4B.2 **GREEN:** Implement independent hybrid resolution and receipt-driven missing-side retry in `jarvis-cli/internal/sddprogress/hybrid.go`; add next-mutation legacy conversion through `store.go`/`openspec.go`; and update `jarvis-cli/cmd/jarvis/{cmd_sdd_progress,cmd_sdd}.go` so `backend_diverged` blocks lifecycle/archive and a successful archive moves the complete OpenSpec topology together. <!-- sdd-owner: implementation -->
- [ ] 4B.3 **TRIANGULATE/REFACTOR:** Prove neither backend wins on disagreement, read-only legacy paths do not mutate, retry repairs only the recorded missing outcome, and retained/orphan batch behavior remains non-authoritative in `jarvis-cli/internal/sddprogress/{hybrid,openspec,store}_test.go`; keep the PR 4B delta below 400 additions plus deletions and run `cd jarvis-cli && go test ./... && go vet ./...`. <!-- sdd-owner: implementation -->
- [ ] 5.1 **RED:** Extend `jarvis-cli/internal/skills/catalog_contract_test.go` to require batch append, guarded advance, receipt retry, and continuation/conflict/migration recovery guidance. <!-- sdd-owner: implementation -->
- [ ] 5.2 **GREEN:** Update source assets `embed/skills/{sdd-apply,sdd-archive}/SKILL.md` and `embed/orchestrator/sdd-orchestrator.md`; do not edit generated machine files. <!-- sdd-owner: implementation -->
- [ ] 5.3 **TRIANGULATE/REFACTOR:** Reject legacy cumulative-merge instructions and run `cd jarvis-cli && go test ./... && go vet ./...`; format changed Go with `gofmt`. <!-- sdd-owner: implementation -->
- [ ] Start or reuse bounded review after each selected work unit; confirm tests, rollback boundary, and total diff against the 400-line budget. <!-- sdd-owner: parent -->

## PR 4A1 Correction — Durable Filesystem and Publication Validation

status: partial

- **Status consumed:** parent-selected `issue-653-bounded-apply-progress`; repo-local allowed roots with no warnings. The maintainer authorized this cohesive PR 4A1 correction up to **550 Go lines**. PR 4A2, CLI adapter, hybrid, legacy upgrade, archive, PR 5, and generated files remain untouched.
- **Attempt / remediation:** correction attempt `sha256:5502dd03b2d991ca4dc580e6d7670700b5af03faca5d4acce0824990f21e58d7` remediates verifier evidence revision `sha256:6743a698d7fb58cc52c81c34bc841f13012cecbb4b5e13fe606baa908a8c377e`. Apply creates no settlement, receipt, or review approval; parent-owned independent verification remains required.
- **Correction:** create-only evidence and receipt files fsync their parent directories; request identity includes expected generation/revision/digest; an `flock`-held persistent lock file releases on process crash; and publication rejects non-successor generation/revision, mismatched `PreviousDigest`, referenced batch project/change/hash mismatches, and invalid evidence coverage through `ValidateEvidenceCoverage`.
- **Preserved behavior:** byte-identical batch reuse, interruption staging, old-snapshot authority, and same-request retry remain covered.
- **Workload / boundary:** `jarvis-cli/internal/sddprogress/{store,openspec,openspec_test}.go` totals **465 Go lines**, within the authorized 550-line exception.

### TDD Cycle Evidence

| Task | Test file | Layer | Safety net | RED | GREEN | TRIANGULATE / REFACTOR |
| --- | --- | --- | --- | --- | --- |
| 4A1.1–4A1.3 correction | `jarvis-cli/internal/sddprogress/openspec_test.go` | Filesystem unit | `go test ./internal/sddprogress -count=1` passed. | The new durability test failed to compile because `OpenSpec.SyncDir` did not exist. | Added the minimal sync seam, flock lock, guarded successor validation, and referenced-evidence validation; focused tests passed. | Added generation, revision, and digest request-coordinate reuse cases; successor, previous-digest, batch-identity, coverage, stale-lock, interruption, and old-authority cases pass after `gofmt`. |

### Verification

- `cd jarvis-cli && go test ./internal/sddprogress -count=1` — passed (safety net and GREEN/triangulation).
- `cd jarvis-cli && go test ./... -count=1` — passed.
- `cd jarvis-cli && go vet ./...` — passed.
- `gofmt -l` on all PR 4A1 Go files, tracked and no-index `git diff --check` whitespace checks — clean.

### Task Reconciliation / Remaining Work

- Persisted implementation-owned rows **4A1.1–4A1.3 remain visibly `[x]`**; no parent-owned task was changed.
- Exact unchecked implementation rows remain 4A2.1–4A2.3, 4B.1–4B.3, and 5.1–5.3 as listed immediately above. The unchanged deferred lifecycle action is the parent-owned bounded/independent review.

## PR 4A1 Correction — Bidirectional Evidence-Coverage Reconciliation

status: partial

- **Status consumed / action context:** The native status was stale only for ambiguous active-change selection. The parent explicitly selected `issue-653-bounded-apply-progress`, resolving that blocker. This is `repo-local`; every changed path is within the supplied edit surfaces and workspace root, with no action-context warning. The parent-authorized `size:exception` is **550 lines** under `sha256:5502dd03b2d991ca4dc580e6d7670700b5af03faca5d4acce0824990f21e58d7`.
- **Correction:** `ValidateEvidenceCoverage` now reconciles both directions: each snapshot coverage triple must still resolve to a completed immutable evidence entry, and every completed task in referenced evidence must appear once in snapshot coverage. Thus an omitted completion is invalid, while matching partial and complete snapshots retain their valid semantics.
- **Scope preserved:** Only `hivederive/applyprogress/{validate,validate_test}.go`, `jarvis-cli/internal/sddprogress/openspec_test.go`, and the PR 4A1 OpenSpec artifacts changed. No CLI adapter, hybrid, legacy upgrade, archive, PR 5, generated file, or parent-owned task was touched.
- **Workload / PR boundary:** The prior PR 4A1 Go delta was 465 lines. This correction adds exactly **35 additions / 0 deletions** (3 `validate.go`, 12 `validate_test.go`, 20 `openspec_test.go`), for a final PR 4A1 Go delta of **500 additions / 0 deletions = 500 changed lines**, within the explicit 550-line exception. `tasks.md` now records that exception rather than the stale hard-400 requirement.

### TDD Cycle Evidence

| Task | Test file | Layer | Safety net | RED | GREEN | TRIANGULATE | REFACTOR |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 4A1.1–4A1.3 correction | `hivederive/applyprogress/validate_test.go`, `jarvis-cli/internal/sddprogress/openspec_test.go` | Unit / filesystem | `go test ./applyprogress -run '^TestValidateEvidenceCoverage$'` and `go test ./internal/sddprogress -run '^TestOpenSpecAdvance'` passed. | New omitted-completion cases failed: direct validation and `OpenSpec.Advance` both returned `nil`. | The bidirectional cardinality check made both focused RED tests pass. | Covers matching complete coverage, matching partial coverage, coverage without a completion, and completed evidence omitted from partial coverage through both the shared validator and OpenSpec store seam. | Kept the reconciliation as a minimal cardinality check after exact triple validation; focused suites remained green after `gofmt`. |

### Verification

- `cd hivederive && go test ./applyprogress -run '^TestValidateEvidenceCoverage$' -count=1` — RED failed, then GREEN passed.
- `cd jarvis-cli && go test ./internal/sddprogress -run '^TestOpenSpecAdvanceRejectsCompletedEvidenceOmittedFromCoverage$' -count=1` — RED failed, then GREEN passed.
- `cd hivederive && go test ./applyprogress -count=1 && go test ./... -count=1 && go vet ./...` — passed.
- `cd jarvis-cli && go test ./internal/sddprogress -count=1 && go test ./... -count=1 && go vet ./...` — passed.
- `gofmt -w` and `gofmt -l` for the changed Go files, plus tracked and no-index whitespace checks — clean.

### Task Reconciliation / Remaining Work

- Persisted implementation-owned rows **4A1.1–4A1.3 remain visibly `[x]`**; their forecast now cites the explicit 550-line exception. Parent-owned rows are unchanged.
- [ ] 4A2.1 **RED:** Add `jarvis-cli/cmd/jarvis/cmd_sdd_progress_test.go` end-to-end `t.TempDir()` cases that invoke the public `jarvis sdd progress advance` command path against PR 4A1: stale expected generation/digest returns the typed conflict/current state, and a retry with the same request ID after an injected pre-rename interruption commits one snapshot without duplicating evidence. <!-- sdd-owner: implementation -->
- [ ] 4A2.2 **GREEN:** Implement `jarvis-cli/cmd/jarvis/cmd_sdd_progress.go` as the public `jarvis sdd progress advance` adapter that constructs the typed OpenSpec request, calls the PR 4A1 store seam, and renders committed, conflict, and recovery outcomes without performing direct filesystem durability work. <!-- sdd-owner: implementation -->
- [ ] 4A2.3 **TRIANGULATE/REFACTOR:** In `jarvis-cli/cmd/jarvis/cmd_sdd_progress_test.go` and the reused `jarvis-cli/internal/sddprogress/{openspec,store}_test.go` harness, prove identical retry is idempotent, changed-payload request-ID reuse fails closed, and a crash-retry preserves old-snapshot authority until publication; keep the complete PR 4A2 delta below 400 additions plus deletions and run the stated focused/module tests and vet. <!-- sdd-owner: implementation -->
- [ ] 4B.1 **RED:** Add table-driven and `t.TempDir()` failures in `jarvis-cli/internal/sddprogress/{hybrid,openspec,store}_test.go` and the archive call-site test discovered at `jarvis-cli/cmd/jarvis/cmd_sdd.go` for valid-but-different/missing/invalid backend states, same-request recovery of a partial hybrid publication, failed next-mutation legacy upgrade that preserves its source, and archive retention of snapshot, evidence, and receipts. <!-- sdd-owner: implementation -->
- [ ] 4B.2 **GREEN:** Implement independent hybrid resolution and receipt-driven missing-side retry in `jarvis-cli/internal/sddprogress/hybrid.go`; add next-mutation legacy conversion through `store.go`/`openspec.go`; and update `jarvis-cli/cmd/jarvis/{cmd_sdd_progress,cmd_sdd}.go` so `backend_diverged` blocks lifecycle/archive and a successful archive moves the complete OpenSpec topology together. <!-- sdd-owner: implementation -->
- [ ] 4B.3 **TRIANGULATE/REFACTOR:** Prove neither backend wins on disagreement, read-only legacy paths do not mutate, retry repairs only the recorded missing outcome, and retained/orphan batch behavior remains non-authoritative in `jarvis-cli/internal/sddprogress/{hybrid,openspec,store}_test.go`; keep the PR 4B delta below 400 additions plus deletions and run `cd jarvis-cli && go test ./... && go vet ./...`. <!-- sdd-owner: implementation -->
- [ ] 5.1 **RED:** Extend `jarvis-cli/internal/skills/catalog_contract_test.go` to require batch append, guarded advance, receipt retry, and continuation/conflict/migration recovery guidance. <!-- sdd-owner: implementation -->
- [ ] 5.2 **GREEN:** Update source assets `embed/skills/{sdd-apply,sdd-archive}/SKILL.md` and `embed/orchestrator/sdd-orchestrator.md`; do not edit generated machine files. <!-- sdd-owner: implementation -->
- [ ] 5.3 **TRIANGULATE/REFACTOR:** Reject legacy cumulative-merge instructions and run `cd jarvis-cli && go test ./... && go vet ./...`; format changed Go with `gofmt`. <!-- sdd-owner: implementation -->
- Deferred lifecycle action, unchanged: `- [ ] Start or reuse bounded review after each selected work unit; confirm tests, rollback boundary, and total diff against the 400-line budget. <!-- sdd-owner: parent -->`.

**Parent independent verification remains required. This apply phase neither settles the attempt nor creates/approves a receipt or review.**

## PR 4A2 — Public OpenSpec Advance Adapter and Integration

status: partial

- **Status / action context:** Parent explicitly selected `issue-653-bounded-apply-progress`, resolving the stale ambient-selection ambiguity. Artifact store is `both`; OpenSpec artifacts remain authoritative and the Engram mirror is updated. `repo-local` action context permits all supplied edit surfaces, with no warnings.
- **Boundary / workload:** `stacked-to-main`, PR 4A2 only; the exact token is `sha256:ba43bf8be22bac2144a568adf7bce444b837286f70c9941c7b19dd9e933e9b84`. The two PR 4A2 Go files total **230 additions / 0 deletions**, below the hard 400-line cap. No exception, PR 4A1 store-semantic change, hybrid, legacy upgrade, archive, PR 5 asset, generated-file, review, receipt, or lifecycle action was performed.
- **Implementation:** Added `jarvis sdd progress advance --root <change-root> --request <request.json>`. The adapter decodes a typed `sddprogress.AdvanceRequest`, delegates publication to the PR 4A1 `OpenSpec.Advance` seam, and emits JSON `committed`, typed `conflict` (with current generation/revision/digest), or `recovery` outcomes. Reading the existing canonical snapshot for conflict output does not duplicate lock, receipt, immutable-write, fsync, or rename behavior.

### Completed / Persisted Task Checkboxes

- [x] 4A2.1 **RED:** Command-level `t.TempDir()` tests were written and executed first; the focused command test failed with undefined adapter symbols before production code existed. The retained tests exercise stale expected state and pre-rename interruption/retry.
- [x] 4A2.2 **GREEN:** Added the adapter and registered it below `jarvis sdd progress`; it constructs the typed request, uses the PR 4A1 seam, and renders committed/conflict/recovery JSON.
- [x] 4A2.3 **TRIANGULATE/REFACTOR:** Added same-payload idempotent retry, changed-payload request-ID conflict, old-snapshot authority during interruption, and unique evidence-count assertions; formatting and full module verification pass.

### TDD Cycle Evidence

| Task | RED | GREEN | TRIANGULATE / REFACTOR |
| --- | --- | --- | --- |
| 4A2.1 | `cd jarvis-cli && go test ./cmd/jarvis -run '^TestSddProgressAdvance' -count=1` failed on missing `newSddProgressCommand`, `defaultOpenSpec`, output, and adapter symbols. | The focused command suite passed after the minimal adapter. | Stale expected generation/digest returns `conflict/stale` with the current typed state. |
| 4A2.2 | The command-level missing-symbol test preceded production code. | `advance` decodes the typed request and delegates only to `OpenSpec.Advance`. | Interrupted publication emits `recovery`; retry emits the pending committed state. |
| 4A2.3 | Changed-payload same-ID behavior was added after GREEN. | Same request is idempotent and a changed payload returns `conflict/request_id_conflict`. | The test proves the old canonical snapshot is authoritative before rename and verifies two unique immutable evidence files after retry. |

### Files Changed

- `jarvis-cli/cmd/jarvis/cmd_sdd_progress.go`
- `jarvis-cli/cmd/jarvis/cmd_sdd_progress_test.go`
- `openspec/changes/issue-653-bounded-apply-progress/tasks.md`
- `openspec/changes/issue-653-bounded-apply-progress/apply-progress.md`

### Verification

- `cd jarvis-cli && go test ./cmd/jarvis -run '^TestSddProgressAdvance' -count=1` — RED failed, then GREEN passed.
- `cd jarvis-cli && go test ./internal/sddprogress -count=1` — passed.
- `cd jarvis-cli && go test ./cmd/jarvis -run '^TestSddProgressAdvance' -count=1` — passed after refactor.
- `cd jarvis-cli && go test ./... -count=1` — passed.
- `cd jarvis-cli && go vet ./...` — passed.
- `gofmt -l jarvis-cli/cmd/jarvis/cmd_sdd_progress.go jarvis-cli/cmd/jarvis/cmd_sdd_progress_test.go` and `git diff --check` — clean.

### Remaining / Deferred

- [ ] 4B.1 **RED:** Add table-driven and `t.TempDir()` failures in `jarvis-cli/internal/sddprogress/{hybrid,openspec,store}_test.go` and the archive call-site test discovered at `jarvis-cli/cmd/jarvis/cmd_sdd.go` for valid-but-different/missing/invalid backend states, same-request recovery of a partial hybrid publication, failed next-mutation legacy upgrade that preserves its source, and archive retention of snapshot, evidence, and receipts. <!-- sdd-owner: implementation -->
- [ ] 4B.2 **GREEN:** Implement independent hybrid resolution and receipt-driven missing-side retry in `jarvis-cli/internal/sddprogress/hybrid.go`; add next-mutation legacy conversion through `store.go`/`openspec.go`; and update `jarvis-cli/cmd/jarvis/{cmd_sdd_progress,cmd_sdd}.go` so `backend_diverged` blocks lifecycle/archive and a successful archive moves the complete OpenSpec topology together. <!-- sdd-owner: implementation -->
- [ ] 4B.3 **TRIANGULATE/REFACTOR:** Prove neither backend wins on disagreement, read-only legacy paths do not mutate, retry repairs only the recorded missing outcome, and retained/orphan batch behavior remains non-authoritative in `jarvis-cli/internal/sddprogress/{hybrid,openspec,store}_test.go`; keep the PR 4B delta below 400 additions plus deletions and run `cd jarvis-cli && go test ./... && go vet ./...`. <!-- sdd-owner: implementation -->
- [ ] 5.1 **RED:** Extend `jarvis-cli/internal/skills/catalog_contract_test.go` to require batch append, guarded advance, receipt retry, and continuation/conflict/migration recovery guidance. <!-- sdd-owner: implementation -->
- [ ] 5.2 **GREEN:** Update source assets `embed/skills/{sdd-apply,sdd-archive}/SKILL.md` and `embed/orchestrator/sdd-orchestrator.md`; do not edit generated machine files. <!-- sdd-owner: implementation -->
- [ ] 5.3 **TRIANGULATE/REFACTOR:** Reject legacy cumulative-merge instructions and run `cd jarvis-cli && go test ./... && go vet ./...`; format changed Go with `gofmt`. <!-- sdd-owner: implementation -->
- Deferred parent lifecycle action, unchanged: `- [ ] Start or reuse bounded review after each selected work unit; confirm tests, rollback boundary, and total diff against the 400-line budget. <!-- sdd-owner: parent -->`.

**Parent independent verification remains required. This apply phase does not settle the supplied attempt token or create/approve a receipt or review.**

## PR 4B — Budget-Settled Partial Attempt

status: partial

- **Status / action context:** Parent explicitly selected `issue-653-bounded-apply-progress`; the supplied delta spec is authoritative. Artifact store is `both`; repo-local allowed roots cover the authorized PR4B paths. Attempt token `sha256:c9f714f87b5ef53574398b7c1df50cdee39b360c5d538131a5151d7e08aacf46` remains unsettled.
- **Delivery / workload:** `stacked-to-main`, hard 400-line cap, no exception. This attempt stopped before adding unsafe or incomplete receipt-repair and legacy-conversion shims. No task checkbox was changed.
- **Retained partial safety work:** `sddprogress.ResolveHybrid` fails closed for different, missing, or invalid independently resolved snapshots; OpenSpec v2 reads validate only snapshot-referenced evidence; hybrid lifecycle routing emits `blocked:backend_diverged`; and `OpenSpec.Archive` validates then directory-renames snapshot, evidence, and receipts together.

### TDD Cycle Evidence

| Task | Test file / layer | Safety net | RED | GREEN / triangulation | REFACTOR |
| --- | --- | --- | --- | --- | --- |
| 4B.1 partial | `internal/sddprogress/hybrid_test.go`, `internal/sddstatus/source_test.go` / unit | Focused sddprogress, sddstatus, and command tests passed before edits. | `TestResolveHybrid*` failed on missing resolver/types; typed lifecycle test failed on missing divergence state. | Matching snapshots pass; different, missing, and invalid sides return divergence; all apply/verify/archive paths block. | Kept comparison pure and fail-closed. |
| 4B.1 partial | `internal/sddprogress/openspec_test.go` / filesystem | Existing focused sddprogress tests passed. | Archive topology test failed on missing `OpenSpec.Archive`. | Archive test proves snapshot, referenced immutable batch, and receipt move together after validation. | Directory rename retains the topology atomically. |

### Verification

- Focused RED commands failed as recorded above, then focused `internal/sddprogress` and `internal/sddstatus` tests passed.
- `cd jarvis-cli && go test ./... -count=1` — passed.
- `cd jarvis-cli && go vet ./...` — passed.
- `gofmt -l` on changed Go files and `git diff --check` — clean. Go LSP analysis was unavailable; compiler-backed tests and vet passed.

### Remaining Tasks / Stop Condition

- [ ] 4B.1 **RED:** Add table-driven and `t.TempDir()` failures in `jarvis-cli/internal/sddprogress/{hybrid,openspec,store}_test.go` and the archive call-site test discovered at `jarvis-cli/cmd/jarvis/cmd_sdd.go` for valid-but-different/missing/invalid backend states, same-request recovery of a partial hybrid publication, failed next-mutation legacy upgrade that preserves its source, and archive retention of snapshot, evidence, and receipts. <!-- sdd-owner: implementation -->
- [ ] 4B.2 **GREEN:** Implement independent hybrid resolution and receipt-driven missing-side retry in `jarvis-cli/internal/sddprogress/hybrid.go`; add next-mutation legacy conversion through `store.go`/`openspec.go`; and update `jarvis-cli/cmd/jarvis/{cmd_sdd_progress,cmd_sdd}.go` so `backend_diverged` blocks lifecycle/archive and a successful archive moves the complete OpenSpec topology together. <!-- sdd-owner: implementation -->
- [ ] 4B.3 **TRIANGULATE/REFACTOR:** Prove neither backend wins on disagreement, read-only legacy paths do not mutate, retry repairs only the recorded missing outcome, and retained/orphan batch behavior remains non-authoritative in `jarvis-cli/internal/sddprogress/{hybrid,openspec,store}_test.go`; keep the PR 4B delta below 400 additions plus deletions and run `cd jarvis-cli && go test ./... && go vet ./...`. <!-- sdd-owner: implementation -->

**Stop condition:** receipt-driven partial-publication recovery and conservative next-mutation legacy conversion are still required. They were not approximated because that would violate the fail-closed contract. Parent lifecycle/review must decide whether to retain this partial safety slice or reset it; this apply phase does not settle the token or create/approve any receipt/review.

## PR 4B1 — Fail-Closed Hybrid Reads and Archive Retention

status: partial

- **Status / action context:** Parent selected PR4B1 for `issue-653-bounded-apply-progress`; artifact store `both`; authorized repo-local surfaces cover every change. Attempt token `sha256:9650094b8606279f98776e4656f9ed5740140e203be676b933d788ddd84daaf8` remains unsettled.
- **Delivery / workload:** The carried 248/400-line runtime budget left 152 lines. This successor added only the strict snapshot-coordinate equality check plus its focused regression and stayed within the remaining cap. No exception, PR4B2 receipt retry, or legacy mutation work was added.
- **Completed / persisted checkboxes:** 4B1.1–4B1.3 are visibly `[x]`. PR4B2 and parent-owned rows remain unchanged.

### TDD Cycle Evidence

| Task | Test file / layer | Safety net | RED | GREEN / triangulation | REFACTOR |
| --- | --- | --- | --- | --- | --- |
| 4B1.1–4B1.3 | `jarvis-cli/internal/sddprogress/hybrid_test.go` / unit | Focused sddprogress, sddstatus, and command suites passed. | A side with the same digest but different generation incorrectly resolved as equal. | `ResolveHybrid` now compares identity, generation/revision, predecessor/manifest/status, digest, ordered refs, and coverage. | Existing missing/invalid/different/equal resolver cases and retained OpenSpec/archive/lifecycle tests remain green. |

### Verification

- RED: `cd jarvis-cli && go test ./internal/sddprogress -run '^TestResolveHybridFailsClosedOnDifferentMissingOrInvalidSides/same_digest_different_state$' -count=1` — failed before the comparison fix.
- GREEN: `cd jarvis-cli && go test ./internal/sddprogress -run '^TestResolveHybrid' -count=1` — passed.
- `cd jarvis-cli && go test ./... -count=1` — passed.
- `cd jarvis-cli && go vet ./...` — passed.
- `gofmt -l` on changed Go files and `git diff --check` — clean. Go LSP was unavailable; compiler-backed tests and vet passed.

### Remaining Tasks / Boundary

- [ ] 4B2.1 **RED:** Add table-driven and `t.TempDir()` failures in `jarvis-cli/internal/sddprogress/{hybrid,openspec,store}_test.go` and `jarvis-cli/cmd/jarvis/cmd_sdd_progress_test.go` for durable `pending|committed|failed` per-backend receipt outcomes, same-request retry after partial publication, retry limited to the recorded missing side, request-ID payload conflict, read-only legacy status/verify/archive without mutation, and failed next-mutation conversion or commit that preserves the legacy source. <!-- sdd-owner: implementation -->
- [ ] 4B2.2 **GREEN:** Implement receipt-backed missing-side retry in `jarvis-cli/internal/sddprogress/hybrid.go`; add conservative next-mutation legacy conversion and commit sequencing through `jarvis-cli/internal/sddprogress/{store,openspec}.go`; and wire only the mutating `jarvis-cli/cmd/jarvis/cmd_sdd_progress.go` path so legacy source removal occurs only after durable v2 publication succeeds. <!-- sdd-owner: implementation -->
- [ ] 4B2.3 **TRIANGULATE/REFACTOR:** Prove a partial hybrid publication remains blocked until same-request recovery, recovery does not rewrite an already committed side, changed-payload request-ID reuse fails closed, ambiguous legacy input returns structured migration failure, successful upgrade commits initial v2 generation, and every conversion/commit failure leaves the source readable and authoritative; keep the complete PR 4B2 delta at or below 400 additions plus deletions and run `cd jarvis-cli && go test ./... && go vet ./...`. <!-- sdd-owner: implementation -->

**PR boundary:** PR4B1 is complete. Return to parent lifecycle for bounded review; PR4B2 is a separate future apply slice. This phase does not create/approve receipts or settle the attempt token.

## PR 4B1 Corrective Attempt — Independent Verifier Blockers Closed

status: partial

- **Status / action context:** Parent authorized correction of exactly four PR4B1 blockers under attempt token `sha256:e9bb145e6ae0a4c5aae9ef22180b83de300145f5aada8c3e8db3328b4b7daea0`; all edits remain in authorized repo-local surfaces. This is attempt 1 of at most 2, within the 400-line cap.
- **Correction:** Archive now invokes manifest-aware `ValidateProgress` whenever `tasks.md` exists and leaves stale progress in place; `sdd archive --root --destination` wires `OpenSpec.Archive` into the production SDD command; OpenSpec v2 manifest mismatch retains `blocked:task_manifest_mismatch`; hybrid divergence removes selected apply-progress content so consumers get only the neutral blocking state.
- **Persisted checkboxes:** Re-read confirms 4B1.1–4B1.3 remain visibly `[x]`; PR4B2 rows remain `[ ]` and were not touched.

### Corrective TDD Cycle Evidence

| Blocker | RED | GREEN / triangulation |
| --- | --- | --- |
| Archive manifest validation | `TestOpenSpecArchiveRejectsStaleTaskManifest` initially archived stale tasks; the valid-manifest triangulation initially exposed an incorrect checkbox parser. | Stale manifest blocks without rename; current manifest archives successfully. |
| Production archive routing | `TestSddArchiveInvokesProductionArchiver` initially failed on missing command/interface. | Injected routing and real OpenSpec command invocation both pass. |
| Typed OpenSpec mismatch | OpenSpec v2 mismatch returned generic invalid. | Returns `blocked:task_manifest_mismatch`. |
| Neutral divergence | Hybrid divergence retained selected content. | Divergence returns only `blocked:backend_diverged` with no apply-progress content. |

### Verification

- Focused RED/GREEN archive, source, hybrid, and command tests passed after fixes.
- `cd jarvis-cli && go test ./... -count=1` — passed.
- `cd jarvis-cli && go vet ./...` — passed.
- `gofmt -l` on changed Go files and `git diff --check` — clean. Go LSP was unavailable; compiler-backed tests and vet passed.

### Remaining Boundary

PR4B2 receipt retry and conservative legacy upgrade remain explicitly deferred. This correction does not create/approve a receipt or settle the attempt token; parent lifecycle owns next review.

### Correction Completion Addendum

- Archive now also rejects a missing current `tasks.md`; it never falls back to evidence-only validation. The existing archive topology and real `sdd archive` command tests now construct a manifest-current task artifact, proving valid archives still succeed.
- `cd jarvis-cli && go test ./... -count=1`, `go vet ./...`, `gofmt -l`, and `git diff --check` were rerun after this final guard and passed.

## PR 4B1 Archive Lifecycle Remediation

status: partial

- **Status / action context:** Parent authorized remediation token `sha256:8d8b78404404966b478637b02ad18d8f40849a4220aaf4cbf3c9aefbfc934e75`, with 160 lines remaining in the 400-line corrective objective. PR4B2 remains excluded.
- **Correction:** `jarvis sdd archive` now resolves the configured store/lifecycle status from the supplied OpenSpec change root, permits only OpenSpec or hybrid mode, and invokes `OpenSpec.Archive` only when authoritative apply progress is `done` and archive dependency is `ready`. Partial, diverged, blocked, and Hive-only states return before mutation.

### TDD Cycle Evidence

| Behavior | RED | GREEN / triangulation |
| --- | --- | --- |
| Lifecycle-gated archive | `TestRunSddArchiveFailsClosedOnLifecycleState` initially failed on missing gate. | Partial, `backend_diverged`, and Hive-only modes make zero archiver calls; completed OpenSpec calls once. Existing real command topology test remains green. |

### Verification

- `cd jarvis-cli && go test ./cmd/jarvis -run '^(TestRunSddArchiveFailsClosedOnLifecycleState|TestSddArchive)' -count=1` — passed.
- `cd jarvis-cli && go test ./... -count=1` — passed.
- `cd jarvis-cli && go vet ./...` — passed.
- `gofmt -l` and `git diff --check` — clean. Go LSP was unavailable; compiler-backed tests and vet passed.

PR4B1 rows remain visibly `[x]`; PR4B2 remains unchecked. This remediation does not create/approve a receipt or settle the token; parent lifecycle owns review.

## PR 4B1 Final Archive TOCTOU Closure

status: partial

- **Status / action context:** Parent authorized final token `sha256:7daf2ef141bf5371093c06cb398a79f2d3f8fe2bb3dbd2516a3690f2284b8fd7`, one attempt, hard 200 lines. PR4B2 remains excluded.
- **Correction:** Under the archive lock, `OpenSpec.Archive` now rejects every snapshot whose status is not `complete` before manifest/evidence validation and directory rename. The CLI lifecycle check remains UX-only; the store lock and current snapshot read are authoritative.

### TDD Cycle Evidence

| Behavior | RED | GREEN / triangulation |
| --- | --- | --- |
| Locked partial snapshot | A manifest-valid partial snapshot archived successfully. | Partial snapshots remain at the active root and return failure. |
| Post-lock replacement | The new deterministic post-lock hook was absent. | A hook that replaces the current snapshot with partial data after lock acquisition is re-read and rejected before rename; valid complete fixtures still archive through both store and command tests. |

### Verification

- `cd jarvis-cli && go test ./internal/sddprogress -run '^TestOpenSpecArchive' -count=1` — passed.
- `cd jarvis-cli && go test ./... -count=1` — passed.
- `cd jarvis-cli && go vet ./...` — passed.
- `gofmt -l` and `git diff --check` — clean. Go LSP was unavailable; compiler-backed tests and vet passed.

PR4B1 checkboxes remain `[x]`; PR4B2 is untouched. This phase does not create/approve a receipt or settle the token; parent lifecycle owns review.

## PR4B2 — Blocked: Required Spec Artifact Missing

status: blocked

- **Status consumed/produced:** The parent explicitly selected `issue-653-bounded-apply-progress`, resolving the stale ambient ambiguity in the supplied status. `actionContext.mode` is `repo-local`; the supplied edit surfaces are within the workspace root; no action-context warning applies. However, the authoritative OpenSpec change directory has `proposal.md`, `design.md`, `tasks.md`, and this progress record but **no required spec artifact** (`spec.md` or `specs/**`).
- **Stop condition:** Per the apply contract, implementation cannot start while a required spec is absent. No production or test file was written, no PR4B2 checkbox was changed, and attempt token `sha256:2caf598c3d49ed0411edd5391c3f8173ea62f567ee2fef55ef15d144755e3cb7` remains unsettled.
- **Required parent action:** Restore or provide the authoritative PR4B2 spec artifact, then rerun apply with the same bounded delivery path. The existing proposal/design/tasks are retained untouched; no receipt, review, or lifecycle action was created.

## PR4B2 — Hybrid Receipt Recovery and Legacy Upgrade

status: partial

- **Status consumed:** Parent-selected `issue-653-bounded-apply-progress`; authoritative delta spec `specs/bounded-apply-progress/spec.md`; strict TDD; repo-local action context with only supplied surfaces. The previously recorded missing-spec blocker is superseded. Delivery boundary is stacked-to-main PR4B2, hard 400-line cap, no exception; attempt token `sha256:2caf598c3d49ed0411edd5391c3f8173ea62f567ee2fef55ef15d144755e3cb7` remains parent-owned and unsettled.
- **Completed / persisted checkboxes:** 4B2.1, 4B2.2, and 4B2.3 are visibly `[x]`; parent-owned and PR5 rows are unchanged.
- **Implementation:** Hybrid receipts persist `pending|committed|failed` per side and serialize same-request recovery under a hybrid lock; replay sends the original request/CAS coordinates only to a non-committed side. Changed request payloads and malformed receipt state fail closed. The mutating progress command alone upgrades unambiguous legacy source into initial v2 generation; conversion, stale source, or pre-rename failure leaves legacy `apply-progress.md` authoritative. Legacy archive exits before creating its lock, so read-only legacy routing does not write.

### TDD Cycle Evidence

| Task | Layer | RED | GREEN | TRIANGULATE / REFACTOR |
| --- | --- | --- | --- | --- |
| 4B2.1 | `internal/sddprogress` filesystem/unit and command integration | New hybrid/legacy tests failed on absent receipt and upgrade APIs. | Focused suites passed after minimum receipt, upgrade, and command wiring. | Both interruption directions, replay, failed outcome, payload conflict, conversion failure, interrupted publication, and read-only archive cases pass. |
| 4B2.2 | Filesystem/unit and command integration | Covered by the missing-symbol/legacy-migration RED failures. | Exact request identity and v2 generation 1 publication pass. | Refactored legacy validation to compare source-derived manifest/status/coverage before publication. |
| 4B2.3 | Focused and module integration | Added recovery and source-preservation cases after GREEN. | Focused suites remained green. | `go test ./...`, `go vet ./...`, `gofmt`, and whitespace verification pass. |

### Files / Verification / Remaining

- Changed: `jarvis-cli/internal/sddprogress/{hybrid,openspec,store}.go`, matching `hybrid_test.go` and `openspec_test.go`, and `jarvis-cli/cmd/jarvis/cmd_sdd_progress{,_test}.go`.
- Passed: focused hybrid, legacy, and command tests; `cd jarvis-cli && go test ./... -count=1`; `cd jarvis-cli && go vet ./...`; `gofmt -l`; `git diff --check`.
- No design deviation; `mem_save` semantics and PR5 assets are untouched.

## PR4B2A — Production Hybrid Mutation Routing

status: partial

- **Status consumed:** Parent-selected `issue-653-bounded-apply-progress`, authoritative delta spec, strict TDD, repo-local action context, and token `sha256:2116457e33dfebe85230d394b7996a7fe1d7a6c84874df8f53d05e98fb009fa9`. PR4B2A is the stacked-to-main, hard-400-line slice; no exception was used.
- **Completed / persisted checkboxes:** 4B2A.1–4B2A.3 are visibly `[x]`; 4B2B and parent-owned rows are unchanged.
- **Implementation:** `jar sdd progress advance` now resolves `JARVIS_SDD_STORE_MODE`: OpenSpec and Hive retain their single-backend writers, while hybrid creates `sddprogress.Hybrid` with OpenSpec and Hive guarded writers. The Hive adapter preserves request ID and expected generation/revision/digest. Partial hybrid publication returns `blocked:backend_diverged`; replay calls only receipt-recorded non-committed sides. Stale and request-ID conflicts remain fail-closed.

### TDD Cycle Evidence

| Task | RED | GREEN / TRIANGULATE |
| --- | --- | --- |
| 4B2A.1 | Configured-store command tests failed on missing routing. | Hybrid interruption/replay, stale, and changed-request tests pass; existing hybrid tests cover both directions and receipt transitions. |
| 4B2A.2 | Covered by command-routing RED. | Hybrid is selected in production hybrid mode and Hive receives exact CAS/request coordinates. |
| 4B2A.3 | Hybrid interruption initially emitted generic recovery. | It now emits `blocked:backend_diverged`; module tests, vet, formatting, and whitespace checks pass. |

- **Files / verification:** changed `jarvis-cli/cmd/jarvis/cmd_sdd_progress{,_test}.go` and `jarvis-cli/internal/sddprogress/hybrid.go`; passed focused sddprogress/command tests, `cd jarvis-cli && go test ./... -count=1`, `go vet ./...`, `gofmt -l`, and `git diff --check`.
- **Out of scope:** Existing legacy code remains untouched and is not claimed by PR4B2A. Exact unchecked rows remain 4B2B.1–4B2B.3, PR5.1–PR5.3, and the parent lifecycle row.

## PR4B2A Correction — Backend Conflict State

- **Status / budget:** correction token `sha256:0d60acfc2675cdddf564e1d40319d835b74bffeac62483767fdc23acbbb9c489`; PR4B2A cumulative budget is 144/400. PR4B2B remains untouched.
- **Fix:** Hive writer conflicts retain the daemon-returned generation, revision, and digest; hybrid preserves the failing backend result; command rendering uses that returned state before any OpenSpec fallback. OpenSpec-only conflicts retain their existing local-state fallback.
- **TDD evidence:** New configured Hive and hybrid command tests first failed because output state was zero/local. After the minimal writer-seam and command changes they pass alongside existing OpenSpec conflict regression coverage.
- **Verification:** focused configured/standard command tests and `internal/sddprogress`; `cd jarvis-cli && go test ./... -count=1`; `cd jarvis-cli && go vet ./...`; `gofmt -l`; `git diff --check` — passed.

## PR4B2A Final Correction — OpenSpec-First Stale Coordinates

- **Status / budget:** final token `sha256:ab2a5ac3ad5b9097f3823f5e01d0d907e163dde9bc26b8c18c6b1787598b257b`; 210 lines remained. PR4B2B remains untouched.
- **Fix:** When OpenSpec returns a stale CAS conflict before Hive is advanced, hybrid performs a Hive read-only current-state lookup and emits those authoritative coordinates. No Hive advance occurs; an uncommitted stale-only hybrid receipt is removed and its directory synced. Partial committed-side receipts remain durable for normal missing-side replay.
- **TDD evidence:** The new configured hybrid command test initially returned zero/local coordinates. It now proves Hive generation/revision/digest win, OpenSpec advances once, Hive advances zero times, and Hive current-state reads once.
- **Verification:** focused configured/standard command and `internal/sddprogress` suites; `cd jarvis-cli && go test ./... -count=1`; `cd jarvis-cli && go vet ./...`; `gofmt -l`; `git diff --check` — passed.

## PR4B2B Final Numeric Identity and Rollback Durability

- **Status / budget:** final token `sha256:fc5a4b15adad4374c3385245c556fd734e1dd437fc924e047639e723035b0428`; one attempt, hard 150-line correction budget; no PR5 changes.
- **Fixes:** status and conversion both reject nondigit legacy IDs, including `- [x] garbage words`. Legacy restore now fsyncs the parent directory after fsync+rename, so standalone SyncDir rollback and hybrid rollback use the same durability seam and fail closed when it fails.
- **TDD / verification:** added nondigit multiword classification/conversion coverage; existing injected post-rename SyncDir failure preserves bytes and retry remains successful. `cd jarvis-cli && go test ./... -count=1`; `go vet ./...`; `gofmt -l`; `git diff --check` — passed.

## PR4B2B Final In-Objective Corrections

- **Status / budget:** final token `sha256:4810081c1bf4bbcc16f97b9d32264e094f5762678eb94e224b9e209b03b6728c`; 223 lines remaining; no PR5 changes.
- **Fixes:** all checkbox/task-like legacy rows require identity plus text in status and conversion. If hybrid publication fails after OpenSpec conversion, it restores the original legacy bytes while retaining its durable missing-side receipt; an identical retry re-upgrades OpenSpec and completes the missing Hive side.
- **TDD Cycle Evidence:** RED one-field `- [x] garbage` coverage exposed silent acceptance. GREEN rejects it on both paths. TRIANGULATE injects Hive failure after OpenSpec replacement, verifies byte-identical restoration, then verifies a successful retry and two Hive attempts.
- **Verification:** focused status/conversion/hybrid recovery tests; `cd jarvis-cli && go test ./... -count=1`; `cd jarvis-cli && go vet ./...`; `gofmt -l`; `git diff --check` — passed.
- **Files:** `jarvis-cli/internal/sddstatus/{source,source_test}.go`, `jarvis-cli/internal/sddprogress/{openspec,openspec_test,hybrid,hybrid_test}.go`, apply-progress artifact.

## PR4B2B Verifier Corrections

- **Status / budget:** correction token `sha256:bf3af346d1bb28566c65afa3521635ca33bf1c580db3433d8baa9adbddee7c21`; 342 lines remaining; no PR5 changes.
- **Fixes:** malformed checkbox rows now fail closed in both status classification and mutation conversion; `Hybrid.UpgradeLegacy` executes the OpenSpec conversion seam before its normal durable hybrid advance; post-rename `SyncDir` failure restores byte-identical legacy source and supports the identical retry.
- **TDD Cycle Evidence:** RED tests exposed valid rows plus malformed rows being silently accepted. GREEN rejects malformed markers, proves configured hybrid production upgrade calls Hive, and injects root directory sync failure. TRIANGULATE verifies the restored original bytes and successful identical retry.
- **Verification:** focused status, OpenSpec migration, and configured hybrid tests; `cd jarvis-cli && go test ./... -count=1`; `cd jarvis-cli && go vet ./...`; `gofmt -l`; `git diff --check` — passed.
- **Files:** `jarvis-cli/internal/sddstatus/{source,source_test}.go`, `jarvis-cli/internal/sddprogress/{openspec,openspec_test,hybrid}.go`, `jarvis-cli/cmd/jarvis/cmd_sdd_progress_test.go`, apply-progress artifact.

## PR4B2B Conservative Legacy Completion

- **Status / budget:** token `sha256:5e45abdad31687cf37bf62fafe10efca128ee53f496e3119c57cf9892ce5b443`; hard 400-line PR4B2B budget; no PR5 changes.
- **Completed tasks:** 4B2B.1–4B2B.3 marked `[x]` in `tasks.md`.
- **TDD Cycle Evidence:** RED: explicit legacy `status: complete` with incomplete or duplicate tasks was accepted as done, and conversion allowed incomplete completion. GREEN: read paths require unique all-checked task evidence; mutation conversion rejects incomplete complete-marked source. TRIANGULATE: interrupted conversion keeps the legacy source and a subsequent identical authorized mutation succeeds.
- **Read-only behavior:** status classification consumes only artifact content; legacy archive rejects before acquiring/creating its lock or moving the source. Invalid, ambiguous, and incomplete legacy artifacts are partial/blocked.
- **Verification:** focused `internal/sddstatus` legacy classification and `internal/sddprogress` legacy conversion/upgrade tests; `cd jarvis-cli && go test ./... -count=1`; `cd jarvis-cli && go vet ./...`; `gofmt -l`; `git diff --check` — passed.
- **Files:** `jarvis-cli/internal/sddstatus/{source,source_test}.go`, `jarvis-cli/internal/sddprogress/{openspec,openspec_test}.go`, task/progress artifacts.

## PR4B2A Final Receipt Replay Cleanup

- **Status / budget:** final PR4B2A token `sha256:d62af53a2a6ef4fec6af6d3cf9a742c0414b78eb677e1164d69e02a09e342316`; one attempt, hard 150-line correction budget. PR4B2B remains untouched.
- **Fix:** A stale-only receipt is now removed after a later successful Hive current-state lookup even when the receipt originated in an earlier failed invocation. Receipts with an OpenSpec or Hive committed outcome remain durable for missing-side repair.
- **TDD cycle evidence:** RED: deterministic fail-then-replay test left the prior stale-only receipt behind. GREEN: removal predicate now uses receipt outcomes, not invocation-local creation. TRIANGULATE: partial OpenSpec-committed/Hive-stale receipt remains present.
- **Files / verification:** `jarvis-cli/internal/sddprogress/{hybrid,hybrid_test}.go`; focused stale-replay/partial-receipt tests, full `cd jarvis-cli && go test ./... -count=1`, `go vet ./...`, `gofmt -l`, and `git diff --check` — passed.
- Remaining implementation rows:
  - [ ] 5.1 **RED:** Extend `jarvis-cli/internal/skills/catalog_contract_test.go` to require batch append, guarded advance, receipt retry, and continuation/conflict/migration recovery guidance. <!-- sdd-owner: implementation -->
  - [ ] 5.2 **GREEN:** Update source assets `embed/skills/{sdd-apply,sdd-archive}/SKILL.md` and `embed/orchestrator/sdd-orchestrator.md`; do not edit generated machine files. <!-- sdd-owner: implementation -->
  - [ ] 5.3 **TRIANGULATE/REFACTOR:** Reject legacy cumulative-merge instructions and run `cd jarvis-cli && go test ./... && go vet ./...`; format changed Go with `gofmt`. <!-- sdd-owner: implementation -->
- Deferred parent lifecycle action, unchanged: `- [ ] Start or reuse bounded review after each selected work unit; confirm tests, rollback boundary, and total diff against the 400-line budget. <!-- sdd-owner: parent -->`.

## PR5 — Bounded Executor and Lifecycle Source Assets

status: partial

- **Status consumed:** parent-selected `issue-653-bounded-apply-progress` resolves the ambient selection ambiguity. Strict TDD is active. This is the `stacked-to-main` PR5 slice under the hard 400 changed-line limit; the repo-local allowed surfaces cover every edit and no generated user-machine file changed.
- **Completed / persisted:** implementation-owned rows 5.1–5.3 are marked `[x]` in `tasks.md`. The parent-owned bounded-review action remains unchanged.
- **Implementation:** catalog contracts now require immutable evidence batches, a guarded snapshot advance, 40,000-rune batch/snapshot limits, receipt replay, and typed continuation/conflict/capacity/migration/divergence recovery. Canonical `sdd-apply`, `sdd-archive`, and orchestrator assets reject cumulative observation merging and `mem_save` as an apply-progress replacement; they direct executors to the guarded CLI flow and fail closed.
- **Workload / PR boundary:** 105 additions and 38 deletions across the four PR5 source/test surfaces (143 changed lines), below 400. Rollback is limited to these asset contracts and their catalog test.

### TDD Cycle Evidence

| Task | Layer | Safety net | RED | GREEN | TRIANGULATE / REFACTOR |
| --- | --- | --- | --- | --- | --- |
| 5.1 | Catalog contract | `go test ./internal/skills -run '^TestCatalogContract_SDDApplySourceUsesJarvisAdaptedStatusGuards$'` passed. | New bounded-protocol catalog test failed because the apply, archive, and orchestrator assets lacked required immutable-batch guidance. | Canonical asset updates satisfied the focused contract test. | Added forbidden legacy merge and `mem_save` replacement assertions. |
| 5.2 | Embedded-asset integration | Focused catalog contract passed after GREEN. | The full CLI suite exposed existing installed-asset compatibility expectations for ambiguity-safe retrieval and current apply/checkbox gating. | Added explicit safe compatibility wording while retaining rejection of obsolete cumulative merging; full suite passed. | No production-code refactor required; source assets remain the only behavior source. |
| 5.3 | Catalog/module | Focused contract passed before full verification. | Forbidden historical cumulative-save guidance was asserted before final module verification. | The focused catalog test passed after the canonical assets removed it. | `gofmt`, `git diff --check`, full tests, and vet remained clean. |

### Verification

- RED: `cd jarvis-cli && go test ./internal/skills -run '^TestCatalogContract_BoundedApplyProgressGuidanceUsesGuardedImmutableProtocol$' -count=1` — failed on missing asset guidance.
- GREEN/TRIANGULATE: focused catalog contracts — passed.
- `cd jarvis-cli && go test ./... -count=1` — passed.
- `cd jarvis-cli && go vet ./...` — passed.
- `gofmt -l jarvis-cli/internal/skills/catalog_contract_test.go` and `git diff --check` — clean.

### Remaining / Deferred

- No implementation-owned task remains unchecked.
- Deferred parent lifecycle action, unchanged: `- [ ] Start or reuse bounded review after each selected work unit; confirm tests, rollback boundary, and total diff against the 400-line budget. <!-- sdd-owner: parent -->`.

## PR5 Correction — Native Dependency Ready Literal

status: partial

- **Status / budget:** parent-selected `issue-653-bounded-apply-progress`; correction token `sha256:943e350b33bac6154f6777ea4ff76de187f06d6e71a2d42b14f8a5170a30531c`; PR5 cumulative 220/400. Only the candidate-caused orchestrator literal and its catalog regression coverage changed.
- **Correction:** restored the exact readable native authority contract: `dependencies[phase] ==`ready``. No other asset instruction was rewritten.
- **TDD evidence:** the existing focused catalog test passed as the safety net. RED added the exact required literal and malformed-literal rejection, then failed because the asset omitted the required spacing. GREEN changed only the literal; the catalog test passed. The required/forbidden pair retains regression coverage for both correct and malformed forms.
- **Verification:** `cd jarvis-cli && go test ./internal/skills -run '^TestCatalogContract_BoundedApplyProgressGuidanceUsesGuardedImmutableProtocol$' -count=1`, `go test ./... -count=1`, and `go vet ./...` passed; `gofmt -l` and `git diff --check` were clean.
- **Deferred:** no implementation-owned task changed; the parent-owned bounded-review action remains unchanged.

## PR5 Final Correction — Native Authority Formatting

status: partial

- **Status / budget:** parent-selected `issue-653-bounded-apply-progress`; final PR5 token `sha256:2afdfb8f95793d968752efb2d7363e456212f512090c485905ae6a59e695d212`; 166 lines remain. Only the orchestrator authority sentence and its catalog regression changed.
- **Correction:** restored both exact literals without changing surrounding semantics: dependencies[phase] == `ready` and actionContext.mode == `workspace-edit`. Keys are plain text; code ticks enclose values only.
- **TDD evidence:** safety-net focused catalog test passed. RED added the required workspace-edit literal and malformed-form rejection; it failed against the malformed candidate literal. GREEN changed that literal only. The regression now requires both authority literals and rejects both malformed no-space forms.
- **Verification:** focused catalog gate-drift test, `cd jarvis-cli && go test ./... -count=1`, `cd jarvis-cli && go vet ./...`, `gofmt -l`, and `git diff --check` passed.
- **Deferred:** no implementation-owned task changed; the parent-owned bounded-review action remains unchanged.

## PR 6A — Pure Shared Checkpoint Planner

status: partial

- **Status consumed:** parent-selected `issue-653-bounded-apply-progress` and token `sha256:4135de1cf383ede04fd44743fa1bac5cbcdf2271aa43de06b0cd5bd1a0fa6449`; the supplied selection resolves the native ambiguous-change blocker. `actionContext.mode` is `repo-local`, allowed roots cover every changed path, and no warnings apply.
- **Delivery boundary:** stacked-to-main PR 6A only; strict TDD; hard 400-line source budget. The two new planner files total **339 lines**, and no CLI, persistence, daemon, asset, or advance-semantics surface changed.
- **Completed / persisted:** implementation-owned rows **6A.1–6A.3 are visibly `[x]`** in `tasks.md`; parent-owned rows remain byte-for-byte unchanged.

### TDD Cycle Evidence

| Task | Test file / layer | Safety net | RED | GREEN | TRIANGULATE / REFACTOR |
| --- | --- | --- | --- | --- | --- |
| 6A.1–6A.3 | `hivederive/applyprogress/checkpoint_test.go` / pure unit | `cd hivederive && go test ./applyprogress -count=1` passed. | Focused `TestPlanCheckpoint` failed on undefined `PlanCheckpoint`, `PlanInput`, plan dispositions, and result type. | Minimum pure planner made focused tests pass. | Added exact 40,000/40,001 rune boundaries, both no-write capacity outcomes, cursor/manifest rejection, deterministic replay, successor coordinates, and repeated 162,725-rune ordering evidence. |

### Implementation and Verification

- Added `hivederive/applyprogress/checkpoint.go`: pure one-batch maximal-prefix planning which seals existing canonical batch/snapshot values, preserves manifest-ordered coverage, and returns only `committed`, `continuation_required`, `evidence_item_too_large`, or `snapshot_capacity_exhausted` for domain planning outcomes.
- Added `hivederive/applyprogress/checkpoint_test.go`: repeated 162,725-rune fixture retains all entry values once/in order and confirms every emitted canonical document fits; no planner result performs I/O or changes external state.
- RED: `cd hivederive && go test ./applyprogress -run '^TestPlanCheckpoint' -count=1` — failed as required before production code existed.
- GREEN / focused: `cd hivederive && go test ./applyprogress -run '^TestPlanCheckpoint' -count=1` — passed.
- Full: `cd hivederive && go test ./... && go vet ./...` — passed.
- Formatting / whitespace: `gofmt -l hivederive/applyprogress/checkpoint.go hivederive/applyprogress/checkpoint_test.go` and `git diff --check` — clean.
- **Deviation:** none. `advance`, storage, CLI, daemon, HTTP/MCP, assets, lifecycle, legacy conversion, and ID/request generation remain unchanged.

### Remaining / PR Boundary

- [ ] 6B.1 **RED:** Add `jarvis-cli/cmd/jarvis/cmd_sdd_progress_test.go` `t.TempDir()` command-path cases for canonical checkpoint request/output parsing, initial nullable base and resolved-current/base/expected-coordinate checks, exactly one planned prefix per invocation, and OpenSpec propagation of `committed`, `continuation_required` (committed state, receipt, `next_entry_index`, `next_entry_id`), `evidence_item_too_large`, and `snapshot_capacity_exhausted`; assert either capacity result calls no advance and creates no receipt, batch, snapshot, or coverage. <!-- sdd-owner: implementation -->
- [ ] 6C.1 **RED:** Add focused configured-Hive and configured-hybrid checkpoint tests in `jarvis-cli/cmd/jarvis/cmd_sdd_progress_test.go` and concrete existing test seams under `jarvis-cli/internal/{sddprogress,hiveclient}/` for current-snapshot/base rebasing, exact advance-payload retry, and propagation of committed state/receipt, continuation cursor, and both no-write planner capacity outcomes; assert HTTP/MCP and low-level `advance` still expose only their unchanged generic defensive capacity contract. <!-- sdd-owner: implementation -->
- [ ] 6D.1 **RED:** Extend `jarvis-cli/internal/skills/catalog_contract_test.go` and focused command tests to require canonical `checkpoint` guidance for ordered complete entries, stable base/cursor, same-request byte-identical retry, new request/batch IDs only after continuation, and the exact four checkpoint outcomes; reject guidance that makes `advance`, HTTP, or MCP select prefixes or plan capacity. <!-- sdd-owner: implementation -->
- [ ] 6B.2 **GREEN:** Add the sibling `checkpoint` adapter in `jarvis-cli/cmd/jarvis/cmd_sdd_progress.go` and only the focused existing `jarvis-cli/internal/sddprogress/` seam needed to resolve validated OpenSpec current state and submit the planner-produced `AdvanceRequest`; preserve the original request ID/batch ID/base/cursor/entries for an exact retry, create a new request ID/batch ID only for a returned continuation, and leave `advance` flags, behavior, and generic defensive capacity handling untouched. <!-- sdd-owner: implementation -->
- [ ] 6B.3 **TRIANGULATE/REFACTOR:** Prove a repeated durable OpenSpec command scenario over exactly 162,725 evidence runes: every non-final invocation commits a non-empty maximal prefix and returns matching next index/ID; replaying one uncertain invocation with byte-identical payload returns the same receipt/state without duplicate batch, coverage, or generation; each successor uses returned coordinates with new IDs; the final invocation returns `committed`; resolving recovers every entry once/in order with documents at or below 40,000 runes. Also prove stale CAS and changed-payload request-ID reuse fail closed; run focused/module tests and vet while keeping PR 6B at or below 400 changed lines. <!-- sdd-owner: implementation -->
- [ ] 6C.2 **GREEN:** Extend only the checkpoint coordinator/store-mode selection in `jarvis-cli/cmd/jarvis/cmd_sdd_progress.go` and focused existing resolver seams so Hive calls existing `AdvanceApplyProgress` and hybrid calls existing `Hybrid.Advance` after validated resolution; pass the exact planner output and retain CAS/current-state, request-ID, receipt, legacy, and divergence envelopes rather than translating or duplicating them. <!-- sdd-owner: implementation -->
- [ ] 6C.3 **TRIANGULATE/REFACTOR:** Prove same-request replay repairs only the receipt-recorded missing hybrid side for both OpenSpec→Hive and Hive→OpenSpec interruptions, with no committed-side rewrite or cursor advance until equality; prove stale/rebased coordinates and request-ID payload conflicts commit nothing; prove continuation/capacity output parity across OpenSpec, Hive, and hybrid; run focused/module tests and vet while keeping PR 6C at or below 400 changed lines. <!-- sdd-owner: implementation -->
- [ ] 6D.2 **GREEN:** Update only `jarvis-cli/embed/skills/sdd-apply/SKILL.md` and `jarvis-cli/embed/orchestrator/sdd-orchestrator.md` to use `jarvis sdd progress checkpoint` for normal executor checkpoints and preserve low-level `advance` recovery/compatibility guidance; after PR 6A–6C evidence is complete, reconcile `openspec/changes/issue-653-bounded-apply-progress/apply-progress.md` and the change's Engram task/apply-progress mirrors with the new PR 6 records, without editing generated user-machine artifacts. <!-- sdd-owner: implementation -->
- [ ] 6D.3 **TRIANGULATE/REFACTOR:** Run final cross-mode scenario tests for exact 162,725-rune repeated checkpoints, all four checkpoint outcomes, stale/request-ID/cursor failure no-writes, OpenSpec/Hive/hybrid continuation parity, unchanged `advance`/HTTP/MCP compatibility, and recovered ordered evidence; run the stated focused/module tests and vet, record the commands/results in the PR 6 audit evidence, and keep the complete PR 6D diff at or below 400 changed lines. <!-- sdd-owner: implementation -->
- Parent-owned bounded review is deferred to parent lifecycle.

## PR 6B — OpenSpec Checkpoint Command

- **Status consumed:** parent-selected `issue-653-bounded-apply-progress` resolves the native ambiguous selection; repo-local action context permits every supplied surface with no warnings. This is the `stacked-to-main` PR 6B slice under the supplied hard 400-line budget (`sha256:cdd272e26d0c967a444bee5de07fa716344e83a24a47539699081dcf97932985`).
- **Completed / persisted:** implementation-owned 6B.1, 6B.2, and 6B.3 are visibly `[x]`; no parent-owned row changed.
- **Implementation:** added strict canonical checkpoint input/output handling, one pure `PlanCheckpoint` call, validated OpenSpec current-state resolution, and exactly one existing `OpenSpec.Advance` delegation for a plan. Capacity results do not advance. Continuations return committed coordinates/snapshot, receipt request ID, and the next cursor; exact replay retains the caller IDs and payload. `advance` remains unchanged; Hive/hybrid routing, daemon surfaces, and assets are excluded.

### TDD Cycle Evidence

| Task | Test file / layer | Safety net | RED | GREEN | TRIANGULATE / REFACTOR |
| --- | --- | --- | --- | --- | --- |
| 6B.1 | `cmd_sdd_progress_test.go` / command integration | `go test ./cmd/jarvis ./internal/sddprogress -count=1` passed. | `TestSddProgressCheckpointCommitsInitialStream` failed on undefined checkpoint request, command, and output symbols. | The initial nullable-base checkpoint command test passed. | Added exact capacity/no-advance output cases. |
| 6B.2 | same / command integration | same | Covered by 6B.1 RED. | `OpenSpec.Current` validates referenced evidence and the command delegates one planned request to existing `Advance`. | Existing low-level advance tests remain green. |
| 6B.3 | same / durable command integration | same | Added repeated durable scenario after GREEN. | Focused checkpoint suite passed. | Exact 162,725-rune stream, continuation cursor/IDs, byte-identical replay/no duplicate batch, resolved order, stale CAS, and changed request-ID payload conflict pass. |

### Verification / Boundary

- `cd jarvis-cli && go test ./cmd/jarvis -run '^TestSddProgressCheckpoint' -count=1` — passed.
- `cd jarvis-cli && go test ./cmd/jarvis ./internal/sddprogress -count=1` — passed.
- `cd jarvis-cli && go test ./... && go vet ./...` — passed.
- `gofmt -l` on all allowed Go files and `git diff --check` — clean.
- **PR workload:** 327 Go-line additions across the PR6B command/test/resolver delta before the three task-checkbox and this cumulative audit update; within the 400-line hard boundary. No design deviation.
- Remaining implementation rows:
  - [ ] 6C.1 **RED:** Add focused configured-Hive and configured-hybrid checkpoint tests in `jarvis-cli/cmd/jarvis/cmd_sdd_progress_test.go` and concrete existing test seams under `jarvis-cli/internal/{sddprogress,hiveclient}/` for current-snapshot/base rebasing, exact advance-payload retry, and propagation of committed state/receipt, continuation cursor, and both no-write planner capacity outcomes; assert HTTP/MCP and low-level `advance` still expose only their unchanged generic defensive capacity contract. <!-- sdd-owner: implementation -->
  - [ ] 6C.2 **GREEN:** Extend only the checkpoint coordinator/store-mode selection in `jarvis-cli/cmd/jarvis/cmd_sdd_progress.go` and focused existing resolver seams so Hive calls existing `AdvanceApplyProgress` and hybrid calls existing `Hybrid.Advance` after validated resolution; pass the exact planner output and retain CAS/current-state, request-ID, receipt, legacy, and divergence envelopes rather than translating or duplicating them. <!-- sdd-owner: implementation -->
  - [ ] 6C.3 **TRIANGULATE/REFACTOR:** Prove same-request replay repairs only the receipt-recorded missing hybrid side for both OpenSpec→Hive and Hive→OpenSpec interruptions, with no committed-side rewrite or cursor advance until equality; prove stale/rebased coordinates and request-ID payload conflicts commit nothing; prove continuation/capacity output parity across OpenSpec, Hive, and hybrid; run focused/module tests and vet while keeping PR 6C at or below 400 changed lines. <!-- sdd-owner: implementation -->
  - [ ] 6D.1 **RED:** Extend `jarvis-cli/internal/skills/catalog_contract_test.go` and focused command tests to require canonical `checkpoint` guidance for ordered complete entries, stable base/cursor, same-request byte-identical retry, new request/batch IDs only after continuation, and the exact four checkpoint outcomes; reject guidance that makes `advance`, HTTP, or MCP select prefixes or plan capacity. <!-- sdd-owner: implementation -->
  - [ ] 6D.2 **GREEN:** Update only `jarvis-cli/embed/skills/sdd-apply/SKILL.md` and `jarvis-cli/embed/orchestrator/sdd-orchestrator.md` to use `jarvis sdd progress checkpoint` for normal executor checkpoints and preserve low-level `advance` recovery/compatibility guidance; after PR 6A–6C evidence is complete, reconcile `openspec/changes/issue-653-bounded-apply-progress/apply-progress.md` and the change's Engram task/apply-progress mirrors with the new PR 6 records, without editing generated user-machine artifacts. <!-- sdd-owner: implementation -->
  - [ ] 6D.3 **TRIANGULATE/REFACTOR:** Run final cross-mode scenario tests for exact 162,725-rune repeated checkpoints, all four checkpoint outcomes, stale/request-ID/cursor failure no-writes, OpenSpec/Hive/hybrid continuation parity, unchanged `advance`/HTTP/MCP compatibility, and recovered ordered evidence; run the stated focused/module tests and vet, record the commands/results in the PR 6 audit evidence, and keep the complete PR 6D diff at or below 400 changed lines. <!-- sdd-owner: implementation -->
- Parent bounded review remains deferred unchanged.

## PR 6B Correction — Candidate Replay Receipt Identity

- **Status / token:** parent-selected PR6B correction `sha256:f4f774e6f567e297f50eee14b8d87630ec92b4494628c0c6170e1c1f8ec36bc6`; same repo-local action context and no warnings.
- **TDD:** new command-path candidate replay test failed when a changed request ID returned `committed` with that nonexistent receipt; it passes after receipt identity is required before accepting an already-current digest. Existing same-ID replay remains covered by the 162,725-rune command scenario.
- **Verification:** focused checkpoint tests, full `cd jarvis-cli && go test ./...`, and `go vet ./...` passed; `gofmt`/whitespace clean.
- **Budget:** 23 Go-line correction delta, within the supplied remaining 35 lines. No task checkbox changed; 6B.1–6B.3 remain visibly complete.

## PR 6C — Configured Hive and Hybrid Checkpoint Reuse

status: partial

- **Status consumed:** the parent explicitly selected `issue-653-bounded-apply-progress` with token `sha256:99336f5831c460f9e897c8926ab80d362d02c3b39a6eb4e8affc01e51e62a09b`; it resolves the ambient ambiguous selection. Repo-local action context permits only the supplied paths, with no warnings.
- **Delivery / boundary:** stacked-to-main PR 6C; strict TDD; this slice adds configured checkpoint resolver routing only and stays below the hard 400-line budget. No PR 6D assets, daemon endpoint, wire model, low-level `advance`, HTTP/MCP capacity behavior, persistence topology, legacy routing, or lifecycle code changed.
- **Completed / persisted:** implementation-owned rows **6C.1–6C.3 are visibly `[x]`** in `tasks.md`. Parent-owned lifecycle work is unchanged.

### TDD Cycle Evidence

| Task | Test file / layer | Safety net | RED | GREEN | TRIANGULATE / REFACTOR |
| --- | --- | --- | --- | --- | --- |
| 6C.1 | `cmd_sdd_progress_test.go` / command integration | `go test ./cmd/jarvis ./internal/sddprogress ./internal/hiveclient -count=1` passed. | Configured Hive/hybrid checkpoint test failed in hybrid mode because checkpoint was OpenSpec-only. | Configured routing resolved Hive and hybrid state and delegated the planner result. | Added initial/rebased replay, changed payload conflict, and no committed-side rewrite assertions. |
| 6C.2 | `cmd_sdd_progress.go`, `hybrid.go` / integration | Same focused baseline. | Covered by the configured command RED. | Hive uses existing `AdvanceApplyProgress`; hybrid uses existing `Hybrid.Advance`; both receive the planner's exact request. | Hybrid current-state resolution accepts only equal validated snapshots and preserves divergence. |
| 6C.3 | `hybrid_test.go` / integration | Existing hybrid receipt tests passed. | Existing interruption direction tests remained the RED safety contract. | Existing receipt recovery remained green. | Added equal/missing current-state coverage; both existing interruption directions prove only the recorded missing side retries, with no committed-side rewrite. |

### Files and Verification

- Changed `jarvis-cli/cmd/jarvis/cmd_sdd_progress.go` to resolve configured Hive/hybrid current state before planning and to retain existing advance/CAS/error envelopes.
- Changed `jarvis-cli/internal/sddprogress/hybrid.go` to resolve only matching validated current snapshots for checkpoint planning.
- Changed focused command and hybrid tests. `hiveclient` transport code remains unchanged; its existing client tests preserve the generic low-level capacity envelope.
- RED: `cd jarvis-cli && go test ./cmd/jarvis -run '^TestConfiguredSddProgressCheckpointUsesHiveAndHybridStores$' -count=1` failed before hybrid checkpoint routing existed.
- GREEN and triangulation: focused `cmd/jarvis`, `internal/sddprogress`, and `internal/hiveclient` tests passed.
- Full: `cd jarvis-cli && go test ./... && go vet ./...` — passed.
- Formatting / whitespace: `gofmt -w` and clean `gofmt -l` for changed Go files; `git diff --check` — passed.

### Remaining / Deferred

- [ ] 6D.1 **RED:** Extend `jarvis-cli/internal/skills/catalog_contract_test.go` and focused command tests to require canonical `checkpoint` guidance for ordered complete entries, stable base/cursor, same-request byte-identical retry, new request/batch IDs only after continuation, and the exact four checkpoint outcomes; reject guidance that makes `advance`, HTTP, or MCP select prefixes or plan capacity. <!-- sdd-owner: implementation -->
- [ ] 6D.2 **GREEN:** Update only `jarvis-cli/embed/skills/sdd-apply/SKILL.md` and `jarvis-cli/embed/orchestrator/sdd-orchestrator.md` to use `jarvis sdd progress checkpoint` for normal executor checkpoints and preserve low-level `advance` recovery/compatibility guidance; after PR 6A–6C evidence is complete, reconcile `openspec/changes/issue-653-bounded-apply-progress/apply-progress.md` and the change's Engram task/apply-progress mirrors with the new PR 6 records, without editing generated user-machine artifacts. <!-- sdd-owner: implementation -->
- [ ] 6D.3 **TRIANGULATE/REFACTOR:** Run final cross-mode scenario tests for exact 162,725-rune repeated checkpoints, all four checkpoint outcomes, stale/request-ID/cursor failure no-writes, OpenSpec/Hive/hybrid continuation parity, unchanged `advance`/HTTP/MCP compatibility, and recovered ordered evidence; run the stated focused/module tests and vet, record the commands/results in the PR 6 audit evidence, and keep the complete PR 6D diff at or below 400 changed lines. <!-- sdd-owner: implementation -->
- Deferred parent lifecycle action: `- [ ] Start or reuse bounded review after each selected work unit; confirm tests, rollback boundary, and total diff against the 400-line budget. <!-- sdd-owner: parent -->`.

## PR 6C Correction — Receipt-First Recovery

- **Status / token:** parent-selected correction `sha256:751a8e230dc960b3c25dd897d122a31c16421e20f0a120663080355bc7a6c66b`; strict TDD; 181 lines remained. Repo-local allowed roots and action context remain unchanged.
- **Finding 1 fixed:** checkpoint plans the exact existing advance request first. When a hybrid current read reports expected divergence but a durable hybrid receipt exists for that request ID, it delegates to receipt-aware `Hybrid.Advance` so only the recorded missing side repairs. Unrelated divergence remains blocked.
- **Finding 2 fixed:** configured Hive recognizes the daemon's apply-progress not-found validation envelope as the initial empty state for checkpoint resolution; no daemon endpoint, planner, or wire model changed.
- **Finding 3 fixed:** stale coordinates no longer preempt the existing guarded advance. A changed payload reusing a committed request ID reaches receipt-aware advance and returns `request_id_conflict`; ordinary stale CAS still returns `stale` with no commit.

### Correction TDD Evidence

| Finding | RED | GREEN / triangulation |
| --- | --- | --- |
| Hybrid partial recovery | Command-path tests failed because strict hybrid equality returned `read_current_failed` before the durable receipt could repair either interruption direction. | Both OpenSpec→Hive and Hive→OpenSpec command scenarios replay the same request and invoke only the previously missing side. |
| Hive initial state | Daemon-style HTTP 422 validation response failed as `read_current_failed`. | Configured Hive command test treats it as initial state and commits generation 1 through existing `AdvanceApplyProgress`. |
| Changed request ID payload | Same original coordinates were classified as `stale` before reaching advance. | Configured Hive and hybrid tests now receive `request_id_conflict`; existing stale tests remain green. |

### Verification

- Focused correction tests: `cd jarvis-cli && go test ./cmd/jarvis -run '^(TestConfiguredSddProgressCheckpointUsesHiveAndHybridStores|TestSddProgressCheckpointRecoversHybridReceiptBeforeDivergence|TestConfiguredSddProgressCheckpointTreatsHiveNotFoundAsInitial)$' -count=1` — passed.
- `cd jarvis-cli && go test ./cmd/jarvis ./internal/sddprogress ./internal/hiveclient -count=1` — passed.
- `cd jarvis-cli && go test ./... && go vet ./...` — passed.
- `gofmt -l` and `git diff --check` — clean.
- **Boundary:** no PR6D asset work, daemon endpoint, daemon planner, persistence path, low-level capacity contract, or Hive wire model was changed. Implementation rows 6C.1–6C.3 remain visibly complete; PR6D and the parent lifecycle action remain deferred.

## PR 6C Final Correction — Exact Partial-Receipt Gate

- **Status / token:** parent-selected correction `sha256:bff995a39f450d5a635e2956e4bb0b28506abc810a5bcdc088ff22e1132de3a6`; strict TDD; 50 lines remained.
- **Correction:** `Hybrid.HasReceipt` now bypasses strict current-state equality only for the exact sealed request payload with exactly one `committed` backend and one `pending|failed` backend. Fully committed, no-commit, malformed, or payload-mismatched receipts leave divergence fail-closed.
- **TDD:** `TestSddProgressCheckpointBlocksDivergentCompletedHybridReceipt` failed before the gate narrowed (a fully committed receipt masked divergence), then passed; both existing one-sided recovery direction tests remain green.
- **Verification:** focused correction test, `cd jarvis-cli && go test ./... && go vet ./...`, `gofmt -l`, and `git diff --check` passed.
- **Boundary:** only receipt gating and its focused command test changed. No PR6D, daemon, planner, endpoint, or wire-model work. Rows 6C.1–6C.3 remain visibly `[x]`; parent lifecycle remains deferred.

## PR 6D — Canonical Checkpoint Guidance and Final Reconciliation

- **Status / token:** parent-selected `issue-653-bounded-apply-progress`, `sha256:97841dc007406fde8231b82506002964c46e1347f4d81a3e7a8e2d56a7feb4ce`; strict TDD; stacked-to-main PR6D. The explicit selection resolves the native ambiguous-change blocker. `actionContext` is repo-local with the repository as the allowed edit root and no warnings.
- **Completed / persisted:** rows **6D.1–6D.3 are visibly `[x]`**. All implementation-owned rows are now complete; the parent-owned bounded-review row is unchanged.
- **Contract:** normal executors use `jarvis sdd progress checkpoint` with ordered complete entries and a stable base/cursor. Exact retries retain request ID, batch ID, base, cursor, and byte-identical payload; a continuation alone authorizes new IDs. Guidance defines `committed`, `continuation_required`, `evidence_item_too_large`, and `snapshot_capacity_exhausted`, including no-write capacity handling. Low-level `advance`, HTTP, and MCP remain recovery/compatibility boundaries and never plan prefixes.
- **Scope / rollback:** changed canonical embedded sources, catalog contracts, and focused command regressions only. No generated machine artifact, daemon endpoint, wire model, storage topology, or low-level `advance` behavior changed. Roll back the guidance/contracts first; retain committed progress data and recovery primitives.

### TDD Cycle Evidence

| Task | Layer | Safety net | RED | GREEN | TRIANGULATE / REFACTOR |
| --- | --- | --- | --- | --- | --- |
| 6D.1 | Catalog contract | Focused bounded-guidance test passed before edits. | Added checkpoint-only contract assertions; it failed because both canonical sources still prescribed `advance`. | Updated source assets; focused catalog test passed. | Added explicit rejection of asking `advance`, HTTP, or MCP to plan prefixes/capacity. |
| 6D.2 | Embedded-source integration | Focused catalog test was green after 6D.1. | Covered by 6D.1's missing-checkpoint RED. | Guidance now prescribes exact retry/continuation and four-outcome handling. | No production-code refactor: only canonical source assets changed. |
| 6D.3 | Command integration | Existing advance/checkpoint focused suite passed before the final regression. | PR6B/6C RED evidence remains the prerequisite command behavior. | Existing 162,725-rune, capacity, stale/retry, and ordered-recovery tests remain green. | Added continuation parity across OpenSpec, Hive, and hybrid; all paths commit the same first prefix and final successor. |

### Final Verification

- `cd jarvis-cli && go test ./internal/skills -run '^TestCatalogContract_BoundedApplyProgressGuidanceUsesGuardedImmutableProtocol$' -count=1` — RED failed before asset updates, then passed.
- `cd jarvis-cli && go test ./cmd/jarvis -run '^(TestSddProgressAdvance|TestSddProgressCheckpoint|TestConfiguredSddProgressCheckpoint)' -count=1` — passed: exact 162,725-rune repeated OpenSpec checkpoints, all four outcomes, stale/request-ID/cursor no-write failures, ordered recovery, low-level advance compatibility, and new OpenSpec/Hive/hybrid continuation parity.
- `cd hivederive && go test ./... && go vet ./...` — passed.
- `cd hive-daemon && go test ./... && go vet ./...` — passed.
- `cd jarvis-cli && go test ./... && go vet ./...` — passed.
- `gofmt -w` / `gofmt -d` on changed Go tests and `git diff --check` — clean.

### Workload / Remaining

- **PR boundary:** PR6D assets, contract tests, final command regression, and audit reconciliation only; the work-unit delta remains below the hard 400 changed-line limit.
- **Deviation:** none. The OpenSpec progress record transitions to `status: complete` only after the final green test/vet evidence above.
- **Deferred parent action (unchanged):** `- [ ] Start or reuse bounded review after each selected work unit; confirm tests, rollback boundary, and total diff against the 400-line budget. <!-- sdd-owner: parent -->`.

## PR 6E — Stream-Digest Recovery Binding

- **Status / scope:** Parent-selected `issue-653-bounded-apply-progress`, token `sha256:cc9a9721fa7c7702a1b19d12ecbc41d8a8b8b52995030e0e50079adca063f078`; strict TDD; repo-local action context with the repository as the allowed edit root and no warnings. The explicit selection resolves the supplied ambient ambiguity. The verifier-owned `verify-report.md` was intentionally not edited.
- **Completed / persisted:** Implementation-owned rows **6E.1–6E.5 are visibly `[x]`** in `tasks.md`. Historical 6A.3 remains deliberately unchecked as instructed; its missing reordered-resume coverage is supplied by this corrective PR. The parent-owned bounded-review row is unchanged.
- **Implementation:** `PlanCheckpoint` canonically serializes the complete ordered `[]EvidenceEntry` stream and returns its SHA-256. A resumed cursor (`entry_index > 0`) must supply the exact digest; a changed, reordered, truncated, or extended stream fails with `ErrInvalidValue` before CLI current-state resolution, planning publication, receipt creation, or backend calls. The high-level checkpoint JSON carries `stream_sha256`; `AdvanceRequest`, low-level `advance`, storage, daemon, HTTP, MCP, and Hive client contracts are unchanged.
- **Guidance:** Canonical apply/orchestrator source assets now require retaining and supplying `stream_sha256` unchanged with the returned index and ID. Catalog contracts enforce the wording. No generated user-machine artifact changed.

### TDD Cycle Evidence

| Task | Test file / layer | Safety net | RED | GREEN | TRIANGULATE / REFACTOR |
| --- | --- | --- | --- | --- | --- |
| 6E.1–6E.3 | `hivederive/applyprogress/checkpoint_test.go` / pure unit | Focused planner suite passed. | `TestPlanCheckpointBindsOrderedStreamDigest` failed on missing stream fields; the missing-digest continuation case then failed because a resume without the digest was accepted. | Canonical ordered-stream SHA-256 binding and mandatory resumed digest made focused tests pass. | Unicode deterministic fixture, unchanged resume, missing digest, and reordered/modified/truncated/extended streams are covered. |
| 6E.1–6E.3 | `jarvis-cli/cmd/jarvis/cmd_sdd_progress_test.go` / command integration | Focused command suite passed. | Changed-continuation test failed to compile before high-level cursor fields existed. | Planner binding is passed before `checkpointCurrent`; focused command tests pass. | All changed-stream classes assert typed invalid output with zero resolve and advance calls; 162,725-rune and OpenSpec/Hive/hybrid parity continuations retain the digest. |
| 6E.4 | `jarvis-cli/internal/skills/catalog_contract_test.go` / source contract | Focused catalog suite passed. | Contract failed because both canonical sources omitted the required `stream_sha256` wording. | Both source assets now pass the contract. | Existing low-level advance/HTTP/MCP compatibility requirements remain green. |
| 6E.5 | Module integration | Focused suites passed. | Existing parity continuation test exposed the new required digest at resume. | It passes after propagating the returned field. | Full module tests/vet, formatter, and whitespace checks pass. |

### Verification

- Focused RED/GREEN: `cd hivederive && go test ./applyprogress -run '^TestPlanCheckpointBindsOrderedStreamDigest$' -count=1`; `cd jarvis-cli && go test ./cmd/jarvis -run '^TestSddProgressCheckpointRejectsChangedContinuationBeforeBackend$' -count=1`; catalog contract focused test — passed after GREEN.
- Full: `go test ./... -count=1 && go vet ./...` — passed in `hivederive`, `jarvis-cli`, `hive-daemon`, and `hive-api`.
- Formatting / whitespace: `gofmt -l` on all changed Go files and `git diff --check` — clean.
- **Workload / PR boundary:** PR6E only; high-level planner/cursor, command request/output, canonical guidance, tests, and audit records. The implementation/test delta remains below the hard 400-line budget. No deviation from the approved recovery-binding design.

### Remaining / Deferred

- Historical unchecked implementation line preserved verbatim: `- [ ] 6A.3 **TRIANGULATE/REFACTOR (incomplete):** The recorded PR 6A tests cover malformed cursor pairs but not the required reordered-resume rejection; final verification proved a reordered stream can pass when \`entry_index\`/\`entry_id\` still match. Preserve this incomplete historical task and complete its missing behavior through the bounded PR 6E stream-digest correction below; do not reclassify PR 6A–6D as discarded. <!-- sdd-owner: implementation -->`
- Deferred parent lifecycle action, unchanged: `- [ ] Start or reuse bounded review after each selected work unit; confirm tests, rollback boundary, and total diff against the 400-line budget. <!-- sdd-owner: parent -->`.
