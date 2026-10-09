# Issue 779 — Deluge collection method receivers

## Objective and rationale
Prevent generated methods directly on Map/List literals through a generic compatibility convention. The CRM failure is user-reported, not proof of a universal parser restriction.

Issue: https://github.com/Thrasno/jarvis-ai-devs/issues/779 (approved).
Branch: feat/779-deluge-collection-receivers.

## Scope and constraints
Only generic zoho-deluge assets and focused contract tests. Assigned literals remain valid; constructors are optional; text-literal methods remain valid. No CRM duplication or local configuration edits. User authorized commit, push, PR, merge conditional on passing CI, then Beta Release workflow. No local builds.

## Tasks
- [x] T1: Add failing convention contract, observe RED, update entry point and collection/convention references, observe GREEN. Worker reported RED exit 1 and GREEN pass; four allowed files changed.
- [x] T2: Independent focused tests, go vet and diff checks passed; native review approved and acknowledgement completed.

## Acceptance criteria and checks
- Entry point contains scoped Map/List receiver rule.
- References include variable assignment and method examples, including list.contains and map.toString.
- No universal language prohibition or prohibition on text literals.
- Focused command: cd jarvis-cli && go test ./internal/skills -run 'TestCatalogContract_ZohoDeluge' -count=1.
- No build or real Zoho execution; contract tests verify packaged guidance, not host parser behavior.

## Evidence
Issue creation confirmed by target readback including title, body and approved label. No duplicate found in open/closed searches.

## Verification progress
Worker: focused Deluge contracts passed after observed RED; gofmt and git diff --check passed. Parent structural diff and git diff --check passed. Initial native assessment unassessable due to untracked task file; independent verification subsequently passed focused tests and go vet ./internal/skills. Native review excluded tracking file, approved review-fd20da20d06f7c5c, and exact acknowledgement consumed authority successfully.

No broader suite or live Zoho runtime tests performed; no builds. Commit/push/PR/conditional merge/beta subsequently authorized by the user. Functional verification is limited to packaged guidance contracts.

## Delivery tasks
- [ ] T3 (in progress): Commit verified implementation, push feature branch, open PR linked to approved issue 779.
- [ ] T4 (pending): Observe CI success for exact PR head, then merge without bypass.
- [ ] T5 (pending): Update master, run Beta Release, verify source SHA, prerelease and fresh assets for jarvis and hive-daemon.

## Next step
Full affected-module checks passed: cd jarvis-cli && go test ./... && go vet ./... (30 packages, no failures). Implementation work-unit commit: f826b7e0. Prepare tracking commit, push and PR.
