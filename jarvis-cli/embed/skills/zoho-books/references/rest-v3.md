# Zoho Books REST API v3

The approved 2026-08-31 snapshot closes at 849 operations across 42 resources: GET 263, POST 338, PUT 131, and DELETE 117. The official OpenAPI/index reconciliation has zero operation omissions. Source ZIP SHA-256: `c6a841bbc81ef882c64b1f2ad4761e350faefba1db7fb36a32daf7112b647559`.

Use the selected official operation contract for its exact method, path, parameters, request/response schemas, OAuth scopes, and regional domain. Of the snapshot operations, 841 require `organization_id` and 8 omit it. Therefore `organization_id` is a per-operation prerequisite, never universal ambient context. Do not inject it where the operation does not publish it; `email_sales_receipt` is an explicit anomalous omission.

New REST code uses Books API v3. For legacy REST review or modification, identify the current API version, warn about migration impact, and ask one focused question: migrate or preserve? Never change API versions without explicit confirmation. If migration is declined or not approved, preserve the existing version and calls in that function. Integration Tasks are a separate surface, not legacy REST.

Catalog identity is `(source resource file, method, path, operationId)` because duplicate operation IDs and method/path identities exist. Static matches do not prove tenant availability. Keep request and response handling operation-specific; do not infer a universal envelope or transfer a REST response shape to Deluge.

Batch and paginate when the selected operation supports it, avoid redundant metadata reads and calls inside loops, and keep concurrency qualitative. Do not generate volatile quotas, credit formulas, capacity estimates, numeric timeouts, or exact concurrency thresholds.
