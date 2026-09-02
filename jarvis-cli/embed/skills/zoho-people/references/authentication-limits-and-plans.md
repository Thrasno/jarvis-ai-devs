# Authentication, data centres, limits, and plans

Use OAuth 2.0 only and named connections with exact operation scopes. Do not generate legacy authtoken authentication. Never request or embed access tokens, refresh tokens, client secrets, grant codes, passwords, API keys, or credential-bearing URLs.

Access tokens last one hour. Grant codes are short-lived and one-time. Refresh tokens remain valid until revoked. Use the organization's Accounts data centre: official documentation lists US, AU, EU, IN, CN, and JP. The token response's `api_domain` and the exact operation page are runtime authority; never hard-code the US domain.

| Plan | Daily API limit |
|---|---|
| Essential HR | 250 calls per user licence, maximum 5,000/day. |
| Professional | 250 calls per user licence, maximum 10,000/day. |
| Premium | 250 calls per user licence, maximum 15,000/day. |
| Enterprise | 500 calls per user licence, maximum 25,000/day. |

Legacy operation pages may define independent endpoint thresholds and lock periods. Preserve each exact documented value; do not generalize it. The overview does not prove every specialized module is available in every plan. Unknown edition gates, quotas, or costs remain `TBD`, must be surfaced before generation, and require runtime validation. Known permission failures must not be hidden.
