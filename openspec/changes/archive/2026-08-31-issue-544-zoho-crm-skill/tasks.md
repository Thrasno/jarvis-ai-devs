# Tasks: Add the Zoho CRM Skill

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~2,400–3,200 additions + deletions: 609-field/21-module catalogs, 10 focused references, SKILL.md, tests, and selection wiring |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR 1 content contracts/assets → PR 2 catalogs → PR 3 selection integration |
| Delivery strategy | single-pr |
| Chain strategy | pending |

Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: pending
400-line budget risk: High

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|------|------|-----------|----------------------|-----------------|-------------------|
| 1 | CRM Markdown and contract coverage | PR 1 | `go test ./jarvis-cli/internal/skills -run ZohoCRM` | N/A: no live Zoho runtime is in scope | Revert `embed/skills/zoho-crm/` and its contract tests |
| 2 | Recognition catalogs and authority checks | PR 2 | `go test ./jarvis-cli/internal/skills -run Catalog` | N/A: catalogs are static recognition data | Revert the two CRM catalog files and catalog assertions |
| 3 | Existing Zoho selection integration | PR 3 | `go test ./jarvis-cli/internal/skills ./jarvis-cli/internal/tui` | N/A: in-process planner only | Revert the two selection edits and their tests |

## Phase 1: RED Contract Tests

- [x] 1.1 Create `jarvis-cli/internal/skills/zoho_crm_contract_test.go` with table-driven failing tests for all spec scenarios: four activation/placement paths, V8 allow/miss routing, new/legacy/API names, catalog/runtime authority, response alternatives, bounded routing/limits, and safe exclusions.
- [x] 1.2 Extend `jarvis-cli/internal/skills/interactive_test.go` and `jarvis-cli/internal/tui/model_test.go` with failing tests proving `zoho-crm` is explicitly interactive, grouped with Deluge, defaults/toggles together, and isolated from non-Zoho prompts; run focused RED commands.

## Phase 2: GREEN Assets and Wiring

- [x] 2.1 Add `jarvis-cli/embed/skills/zoho-crm/SKILL.md` plus `references/routing.md`, `execution-contexts.md`, `deluge-tasks-v8.md`, `rest-v8.md`, `client-script.md`, `metadata-and-prerequisites.md`, `authentication.md`, `standard-capabilities.md`, `uncertainty-and-errors.md`, and `sources.md` with approved links and contracts.
- [x] 2.2 Add recognition-only `references/zoho-crm-standard-modules.md` and `zoho-crm-standard-fields.md`, adapted from `docs/zoho/`, with exactly 21 modules and 609 fields; runtime metadata remains authoritative.
- [x] 2.3 Modify `jarvis-cli/internal/skills/interactive.go` and `jarvis-cli/internal/tui/skills_selection.go` to add CRM to explicit-selection authority and the existing `zoho-deluge` prompt; rerun GREEN tests.

## Phase 3: REFACTOR and Verification

- [x] 3.1 Refactor only after GREEN; verify frontmatter, recursive installation, local reference links, exact 13-task allowlist, extensions, operation-specific response handling, exclusions, and non-CRM isolation.
- [x] 3.2 Run `go test ./jarvis-cli/internal/skills ./jarvis-cli/internal/tui`, then `go test ./...`; confirm no generated user files, live-runtime claims, shell/process tasks, or unrelated Zoho changes.
