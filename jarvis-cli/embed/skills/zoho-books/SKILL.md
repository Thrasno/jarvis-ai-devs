---
name: zoho-books
display_name: "Zoho Books"
description: "Trigger: writing, reviewing, debugging, or planning Zoho Books automation, Books Deluge Integration Tasks, REST API v3, transactions, resources, fields, extensions, or widgets. Evidence-backed Books routing and generation safety."
scope: optional
---

# Zoho Books

Use this skill for Books application facts whether Books is the host, target, or a cross-application participant. Load a language skill only when that language is actually used; load `zoho-deluge` for Deluge.

## Activation Contract

- Keep application ownership and language ownership orthogonal.
- Preserve the requested language, runtime, placement, and output extension.
- This skill generates guidance and code; it does not execute live Books operations.

## Hard Rules

- Check bounded standard Books functionality first; advice never blocks an explicit code request.
- Prefer a fully compatible Books Integration Task before REST v3. A task miss does not select REST automatically.
- Treat static catalogs as recognition hints. Runtime organization metadata and operation evidence decide availability and safe shape.
- Default named authentication to `conpas_books`; never request or emit secrets.
- Use REST v3 for new REST code. Never migrate legacy calls without explicit confirmation.

## Decision Gates

1. Identify host, targets, behavior, language, context, placement, and generation versus execution.
2. Route through standard capability, compatible Integration Task, REST v3, another verified surface, then manual or unsupported outcome.
3. Ask only for missing facts that change routing or prevent safe generation. Recommend one of equally optimal paths and wait.

## Output Contract

State the selected surface, language, placement/configuration, arguments, connection, assumptions, runtime prerequisites, standard advisory, and tests with expected outcomes. Keep unknowns explicit.

## References

- [Routing](references/routing.md)
- [Integration Tasks](references/integration-tasks.md)
- [REST v3](references/rest-v3.md)
- [Standard capabilities](references/standard-capabilities.md)
- [Standard resources and fields](references/standard-resources-and-fields.md)
- [Runtime safety and output](references/runtime-safety-and-output.md)
- [Sources](references/sources.md)
