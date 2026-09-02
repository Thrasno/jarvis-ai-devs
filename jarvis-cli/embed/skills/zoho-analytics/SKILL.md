---
name: zoho-analytics
display_name: "Zoho Analytics"
description: "Trigger: Zoho Analytics workspaces, views, data, imports, exports, reports, dashboards, SQL, REST v2, Deluge reports tasks, or Creator analytics. Evidence-backed Analytics routing and safety guidance."
scope: optional
---

# Zoho Analytics

## Activation Contract

Use this skill for Zoho Analytics target facts, routing, and API constraints. When a verified Deluge-capable host invokes Analytics, also load `zoho-deluge`; that skill owns Deluge grammar while this skill owns Analytics task semantics. External runtimes retain their requested language and placement.

## Hard Rules

- Treat Analytics as an API/integration target with no local Deluge runtime. Use only the three verified `zoho.reports.*` tasks; there is no read task.
- Use OAuth 2.0, the correct data-centre API domain, exact operation scope, `orgId`, `workspaceId`, and operation-specific IDs. Discover identifiers through metadata; never guess them.
- Obtain write confirmation before data, modeling, sharing, embed, user-management, or other mutations. State scope and known API-unit impact first.
- Preserve asynchronous job creation, polling, download, and optional callback behavior. A job callback is not a general webhook/event system.
- Keep REST SQL export and persistent Query Tables distinct. CloudSQL JDBC is outside this routing and execution-contract snapshot.
- Fail closed on contradictory paths/scopes. Warn about unknown plan gates, quotas, and unlisted costs; require runtime validation.

## Decision Gates

1. Identify source host, target organization/workspace, capability, cadence, volume, identity form, connection, and placement.
2. For Creator reporting/dashboard use cases, prefer the standard Creator Advanced Analytics connector when its verified capability and cadence suffice.
3. Use an exact Deluge task for immediate row mutation from a verified host; use REST for verified reads, metadata, bulk, modeling, sharing, embed, user-management, and other operations.
4. Use asynchronous REST SQL export only for a genuine included SQL-read need; use Query Tables for persistent analytical models.
5. Return unsupported when no verified non-contradictory surface satisfies the request.

## Execution Steps

1. Select the surface with [routing](references/routing.md).
2. Resolve [entities and identifiers](references/entities-and-identifiers.md), then check authentication, scopes, units, and limits.
3. For REST, verify the exact method, path, and scope against current official documentation. Confirm writes before generation or execution.
4. State polling/callback behavior and every unresolved runtime fact.

## Output Contract

State the selected surface, source host, placement/language, target IDs, connection, OAuth scope, unit/limit implications, asynchronous lifecycle, confirmation state, and unresolved facts. Never invent operations, capabilities, costs, plans, identifiers, or real-time guarantees.

## References

- [Routing and Creator connector](references/routing.md)
- [Entities and identifiers](references/entities-and-identifiers.md)
- [Deluge task allowlist](references/deluge-tasks.md)
- [Asynchronous jobs and SQL boundaries](references/async-and-sql.md)
- [Authentication and limits](references/authentication-and-limits.md)
- [Uncertainty and safe errors](references/uncertainty-and-errors.md)
- [Sources](references/sources.md)
