---
name: zoho-creator
display_name: "Zoho Creator"
description: "Trigger: writing, reviewing, debugging, or planning Zoho Creator applications, forms, reports, records, subforms, workflows, Deluge, REST APIs, files, metadata, publishing, bulk reads, or Creator-to-Analytics reporting. Evidence-backed Creator routing and safety guidance."
scope: optional
---

# Zoho Creator

Use `zoho-creator` for Creator entities, applicability, execution context, workflows, permissions, events, and routing semantics. Load `zoho-deluge` for generic Deluge grammar and syntax; do not duplicate its language rules here.

## Hard Rules

- Resolve applications, forms, reports, pages, sections, fields, records, lookups, and subforms by link name or ID from metadata, never display labels.
- Route by host, target application, operation, version, environment, and execution context before generating code.
- Require explicit confirmation immediately before any create, update, delete, upload, publish, or metadata-changing execution. Planning and code generation alone are not execution.
- Never expose OAuth tokens or published private links. Treat permissions, plans, quotas, costs, and tenant capabilities as runtime facts.
- Fail closed for contradictory or unavailable operations; do not invent endpoints, tasks, controls, scopes, or limits.

## Decision Gates

1. Prefer native Creator statements for permitted same-application data access.
2. Use only the exact five v2 integration tasks when their operation and semantics fit.
3. Use REST for delete, v2.1 behavior, metadata, files, publish, bulk read, or unsupported task capabilities.
4. Ask only for missing facts that change routing or prevent safe generation.

## Output Contract

State the selected surface, host and placement, application/form/report identities, version, environment, authentication, scopes, workflow effects, confirmation state, and unresolved runtime facts.

## References

- [Routing and Creator-to-Analytics](references/routing.md)
- [Native Creator Deluge](references/native-deluge.md)
- [Creator integration tasks v2](references/integration-tasks-v2.md)
- [Identity, authentication, and environments](references/identity-auth-and-environments.md)
- [Uncertainty and safe exclusions](references/uncertainty-and-errors.md)
- [Official sources](references/sources.md)
