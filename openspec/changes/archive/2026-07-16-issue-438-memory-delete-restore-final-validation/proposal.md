# Proposal: Final Validation for Issue 438 Memory Delete and Restore

## Intent

Create a minimal validation-only successor for issue #438. The current implementation and tests are pre-existing evidence; every predecessor and prior-revalidation artifact is immutable. This change exists only to establish one final, auditable review, verification, and native-gated archive path.

## Scope

### In Scope
- Author successor-only SDD evidence with a future spec fixed at exactly 5 requirements and 7 `Scenario` headings.
- Use native `nextRecommended` and `dependencies`; complete successor-bound review/binding before one verification.
- Write the verification report exactly once, classify broad failures and issue #441 honestly, and archive only when natively authorized.

### Out of Scope
- Production or test edits, implementation replay, remediation, or new product behavior.
- Editing, regenerating, replacing, or reclassifying any prior artifact or verification report.
- Claiming unrelated broad-suite failures or issue #441 as green or as issue-438 defects.

## Capabilities

### New Capabilities
- `memory-delete-restore-final-validation`: Immutable validation lineage, exact 5-requirement/7-scenario completeness, native authorization, honest failure classification, and write-once verification evidence.

### Modified Capabilities
None. Product semantics remain governed by the existing issue-438 lineage.

## Approach

Treat the current tree as immutable verification input. Create only this successor's SDD artifacts, preserve explicit production/test and prior-artifact boundaries, obtain and bind review authority before verification, then run verify once. The resulting report is final and MUST NOT be edited. Stop whenever native status withholds the next phase; archive only through the native archive gate.

## Affected Areas

| Area | Impact | Description |
|---|---|---|
| `openspec/changes/issue-438-memory-delete-restore-final-validation/` | New | Successor validation artifacts only |
| `openspec/changes/issue-438-memory-delete-restore*/` | Referenced | Immutable historical evidence |
| `jarvis-cli/internal/sddstatus/` | Referenced | Native phase authorization |

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| Spec count drifts from 5/7 | Med | Count only requirement and `Scenario` headings |
| Existing changes are attributed here | Med | Enforce zero production/test edits and compare boundaries |
| Broad failures are misrepresented | High | Report environmental/base failures and #441 explicitly; do not remediate |
| Report or archive bypasses native flow | Med | One report write; stop unless native status authorizes archive |

## Rollback Plan

Withdraw only this successor's pre-verification artifacts. Never alter implementation, tests, prior artifacts, or any written verification report; a stale report requires a new authorized successor.

## Dependencies

- Gentle AI 2.1.6 native status/dispatcher and valid successor-bound review authority.
- Existing issue-438 implementation, tests, and immutable historical evidence.

## Success Criteria

- [ ] The future spec contains exactly 5 requirements and 7 `Scenario` headings.
- [ ] No production, test, or prior-artifact path is edited.
- [ ] Successor-bound review/binding precedes exactly one verify; its report is written once and remains unchanged.
- [ ] Broad failures and issue #441 are reported honestly without remediation.
- [ ] Archive occurs only when native status authorizes it.
