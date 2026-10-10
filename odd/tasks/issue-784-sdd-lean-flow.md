# Issue 784 — SDD lean flow

## Objective and rationale
Cut SDD procedural overhead (tool calls, repeated questions, prompt-enforced ceremony) without losing safety. The developer decides how SDD runs in a single preflight; decisions stay for the whole feature; rules move from prompts into Go.

Issue: https://github.com/Thrasno/jarvis-ai-devs/issues/784 (approved, type:feature).
Related: #781 operator handoff (approved), lands before the adaptive verify slice.

## Scope and constraints
- Sources of truth only: `jarvis-cli/embed/**`, Go packages, contract tests. Never edit generated agent files.
- Phase order unchanged and sequential: init → preflight → explore → propose → spec ∥ design → tasks → apply → verify → archive.
- Question round always offered (interactive and automatic); skipped on "no" unless scope is unusable.
- Preflight decides mode, store, TDD mode, size policy (budget or sticky unlimited `size:exception`), chain strategy for the whole feature.
- Keep guarded checkpoints and store binding. No Hive schema change.
- Delivery: one chained PR per slice, stacked to main. No local builds.

## Tasks (slices)
- [x] S1: Delivery decision gate — resolve only from whole single-valued lines (`Chain strategy: <value>` or `Decision needed before apply: No`); template uses non-matching `<one of: ...>` placeholders (fixes substring bypass).
- [x] S2: Preflight as the single decision point (TDD mode, size policy, chain strategy); tasks and orchestrator stop re-asking; sticky size exception. Also close S1 advisory: an unfilled `Decision needed before apply: <one of: Yes, No>` placeholder leaves Required=false (gate fails open; pre-existing) and the verbatim-template test injects a literal Yes.
- [x] S3: TDD detection only suggests (real test command required; Deluge suggests standard; one-line reason); apply consumes forwarded mode.
- [x] S4: Question round single owner (orchestrator), offered in both modes, forced only for unusable scope.
- [x] S5a: Checkpoint CLI derives request id and stream digest; returns a `next` action.
- [ ] S5b: Checkpoint CLI accepts a compact per-task strict-TDD record and expands it into evidence entries.
- [ ] S6: Trim apply prompts (one checkpoint per batch, follow `next`, compact TDD).
- [ ] S7: #781 operator handoff (EvidenceOperator kind + tasks/apply/verify skills).
- [ ] S8: Adaptive verify (stop early on pending tasks; run detected commands once; static review without runner; operator-attested).
- [ ] S9a: Spec-merge engine in Go.
- [ ] S9b: `jarvis sdd archive` performs spec sync; trim archive skill.
- [ ] S10: Validate authority once in the orchestrator; executors trust forwarded status.
- [ ] S11: Before/after measurement on the same small real change (user-run; records time, tool calls, test runs, user messages).

## Acceptance criteria and checks
- Per slice: observed RED then GREEN for Go behavior; contract tests updated for prompt changes; `cd jarvis-cli && go test ./... && go vet ./...` (plus `hivederive` when touched).
- Ceremony budget from the issue holds after S10.

## Evidence
### S1
- Worker: RED `go test ./internal/sddstatus/... -run TestApplyDecisionGate` failed (11 subtests + template-copy test), then GREEN.
- Verifier: `cd jarvis-cli && go test ./... -count=1` 30 packages ok; `go vet ./...` clean; gofmt and `git diff --check` clean.
- Commit d09c487a `fix(sdd): resolve the apply delivery gate only from single-valued decision lines`.
- Native review review-a0314eb79b65bc1b (medium, reliability lens) approved; acknowledgement burned authority. Advisory findings carried to S2.

### S1 delivery
- PR #785 merged (squash ca16dde4) after all CI checks passed.

### S2
- Worker: RED `go test ./internal/sddstatus/ -run TestApplyDecisionGate` (unfilled placeholder left gate inactive), GREEN; full `go test ./...`, `go vet ./...`, gofmt, `git diff --check` clean.
- Commit 38f97037 `feat(sdd): make the session preflight the single decision point for a feature`; review review-f42afa2356658bd7 approved + acknowledged.
- Review advisory fixed test-first: qualified `No (within budget)` regressed to blocked -> commit 0d183a03 (review review-8054a6ba640799cd approved); No-first option lists -> commit bfdf70ba (review review-35b487c973f2e8e0 approved).
- Known limitation (accepted): free-text values like `No, Yes` or `No / within budget` stay ambiguous; structured preflight decisions written by sdd-tasks are the real fix.
- Carried to S3: sdd-verify still lets cached strict_tdd win over a Standard choice; legacy in-flight changes without `## SDD Decisions` get the preflight again.
- Carried to later: chained-pr and work-unit-commits skills still describe the 400-line ask-on-risk flow.

### S2 delivery
- Windows CI exposed CRLF in multi-line contract snippets; fixed in edad8517 (review review-4f087f567290f6dd approved). PR #786 merged (squash 881412ea), 14/14 checks.

### S3
- Worker: RED on 4 new contract tests (catalog_contract_test.go, sdd_activation_policy_contract_test.go), GREEN; full suite, vet, gofmt, diff --check clean.
- Commit `feat(sdd): make TDD detection a suggestion and obey the preflight TDD mode`; review review-39f0d7ffd77510bb approved + acknowledged.
- Advisory addressed: cached TDD mode refreshed when the user switches after strict-tdd-unrunnable (docs commit).
- Known limitation: a legacy `strict_tdd: true` in openspec/config.yaml still seeds a strict suggestion; harmless because the preflight decides.

### S3 delivery
- PR #787 merged (squash 34a0a4a8), 14/14 checks.

### S4
- Worker: RED on TestSDDOrchestrator_OwnsTheProposalQuestionRoundInBothModes and TestCatalogContract_SDDProposeNeverRunsItsOwnQuestionRound, GREEN; full suite, vet, gofmt, diff --check clean.
- Commit `feat(sdd): give the proposal question round a single owner in both modes`; review review-b2b4a8841de42803 approved + acknowledged. Advisories (headless offer handling, input value list) accepted as minor; propose already proceeds best-effort and reports not-run.

### S4 delivery
- PR #788 merged (squash b34b46f6), 14/14 checks.

### S5a
- Worker: RED (build failure: output.Next undefined, PlanOutcomes undefined), GREEN; jarvis-cli, hivederive, hive-daemon test suites ok; vet clean on jarvis-cli, hivederive, hive-api.
- Derived ids: request_id = `ckpt-` + 32 hex over a domain-prefixed canonical payload (project/change included because Hive receipts are keyed globally by request_id); batch_id = `apb-` + 32 hex from request_id; stream_sha256 from planEntries. Base/Expected* stay required.
- `next` actions: done, continue_stream, continue_tasks, stop_fix_entry, stop_new_change, stop_consolidate, refresh_and_retry, retry_new_request_id, fix_request, run_upgrade_continuation, retry_identical, stop_blocked; exhaustive mapping test plus AST guard on PlanOutcome constants.
- Commits `feat(sdd): derive checkpoint identities and return a next action` (review review-6810382206a126a2 approved) and coverage restore 'test(sdd): keep coverage for stale explicit digests on reused request IDs' (review review-79b0cbefdf4a97a9 approved).
- Prompt slice S6 must stop demanding stream_sha256/request_id/batch_id and follow `next`.

## Next step
S5a push + PR + merge, then S5b (compact strict-TDD record).
