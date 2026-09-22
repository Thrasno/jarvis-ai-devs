# Issue 724 — Change-Level SDD Supersession

## Objective

On authorized task replanning, seal the old protected stream under its original manifest, then create one new change with a fresh stream under the new manifest. Preserve every historical head, batch and receipt without transferring task credit. GitHub contract: https://github.com/Thrasno/jarvis-ai-devs/issues/724 (`status:approved`).

## Problem and why

The task manifest is stream identity. A mismatch currently blocks progress but offers no supported low-friction recovery. Changing the manifest in place, adopting an existing successor, or inferring that zero credited tasks means zero immutable evidence would compromise authority. The ordered two-change protocol must tolerate crashes without pretending to be a distributed transaction.

## Scope and constraints

- One terminal `superseded` seal in the predecessor, with its old manifest, no new evidence/coverage and signed frozen successor intent. New successor has no head, starts at 1/1 and signs `supersedes`.
- The single-change shared seal-shape predicate lives in `hivederive/applyprogress`; store adapters branch only on that exact form, validate the published old head and referenced immutable evidence, and do not reconstruct old tasks. Ordinary advances remain strict.
- Cross-change link integrity, derived `superseded_pending_successor`, routing and diagnostics belong to SDD status/continue, not the single-change model.
- No transferred completion credit. Zero credited tasks permits automatic handoff after all safety checks; nonzero requires explicit consent before sealing; a decline changes neither change.
- New successor name must be unoccupied. Intent frozen in seal includes target manifest, actor, reason, timestamp and stable operation/request identity so retries reconstruct exact input.
- Existing per-change OpenSpec/Hive/hybrid binding, CAS and receipts own publication. No cross-change transaction, existing-change adoption, Git identity promotion or `none`-mode protected stream.
- Versioned snapshot schema preserves historical v2 bytes/digests and historical continuation decode. Generated user-machine configuration is never edited; source templates only.

## Delivery and checks

- Branch: `feat/issue-724-change-supersession` from `master`/`public/master` at `2ca854c84040c593e9232beac72305c2a6a23d27`.
- Forecast: 1,740–3,310 authored diff lines (730–1,350 production Go, 840–1,560 tests, 170–400 docs), approximately 17–27 files. Addition + deletion both count; the per-task 400-line target is advisory, not a reason to omit tests or artificially split a behavior.
- Delivery strategy: `ask-on-risk`, resolved by the maintainer to `feature-branch-chain`. Plan one tracker plus independently reviewable child slices; push/PR/merge remain separate user decisions. Never create a single oversized PR by default.
- TDD: **on**, source: `AGENTS.md` strict TDD for SDD implementation and `openspec/config.yaml` `strict_tdd: true`; exact focused runner: `go test -count=1 ./applyprogress` from `hivederive` for model, `go test -count=1 ./internal/sddprogress ./internal/sddstatus ./cmd/jarvis` from `jarvis-cli` for integration, and `go test -count=1 ./internal/db` from `hive-daemon` for daemon. Record observed RED, GREEN and refactor checks per task; no builds without explicit request. Final applicable checks: `go test -count=1 ./...` and `go vet ./...` separately in each affected Go module.
- All tasks route through a bounded `gentle-ai-worker` for 2+ non-trivial files. Workers may edit only the named surfaces, run authorized focused tests, and never commit/push/approve. Parent keeps task file, memory mirror, review, and work-unit commits.

## Actionable checklist

