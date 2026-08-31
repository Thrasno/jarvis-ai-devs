# REST API V8 and structural limits

Use REST API V8 for verified operations outside the closed Deluge task catalog, external runtimes, COQL, metadata, or bulk lifecycles. The API domain is regional: use an explicitly supplied or documented placeholder domain; never infer it.

Use module and field API names, exact operation scopes, and runtime metadata. Keep REST response handling endpoint-specific; its `data` conventions do not transfer to Deluge tasks.

Batch, paginate, select only needed fields, avoid redundant metadata reads, and avoid calls inside loops where a verified batch operation exists. Avoid unnecessary parallelism: concurrency is a qualitative operational risk, not a tenant threshold.

Guidance must not include quotas, credit formulas, capacity estimates, numeric timeouts, or exact concurrency thresholds. Ask runtime metadata or the administrator when those facts decide safety.
