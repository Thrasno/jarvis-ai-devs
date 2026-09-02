# Uncertainty, confirmation, and safe errors

Ask only for facts that change routing or prevent safe generation: source host, organization/workspace, IDs or names required by the chosen surface, cadence, volume, connection, data centre, scope, placement, and write intent.

Require explicit write confirmation immediately before any data, modeling, sharing, embed, user-management, datasource-sync, or other mutation. Summarize target identity, operation, scope, known units, and material effects first. Read/discovery planning does not need write confirmation.

Keep these non-decision states explicit:

- Keep contradictory operation paths or scopes explicit and fail closed until newly verified official evidence resolves them.
- Keep unknown plan gates, quotas, and unlisted costs explicit; warn and require runtime validation, and never invent values.
- Connector cadence, entity coverage, plan gate, conflict handling, and failure/recovery semantics remain `TBD`; never promise real-time behavior.
- Async callbacks are job notifications, not a general event/webhook capability.
- CloudSQL JDBC is excluded from this routing and execution-contract snapshot.

When a verified surface cannot satisfy the request, return unsupported and explain the missing capability or evidence. Offer a safe clarification or verified alternative without silently changing host, identity model, cadence, or semantics.
