---
name: zoho-projects
display_name: "Zoho Projects"
description: "Trigger: Zoho Projects automation, Deluge tasks, REST v3/v3.1, portals, projects, tasks, issues, time logs, metadata, files, or lifecycle operations. Route Projects work through evidence-backed operations."
scope: optional
---

## Activation Contract

Load this skill whenever Zoho Projects is a host or target. Load `zoho-deluge` only for actual Deluge output. Load every other target application skill for cross-product work.

## Hard Rules

- Use only an exact closed-catalog Deluge task or a verified operation-level REST record. A family aggregate never authorizes an endpoint.
- Never rewrite paths among `/restapi`, `/api/v3`, and `/api/v3.1`. Warn and obtain consent before migrating legacy code.
- Resolve hierarchy IDs, metadata, scopes, plans, permissions, pagination, and placement from the exact operation and runtime.
- Fail closed on `contradictory` or `unavailable` evidence. Stop generation and execution until runtime authority verifies every `TBD` metadata value. If runtime authority cannot verify the missing metadata, return unsupported.
- Require confirmation immediately before destructive actions. Never request or embed secrets.

## Decision Gates

| Situation | Action |
| --- | --- |
| Exact Deluge task match | Use the closed task signature with its progressive context. |
| Verified REST catalog row | Use that exact method, path, version, hierarchy, and scopes. |
| Contradictory or unavailable evidence | Stop; request approved policy or report unsupported behavior. |
| `TBD` metadata | Stop generation and execution until runtime authority verifies the missing metadata; return unsupported when it cannot. |
| Several equivalent routes | Recommend one and wait for selection. |
| Destructive operation | Resolve target/effect/recovery, then request confirmation. |

## Execution Steps

1. Identify the host, targets, execution context, requested operation, and output placement.
2. Check standard Projects functionality and state any non-blocking alternative.
3. Select one catalog record or exact Deluge task; reject incompatible context, version, plan, or availability.
4. Resolve runtime IDs, permissions, plan, pagination, connections, and cross-product skills.
5. Return the output contract; do not generate from family aggregates or unresolved evidence.

## Output Contract

Return the selected surface and operation/version; placement/configuration; argument mapping; hierarchy IDs; named connections and scopes; pagination; plan/permission unknowns; standard-feature warning; expected result; and unsupported boundaries.

- **Assumptions:** state each inferred or runtime-dependent fact, mark unresolved facts `TBD`, and identify required validation.
- **Expected test outcomes:** pair each generated test case with its expected result; state when no verifiable runtime exists.

## References

- [Routing and dependencies](references/routing.md)
- [Closed Deluge task catalog](references/deluge-tasks.md)
- [Current REST family manifest](references/current-rest-api.md)
- [Current REST operation catalog](references/current-rest-operations.csv)
- [Identifiers and metadata](references/identifiers-and-metadata.md)
- [Authentication, limits, and plans](references/authentication-limits-and-plans.md)
- [Lifecycle and uncertainty](references/lifecycle-and-uncertainty.md)
- [Official sources](references/sources.md)
