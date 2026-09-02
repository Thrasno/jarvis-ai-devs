---
name: zoho-people
display_name: "Zoho People"
description: "Trigger: writing, reviewing, debugging, or planning Zoho People automation, People Deluge, REST APIs, forms, attendance, leave, timesheets, LMS, files, performance, surveys, or HR processes. Evidence-backed People routing and safety guidance."
scope: optional
---

# Zoho People

Use this skill whenever People is a host or target application. Load `zoho-deluge` only when the generated code is Deluge; this skill supplies People product facts, API routing, identifiers, and safety boundaries.

## Activation Contract

- Identify host, target, execution context, requested behavior, API family, exact operation, plan, and required runtime identifiers before generation.
- Cross-application work loads every involved application skill. Never assume identifiers interoperate across Zoho products.
- Prefer one of the four native People tasks only for an exact allowlist match and supported connection semantics; otherwise evaluate current REST, another documented surface, standard People functionality, or an explicit unsupported result.

## Hard Rules

- Keep v1/v2 and v3 operations separate. Never rewrite a route to change versions; use the exact official operation page.
- runtime metadata is authoritative for forms, views, fields, components, sections, records, specialized IDs, permissions, plan access, and limits.
- Evidence states are `verified`, `contradictory`, `unavailable`, and `TBD`. The latter three fail closed for operation generation.
- Use OAuth 2.0 named connections. Never request, expose, or embed credentials.
- Require explicit confirmation before bulk or destructive changes and before any mutation the user has not already authorized; state the exact target and impact.
- minimize personal data. Use placeholders in examples and exclude sensitive employee data from code, output, and logs unless required and authorized.

## Decision Gates

1. Check standard People functionality before custom code without blocking valid requested work.
2. Ask only for facts that change routing or prevent safe generation.
3. Use and explain one verified path. If verified paths are equally optimal, recommend one and wait for selection.

## Output Contract

State the selected surface, API family, operation, placement, language, identifiers, scopes, plan/limit uncertainty, connection, assumptions, expected test outcome, and unsupported boundaries.

## References

- [Routing](references/routing.md)
- [Deluge tasks](references/deluge-tasks.md)
- [REST v1/v2 catalog](references/rest-v1-v2-catalog.md)
- [REST v3 catalog](references/rest-v3-catalog.md)
- [Entities and identifiers](references/entity-identifiers.md)
- [Authentication, limits, and plans](references/authentication-limits-and-plans.md)
- [Lifecycle operations and webhooks](references/lifecycle-and-webhooks.md)
- [Uncertainty and errors](references/uncertainty-and-errors.md)
- [Sources](references/sources.md)
