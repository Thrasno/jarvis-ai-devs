# invokeUrl

Verified: 2026-08-16 against the official Deluge documentation. Every URL below was requested and answered on that date.

Sources:

- `invokeUrl` task: https://www.zoho.com/deluge/help/webhook/invokeurl-api-task.html
- Connections: https://www.zoho.com/deluge/help/deluge-connections.html
- Try-catch: https://www.zoho.com/deluge/help/misc-statements/try-catch.html

## When to use it

`invokeUrl` is the HTTP client of the language. Use it for any service that does not expose an integration task for the operation, and for every non-Zoho endpoint.

When the application does expose an integration task for the same operation, prefer the task. The task carries authentication, the response contract, and the application semantics; a raw HTTP call re-implements all three by hand.

## Syntax

```deluge
response = invokeUrl
[
	url: "https://api.example.com/v1/orders"
	type: POST
	headers: {"Content-Type": "application/json"}
	body: payload
	connection: "example_service"
];
```

| Key | Purpose |
|---|---|
| `url` | Absolute endpoint. Required |
| `type` | `GET`, `POST`, `PUT`, `PATCH`, `DELETE`, `OPTIONS`. Optional; defaults to `GET` |
| `headers` | KEY-VALUE of request headers |
| `body` | The request body, as TEXT, FILE, or KEY-VALUE. The documented way to send a body |
| `parameters` | The body when `body` is not used. For `GET` and `DELETE` it behaves as query parameters |
| `files` | FILE, or a list of them, for multipart form-data |
| `connection` | Link name of the configured connection that supplies authentication |
| `detailed` | BOOLEAN. `true` returns status code, response header, and content as KEY-VALUE; `false` returns the content. Defaults to `false` |
| `response-format` | `NONE`, `STRING`, or `FILE`. Controls how the response is handled |
| `response-decoding` | Character encoding used to decode the response. Defaults to UTF-8 |

The keys are written without commas between them, inside square brackets. `body` and `parameters` are mutually exclusive — never pass both in the same call.

There is no `content-type` key. The content type travels in `headers`, and it must match the data format of the body.

## Connections, never literal credentials

Authentication belongs to a connection configured outside the script. Reference it by its link name.

```deluge
response = invokeUrl
[
	url: "https://api.example.com/v1/orders"
	type: GET
	connection: "example_service"
];
```

For a service with no connection available, guide the user toward environment configuration or the shared secret location the team already uses. Never write a token, key, password, or bearer value as a literal in the script, not even as a placeholder that "will be replaced later".

## GET with query parameters

```deluge
query = Map();
query.put("status", "open");
query.put("page", "1");

response = invokeUrl
[
	url: "https://api.example.com/v1/orders"
	type: GET
	parameters: query
	connection: "example_service"
];
```

For a `GET`, the values are appended to the URL as query parameters rather than sent in the body.

## POST with a JSON body

Declare the content type in the headers so it matches the body format.

```deluge
payload = Map();
payload.put("reference", reference);
payload.put("total", total);

info "create order payload: " + payload.toString();

response = invokeUrl
[
	url: "https://api.example.com/v1/orders"
	type: POST
	headers: {"Content-Type": "application/json"}
	body: payload
	connection: "example_service"
];

info "create order response: " + response.toString();
```

A KEY-VALUE body defaults to `multipart/form-data`; a TEXT body defaults to `text/plain`; a FILE body defaults to `application/octet-stream`. An explicit `Content-Type` header overrides the default.

## Reading the status code

Without `detailed`, the return value is the response content. With `detailed: true` the response is a KEY-VALUE carrying three documented keys — `responseCode`, `responseText`, and `responseHeader`.

```deluge
response = invokeUrl
[
	url: "https://api.example.com/v1/orders/" + orderId
	type: GET
	connection: "example_service"
	detailed: true
];

statusCode = response.get("responseCode");
if(statusCode != 200)
{
	info "order fetch returned " + statusCode;
	return {"ok": false, "reason": "unexpected status " + statusCode};
}
body = response.get("responseText");
contentType = response.get("responseHeader").get("content-type");
```

## Wrap the call, log both sides

An HTTP call crosses the boundary out of the application, so it is exactly the case `try`/`catch` is for. Log the payload before and the response after.

```deluge
try
{
	info "payload: " + payload.toString();
	response = invokeUrl
	[
		url: "https://api.example.com/v1/orders"
		type: POST
		headers: {"Content-Type": "application/json"}
		body: payload
		connection: "example_service"
	];
	info "response: " + response.toString();
}
catch(e)
{
	info "order call failed at line " + e.lineNo + ": " + e.message;
	return {"ok": false, "reason": e.message};
}
```

## Calls inside loops

One call per iteration is N calls: the documentation is explicit that a task inside a `for each` that runs five times consumes five calls, even though it appears once in the script. Prefer collecting the identifiers first and making a single batched call when the service offers one. When it does not, and the work genuinely requires a call per item, write the loop — cost does not dictate code shape.

Every applicable ceiling — external call allowance, upload and download sizes, request timeout — is an application fact and differs per service and plan. Ask the loaded `zoho-[app]` skill, or the user.
