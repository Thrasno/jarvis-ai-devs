## Exploration: issue-438-memory-delete-restore-revalidation

### Current State
The substantive change `issue-438-memory-delete-restore` is already implemented in the working tree and its focused behavior evidence is strong, but its original `verify-report.md` is a permanent historical FAIL. The blocker is lifecycle/version-related: the report belongs to the prior Gentle AI 2.1.5→2.1.6 transition and must not be edited or regenerated. The final successor review lineage `review-issue438-fixtures-20260715` is approved and records the explicit mutation acknowledgements, restore tombstone revalidation, empty-v2 behavior, stale fixture corrections, and terminal mutation rejection.

This successor is therefore an administrative revalidation, not a product change. It can model the current implementation as pre-existing evidence, reference the original change and its blocked report, and limit future work to evidence collection, lineage/status inspection, and verification under Gentle AI 2.1.6. It must explicitly prohibit production or test edits.

Native dispatcher expectations are strict: `jarvis.sdd-status` JSON is authoritative; routing uses only `nextRecommended` and `dependencies`; prose must not infer routing. Native status reports tasks and lifecycle state, while `/sdd-status` is read-only direct orchestrator handling and is not an autocomplete skill. A validation-only change must not attempt to bypass those gates or manipulate the old report.

### Affected Areas
- `openspec/changes/issue-438-memory-delete-restore/verify-report.md` — immutable historical evidence to reference, never rewrite.
- `openspec/changes/issue-438-memory-delete-restore/` — original proposal/spec/design/tasks and implementation lineage to treat as pre-existing inputs.
- `openspec/changes/issue-438-memory-delete-restore-revalidation/` — new change folder; only exploration may be created now, with later proposal/spec/design/tasks constrained to evidence-only work.
- `jarvis-cli/internal/config/sdd_activation_policy_contract_test.go` — confirms native status routing and read-only dispatcher expectations; source evidence only, not a target for edits.
- Current implementation/test paths named by the original change and final successor review — verification inputs for unchanged-tree evidence and focused/full test results.

### Approaches
1. **Validation-only successor change** — declare the issue-438 implementation pre-existing, reference/supersede the blocked original lifecycle, and define later tasks only for native status inspection, lineage/evidence review, and Gentle AI 2.1.6 verification.
   - Pros: preserves the audit trail; avoids unsafe mutation of the old report; gives the current tree a fresh, correctly scoped verification boundary.
   - Cons: cannot repair historical evidence or make unrelated red suites green; verification must clearly distinguish inherited evidence from newly collected evidence.
   - Effort: Low

2. **Reopen or regenerate the original change** — reuse the old folder and replace or amend its report.
   - Pros: fewer change folders.
   - Cons: violates the explicit immutable-history constraint, risks invalidating lifecycle receipts, and conflates a version-transition recovery with product scope.
   - Effort: High/Risky

### Recommendation
Use the validation-only successor. The future proposal/spec/design/tasks should contain no implementation requirements and no paths permitted for product edits. Tasks should be limited to: record the predecessor and approved successor lineage; inspect native `jarvis.sdd-status` and dispatcher dependencies; verify the working tree is unchanged by the revalidation; collect or reconcile Gentle AI 2.1.6 evidence; and produce a new revalidation report. The task contract should state that apply is evidence-only, must not run code-generation/remediation, must not edit production code/tests, and must not modify the predecessor report. A successful outcome may certify current behavior, but must not claim the original historical FAIL was rewritten.

### Risks
- Native status may still route to `resolve-review` or another blocked action; the revalidation must report that state rather than infer or bypass a transition.
- Treating pre-existing implementation as newly applied could cause apply to regenerate code; explicit zero-product-edit constraints and evidence-only task wording are mandatory.
- Reusing the old report or lineage could corrupt immutable audit history; all new evidence must live in the successor change.
- Existing broad-suite/environment failures must remain separately identified and must not be silently reclassified as product regressions or ignored.
- A validation-only successor can verify evidence, but cannot retroactively establish TDD RED/GREEN history that belongs to the original change.

### Ready for Proposal
Yes — provided the proposal is explicitly administrative and validation-only. It should define the predecessor reference, the approved successor review lineage, Gentle AI 2.1.6 as the verification environment, native dispatcher/status authority, immutable old-report rule, and a hard zero-production-edit/test-edit boundary. Do not create proposal/spec/design/tasks until the orchestrator launches the next phase.
