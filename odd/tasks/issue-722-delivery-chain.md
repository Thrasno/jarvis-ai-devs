# Issue 722 Delivery Chain

## Purpose

This document marks the draft integration branch for issue #722. The tracker must not merge into `master` until every child pull request has been reviewed and integrated in dependency order.

## Chain

1. `fix/issue-722-01-project-keys` — separate canonical project keys from display names.
2. `feat/issue-722-02-workspace-promotion` — persist workspace bindings and promote directory identities to Git identities.
3. `feat/issue-722-03-ingress-reconciliation` — reconcile retired identity ingress, sync relocation evidence, and SDD store bindings.
4. `feat/issue-722-04-project-lifecycle` — preserve archive state and completely purge canonical projects with their retired predecessors.

Each child branch targets the immediately preceding branch. The first child targets this tracker branch. The final child represents the complete verified implementation.

## Review Budget

The four cohesive child slices contain approximately 623, 1,317, 2,400, and 1,279 changed lines. The maintainer explicitly authorized `size:exception` for these boundaries because splitting them further would separate tests, migration invariants, or transactional behavior from the code they verify.

## Merge Gate

- Issue #722 has `status:approved`.
- Every child PR must pass its relevant checks and carry exactly one `type:*` label plus `size:exception`.
- The tracker remains draft/no-merge until all children are reviewed and integrated.
- No release is authorized by this delivery chain.
