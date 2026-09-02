# Authentication, limits, and plans

Use named OAuth connections and exact operation scopes in the `ZohoProjects.<resource>.<operation>` family. Registration and token requests use the organization's Accounts data centre; official documentation identifies US, EU, IN, AU, CN, and JP domains. Never request or embed secrets, tokens, client secrets, passwords, or credential-bearing URLs. Keep connection creation, sharing, and authorization in secure deployment configuration.

REST and Deluge limits are independent:

- Current REST allows 200 requests per endpoint in two minutes. The limit is per endpoint; exceeding it blocks that endpoint for ten minutes. Inspect rate-limit and `Retry-After` headers.
- More than 100 Projects integration-task executions in two minutes causes a 30-minute restriction.

Do not merge these budgets, invent tenant capacity, or conceal a throttle response.

Verified plan gates are limited to these facts:

- Project custom fields: Enterprise-only in official legacy documentation.
- Task custom fields: Enterprise-only in official legacy documentation.
- Strict projects: paid plans only.
- User hierarchy: current API exposes an Enterprise-only error.

Every other family-level plan claim is `TBD`. Portal plan details, runtime permissions, and exact operation responses are authoritative. Unknown plans, permissions, quotas, and costs require a clear warning and runtime validation; known permission failures must remain visible.

Documents and attachments can require both Projects and operation-specific WorkDrive scopes, including documented combinations of `WorkDrive.workspace.ALL`, `Workdrive.team.ALL`, and `WorkDrive.files.ALL`. Never broaden or normalize scope spelling without exact operation evidence.
