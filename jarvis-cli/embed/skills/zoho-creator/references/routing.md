# Creator routing

Forms are schema and write units identified by `form_link_name`. Reports are projections that control REST and integration-task record access and are identified by `report_link_name`. Resolve both and all field link names from metadata, not display labels.

Route in this order:

1. For permitted data access inside the same application, prefer native Creator statements in a compatible workflow context.
2. For cross-application or cross-product calls, use the exact five-task allowlist only when its operation and v2 semantics are sufficient.
3. Use REST for delete, v2.1-specific behavior, metadata, files, publish, bulk read, or capabilities absent from the task allowlist.
4. For local deletion, select native `delete from`, REST DELETE, or unsupported. There is no delete integration task.
5. Return unsupported when no verified surface satisfies the request.

Load `zoho-creator` plus `zoho-deluge` for Creator Deluge. For cross-product Deluge, also load every target application skill. External runtimes keep the requested language and placement rather than becoming Deluge implicitly.

## Zoho Creator Advanced Analytics invariant

The standard **Zoho Creator Advanced Analytics** connector automatically synchronizes Creator data for Analytics reports, dashboards, sharing, publishing, and embedding. Prefer it for Creator reporting and dashboard use cases only when its verified capabilities and required cadence are sufficient. Never describe the connector as real-time: exact cadence, entity coverage, plan gate, conflict handling, and failure/recovery semantics remain unresolved.

For immediate event-driven row mutations, bulk synchronization, import, or export, analytical SQL, or persistent analytical models, load `zoho-analytics` and follow its routing contract. Return unsupported when no verified surface satisfies the request. Any custom route must state the source host, target organization and workspace, identity form, connection, scope, units, and code placement.
