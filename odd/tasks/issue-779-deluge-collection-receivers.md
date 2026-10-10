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
- [x] T3: Implementation f826b7e0 and tracking 74d38536 committed; feature branch pushed; PR #780 opened with type:feature, closing approved #779.
- [x] T4: All 14 CI checks passed exact head 74d38536f893bda3d926272fab3b9463b2d4d863. Squash merged as a742eea6ed303e3aec7582d0dd412823c85d90d5 without bypass.
- [x] T5: master updated ff-only; Beta Release run 37885188389 succeeded. Remote master, beta, v0.0.1-beta and release target match merge SHA. Prerelease not draft, 19 fresh assets; jarvis and hive-daemon Linux archive availability HTTP 200.

## Final evidence and limitations
Full affected-module checks passed: cd jarvis-cli && go test ./... && go vet ./... (30 packages, no failures). Release: https://github.com/Thrasno/jarvis-ai-devs/releases/tag/beta . Published 2026-10-09T04:44:28Z. Both installer sources support JARVIS_INSTALL_VERSION=beta. Installers and archive runtime execution were not run; macOS remains best effort. No local builds or real Zoho parser execution.

## Next step
User can update testing machine from beta and reapply managed configuration. Final delivery tracking updated locally after release; no additional remote source commit created.
