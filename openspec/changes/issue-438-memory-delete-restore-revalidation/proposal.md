# Proposal: Revalidate Issue 438 Memory Delete and Restore

## Intent

Create a validation-only successor for `issue-438-memory-delete-restore`. The predecessor implementation is pre-existing evidence; its blocked change and historical FAIL report remain unchanged. This successor supersedes it only as the vehicle for verification and closure, never for product semantics.

## Scope

### In Scope
- Record predecessor, approved review lineage, and current-tree evidence under Gentle AI 2.1.6.
- Follow native `jarvis.sdd-status` `nextRecommended` and `dependencies` before every phase; obtain a fresh review binding only if required.
- Run independent verification against current focused and full available tests, preserving unrelated limitation evidence.

### Out of Scope
- Production or test edits, code generation, remediation, implementation replay, or new product behavior.
- Rewriting, replacing, deleting, or reclassifying predecessor artifacts or historical evidence.
- Claiming unavailable Windows/Docker suites are green.

## Capabilities

### New Capabilities
None.

### Modified Capabilities
None. Product semantics remain governed by the predecessor `memory-delete-restore` specification.

## Approach

Treat the current issue-438 tree as immutable verification input. Reference the approved successor lineage `review-issue438-fixtures-20260715`; create a fresh binding only when native status requires it. Each phase must stop if the Gentle AI 2.1.6 dispatcher does not allow it. Independent verify collects new evidence in this successor without altering predecessor history.

## Affected Areas

| Area | Impact | Description |
|---|---|---|
| `openspec/changes/issue-438-memory-delete-restore-revalidation/` | New | Validation and closure artifacts only |
| `openspec/changes/issue-438-memory-delete-restore/` | Referenced | Immutable predecessor evidence and semantics |

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| Dispatcher blocks progression | Med | Report native status; never bypass it |
| Revalidation is mistaken for implementation | Med | Enforce zero production/test edits |
| Environment failures are overstated | High | Report Windows symlink/persona and rootless-Docker limitations separately |

## Rollback Plan

Withdraw only this successor's new artifacts. Do not modify the predecessor, implementation, tests, review receipts, or historical report.

## Dependencies

- Gentle AI 2.1.6 native status/dispatcher, valid review authority, and independent verify.
- Single PR, 2,300-line budget; expected authored product/test delta is zero.

## Success Criteria

- [ ] Native dispatcher allows every executed phase and required binding validation succeeds.
- [ ] Current focused and full available tests prove issue-438 behavior with no production/test changes.
- [ ] Known unrelated Windows symlink/persona and Docker limitations are recorded honestly.
- [ ] The successor closes verification without changing predecessor semantics or history.
