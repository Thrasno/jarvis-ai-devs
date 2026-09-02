# Asynchronous jobs and SQL boundaries

## Import and export jobs

Asynchronous import/export creation returns a `jobId`. Persist it, poll the matching import-job or export-job endpoint until a terminal state, and download an export only after successful completion. Do not assume immediate data availability or treat job creation as completion.

Asynchronous import/export may include `callbackUrl`; Analytics sends an HTTP POST on success or failure. This is a job notification, not evidence of a general Analytics event/webhook subsystem. Design polling as the authoritative fallback when callback delivery or recovery semantics are not established.

## Three distinct SQL surfaces

1. **Ad-hoc REST SQL read/export — included and verified.** Use `GET /restapi/v2/bulk/workspaces/{workspace-id}/data` with `CONFIG.sqlQuery` containing a SQL `SELECT`. Scope is `ZohoAnalytics.data.read`. The operation is asynchronous: retain `jobId`, poll, then download, or use the optional job callback.
2. **Persistent Query Table modeling — included and verified.** Use `POST /restapi/v2/workspaces/{workspace-id}/querytables`; it creates a persistent Query Table view and returns `viewId`. Scope is `ZohoAnalytics.modeling.create`.
3. **CloudSQL JDBC — evidence retained only as excluded.** It requires the official JDBC driver and OAuth credentials and supports separately documented SQL behavior. CloudSQL JDBC is outside V0 routing, generation, and catalog scope.

Never collapse these surfaces. Use asynchronous REST SQL export only when SQL is genuinely required for an included read/export; use Query Tables for durable analytical models. Do not generate CloudSQL URLs, SQL, or connection code in V0.
