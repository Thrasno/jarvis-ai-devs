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
- [x] S5b: Checkpoint CLI accepts a compact per-task strict-TDD record and expands it into evidence entries.
- [x] S6: Trim apply prompts (one checkpoint per batch, follow `next`, compact TDD).
- [x] S7: #781 operator handoff (EvidenceOperator kind + tasks/apply/verify skills).
- [x] S8: Adaptive verify (stop early on pending tasks; run detected commands once; static review without runner; operator-attested).
- [x] S9a: Spec-merge engine in Go.
- [x] S9b: `jarvis sdd archive` performs spec sync; trim archive skill.
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

### S5a delivery
- PR #789 merged (squash 45197bf1), 14/14 checks.

### S5b
- Worker: `task_records` input (one record per task: red/green/triangulate/refactor/verification, skip_reason only on triangulate) expanded by `applyprogress.ExpandTaskRecords` into canonical entries before identity derivation; mutually exclusive with `entries`; entry ids `<task>-<step>` with bounded digest stems. RED (stub + CLI tests), GREEN; hivederive, jarvis-cli, hive-daemon, hive-api suites ok.
- Commit `feat(sdd): accept compact per-task records in apply checkpoints`; review review-fd225f30221b2423 approved.
- Advisory fixed test-first: completion attaches to the last step that ran and it must pass (no completing on RED/failure) — commit `fix(sdd): require a passing last run step before a task record completes`; review review-c3476a1908530cd5 approved.
- Note: existing validators enforce structure only (no RED-must-fail rule); unchanged by design.

### S5b delivery
- PR #790 merged (squash e64e1a7c), 14/14 checks.

### S6
- Worker: RED on TestCatalogContract_ApplyCheckpointsOncePerBatchWithTaskRecords, GREEN; full suite + vet clean. strict-tdd.md 419 -> 158 lines; SKILL.md 28.0 KB -> 23.6 KB; one checkpoint per batch with task_records; follow `next`.
- Review review-e2b039d2e1804dab approved; four advisories fixed (RED observed on new contract by stashing Markdown): executor owns continue_stream, fail closed on missing/unknown `next`, retry_identical at most twice, skipped triangulation is not a run step (review review-09382464fbde3ea7), then stalled-stream stop and single owner wording (review review-d7593b56b8b287f2).
- Accepted residual: "cursor advance" wording nuance.
- Workaround to revisit in S8: safety-net baseline is recorded in the first task's red summary (no dedicated step kind).

### S6 delivery
- PR #791 merged (squash 1369a8c1), 14/14 checks.

### S7 (#781)
- Worker: EvidenceOperator kind, TaskRecord `operator` step (summary only, exclusive with agent steps), Operator Handoff section in sdd-tasks (ID first), apply pause `operator-handoff` + `OPERATOR_ACK` relaunch, orchestrator relay in both modes, verify `pending-operator` / `operator-attested`, strict-tdd-verify OPERATOR branch. RED (compile + behavior + contract), GREEN; hivederive, jarvis-cli, hive-daemon, hive-api suites ok.
- Parent: report-format.md statuses. Commit `feat(sdd): pause apply at operator handoff tasks and complete them on chat ack` (Closes #781); review review-5b5709067461c4ce approved.
- Advisories fixed test-first: operator evidence only on `[operator]` tasks and attestation shape (no command/exit/non-pass) — review review-ac9406ac0e602cf6 approved.
- Accepted residuals: tag variants (e.g. different casing) are not recognized as operator tasks; low-level `advance` recovery primitive is not re-validated here.

### S7 delivery
- PR #792 merged (squash 1006b8c3), 14/14 checks; #781 closed.

### S8
- Worker: verify stops before any command on pending implementation work; runtime mode runs test + quality commands once and confirms GREEN via the suite; static mode (no runner) reports `static-reviewed`, max archive-ready PASS WITH WARNINGS; strict-tdd-verify.md 300 -> ~140 lines; install_test pins rescoped. RED on new catalog + install pins, GREEN. Archive acceptance already in Go: hivederive/applyprogress/verify.go:43-46, sddstatus/status.go:580,657.
- Parent fixes (RED observed by stashing Markdown): tests that cannot fail stay CRITICAL when the optional audit runs; static mode only when no real test command exists (missing cache never selects it).
- Reviews: review-614fed8046e4be00 (S8) and review-b5885e38b6e6d299 (static-mode fix) approved.
- Process note: a commit was created while one test failed (grep pipeline hid the exit code); amended before push with the pin fixed and full suite green.

### S8 delivery
- PR #793 merged (squash 05751445), 14/14 checks.

### S9a
- Worker: new `jarvis-cli/internal/sddspecsync` (MergeSpec, BuildPlan, Apply, OSStore) + `atomicfile.Remove`; ~1,250 production + ~770 test lines (over the 400 budget; self-contained, not wired). RED against stubs (76 failures), GREEN, coverage 85.7%.
- Review review-811c1b4fbca90168 approved. Advisory fixed test-first: rollback no longer overwrites unexpected post-write bytes (treated as another writer, reported for recovery) — review review-14d6c38fcf10afd0 approved.
- For S9b: fail-closed rules reject (a) a delta `##` section outside the four requirement sections (one archived delta has one) and (b) RENAMED blocks with body text (sdd-spec template allows it). Align template and decide tolerance before wiring; decide destructive-plan confirmation; map ApplyError codes to CLI output; empty capability dirs after rollback.

### S9a delivery
- PR #794 merged (squash 4f92e163). Windows CI: POSIX mode assertions skipped on Windows (test-only fix, review review-0d318e65e5abf895); one rerun of the known OpenCode lifecycle flake (unrelated, no bypass).

### S9b
- Worker: `jarvis sdd archive` plans, confirms (`--confirm-destructive`), applies the spec merge and moves under the same archive lock; reverts the sync if the move fails; JSON output with typed spec_sync codes; `--plan` dry run; non-requirement delta sections ignored and reported; sdd-archive skill delegates to the CLI; sdd-spec RENAMED without body. RED/GREEN; full suite, vet (linux + windows), gofmt clean. Review review-53b2f6d609373589 approved.
- Review fixes (RED observed each): misleveled requirement headings in ignored sections block; durable `.spec-sync-journal.json` lets a rerun resume an interrupted archive; `--plan` on Hive reports nothing to sync. Review review-54041fb34af473f9 approved.
- Accepted residuals: delta edited after an interrupted sync is not re-checked on resume; a journal left in an archived change after a failed delete is reported, not auto-cleaned.
- One unrelated flake seen once locally (TestBoundSddArchiveOpenSpecPersistedBindingIgnoresHiveEnvironmentAndRenames); 20/20 on rerun.

## Next step
S9b push + PR + merge, then S10 (authority validated once).
