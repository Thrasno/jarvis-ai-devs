# Pre-Proposal Gate: `issue-546-zoho-skill-pack`

## Status

- Product decisions: confirmed by the maintainer
- Optional research: unselected
- Evidence readiness: complete for proposal
- Artifact store: hybrid
- Execution mode: interactive

## Confirmed Product Decisions

- Expose exactly one interactive `Zoho Skills Pack` choice; never expose individual Zoho application choices.
- Every current and future embedded catalog ID beginning with `zoho-` belongs to the pack.
- Persist every selected concrete Zoho ID in lexicographic ID order.
- Treat `zoho-deluge` as the only released legacy selection anchor.
- Expand legacy and future pack members during flagless, non-interactive `jarvis sync`.
- Persist expansion only after complete successful convergence; preserve prior state on failure or blocked convergence.
- Report every successfully added Zoho ID for both legacy and future expansion.
- Deselection removes all Zoho IDs from desired state and stops management without uninstalling existing copies.
- Preserve the current atomic/idempotent overwrite and symlink-safety behavior for selected managed files.
- Keep final nested-reference and Claude Code/OpenCode end-to-end parity in issue #547.

## Evidence

- Approved issue: <https://github.com/Thrasno/jarvis-ai-devs/issues/546>
- Exploration: `openspec/changes/issue-546-zoho-skill-pack/exploration.md`
- Project context: `openspec/config.yaml`
- Engram exploration: `sdd/issue-546-zoho-skill-pack/explore`

## Design-Owned Questions

The proposal must not reopen product decisions. Design may resolve:

- the internal catalog-derived membership API;
- the exact post-verification bookkeeping transaction;
- concurrent manifest mutation protection;
- test seams and implementation slicing under the 400-line review budget.
