# Design: Add the Zoho CRM Skill

## Technical Approach

Add `zoho-crm` as an embedded, optional application skill whose Markdown controls activation and guidance routing. Keep Deluge language behavior in `zoho-deluge`; reuse `SkillsFS`, recursive installation, frontmatter discovery, and the current grouped selection planner unchanged except for adding CRM to the Zoho group. No generated user-machine file is a source.

## Architecture Decisions

| Option | Tradeoff | Decision and rationale |
|---|---|---|
| Embedded focused skill tree | More Markdown assets, but progressive loading and existing packaging work automatically | **Choose.** It matches the shipped skill architecture and keeps evidence-backed product facts out of Go. |
| Generic Go CRM router/installer abstraction | Centralizes hypothetical future apps but introduces an unproven runtime model | **Reject.** Routing here is skill-content behavior; existing discovery and installation already recurse. |
| One Zoho selection controlling CRM and Deluge availability | Existing Deluge users receive CRM only after reconfiguration, but composition remains explicit at activation | **Choose.** Add both IDs to the existing prompt and interactive-ID authority; do not use the currently unconsumed stack detector. |

## Data / Content Flow

```text
embedded SKILL.md frontmatter -> ListSkills -> Zoho selection plan
           |                                      |
focused references/catalogs -> recursive installer -> agent skill directory
           |
runtime request -> activation/context gates -> chosen surface, language, placement
```

Static catalogs resolve recognition hints only; authenticated tenant metadata remains authoritative for safety and operation support.

## File Changes

| File | Action | Description |
|---|---|---|
| `jarvis-cli/embed/skills/zoho-crm/SKILL.md` | Create | Frontmatter, activation gates, hard rules, output contract, reference routing. |
| `jarvis-cli/embed/skills/zoho-crm/references/{routing,execution-contexts,deluge-tasks-v8,rest-v8,client-script,metadata-and-prerequisites,authentication,standard-capabilities,uncertainty-and-errors,sources}.md` | Create | Focused approved guidance and provenance. |
| `jarvis-cli/embed/skills/zoho-crm/references/zoho-crm-standard-{modules,fields}.md` | Create | Recognition-only 21-module/609-field catalogs adapted from `docs/zoho/`. |
| `jarvis-cli/internal/skills/interactive.go` | Modify | Add `zoho-crm` to the explicit-selection ID set. |
| `jarvis-cli/internal/tui/skills_selection.go` | Modify | Group `zoho-crm` and `zoho-deluge` under the existing Zoho prompt. |
| `jarvis-cli/internal/skills/zoho_crm_contract_test.go` | Create | Embedded content, discovery, recursive packaging, links, catalogs, and behavioral contracts. |
| `jarvis-cli/internal/skills/interactive_test.go` | Modify | Prove CRM is interactive rather than globally auto-installed. |
| `jarvis-cli/internal/tui/model_test.go` | Modify | Prove grouped defaults/toggling and non-Zoho isolation. |

`assets.go`, registry, installer, agent adapters, project detector, and generated configuration require no production changes.

## Interfaces / Contracts

Frontmatter remains `name: zoho-crm`, distinct `display_name`, description-embedded `Trigger:`, and `scope: optional`. The Zoho prompt owns `[]string{"zoho-deluge", "zoho-crm"}`. Content contracts are: new code uses V8; the allowlist is exactly 13 names; misses evaluate verified alternatives; Client Script is JavaScript/`.js` without Deluge; Deluge is `.deluge`; external runtimes retain requested language/placement.

## Specification-to-Test Mapping

| Requirement | Source assets | RED contract tests |
|---|---|---|
| Activation, composition, placement | `SKILL.md`, `routing.md`, `execution-contexts.md`, `client-script.md`; selection files | CRM-only/cross-app/external composition, Client Script isolation, extensions, grouped selection. |
| Routing/V8; legacy/API names | `deluge-tasks-v8.md`, `routing.md` | Exact set equality for 13 tasks, routing misses, V8-only new code, migrate/preserve, API names. |
| Catalog/runtime authority | two catalogs, `metadata-and-prerequisites.md` | Exactly 21 modules/sections and 609 fields; forbidden tenant-authority claims. |
| Responses/alternatives | `deluge-tasks-v8.md`, `standard-capabilities.md` | `getRecords` JSON-list, `searchRecords` iteration, three opaque operations, no universal wrapper, advisory alternatives. |
| Questions/limits | `routing.md`, `rest-v8.md`, `uncertainty-and-errors.md` | Single-path/equal-path behavior, bounded questions, structural limits, forbidden quotas/thresholds/timeouts. |
| Authentication/exclusions | `authentication.md`, `uncertainty-and-errors.md`, `sources.md` | `conpas_crm`, scopes/deployment, no secrets, placement, all deterministic exclusions and valid local links. |

Follow strict TDD per slice: add table-driven failing contract/selection tests, run focused package RED, add minimum assets/code, rerun GREEN, then refactor and rerun. No live Zoho or executable Deluge test is claimed without a verifiable runtime.

## Threat Matrix

N/A — this changes content guidance and in-process selection data only; it introduces no shell, subprocess, VCS/PR, executable classification, routing boundary, or process integration.

## Migration / Rollout and Rollback

No data migration or feature flag. New installs can select the grouped Zoho pack; existing installs regenerate through supported init/reconfiguration. Roll back by reverting the embedded tree and two selection entries; previously generated copies remain until normal regeneration/reconciliation.

## Open Questions

None blocking. The approved evidence assigns unknown facts to explicit exclusion or runtime clarification.
