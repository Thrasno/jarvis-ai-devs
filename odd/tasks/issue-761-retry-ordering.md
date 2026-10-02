# Fix issue #761 client-local retry ordering

## Objective and scope
Correct the flaky OpenCode Hive lifecycle harness without changing product behavior. Server handler observation timestamps cannot establish client cancellation causality. Preserve native abort, settlement-before-created-retry, duplicate suppression, distinct evidence, payloads and prompt independence. Deletion checks must not invent product coalescing.

Target: isolated worktree `jarvis-dev-issue-761-retry-ordering`, branch `fix/issue-761-retry-ordering`, base `dec01413`.
Allowed source edits: `jarvis-cli/internal/agent/opencode_lifecycle_test.go` only. Product template, generated configuration and onboarding remain untouched. User authorized commit, push and a separate PR. No builds, merge or direct issue closure.

## Tasks
- [x] T1 (implemented and Linux checks passed): Add deterministic adverse scheduling regression, observe RED, implement client-local abort/settlement instrumentation and negative controls, observe GREEN. Route: delegated worker; test fixture complexity and verification trigger.
- [x] T2 (Linux independent verification passed): Run focused lifecycle repetitions, full CLI tests and vet; independently inspect guarantees. Route: verifier. Windows evidence remains pending unless actually exercised.
- [x] T3 (native review approved and acknowledged): Run enabled native review over exact candidate, reconcile findings and report verified outcome. Delivery/commit pending human authorization.

## Acceptance and verification
- Client abort and rejected settlement precede matching created retry.
- Adverse server scheduling does not trigger false ordering failure.
- Live-signal, unsettled-fetch and non-abort-failure negative controls fail correctly.
- Native timeout/cancellation and existing lifecycle assertions preserved; no longer sleeps or timeouts.
- `go test ./internal/agent -run '^TestOpenCodeHiveTemplate_' -count=1 -v`
- Repeated focused created/deleted tests, `-count=30 -v`; full CLI `go test ./... -count=1`; `go vet ./...`.
- Confirm integration tests not skipped (Node present).

## Evidence and next step
Remote run 36969481755 Windows job 110720246067 records ~2.02ms observation inversion after #771. Source mapping confirms server scheduling ambiguity. Worker observed RED (84µs forced observation inversion), GREEN and four observer controls. Linux lifecycle suite passed without skips, 60 repetitions passed, full CLI tests/vet/diff checks passed. Diff: 153 insertions, 9 deletions. Windows remains pending.
Estimated change: one source test file, roughly 150–300 diff lines plus this tracking document; reassess if larger. Delivery strategy: ask-on-risk. Work-unit commit: `423c799aa42b6af4c28b638511b04468de082255`.
Native review: medium, one reliability lens, lineage `review-6392aab7a5d01fec`, approved and exact acknowledgement completed; authority burned. Candidate included only the source test file, tracking document excluded. Reviewed source committed as `423c799aa42b6af4c28b638511b04468de082255` after explicit user authorization.
Independent verifier `muqxf09n-4-xjpg` completed: regression/controls repeated 3 times, 20 created/deleted lifecycle executions, full CLI tests, vet and diff checks passed; targeted Node tests ran without skips. No blocking inspection findings. Windows remains pending. Observer adds a promise reaction; no race/coverage or other-module checks run. No source changes during verification.
- [ ] T4 (in progress): Publish draft PR and verify Windows CI. User authorized commit/push/PR; repo template requires Closes #761, effective only on merge. No merge authorized.
Next: publish branch and draft PR; Windows CI pending.
