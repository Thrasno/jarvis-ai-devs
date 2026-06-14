# Gentle AI Skill Parity Maintenance

This maintainer-only runbook defines how to compare Jarvis-adopted skills against a selected Gentle AI reference without turning parity maintenance into product behavior.

> **Guardrail:** parity work operates against Jarvis source templates and embedded assets that Jarvis installs. It never edits generated user-machine configs, installed runtime copies, or team environments directly.

## Quick path

1. Select the Gentle AI tag, commit, or snapshot for this run.
2. Record the selected reference, retrieval date, Jarvis commit, and last adopted Gentle AI version.
3. Build the inventory from Jarvis source assets, including adopted unstamped skills listed in this runbook.
4. Compare each inventory item against the selected Gentle AI reference.
5. Classify every material difference as `apply`, `adapt`, `ignore`, or `investigate`.
6. Implement only approved `apply` or `adapt` decisions in Jarvis source templates/assets.
7. Verify generated outputs by regeneration when needed; never patch generated files by hand.

## Scope boundaries

| Area | Rule |
|------|------|
| Workflow audience | Maintainers only. Do not add public CLI, doctor, install, or automatic sync behavior for this process. |
| Editable source | Jarvis source-of-truth files, especially `jarvis-cli/embed/**`, `jarvis-cli/internal/agent/**`, `jarvis-cli/internal/persona/**`, and `jarvis-cli/internal/sddruntime/**` when a decision requires them. |
| Forbidden edit targets | `~/.claude/**`, `~/.config/opencode/**`, generated registries, installed `.jarvis/skills/**` copies, and team environments. |
| Upstream updates | No blind upstream sync. Every material difference needs a recorded decision and rationale first. |
| Skill content updates | Out of scope for the docs-only maintenance slice. Apply later in reviewable chained PR slices. |

If a generated artifact exposes a parity issue, fix the Jarvis source template or renderer that produced it, then regenerate and verify the output.

## Reference selection

Each run selects the current Gentle AI reference at maintenance time. No version is permanent.

Record these fields before comparing content:

| Field | Required value |
|-------|----------------|
| Gentle AI reference | Tag, commit, or snapshot identifier selected for this run. |
| Retrieval date | Date when the selected reference was inspected. |
| Jarvis commit | Commit or branch being reviewed. |
| Last adopted Gentle AI version | Most recent version already adopted by Jarvis source stamps or prior report. |
| Reference availability | Confirmed, unavailable, or incomplete. |

If the selected reference cannot be inspected, mark the run incomplete and do not update Jarvis sources from that reference.

## Inventory rules

Start with `jarvis-cli/embed/skills/**` and include shared references used by those skills. Source stamps are useful provenance, but stamps are not the only inclusion rule.

| Item | Parity status | Rule |
|------|---------------|------|
| Stamped Gentle AI skill files | In scope | Compare against the selected upstream path and reference. |
| Stamped `_shared` or reference files | In scope | Compare because phase skills depend on shared contracts. |
| Adopted unstamped skills | In scope | Include if Jarvis intentionally adopted the Gentle AI skill, even when the source stamp is absent. |
| `go-testing` | In scope | Adopted Gentle AI skill; include even if stamp metadata is absent. |
| `skill-creator` | In scope | Adopted Gentle AI skill; include even if stamp metadata is absent. |
| `skill-improver` | In scope | Adopted Gentle AI skill; include even if stamp metadata is absent. |
| `skill-registry` | In scope | Adopted Gentle AI skill; include even if stamp metadata is absent. |
| `hive` | Adapted equivalent | Treat as Jarvis' adapted equivalent to Gentle AI Engram and compare intent, not naming literally. |
| `qa-checklist` | Out of Gentle AI parity | Jarvis-local unless a future upstream equivalent appears. |
| `sdd-workflow` | Retired/removed | Excluded because orchestration authority lives in the orchestrator, shared SDD contracts, and phase skills. |
| Ambiguous local files | Investigate | Do not silently include, exclude, or edit. Record the ambiguity and owner. |

## Comparison workflow

For each inventory item:

1. Identify the Jarvis source path.
2. Identify the upstream Gentle AI path when available.
3. Compare Jarvis source against the selected Gentle AI reference.
4. Separate Jarvis adaptations from unintended drift.
5. Record every material difference in the run report.
6. Wait for maintainer approval before changing skill content or workflow semantics.

Use the run report template in `docs/maintenance/skill-parity-run-report-template.md`.

## Decision categories

| Category | Meaning | Allowed action |
|----------|---------|----------------|
| `apply` | Upstream change can be copied into Jarvis source with no semantic adaptation. | Update source after approval and verify. |
| `adapt` | Upstream intent is accepted but must be translated for Jarvis, Hive, `.jarvis`, packaging, or path-injected loading. | Implement the adapted Jarvis outcome after approval. |
| `ignore` | Difference is intentionally not applicable to Jarvis. | Leave source unchanged and record rationale. |
| `investigate` | Adoption marker, upstream meaning, or Jarvis impact is unclear. | Make no source update until resolved. |

## Greenfield upgrade note

This change is greenfield. No existing-install stale skill cleanup is required for the retired `sdd-workflow` decision because no supported user base has this system preinstalled yet.

Future non-greenfield parity changes must reassess whether stale generated or installed copies need cleanup through normal source-driven install, reconfigure, or migration paths. Even then, do not edit user-machine generated artifacts directly.

## Verification checklist

- [ ] The run records selected Gentle AI tag/commit/snapshot, retrieval date, Jarvis commit, and last adopted version.
- [ ] The inventory includes all adopted Gentle AI skills, including adopted unstamped skills.
- [ ] `hive`, `qa-checklist`, and `sdd-workflow` are handled according to the scope table.
- [ ] Every material difference has one of `apply`, `adapt`, `ignore`, or `investigate`.
- [ ] Accepted changes touch Jarvis source templates/assets only.
- [ ] Generated artifacts and team environments remain untouched.
- [ ] The PR slice stays within the review budget or is split into a chained PR.
