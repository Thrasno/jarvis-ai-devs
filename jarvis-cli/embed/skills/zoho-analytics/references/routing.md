# Analytics routing

Route independently by source host, target organization/workspace, capability, required cadence, volume, identity form, connection, OAuth scope, units, and code placement.

Zoho Analytics is an API/integration target and has no local Deluge execution runtime. A `zoho.reports.*` task runs from another verified Deluge-capable host; external runtimes retain the requested language and placement.

## Creator Advanced Analytics invariant

The standard **Zoho Creator Advanced Analytics** connector automatically synchronises Creator data for Analytics reports, dashboards, sharing, publishing, and embedding. Its exact cadence, entity coverage, plan gate, conflict handling, and failure/recovery semantics were not established by the accessible evidence.

Routing precedence:

1. Use the standard Zoho Creator Advanced Analytics connector for Creator reporting/dashboard use cases when its verified capabilities and required cadence are sufficient.
2. Use the exact `zoho.reports.*` task allowlist for immediate event-driven row mutations from a verified Deluge host.
3. Use bulk REST APIs for bulk synchronisation/import/export.
4. Use asynchronous REST SQL export only when SQL is genuinely required by the included verified REST surface; use Query Tables for persistent analytical models. CloudSQL is out of V0 scope.
5. Return unsupported when no verified surface satisfies the request.

Never describe the standard connector as real-time until cadence is verified. Every custom route must state source host, target organization/workspace, identity form, connection, scope, units, and code placement.

## General surface selection

| Need | Surface |
|---|---|
| Immediate create/update/delete rows from verified Deluge host | Exact `zoho.reports.*` task when its name-based identity fits. |
| REST row mutation | Data API with `orgId`, `workspaceId`, and `viewId`. |
| Bulk import/export or SQL export | Bulk API; preserve synchronous versus asynchronous behavior. |
| Workspace/view/query-table/report schema | Modeling API. |
| Discovery and current tenant facts | Metadata API. |
| Sharing, publishing/embed, or users | Corresponding REST family with explicit write confirmation. |

If one verified route remains, explain and use it. If equally suitable routes remain, recommend one and wait for selection. Never route through a contradictory operation without newly verified official evidence.
