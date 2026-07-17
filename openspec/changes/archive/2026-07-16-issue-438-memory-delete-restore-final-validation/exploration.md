## Exploration: issue-438-memory-delete-restore-final-validation

### Current State
The issue-438 delete/restore implementation and tests already exist as working-tree evidence, alongside two historical OpenSpec lineages: the original implementation change and a prior revalidation. The prior revalidation contains useful evidence but is itself historical and must remain unchanged. The repository currently has pre-existing production/test modifications, so this change must not attribute, rewrite, or remediate them.

The safe purpose of this successor is one final validation-only flow under Gentle AI 2.1.6: inspect the predecessor and approved review/binding evidence, use native status authorization, verify the unchanged implementation once, write the final report once, and archive only if native status permits. The final spec must be authored with exactly 5 requirements and exactly 7 `Scenario` headings; later verify must report 5/5 and 7/7 without adding pseudo-scenarios or duplicating scenario headings.

Relevant evidence includes guarded delete/restore and deleted-only restore lookup in `hive-daemon`/`jarvis-cli`, mutation-protocol-v2 acknowledgement and rejection tests, canonical TUI tests, and MCP least-privilege tests. Known broad failures—including Windows symlink/persona constraints, rootless Docker, missing daemon E2E executable, and Hive API issue #441—must remain separately classified and not be presented as issue-438 defects or silently ignored.

### Affected Areas
- `openspec/changes/issue-438-memory-delete-restore/` — immutable predecessor proposal, spec, implementation lineage, and historical verification evidence.
- `openspec/changes/issue-438-memory-delete-restore-revalidation/` — immutable prior revalidation evidence; do not edit or regenerate its report.
- `openspec/changes/issue-438-memory-delete-restore-final-validation/` — only this successor's validation artifacts; exploration is the sole artifact created in this phase.
- `jarvis-cli/internal/sddstatus/` and native SDD dispatcher contracts — authoritative `nextRecommended` and `dependencies` routing; do not infer or bypass authorization from prose.
- `hive-daemon/internal/{db,governance,httpapi,sync,mcp}` — existing behavioral evidence for guarded mutation, tombstones, acknowledgements, rejection, and least privilege.
- `hive-api/internal/{service,handler}` and `jarvis-cli/internal/{hiveclient,hiveui}` — existing propagation, restore-boundary, managed-client, and TUI evidence.

### Approaches
1. **Single validation-only successor flow** — create the proposal/spec/design/tasks as evidence-only artifacts, bind the final review to the current tree, apply without code or test edits, verify once, write no report afterward, and archive only when native status authorizes it.
   - Pros: preserves all historical evidence, enforces one final 5/5 and 7/7 gate, and gives the current tree one auditable validation boundary.
   - Cons: cannot repair historical reports, unrelated failures, or issue #441; native blockers must stop the flow.
   - Effort: Low

2. **Reopen or regenerate an earlier change/report** — reuse an existing lineage or edit its verification output.
   - Pros: fewer apparent folders.
   - Cons: violates immutable-history and no-report-edit constraints, risks duplicate pseudo-scenarios, and conflates historical evidence with final validation.
   - Effort: High/Risky

### Recommendation
Use the single validation-only successor. Keep the final spec structurally fixed at exactly 5 requirements and exactly 7 scenario headings, with each scenario covering one evidence obligation and no supplementary pseudo-scenarios. Treat apply as a zero-production/zero-test-edit evidence checkpoint. Let native status control proposal/spec/design/apply/review/verify/archive transitions through `nextRecommended` and `dependencies`; if a required binding or phase is blocked, stop and record that state. Execute verification once, produce the final report once, and perform no report edits afterward. Archive only if the native archive gate is authorized.

### Risks
- Any extra `Scenario` heading, duplicated heading, or pseudo-scenario would invalidate the required 5/5 and 7/7 completeness contract.
- Existing working-tree changes can be mistaken for successor edits; capture and compare an explicit production/test boundary without modifying those changes.
- Native dispatcher authorization may block review, verify, or archive; bypassing it would invalidate the validation lineage.
- Broad/environmental failures and issue #441 must remain honest non-green classifications, not remediation targets.
- A final report written after verify must not be edited afterward; stale evidence requires a new authorized validation flow rather than manual correction.

### Ready for Proposal
Yes. The orchestrator should state that this is a single-PR, validation-only successor under Gentle AI 2.1.6, with no code/test generation or remediation, immutable prior artifacts, exact 5-requirement/7-scenario spec structure, one final review binding, one verify, no post-verify report edits, and native-gated archive if available.
