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
- [ ] S2: Preflight as the single decision point (TDD mode, size policy, chain strategy); tasks and orchestrator stop re-asking; sticky size exception. Also close S1 advisory: an unfilled `Decision needed before apply: <one of: Yes, No>` placeholder leaves Required=false (gate fails open; pre-existing) and the verbatim-template test injects a literal Yes.
- [ ] S3: TDD detection only suggests (real test command required; Deluge suggests standard; one-line reason); apply consumes forwarded mode.
- [ ] S4: Question round single owner (orchestrator), offered in both modes, forced only for unusable scope.
- [ ] S5a: Checkpoint CLI derives request id and stream digest; returns a `next` action.
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

## Next step
S1 push + PR, then S2.
