# Proposal: Manage Zoho skills as one convergent pack

## Decision summary

Treat every embedded skill whose catalog ID starts with `zoho-` as one **Zoho Skills Pack**. Setup exposes one pack-level choice, persists selected member IDs in lexicographic order, and `jarvis sync` safely expands both the released legacy selection and later catalog additions. Expansion becomes durable only after complete successful convergence, and the command reports every Zoho ID it successfully adds.

This change fixes inconsistent ownership semantics: several embedded Zoho skills are currently selected outside the visible pack choice, so deselecting that choice cannot remove all Zoho IDs from desired state and future additions have no explicit convergence contract.

## Intent

Provide one understandable and durable lifecycle for Zoho skills across interactive setup, non-interactive setup, persisted desired state, and replay sync. Users should be able to reason about Zoho as a single managed product choice while the system retains concrete skill IDs for deterministic installation and synchronization.

The proposal preserves existing installation safety and uses the repository's current catalog, selection, replay, and state boundaries rather than introducing a separate installation system.

## Product outcome

After this change:

- Users see exactly one setup choice named **Zoho Skills Pack**, never individual Zoho application choices.
- Selecting the pack manages every current embedded `zoho-*` catalog member.
- Persisted selected Zoho IDs are concrete and lexicographically ordered.
- A selected installation converges to future embedded `zoho-*` additions on a later flagless, non-interactive `jarvis sync`.
- A legacy desired state containing the released anchor `zoho-deluge` converges to the complete current pack.
- Each successful legacy or future expansion explicitly reports every Zoho ID added during that run.
- A failed or blocked convergence does not claim or persist additions that did not successfully converge.
- Deselecting the pack removes all Zoho IDs from desired state and ends future management without deleting already-installed copies.

## Scope

### In scope

1. **Pack membership**
   - Define pack membership from the embedded catalog: every current and future ID beginning with `zoho-` is a member.
   - Cover the current members: `zoho-analytics`, `zoho-books`, `zoho-creator`, `zoho-crm`, `zoho-deluge`, `zoho-people`, and `zoho-projects`.
   - Use lexicographic ID order wherever selected Zoho IDs are persisted or reported.

2. **Unified setup behavior**
   - Apply one pack-level answer consistently in the TUI and non-TUI setup paths.
   - Prevent pack members from also being treated as independently auto-selected optional skills.
   - Preserve unrelated skill selections when the pack is selected or deselected.

3. **Desired-state persistence**
   - Persist every concrete current pack member after fresh selection.
   - Remove all recorded Zoho IDs from desired state after deselection.
   - Keep deselection non-destructive for files already installed on disk.

4. **Safe sync expansion**
   - Recognize `zoho-deluge` as the only released legacy selection anchor.
   - Expand eligible legacy selections and already-selected packs to current catalog membership in memory before planning and applying sync.
   - Persist newly added IDs only after the complete run has successfully converged and final verification has succeeded.
   - Reconcile expansion against freshly read desired state so unrelated concurrent changes are preserved and a concurrent pack deselection is not reversed.
   - Leave prior durable desired state unchanged on planning failure, blocked application, partial application, or failed final verification.

5. **Observable reporting**
   - On successful expansion, report every Zoho ID newly added to durable desired state, whether the addition comes from legacy migration or a later pack release.
   - Report IDs deterministically in lexicographic order.
   - Do not report an ID as added when convergence or durable persistence for that addition did not succeed.

6. **Existing file-safety guarantees**
   - Retain atomic and idempotent managed-file overwrite behavior.
   - Retain existing symlink refusal and safe final-file handling.
   - Include newly selected pack files in the established planning, tracking, backup, mode enforcement, and verification lifecycle.

### Non-goals

- Adding, removing, or changing embedded Zoho skill content.
- Introducing remote Zoho API calls, credentials, or runtime service integrations.
- Uninstalling skill directories or files when the pack is deselected.
- Adding per-application Zoho choices.
- Inferring pack enrollment from arbitrary orphaned `zoho-*` IDs; only `zoho-deluge` is the approved released legacy anchor.
- Changing `jarvis sync` flags, prompts, or non-interactive behavior.
- Building a generic pack framework, installer abstraction, or agent-routing layer beyond what this change needs.
- Editing generated agent configuration or user-machine skill copies as source files.
- Completing nested-reference behavior or Claude Code/OpenCode end-to-end parity, which remains owned by issue #547.

## User-visible behavior

| Situation | Expected behavior |
| --- | --- |
| Fresh setup, pack selected | All current embedded `zoho-*` IDs are selected and persisted in lexicographic order. |
| Fresh setup, pack deselected | No Zoho ID is added to desired state; unrelated selections are unchanged. |
| Existing setup contains only `zoho-deluge` | The next successful `jarvis sync` installs and persists all missing current pack members, then reports each added ID. |
| Selected pack encounters a future embedded `zoho-*` ID | The next successful `jarvis sync` installs and persists that ID, then reports it. |
| Expansion is blocked or fails | The pre-run desired-state list remains durable; no unsuccessful addition is reported as completed. |
| Pack is deselected | All Zoho IDs are removed from desired state, future management stops, and existing installed copies remain. |
| Sync is run with flags or requires input | Existing flag rejection and non-interactive behavior remain unchanged. |

## Approach and affected areas

The implementation should establish one catalog-derived pack contract and apply it at the existing repository seams:

