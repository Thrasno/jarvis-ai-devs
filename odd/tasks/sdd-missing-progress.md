# SDD missing progress fix

## Objective
Allow a fresh SDD change with no Hive progress to resolve its store binding without false legacy divergence, preserving hybrid selection and protected history checks.

## Scope and constraints
Worktree: jarvis-dev-sdd-missing-progress; branch: fix/sdd-missing-progress; base: public/master at 537b22a3. Do not touch the original documentation worktree. User authorized commit, push, PR, merge and beta release. No local builds or generated developer configuration edits. Fix absence handling only; normalization/CWD concerns are follow-ups.

## Tasks
- [x] T1 (verified implementation; commit not authorized): Add real-shaped missing/compatibility response regressions, observe RED, fix absent progress representation, observe GREEN; preserve invalid existing progress rejection.
- [x] T2 (verified; delivery pending authorization): Independently verify focused and CLI checks, review source diff and review-mode policy, record outcomes and remote validation next steps.

## Acceptance and verification
Typed 404 not_found and compatibility without legacy progress yield absent observations; initial hybrid binding succeeds. Real empty/malformed/blocked/divergent progress still fails closed. Run focused sddstatus/sddbinding and CLI tests, then CLI module go test ./... and go vet ./... where feasible. No builds. Test commands and results must be recorded, with failed/skipped checks explicit.

## Evidence
Read-only scout identified source.go artifact map insertion of missing sentinel interpreted as presence by ObserveLegacyProgress. Remote PC reportedly has no change artifacts or prior Hive progress; installed beta build provenance remains unconfirmed.

## Next step
Delivery authorized: commit, PR to master, merge after CI and Beta Release workflow. Remote PC must update published beta and re-query original hybrid change; remote validation remains pending.

## T1 evidence
Worker observed RED: six source absence cases incorrectly present and two initial-hybrid CLI cases falsely divergent. GREEN after minimal early return and fixture directory correction. go test ./internal/sddstatus ./internal/sddbinding and go test ./cmd/jarvis passed; gofmt and git diff --check passed. Empty, whitespace, malformed, blocked and divergent progress remain rejected. Parent inspected production diff (4 changed lines) and review mode is globally on. No builds/commits/publication.

## T2 evidence
Independent verifier: jarvis-cli go test ./... PASS (30 packages, some cached); go vet ./... PASS; focused source and CLI regressions rerun uncached PASS; git diff --check PASS. No requested checks failed/skipped (internal individual skips not established by nonverbose output). Parent uncached source spot check PASS. Native review review-d35d4df860d5f7ca approved and exact acknowledgement completed, authority burned for source/test candidate. No builds, commits, push or releases. Remote validation pending. Original documentation worktree untouched by this feature.

## Delivery
- [ ] T3 (in progress): Commit isolated change and create PR; merge after required checks pass.
- [ ] T4 (pending): Publish fixed beta channel and verify source identity, metadata and fresh Jarvis/Hive assets.
Existing approved issue #723 defines zero-progress adoption and hybrid selection; this regression restores that accepted behavior. Beta source remains updated master.
