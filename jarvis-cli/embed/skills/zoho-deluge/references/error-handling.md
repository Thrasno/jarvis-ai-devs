# Error Handling

Verified: 2026-08-16 against the official Deluge documentation. Every URL below was requested and answered on that date.

Sources:

- Try-catch statement: https://www.zoho.com/deluge/help/misc-statements/try-catch.html
- `info` statement: https://www.zoho.com/deluge/help/debug/info.html
- Return statement and function syntax: https://www.zoho.com/deluge/help/misc-statements/return-statement.html
- `invokeUrl` task: https://www.zoho.com/deluge/help/webhook/invokeurl-api-task.html

## Deluge has `try`/`catch`

The exception variable exposes `message` and `lineNo`.

```deluge
try
{
	response = invokeUrl
	[
		url: "https://api.example.com/v1/orders"
		type: GET
		connection: "example_service"
	];
	info "orders response: " + response.toString();
}
catch(e)
{
	info "orders call failed at line " + e.lineNo + ": " + e.message;
	return {"ok": false, "reason": e.message};
}
```

## Reserve `try`/`catch` for the boundary

Wrap calls you do not control and that leave the current application: `invokeUrl` against an external service, an integration task against another product, a parse of a payload someone else produced.

Do not wrap ordinary in-script logic in `try`/`catch`. A guard clause is the correct tool for a value you can check.

```deluge
// Guard, not catch.
if(isBlank(customerEmail))
{
	return {"ok": false, "reason": "customer email is required"};
}
```

## Never swallow an error

An empty `catch` turns a failure into a silent wrong result. Every `catch` either logs and returns a failure shape, or re-raises the condition to the caller through the return value.

```deluge
catch(e)
{
	info "sync failed: " + e.message;
	return {"ok": false, "reason": e.message};
}
```

`return` is legal only inside a function that declared a return type. In a script fragment with no return type, log the failure and let the control flow fall through instead.

## Inspect the result; do not assume its shape

The response contract of an integration task belongs to the application, not to the language. Do not code against a success or error key you have not confirmed in the loaded `zoho-[app]` skill. If the fact is not there, ask the user before writing the check.

The safe language-level pattern is: log the payload, log the response, verify the key you intend to read is present, then branch.

```deluge
info "payload: " + payload.toString();
response = invokeUrl
[
	url: "https://api.example.com/v1/orders"
	type: POST
	body: payload
	headers: {"Content-Type": "application/json"}
	connection: "example_service"
];
info "response: " + response.toString();

if(isNull(response))
{
	return {"ok": false, "reason": "no response"};
}
if(!response.containKey("id"))
{
	return {"ok": false, "reason": "unexpected response shape", "details": response};
}
return {"ok": true, "id": response.get("id")};
```

## `info` discipline

Every integration task call and every `invokeUrl` logs an `info` of the payload before the call and an `info` of the response after it. This serves debugging and makes the response shape observable instead of assumed.

- Log the payload and the response, not a running commentary of the script.
- Never log a credential, a token, or a connection secret.
- Keep the message identifiable: name the operation, then the value.

Where `info` output surfaces, and any size ceiling on it, is an application fact. Check the loaded `zoho-[app]` skill or ask the user.

## Return the same shape on every path

A caller must never infer what came back. A function declares its return type, its typed parameters, and braces around the body.

```deluge
map processOrder(map order)
{
	// Returns the same result shape on every exit path.
	if(isNull(order))
	{
		return {"ok": false, "reason": "order is required"};
	}
	if(isBlank(order.get("reference")))
	{
		return {"ok": false, "reason": "reference is required"};
	}
	return {"ok": true, "reference": order.get("reference")};
}
```