- `jarvis-cli/internal/skills/registry.go` and `jarvis-cli/internal/skills/interactive.go` provide the embedded catalog and interactive classification boundaries.
- `jarvis-cli/internal/tui/skills_selection.go`, `steps.go`, and `nontui.go` govern the single prompt, TUI/non-TUI parity, and fresh desired-state selection.
- `jarvis-cli/cmd/jarvis/cmd_sync.go` is the boundary for constructing one expanded in-memory replay input before rendering and application.
- `jarvis-cli/internal/sync/plan.go`, `runner.go`, `backup.go`, and `bookkeeping.go` provide the existing planning, tracked-file protection, convergence verification, and locked state-update boundaries.
- `jarvis-cli/internal/state/state.go` remains the durable desired-state model and ownership record.
- `jarvis-cli/embed/skills` remains the source of truth for embedded skill content; generated user-machine files remain outputs.

Specs and design should preserve these boundaries while deciding the smallest cohesive internal API and the precise post-verification transaction. In particular, persistence must use the established state lock and must not use `config.Save()`, because manifest locking is non-reentrant.

## Safe migration and convergence semantics

Migration is a replay behavior, not an eager manifest rewrite:

1. Load durable desired state and the current embedded catalog.
2. Determine whether the state is eligible for pack expansion: the released legacy anchor is selected, or the pack is already selected under the current contract.
3. Construct one in-memory desired view containing all current pack members in lexicographic order.
4. Use that same view for instruction rendering, planning, tracked-file backup, application, and verification.
5. Only after complete successful convergence and final verification, re-read state under the manifest lock and commit eligible additions without overwriting unrelated changes or resurrecting a concurrently deselected pack.
6. Report each ID that was successfully and durably added, in lexicographic order.

This makes retries convergent and idempotent: an interrupted, blocked, or failed run retains the previous durable state; a later successful run can plan the same missing members again; and a subsequent run after success has no additions to persist or report.

## Ecosystem impact

- **CLI users:** receive one coherent setup choice and explicit sync reporting for added Zoho members.
- **Existing installations:** legacy `zoho-deluge` selections gain a safe migration path without a separate command or prompt.
- **Future skill-pack maintainers:** adding an embedded `zoho-*` directory automatically enrolls it in the selected pack's next successful convergence; no second hard-coded membership list should be required.
- **State and sync maintainers:** gain an explicit success-time transaction contract while retaining current locking, backup, and replay responsibilities.
- **Support and operations:** can distinguish successful additions from failed attempts through explicit per-ID output; retries remain safe.
- **Agent integrations:** continue consuming generated instructions and managed skill files through existing paths. Final nested-reference and cross-agent E2E parity are deliberately deferred to #547.

## Risks and mitigations

| Risk | Mitigation required by specs/design |
| --- | --- |
| State is expanded before files fully converge | Make final successful verification a hard precondition for durable expansion. |
| A concurrent update is overwritten | Re-read and merge under the existing state lock; preserve unrelated desired-state entries. |
| A concurrent deselection is resurrected | Revalidate pack eligibility at commit time and skip expansion when current state no longer selects the pack. |
| Catalog traversal order changes | Sort concrete Zoho IDs lexicographically before persistence and reporting. |
| TUI and non-TUI behavior diverge | Derive both paths from the same pack membership and selection contract. |
| Future IDs install but are not tracked or backed up | Feed one expanded desired view through the existing planner, tracker, backup, runner, and verifier. |
| Output claims an addition that was not committed | Emit successful-addition reporting from the confirmed post-convergence state transition. |
| Scope expands into integration parity | Keep nested-reference and Claude Code/OpenCode E2E work in #547. |
| Work exceeds the 400 changed-line review budget | Tasks must forecast changed lines and trigger the configured ask-on-risk decision before apply if necessary. |

## Rollback

Rollback should restore the previous selection and sync behavior without deleting installed skill copies:

1. Revert the pack-level selection and sync-expansion behavior.
2. Leave already-persisted concrete Zoho IDs intact unless the user explicitly changes selection; they remain valid catalog IDs and preserve ownership evidence.
3. Allow existing sync behavior to continue managing those persisted IDs safely.
4. Do not attempt automated filesystem cleanup or desired-state contraction as part of rollback.

This rollback avoids destructive migration and keeps user installations recoverable. If a release must be disabled before code rollback, operators can avoid running setup or sync that would introduce new pack expansion; previously completed state remains explicit and replayable.

## Success criteria

- Setup presents exactly one **Zoho Skills Pack** choice in both TUI and non-TUI flows.
- Selecting the pack persists all current embedded `zoho-*` IDs in lexicographic order; deselecting it persists none while preserving unrelated selections.
- No pack member is independently auto-selected outside the pack choice.
- A legacy state anchored by `zoho-deluge` expands to all current members only after a completely successful, verified sync.
- A future embedded `zoho-*` member is included automatically for selected packs and persisted only after successful convergence.
- Every successfully added Zoho ID is explicitly reported in lexicographic order for both legacy migration and future expansion.
- Failed, blocked, partial, or verification-failing runs leave prior desired state unchanged and do not report additions as successful.
- Concurrent unrelated state changes survive expansion, and concurrent pack deselection is not reversed.
- Existing atomic/idempotent file behavior, tracked backup coverage, mode handling, and symlink safety remain intact.
- `jarvis sync` remains flagless and non-interactive and avoids re-entrant manifest saving.
- No source or test work owned by #547 is duplicated.

## Proposal question round

An interactive product question round was completed and confirmed before this proposal was finalized. It resolved two product decisions:

- Persist and evaluate Zoho pack IDs in lexicographic ID order.
- `jarvis sync` explicitly reports each successfully added Zoho ID during both legacy migration and every future pack expansion.

These confirmed decisions are incorporated throughout this proposal. Specs and design may resolve only the internal membership API, post-verification bookkeeping transaction, concurrency protection, test seams, and implementation slicing; they must not reopen the product contract above.
