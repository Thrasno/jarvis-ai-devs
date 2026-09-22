# Issue 728 — Hive Repository Persistence Boundary

## Objective
Prevent read-only investigations, status reviews, reusable discoveries, checkpoints, and handoff notes from creating unsolicited repository report files. Keep Hive as the default persistence destination outside explicitly file-backed workflows.

## Problem
The canonical Hive protocol mandates proactive saves but does not explicitly state that ordinary investigation findings do not authorize repository writes. Agents can therefore create ad-hoc Markdown reports without a user request.

## Why
Unsolicited files mutate the repository, duplicate Hive persistence, and blur the boundary between product artifacts and assistant memory.

## Scope
- Add one canonical repository-file persistence boundary to `jarvis-cli/embed/hive-protocol.md`.
- Align the embedded Hive skill without creating a competing source of truth.
- Preserve explicit file requests, established repository artifacts, and SDD `openspec`/`hybrid` file-backed behavior.
- Preserve `hive` as no-project-file persistence and `none` as inline-only.
- Add focused contract and rendered-surface regression tests for Claude and OpenCode.

## Constraints
- Do not edit generated user-machine configuration.
- Do not broaden the global technical contract.
- Do not silently fall back to repository files when Hive is unavailable.
- Keep canonical behavior in `jarvis-cli/embed/hive-protocol.md`.
- Technical artifacts remain in English.

## Delivery
- Strategy: single PR.
- Forecast: 125–190 authored changed lines; investigate if the diff exceeds 220 lines.
- Branch: `fix/issue-728-hive-persistence-boundary`.

## TDD and verification
- TDD mode: ordinary ODD verification; strict TDD is scoped to SDD by `AGENTS.md`.
- Runner: focused `go test` in `jarvis-cli`, then `go test ./...` and `go vet ./...` as applicable.

## Tasks

### 728-01 — Define and verify the persistence boundary
- [ ] Add failing contract/rendering tests covering inline findings, Hive-default persistence, explicit file requests, established artifacts, all four SDD store modes, and no filesystem fallback.
- [ ] Add the canonical protocol boundary and align the Hive skill.
- [ ] Run focused tests and applicable module verification.
- [ ] Record the work-unit commit and native assessment/review outcome.

Route: delegated writer. Trigger: multi-file write across canonical assets and tests.

## Acceptance criteria
- Investigation/status requests without a file request return findings inline and use Hive for reusable knowledge without creating a report file.
- “Persist these findings” defaults to Hive.
- An explicit path request may create that file.
- Established repository artifacts remain writable by their workflow.
- SDD `hive`, `openspec`, `hybrid`, and `none` preserve their defined persistence semantics.
- Hive unavailability does not trigger a filesystem fallback.
- Generated Claude and OpenCode instruction surfaces contain the boundary exactly once.

## Progress
- Issue approved on GitHub.
- Implementation authorized by the user.
- Tracking document created before source writes.

## Verification evidence
Pending.

## Next step
Delegate task 728-01 in this isolated worktree.
