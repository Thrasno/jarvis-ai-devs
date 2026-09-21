# Issue 723 — Persistent SDD Artifact Store Binding

## Objective

Persist one authoritative artifact-store binding per project/change so every SDD lifecycle command reads and writes the same backend across processes, restarts, legacy changes, and hybrid operation.

## Problem

The current lifecycle resolves storage independently per command: status/continue default to Hive while protected progress defaults to OpenSpec. A change can therefore be read from one authority and written to another without an explicit user decision. Repeating `JARVIS_SDD_STORE_MODE` is not a durable contract.

## Why

A stable binding prevents accidental split authority, misleading `not_found` diagnostics, and unsafe recovery. It also gives later supersession work (#724) an explicit store contract without coupling this change to workspace identity promotion (#722).

## Scope

- Persist an immutable, idempotent binding keyed by canonical project and change.
- Support Hive, OpenSpec, and hybrid bindings with provenance.
- Adopt legacy changes safely after read-only inspection.
- Make every lifecycle reader/writer consume the persisted binding.
- Report binding mode and provenance in status/diagnostics.
- Preserve strict hybrid equality and fail closed on divergence.
- Keep `JARVIS_SDD_STORE_MODE` only for initial selection, diagnostics, or explicit administrative override.

## Constraints and non-goals

- Do not implement workspace identity promotion or redirects from #722.
- Do not rewrite protected progress, receipts, manifests, or evidence.
- Do not implement supersession from #724.
- Do not conflate artifact storage with Hive memory sync or `jarvis sync` configuration replay.
- Do not modify the concurrent #722 worktree or its branch.
- Generated user-machine configuration is never edited directly.
- Binding persistence must preserve unrelated OpenSpec `state.yaml` fields and existing Hive data.
- Windows read-only replacement must use one atomic rename operation that ignores the destination read-only attribute; it must not clear the destination attribute before the commit point.

## Product decisions

- Binding placement is backend-owned: Hive stores its binding in SQLite, OpenSpec stores it in the change-local `state.yaml`, and hybrid persists both copies and requires exact equality.
- `none` is not persisted because no protected write exists to bind; a later write must select a real mode.
- Legacy equality fails closed and reuses the strictest existing normalized protected-state equality; divergent state requires explicit reconciliation.
- A Hive-bound archive is a logical lifecycle closure and must not require a local OpenSpec move.
- TDD mode is **on** by explicit user choice: every work unit must record observed RED, GREEN, and refactor evidence using its focused Go test command.

## Delivery strategy

- Forecast: approximately 1,000–1,500 authored diff lines across five cohesive work units.
- Delivery strategy: `ask-on-risk`, resolved before implementation.
- Chain strategy: `feature-branch-chain` by explicit user choice; use a draft/no-merge tracker and chained child PRs when delivery begins.
- Review budget: approximately 400 authored changed lines per slice; tests and docs remain with each behavior.

## Actionable checklist

- [x] **ODD-723-01 — Persist the OpenSpec binding contract**
  - Route: delegated direct writer; multi-file write trigger.
  - Add the immutable/idempotent binding representation, validation, and provenance plus lossless atomic persistence in change-local OpenSpec `state.yaml`.
  - Preserve unknown DAG-state fields, reject `none`, allow exact replay, and reject conflicting rebinding.
  - Correction required after independent verification: reject YAML merge/duplicate-key ambiguity, avoid replay/error lock mutations, make containment race-safe, preserve existing permissions, eliminate post-commit ordinary errors, provide functional Windows parity, and reject special files before blocking opens.
  - Focused check: `go test ./internal/sddbinding` plus `go test ./internal/sddruntime ./internal/sddbinding`.

- [x] **ODD-723-02 — Persist and expose the Hive binding**
  - Route: delegated direct writer; multi-file write trigger.
  - Add SQLite persistence plus daemon/client read and atomic adopt-if-absent operations without silent overwrite.
  - Coordinate the migration-list insertion in `hive-daemon/internal/db/db.go` with #722 before editing it; `jarvis-cli/internal/hiveclient/client.go` is currently stable.
  - Focused checks: DB, daemon governance/http, and CLI hiveclient binding tests.

- [x] **ODD-723-03 — Resolve and adopt legacy changes**
  - Route: delegated direct writer; multi-file write trigger.
  - Inspect Hive and OpenSpec read-only, adopt zero/one/equivalent state, and block divergence.
  - Surface binding provenance in machine and human status output.
  - Focused checks: `jarvis-cli/internal/sddstatus` and `cmd/jarvis` binding/legacy tests.

- [x] **ODD-723-04 — Route protected writes and archive through the binding**
  - Route: delegated direct writer; multi-file write trigger.
  - Slice 04-A: make progress checkpoint/advance/upgrade consume the binding; keep the existing hybrid progress receipt and fail closed on divergent backends.
  - Slice 04-B: treat executor-authored `archive-report` as the Hive logical-closure signal; add no typed closure state and no archive receipt.
  - OpenSpec archive keeps the existing validated rename; hybrid requires equivalent reports in both stores before that rename.
  - Prevent environment changes from silently switching a bound change; never choose an arbitrary hybrid copy when protected state diverges.
  - Focused checks: command-level progress and archive tests.

- [ ] **ODD-723-05 — Update diagnostics, guidance, and regression coverage**
  - Route: delegated direct writer; multi-file write trigger.
  - Update embedded source-of-truth contracts and diagnostics.
  - Cover process restart, overrides, equivalent/divergent legacy state, and proof that no second store is written accidentally.
  - Focused checks: runtime/config/agent integration tests and relevant generated-output validators.

- [ ] **ODD-723-06 — Final verification and delivery preparation**
  - Route: verification delegated according to native assessment and RDD state.
  - Run focused checks for every completed unit, then `go test ./...` and `go vet ./...`.
  - Record work-unit commits, assessed risk/outcome, authored line counts, and PR slice boundaries.

## Acceptance criteria

- Every SDD lifecycle command resolves the same persisted store for a bound change.
- A bound change never writes to a second store because an environment variable or command default changed.
- Legacy zero/one/equivalent states are adopted deterministically; divergence fails closed.
- Hybrid remains equality-validated and cannot silently degrade to one side.
- Status reports the binding and its provenance.
- Process restarts preserve the decision.
- Archive respects Hive, OpenSpec, and hybrid semantics without conflating them.
- Existing protected evidence remains immutable.

## Progress

- GitHub issue #723 was explicitly approved and carries `status:approved`.
- Isolated worktree created at `../jarvis-dev-issue-723` on `feat/issue-723-sdd-store-binding` from `public/master` at `a315902a`.
- Read-only mapping completed; no source files or tests have been changed.
- Coordination boundary shared with the #722 agent: #723 owns store binding/routing; #722 owns identity promotion/redirects; shared Hive schema/DTO work requires coordination.
- User selected backend-owned persistence, strict TDD, and a feature-branch PR chain.
- #722 released `hive-daemon/internal/db/db.go`; this branch is rebased through its stable technical boundary `2e361374`. ODD-723-02 DB persistence is in progress under strict TDD.
- Identity-promotion ownership is coordinated: #723 exposes immutable binding values and insert-if-absent persistence only; after integrating the #723 commit, #722 will add atomic move/converge/fail-closed behavior to its promotion transaction and `ProjectKeyedStates()`.
- The ODD-723-02 DB slice implements fresh/migration schema, exact read, atomic first-writer adoption, replay, typed conflict, real upgrade coverage, and divergent concurrency. SQLite and Go reject the same complete 25-rune whitespace set; final independent and four-lens native review passed, and the slice was committed as `40003775` (`feat(hive): persist SDD store bindings`).
- ODD-723-01 completed its initial RED/GREEN/refactor cycle and seven correction passes, passed final independent and native review, and was committed as `4d69d068` (`feat(sdd): persist OpenSpec store binding`) after rebasing onto the stable #722 boundary.
- The work-unit implementation is 1,379 added lines across nine package files. This exceeds the preferred review-slice budget because the cohesive contract includes strict YAML validation, rooted atomic persistence, Unix/Windows parity, and their regressions; native review assessed the complete frozen 1,516-line candidate including this task record.

## Verification evidence

- Current default mismatch observed at `jarvis-cli/cmd/jarvis/cmd_sdd.go` and `jarvis-cli/cmd/jarvis/cmd_sdd_progress.go`.
- Existing hybrid behavior fails closed on unequal v2 snapshots.
- ODD-723-01 RED: `go test ./internal/sddbinding` failed on undefined production symbols after tests were added.
- ODD-723-01 GREEN/refactor: `go test ./internal/sddbinding` passed after implementation and gofmt.
- Independent commands passed: `go test ./internal/sddbinding`, `go test ./internal/sddruntime ./internal/sddbinding`, and scoped `git diff --check`.
- First independent semantic verification failed on five blockers: YAML merge-key authority bypass, persistent lock mutation on replay/rejection, path-replacement races, post-rename error ambiguity, and permission replacement.
- The correction pass resolved those five Unix-path behaviors and kept focused tests green.
- Second independent verification still failed: Windows unconditionally returned `ErrUnsafePath`; Unix opened a FIFO `state.yaml` in blocking mode before type validation. It also identified equivalent numeric YAML keys as an ambiguity gap.
- The verifier's initial second-pass command ran from the repository root and found no `go.mod`; the authorized rerun with the correct `jarvis-cli` cwd passed and no repository mutation was observed.
- The second correction pass added functional Windows rooted access with a named mutex, descriptor-relative nonblocking Unix validation, canonical YAML scalar-key comparison, and host-runnable regressions for FIFO and equivalent numeric keys.
- Post-correction focused tests and scoped diff-check pass. Windows runtime/build evidence remains unavailable because repository policy has not authorized builds or cross-compilation.
- Third independent verification found five deterministic blockers: `CreateMutex` mishandled `ERROR_ALREADY_EXISTS` and leaked its valid handle; thread-owned mutex release was not pinned to one OS thread; lock names used path/session identity instead of physical directory identity; `0.0` and `-0.0` remained distinct canonical YAML keys; Unix rejected ordinary workspaces beneath symlinked ancestors.
- The third correction pass now accepts existing Windows mutex handles, pins ownership through release, derives a `Global\\` mutex name from physical volume/file identity, normalizes signed-zero/NaN/infinite YAML keys, and resolves symlink ancestors before descriptor-rooted traversal. Focused tests and diff-check pass.
- Fourth independent verification confirmed the prior blockers resolved but found one deterministic test-portability defect: shared permission assertions expected Unix `0640`/`0600`, while Windows reports writable regular files with Windows-specific mode semantics.
- The fourth correction pass split permission expectations by platform: Unix still requires exact `0640` preservation and `0600` creation; Windows requires regular writable state files without assuming POSIX bit patterns. Focused host tests and diff-check pass; no Windows build/runtime evidence was produced.
- Final reverification confirmed the implementation blockers remain resolved but found that all shared permission cases were writable; no test proved that adopting into an existing Windows read-only `state.yaml` preserved read-only status.
- The fifth correction pass added shared `0444` preservation coverage and platform-specific assertions. Its initial Windows workaround clears the destination read-only bit immediately before rename and restores it only on normal failure, leaving a deterministic process-crash window that violates pre-commit immutability.
- Microsoft documents `FileRenameInfoEx` with `FILE_RENAME_REPLACE_IF_EXISTS | FILE_RENAME_POSIX_SEMANTICS | FILE_RENAME_IGNORE_READONLY_ATTRIBUTE` for atomic replacement of a read-only target. This is the required Windows correction path; ordinary `MoveFileEx`/attribute toggling is insufficient.
- The sixth correction pass stages through rooted `NtCreateFile` with `DELETE` access and commits through `SetFileInformationByHandle(FileRenameInfoEx)` with all three required flags. Source-contract tests assert the flags and absence of destination `Chmod`; host tests and diff-check pass.
- Independent source reverification confirmed the Win32 structure/access/flags and cleanup semantics, but found that `lock_windows.go` returned an ordinary error when the staged handle `Close` failed after the rename had already committed.
- The seventh correction pass makes post-commit staged-handle close best-effort and adds a source-contract regression. Final independent reverification passed with no remaining deterministic findings.
- Authorized Windows-target compilation passed with `GOOS=windows GOARCH=amd64 go test -c -o /tmp/jarvis-sddbinding-windows-amd64.test.exe ./internal/sddbinding`; the temporary artifact was removed. Windows runtime behavior remains unexecuted.
- LSP diagnostics reported no findings across the host-platform package files. Explicit no-index whitespace checks covered all previously untracked files.
- Native review lineage `review-623f0f936cff00a5` approved and was acknowledged for the frozen candidate. The subsequent read-only assessment was unavailable natively but confirmed RDD-on handling from the closed review outcome.
- ODD-723-02 DB RED: focused tests initially failed on missing binding symbols/table. GREEN/refactor: focused binding tests and `go test ./internal/db` pass with atomic independent-handle concurrency.
- First DB verification passed the transaction/API semantics but requested divergent concurrency, true pre-binding upgrade, identity-read nonmutation, and direct schema constraints; those tests were added and pass.
- DB reverification found that SQLite constraints trimming only space/tab/LF/CR admitted whitespace-only U+00A0/U+2003, vertical-tab, form-feed, and other Go whitespace. The correction encodes all 25 `strings.TrimSpace` runes in both schema declarations and proves 100 guarded-column rejection cases; final reverification passed with no deterministic findings.
- Native DB review lineage `review-297bad791c44363e` ran risk, resilience, readability, and reliability lenses over the 606-line frozen candidate, approved, and was acknowledged. The DB implementation commit contains 597 added lines across schema, repository, and tests; the cohesive SQLite authority contract exceeds the preferred slice size but was reviewed as one security boundary.
- Commit `40003775` was handed to #722 for its separately owned `ProjectKeyedStates()` and atomic identity-promotion integration.
- The ODD-723-02 governance/HTTP slice exposes exact GET/adopt routes, typed status envelopes, pre-artifact adoption, bounded strict JSON, sanitized unavailable logs, and fail-closed timestamp projection without adding client or lifecycle routing. Final independent and native review passed; it was committed as `47093c48` (`feat(hive): expose SDD store bindings`).
- The bounded Hive client slice exposes strict GET/adopt methods, a conflict sentinel and typed exact-value conflict, 64-KiB response limits, exact-key decoding at every envelope level, future-schema reads, status/value coherence, and authenticated conflict projections without adding legacy or lifecycle routing. It was originally committed as `41820fce` and rebased as `85fa851b` (`feat(cli): add SDD store binding client`).
- ODD-723-03 began with a read-only protected-progress observation/equality boundary in `a5c90b12` (`feat(sdd): compare legacy protected progress`). Planning artifacts do not select legacy authority; outages remain errors; blocked or noncanonical JSON-like progress cannot prove equality; canonical v2 uses normalized snapshot equality; legacy progress requires exact nonblank state/content equality.
- The legacy binding state machine is committed as `028925c8` (`feat(sdd): adopt legacy store bindings`). It resolves persisted copies before inspection, rejects future/misplaced/divergent copies, adopts the zero/one/equivalent progress matrix, keeps `none` unpersisted, repairs only proven-equal hybrid mirrors, validates every adapter response, and preserves backend-first partial-adoption recovery without rollback.
- Governance/HTTP RED was initially missing symbols/routes. The first GREEN passed focused and package tests; independent verification then proved standard JSON decoding accepted duplicate/case-aliased authority keys and duplicate project queries, and emitted zero timestamps. Adversarial RED reproduced all cases; token-level exact-key/string-value decoding, single-query enforcement, sanitized logs, and zero-timestamp rejection restored GREEN.
- Final governance/HTTP reverification passed with no remaining findings. Native review lineage `review-d1f853c5817099db` approved and was acknowledged over the 449-line frozen candidate; the implementation commit contains 445 added lines across service, HTTP, and tests.
- Hive client RED first failed on missing public symbols. GREEN/refactor passed `go test ./internal/hiveclient -run 'SDDStoreBinding'`, `go test ./internal/hiveclient`, and scoped `git diff --check` after adding strict bounded decoding.
- Parent readback found that a syntactically valid 409 could misrepresent the submitted immutable request. An adversarial RED covered future schema, mode/provenance mismatch, and equivalent existing/requested projections; the correction authenticates the requested projection against the normalized submission and rejects non-divergent conflicts.
- Independent verification passed the client transport and authority boundary. Native review lineage `review-5c0b2c43ec11fd3c` approved and was acknowledged over the complete 620-line frozen candidate. This cohesive security boundary exceeds the preferred slice size because the client must validate every success, absence, error, and conflict envelope before lifecycle code may trust it.
- The coordinated rebase onto verified #722 HEAD `156dbf8f` retained the equivalent Hive DB commit `e2deb60f` and excluded old `40003775`. Because the requested range also excluded the earlier OpenSpec source, coordination confirmed restoring source-only `4d69d068` as `a4c3ad16`; the later ODD snapshot already retained its evidence. Focused sddbinding/sddruntime/hiveclient and DB/governance/httpapi tests passed after repair.
- Legacy progress observation RED began on missing APIs. Independent verification then found whitespace-only/blank legacy evidence and JSON-escaped v2 lookalikes could be misclassified as equal; adversarial RED reproduced each case. The correction requires nonblank state/content and rejects all noncanonical JSON-object-looking progress before legacy comparison. Focused/package tests and diff-check pass; native review lineage `review-2630e4751e254979` approved and was acknowledged over the 148-line candidate.
- Legacy resolver RED began on missing state-machine APIs. Independent verification found two high-risk gaps: one-sided blocked progress could select authority, and concurrent Hive/OpenSpec initial selections could each commit to a different backend. Adversarial RED reproduced both. The correction requires every present observation to prove self-equivalence and serializes the complete persisted-read/inspect/adopt transaction with the existing physical OpenSpec directory lock; shared locked adoption avoids recursive locking. Uncached focused/full package tests and explicit untracked whitespace checks pass. Native review lineage `review-ae0946c787bc8b3d` approved and was acknowledged over the complete 867-line candidate; its one reliability advisory about the lock boundary was informational and did not open correction.
- The state-machine slice exceeds the preferred review budget because its 865-line cohesive authority boundary includes the full persisted-copy matrix, legacy selection matrix, partial-write recovery, malformed-adapter validation, and deterministic concurrency regression. Same-directory processes serialize; production multi-process Windows execution and separate-clone/shared-coordinate races remain unexecuted risks.
- Follow-up `d817c089` (`fix(sdd): serialize binding resolution safely`) moved production serialization to an explicit project-root lock, preserved Hive-only resolution when no OpenSpec change directory exists, rejected dangling path components and physical directory aliases, and kept adoption rooted on the checked directory. Adversarial RED covered missing directories, final and ancestor symlinks, physical aliases, and same-directory recursion. Independent reverification passed; native review lineage `review-8185edeb244dc066` approved and was acknowledged. Authorized Windows target compilation passed; Windows runtime locking remains unexecuted.
- Status/continue integration is committed as `59049993` (`feat(sdd): route status through store bindings`). Both commands resolve/adopt before selecting a source; a persisted binding wins over the current environment; explicit coordinates are normalized once; inference strictly unions, deduplicates, and orders Hive/OpenSpec names while propagating either backend error; and JSON/human status surfaces binding mode, provenance, and persistence separately from the compatibility `artifactStore` field. Tests cover Hive, OpenSpec, hybrid, unpersisted none, missing OpenSpec directories, strict HTTP coordinates/bodies, human output, and defensive status copying.
- The first integration verification found inconsistent spaced coordinates and permissive/partial integration tests. Delegated adversarial RED reproduced the coordinate mismatch; normalization before path, resolver, return, and read restored GREEN. Final independent reverification passed `go test -count=1 ./cmd/jarvis ./internal/sddstatus`, focused binding/status/continue tests, and scoped diff-check. Native review lineage `review-9e41d270ecfca63f` approved and was acknowledged over the complete four-file candidate.
- Lock cancellation is closed by `543adfe5` (`fix(sdd): bound binding lock waits`). Resolution now uses context-aware nonblocking polling with bounded backoff on Unix and the existing physical named mutex on Windows, without internal goroutines or delayed writes. It rechecks cancellation after acquisition, releases before returning on the cancellation race, preserves the public non-contextual OpenSpec lock, and keeps unlock idempotent. Adversarial verification removed a mutable retry seam and corrected the shared contention test for recursive Windows mutex ownership by keeping acquisition/release on a dedicated test goroutine. Focused/package/race tests, twenty repeated contention runs, literal Windows cross-compilation with artifact cleanup, LSP, and diff-check passed. Native review lineage `review-5c3720a086dd5da3` approved and was acknowledged. Windows runtime behavior remains unexecuted.
- ODD-723-04A is committed as `dbe4a49e` (`feat(sdd): route progress through store bindings`). Advance, checkpoint, and continuation upgrade now decode their request before resolving the authoritative binding and selecting exactly one bound writer; persisted bindings override valid or invalid environment changes, while `none` remains unpersisted and disabled. The shared resolver enforces canonical project/change/root coordinates, supports rootless pure-Hive operation without creating OpenSpec, preserves the existing hybrid receipt/recovery protocol, and propagates the Cobra context through every production Hive read/write. Independent verification drove fixes for project-key mismatch, whitespace roots, writer cancellation, and a hybrid test that originally stopped during reconciliation instead of reaching the writer. Focused/full command tests, race, twenty cancellation repetitions, LSP, and scoped whitespace checks passed. Native review lineage `review-2633c5cec6d04cf3` approved and was acknowledged; its timing advisory is informational.
- ODD-723-04B is committed as `8e3757c1` (`feat(sdd): route archive through store bindings`). The production archive command resolves/adopts the binding before dispatch. Hive treats a complete executor-authored `archive-report` as idempotent logical closure with no OpenSpec mutation or lifecycle POST; OpenSpec retains its validated rename and in-lock revalidation; hybrid captures one strict Hive view and one OpenSpec view per validation, requires both backends individually ready, exact nonblank reports, and equivalent protected progress, then repeats the same checks under the local archive lock before rename. No closure API/state, archive receipt, rollback, or backend winner was added.
- Archive verification first found an unpaired third hybrid read, path spellings normalized instead of rejected, and weak no-mutation evidence. Delegated RED removed the unpaired read, rejected any root spelling changed by `Abs/Clean`, distinguished permitted one-time binding adoption from forbidden lifecycle writes, snapshot-tested Hive no-op filesystem behavior, and proved a report mutation between prevalidation and the locked callback leaves source intact. Focused/full command tests, race, LSP, and diff-check passed. Native review lineage `review-88dc2a8124375d20` approved and was acknowledged over the complete three-file candidate.
- Deliberate residual trade-off: the local OpenSpec lock cannot freeze generic Hive artifact writes after the final compared fetch. The user explicitly chose report-based closure without distributed closure state or an archive receipt; the implementation fails closed on every observed mismatch but does not claim distributed atomicity.

## Next step

Begin ODD-723-05 diagnostics, source-of-truth guidance, and regression coverage. Keep #722 identity/purge ownership and #724 supersession guards out of the slice.
