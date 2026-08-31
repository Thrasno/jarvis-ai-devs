# Proposal: Add the Zoho CRM Skill

## Intent

Add an evidence-backed `zoho-crm` skill for CRM activation, composition, and routing. Issue #544 is approved; #545 and merged PR #610 are delivered despite stale historical wording.

## Scope

### In Scope
- Add `jarvis-cli/embed/skills/zoho-crm/` with `SKILL.md`, focused approved references, and recognition-only 21-module/609-field catalogs.
- Encode context-aware composition, V8 policy, the exact 13-task Deluge allowlist, verified alternatives, operation-specific responses, runtime-metadata authority, authentication safety, and structural limits.
- Add contract tests and the minimum existing Zoho selection integration needed for CRM to participate in the Zoho skill pack.

### Out of Scope
- Generated user-machine configuration or installed skill copies.
- New generic router, installer, agent-adapter, or application framework abstractions.
- Live APIs, credentials, tenant tests, execution runners, or guessed limits and response wrappers.
- Non-CRM Zoho application implementations and unrelated `zoho-deluge` changes.

## Capabilities

### New Capabilities
- `zoho-crm-skill`: Defines CRM activation and composition, deterministic surface routing, V8/legacy policy, catalog boundaries, authentication safety, output placement, and evidence-backed exclusions.

### Modified Capabilities
- None.

## Approach

Keep product facts and routing in embedded Markdown. Reuse existing embedding, discovery, and installation; change selection/catalog contracts only where tests require CRM integration. Follow strict TDD.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `jarvis-cli/embed/skills/zoho-crm/` | New | Skill and approved focused references/catalogs |
| `jarvis-cli/internal/skills/` | Modified | Contract and selection coverage |
| `jarvis-cli/internal/tui/` | Modified | Minimal Zoho selection behavior and tests |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Static catalogs become tenant truth | Medium | Require runtime metadata for permissions and schema behavior |
| Responses gain invented wrappers | Medium | Preserve operation-specific and opaque boundaries |
| CRM composition is incorrect | Medium | Test Client Script, Deluge, cross-app, and external runtimes |
| Authored diff exceeds the 5,000-line single-PR budget | Medium | Forecast in tasks; stop rather than infer `size:exception` |

## Rollback Plan

Revert the CRM tree and focused selection/test changes. Other skills remain unchanged.

## Dependencies

- Contract #542 and delivered Deluge core #543 where applicable.
- Approved evidence #545, delivered by PR #610, and tracked CRM documents.
- Existing recursive skill embedding/installers; no root `openspec/config.yaml` is required or created.

## Success Criteria

- [ ] Contract tests verify exactly 13 V8 tasks, routing misses, language/output isolation, catalog boundaries, and `conpas_crm` safety.
- [ ] CRM participates in existing Zoho selection without new generic infrastructure.
- [ ] All focused Go tests pass and no generated configuration or unrelated Zoho skill changes occur.
