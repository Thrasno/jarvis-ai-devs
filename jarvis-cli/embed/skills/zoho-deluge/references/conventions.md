# Style Conventions

Verified: 2026-08-16. The Deluge syntax used in the examples below was checked against the official language reference: https://www.zoho.com/deluge/help

Function definition and `return` syntax follows the documented form: https://www.zoho.com/deluge/help/misc-statements/return-statement.html

Provenance note: the style rules themselves are project conventions. No official Zoho documentation prescribes them, and none is claimed. They are stated on maintainer authority, and they apply to Deluge written under this skill.

The hard comment-placement rule lives in SKILL.md, in the critical rules and in the universal no-gos, because it is enforced on every review. The nine conventions below are the style layer around it.

## 1. Good code explains itself

A correct name replaces a comment. Reach for a better name before reaching for an explanation.

```deluge
// Weak: the comment carries what the name should.
d = calc(x, y);

// Strong.
outstandingBalance = subtractPayments(invoiceTotal, receivedPayments);
```

## 2. Guard clauses first

Validate and exit early. Nesting the happy path inside three conditionals hides it.

```deluge
map registerAttendee(map attendee)
{
	// Rejects an incomplete attendee before any work happens.
	if(isNull(attendee))
	{
		return {"ok": false, "reason": "attendee is required"};
	}
	if(isBlank(attendee.get("email")))
	{
		return {"ok": false, "reason": "email is required"};
	}
	return {"ok": true, "email": attendee.get("email").trim().toLowerCase()};
}
```

## 3. One responsibility per function

A function that validates, transforms, calls out, and formats is four functions. Split it. Some applications restrict calling a function from within a function; that restriction is declared in the relevant `zoho-[app]` skill, not here.

## 4. Explicit return on every exit path

Every path returns, and every return on that path has the same shape, so the caller never infers. `return` requires a declared return type on the function, so declare one whenever the caller needs a value.

```deluge
{"ok": true, "value": someValue}
{"ok": false, "reason": "why it failed"}
```

## 5. Declare collections explicitly

Never invoke methods directly on Map/List literals; assign the literal to a variable, then invoke the method. This is a generic compatibility convention, not a universal documented parser rejection. Assigned literals remain valid; `Map()` plus `put()` and `List()` plus `add()` are optional alternatives, not required replacements.

```deluge
codes = {"alpha", "beta"};
hasAlpha = codes.contains("alpha");
result = {"ok": true, "count": 2};
resultText = result.toString();
```

Avoid direct receivers such as `{"alpha", "beta"}.contains("alpha")` and `{"ok": true}.toString()`. Text literal methods remain valid, for example `" alpha ".trim()`; the receiver convention is scoped to Map/List collections.

## 6. Filter and deduplicate before iterating

Reduce the collection first, then loop over what is left. A loop body full of `continue` guards is a filter written in the wrong place.

## 7. camelCase by default

Variables and functions in camelCase: `invoiceTotal`, `buildContactPayload`. Stay consistent within a script.

## 8. Domain names, no cryptic abbreviations

Name the concept, not its abbreviation. `attendeeEmail`, not `attEml`. `pendingInvoices`, not `pInv`. Single letters are acceptable only as an index variable in a documented `for each index` loop.

## 9. Log the payload and the response

Every integration task call and every `invokeUrl` logs an `info` of the payload and an `info` of the response. It serves debugging, it serves the audit trail, and it makes the response shape observable instead of assumed. Never log a credential.

```deluge
info "create attendee payload: " + payload.toString();
info "create attendee response: " + response.toString();
```

## Review checklist

- Is any comment sitting above a function definition?
- Does every comment earn its place, or would a better name remove it?
- Are there traceability or tracking identifiers in the comments?
- Are Map/List literals assigned to variables before invoking methods on them?
- Does every exit path return the same shape?
- Are the guards at the top, or buried in nesting?
- Are payload and response logged for every call that leaves the script?
- Is any credential written as a literal?
- Is any application fact — a field, a task, a limit — asserted without a source?
