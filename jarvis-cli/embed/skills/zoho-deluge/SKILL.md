---
name: zoho-deluge
display_name: "Zoho Deluge"
description: "Trigger: writing, reviewing, debugging, or optimizing Deluge in any Zoho application, including files named .dg, .deluge, or .ds. Application-neutral Deluge language core — routes application specifics to the matching zoho-[app] skill."
scope: optional
---

# Zoho Deluge — Language Core

The application-neutral Deluge foundation: syntax, collections, types, dates, null and type safety, error handling, and `invokeUrl` fundamentals.

Modules, fields, integration task names and signatures, endpoints, scopes, authentication, response contracts, and every numeric limit belong to the matching `zoho-[app]` skill. This skill never states them and never guesses them.

## Activation Contract

Activates when writing, reviewing, debugging, or optimizing Deluge in any Zoho application, including files named `.dg`, `.deluge`, or `.ds`.

This skill co-activates with an application skill; it does not replace one. Working on a specific application normally means both are loaded: `zoho-deluge` supplies the language, `zoho-[app]` supplies the catalog. If only this skill is loaded and the work needs an application fact, follow the escalation chain instead of inventing the fact.

## Decision Gates

### Gate 1 — Is this a language question or an application question?

| The question is about | Owner |
|---|---|
| Syntax, control flow, Map, List, string, date, type conversion, null safety, `try`/`catch`, `invokeUrl` | This skill |
| Module, field, task name or signature, endpoint, scope, connection catalog, response shape, any limit or quota | `zoho-[app]` |
| A fact neither skill carries | The user — ask before writing code |

### Gate 2 — Which mechanism?

1. **Native application action.** If the requested behavior already exists as a standard action of the application, say so, then implement what was asked. The user's request wins; this is advice, not a block.
2. **Integration task.** When the application exposes a task for the operation, prefer it over a raw HTTP call to the same application API.
3. **`invokeUrl`.** For services with no task, and for every non-Zoho endpoint. Always through a connection when the service needs authentication.

The availability of a native action or a task is an application fact. If it is not in the loaded `zoho-[app]` skill, escalate; do not assume.

## Escalation Chain

Three levels, in order. Never skip a level, and never fill a gap with a guess.

1. **Language.** `zoho-deluge` answers it.
2. **Application.** Route the specific to the matching `zoho-[app]` skill.
3. **User.** If the application skill does not carry the fact — or was never loaded at all — stop and ask the user before writing code.

## Critical Rules

- **Function signatures are typed.** A definition declares a return type, a name, and typed parameters, and the body is enclosed in braces: `<returnType> <name>(<type> <param>, <type> <param>) { ... }`. `return` is legal only when a return type was declared. See https://www.zoho.com/deluge/help/misc-statements/return-statement.html
- **Comment placement.** Never place a comment above a function definition. The comment explaining a function is brief and sits immediately below the definition. This is a hard project rule, not a documented language constraint.
- **Guard before you chain.** Validate a value before applying text, date, or conversion functions to it.
- **Explicit returns.** Every exit path returns, and every return on the same path has the same shape.
- **Observability.** Every integration task call and every `invokeUrl` logs an `info` of the payload and an `info` of the response, so the response shape is observed instead of assumed.

```deluge
map buildContactPayload(string fullName, string email)
{
	// Normalizes the input pair into the payload shape the caller sends.
	if(isBlank(fullName) || isBlank(email))
	{
		return {"ok": false, "reason": "name and email are required"};
	}
	payload = Map();
	payload.put("name", fullName.trim());
	payload.put("email", email.trim().toLowerCase());
	return {"ok": true, "payload": payload};
}
```

## Universal No-Gos

1. Never invent a task name, signature, module, field, endpoint, scope, or comparator. Follow the escalation chain.
2. Never assume a response shape, success or error. Verify it against the application contract, then escalate.
3. Never present an application limit as a language rule.
4. Never reach for `invokeUrl` against an application API when an integration task exists for the same operation.
5. Never hardcode credentials. Use a connection. For a non-Zoho service without one, guide the user toward environment configuration or a shared secret location — never a literal token in the script.
6. Avoid quota-consuming calls inside loops where you can, but implement them when there is no alternative. Cost does not dictate code shape.
7. Never chain a text or date function onto an unguarded value.
8. Never convert types on unvalidated input.
9. Never place a comment above a function definition; the brief explaining comment sits immediately below it.
10. Never put memory, tracking, or traceability identifiers in comments.
11. Never comment for the sake of commenting. A correct name replaces a comment.
12. Never leave an empty `catch`, and never swallow an error without logging or propagating it.
13. Reserve `try`/`catch` for uncontrollable calls that cross the boundary out of the current application.

## Cost Heuristics — Advisory

These inform the design; they never forbid a shape the user asked for.

- Call cost scales linearly with iterations. One call in a loop of N is N calls.
- The real decision axis is bounded versus unbounded iteration, not nested versus flat. A nested loop over two small bounded collections is fine; a single flat loop over an unbounded result set is not.
- Collect, then batch: gather the identifiers or records first, then make one call, when the application offers a batched operation.
- Filter and deduplicate before iterating, not inside the loop body.
- The ceiling — statements, credits, quotas, batch size, page size — belongs to `zoho-[app]`. Ask it, or ask the user.

## Execution Contract

1. Classify the request through Gate 1. Route or escalate before writing code.
2. Choose the mechanism through Gate 2. If a native action is more optimal, say so, then implement the request.
3. Guard inputs first, with early returns.
4. Write one responsibility per function, with domain names in camelCase.
5. Log payload and response for every call that leaves the script.
6. Wrap only boundary-crossing calls in `try`/`catch`, and handle the error.

## Output Contract

- Deluge code, ready to paste, with no invented application identifier.
- Every application-specific value either sourced from the `zoho-[app]` skill or flagged as a question for the user.
- A one-line note when a native action would be more optimal than the code produced.
- No numeric limit asserted by this skill.

## Reference Routing

| Load when the work involves | Reference |
|---|---|
| Map and List construction, access, iteration, key and membership checks | [references/collections.md](references/collections.md) |
| Type conversion, date arithmetic and formatting, `isNull` vs `isBlank` vs `isEmpty` | [references/types-and-dates.md](references/types-and-dates.md) |
| `try`/`catch`, inspecting a call result, `info` discipline | [references/error-handling.md](references/error-handling.md) |
| HTTP calls, connections, headers, parameters, file handling | [references/invokeurl.md](references/invokeurl.md) |
| Naming, structure, comments, and style review | [references/conventions.md](references/conventions.md) |