- [x] **ODD-724-01 — Versioned seal and successor model.** Shared snapshot schema compatibility, strict old-v2 decoder/digest preservation, terminal `superseded`, signed seal intent and `supersedes`, shared seal-shape predicate, `ValidateSuccessor` transition and negative cases. Focused `hivederive` package RED/GREEN and vet; include regressions for legacy explicit-zero and continuation. Candidate work-unit commit: `feat(sdd): model change supersession in protected progress`.
- [x] **ODD-724-02 — OpenSpec exact-seal publication.** Branch manifest/candidate/current validation only on shared seal shape, validating signed previous head and immutable referenced batches while bypassing current tasks solely for seal. Preserve receipt replay and ordinary strictness; cover changed tasks, continuation-bound head, concurrent writer, corrupt evidence and retries. Focused CLI `internal/sddprogress` RED/GREEN. Commit with tests.
- [ ] **ODD-724-03A — Hive daemon exact-seal publication.** Branch daemon authoritative-task validation only on shared seal predicate using the signed current head and immutable referenced batches inside the existing transaction. Preserve CAS/receipt identity and ordinary strictness; test changed Hive tasks, existing batch corruption, competing writer, exact replay and terminal read. Focused daemon DB RED/GREEN, commit with tests.
- [ ] **ODD-724-03B — CLI and hybrid seal routing.** Keep normal CLI checkpoint tasks strict; route dedicated seal without its current-tasks guard, with exact hybrid partial-publication recovery and equality of both heads. Test changed tasks, divergent sides, interrupted side, exact request replay and invalid current evidence. Commit with tests.
- [ ] **ODD-724-04 — Fresh successor and cross-change status.** Resolve new unoccupied name; create genesis 1/1 from signed seal intent, validate frozen new tasks, refuse adoption, verify signed pointer and old seal across changes, expose derived pending/mismatch states and exact retry routing. Test interruption between seal and creation, payload conflict and same-project identity. Commit with tests.
- [ ] **ODD-724-05 — Consent, archive and guidance.** Zero/nonzero credited task UX, decline without mutation, old archive rejection, source-of-truth embedded guidance and user docs, command/status integration regressions across stores and historical snapshots. Complete relevant module test/vet checks, record failures/skips, work-unit commit and review boundaries; do not publish PRs without user authorization.

## Acceptance criteria

The bilingual approved issue body is normative. In particular: old immutable evidence survives, `complete` or previously `superseded` heads cannot be sealed, no old tasks are reconstructed, all three bound stores accept only the exact seal, no successor is adopted, a crash leaves visible derived pending status recoverable by identical retry, credit never transfers, archive cannot treat superseded as complete, old v2 digests remain valid, and promotion of Git project identity is out of scope.

## Progress

- [x] Read-only code/issue research and line forecast completed; issue approved by maintainer.
- [x] Feature branch created from clean `master`; delivery chain strategy chosen explicitly.
- [x] ODD-724-01 complete: snapshot v3 model, exact seal predicate, signed intent and pointer, historical v2 compatibility. Delegated writer observed RED then GREEN; independent verifier found and corrected a generation-zero historical-root seal defect; subsequent uncached focused tests, vet and diff checks passed. Work-unit commit `714b9ee4008ee02e75b54343ff279829f9eb6795` (408 authored diff lines, including this plan's initial version). Native review lineage `review-becc0cb1d29ffd85` approved and acknowledged with burned authority.
- [x] ODD-724-02 complete: OpenSpec exact seal after task edit, prior signed head integrity, historical receiptless-head preservation, exact receipt retry and fail-closed staged recovery. Independent verifier initially found two receiptless authority gaps; both corrected, followed by PASS. Work-unit commit `3483eea8d44da237aa3140266026cfcbd0522f7c` (225 authored diff lines). Native review lineage `review-33b18d39b59a1fe2` approved and acknowledged with burned authority.
- [ ] ODD-724-03A in progress next. The original cross-module unit was split into independently verifiable daemon and CLI/hybrid slices after a read-only seam map; no extra product scope.

## Verification evidence and work-unit commits

WU-01: `cd hivederive && go test -count=1 ./applyprogress` PASS; `cd hivederive && go vet ./applyprogress` PASS; `git diff --check` PASS; independent read-only verifier PASS after correcting its first FAIL. Native ASSESS over committed range returned `unassessable` (`schema-incompatible`); committed-range inspect/start/reviewer completed and acknowledgement burned as `review-becc0cb1d29ffd85`. Work-unit commit: `714b9ee4008ee02e75b54343ff279829f9eb6795` (368 additions, 40 deletions).
WU-02: `cd jarvis-cli && go test -count=1 ./internal/sddprogress` PASS; `cd jarvis-cli && go vet ./internal/sddprogress` PASS; `git diff --check` PASS; independent read-only verifier PASS after two receiptless-head corrections. Native ASSESS over committed range returned `unassessable` (`schema-incompatible`); committed-range review `review-33b18d39b59a1fe2` approved and acknowledged. Commit `3483eea8d44da237aa3140266026cfcbd0522f7c` (220 additions, 5 deletions). Broader module tests and remaining work units pending.

## Next step

Dispatch ODD-724-03A for Hive daemon exact-seal publication under strict TDD after synchronizing local task file, Engram mirror and visible todo. WU-01 and WU-02 reviewed commits remain unchanged.
