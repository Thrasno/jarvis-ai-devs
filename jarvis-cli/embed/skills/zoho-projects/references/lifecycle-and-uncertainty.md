# Lifecycle, destructive actions, and uncertainty

Treat lifecycle and non-CRUD operations as first-class exact operations: trash and restore; activate and deactivate; clone, move, and reorder; follow and unfollow; link and unlink; associate and disassociate; import and export; timers and pins; default and favourite selection; status history; blueprint transitions; custom-function execution; and asynchronous bulk jobs. Never translate them into generic create, update, or delete calls.

Before any destructive or hard-to-reverse action, summarize the exact target, parent portal/project, effect, and recovery evidence, then require explicit confirmation immediately before execution. This includes delete, trash when retention is uncertain, unlink/disassociate when data loss is possible, destructive moves, and bulk mutations. A prior general request is not confirmation for a newly resolved target.

Events are project calendar events and comments, not webhook subscriptions. Product automation mentions webhooks, workflows, alerts, and macros, but webhook management endpoints, triggers, payloads, signatures, retries, and plan gates remain `TBD`. Do not generate a webhook-management endpoint; identify manual/product configuration and only verified behavior.

Classify evidence as `verified`, `contradictory`, `unavailable`, or `TBD`:

- `verified`: use only the exact evidenced operation and runtime facts.
- `contradictory`: stop and request approved method/path/version/scope policy.
- `unavailable`: report the operation unsupported; offer a verified alternative.
- `TBD`: warn clearly and require runtime validation before relying on the fact.

Ask only for missing facts that change routing or prevent safe generation. Do not guess methods, paths, versions, IDs, scopes, plans, permissions, pagination, costs, response shapes, or cross-product interoperability. Preserve known permission failures. When one verified route remains, use and explain it; when several are equivalent, recommend one and wait for selection.
