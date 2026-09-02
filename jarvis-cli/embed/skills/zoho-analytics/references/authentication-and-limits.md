# Authentication, data centres, units, and limits

OAuth 2.0 is mandatory. Use `https://{analyticsapi-DC}/restapi/v2`, exact operation scopes, and `ZANALYTICS-ORGID` for normal operations. Do not require that header for initial organization discovery. Never request or embed tokens, client secrets, passwords, or credential-bearing URLs.

Verified API hosts cover US, EU, IN, AU, CN, JP, SA, and CA. Canada uses `analyticsapi.zohocloud.ca`. Resolve the data centre from the authorized account/configuration; never infer it from user location.

## Daily API-unit caps

The dated plan matrix is Free 1,000; Basic 4,000; Standard 10,000; Premium 30,000; Enterprise 100,000 units per day.

| Plan | Daily units |
|---|---:|
| Free | 1,000 |
| Basic | 4,000 |
| Standard | 10,000 |
| Premium | 30,000 |
| Enterprise | 100,000 |

## Frequency limits

- DML 100/minute.
- bulk 40/minute.
- metadata 60/minute.
- overall 100/minute.

## Verified unit examples

- Add row: 0.1; update rows: 0.3; delete rows: 0.1.
- Append/truncate import: 10 per 1,000 rows; update-add import: 15 per 1,000 rows.
- Chart image/PDF export: 10 per request.
- Dashboard or HTML export: 15 per request.
- Other exports: 3 per 1,000 rows.
- Copy workspace: 25 per request plus 1 per 1,000 rows when copying data.
- Job status and download operations: 0 units.

The official unit page does not price every recent operation, and per-operation plan gates are incomplete. Treat an unlisted cost, unknown plan gate, quota, or permission as unresolved: warn and require runtime validation. Never extrapolate a cost or conceal a known permission failure.
