# Identity, authentication, and environments

## Machine identities

| Entity | Required identity |
|---|---|
| Workspace or owner | `account_owner_name`, not a numeric workspace ID |
| Application | `app_link_name` |
| Form | `form_link_name` |
| Report | `report_link_name` |
| Page | `page_link_name` |
| Field | `field_link_name`; composite fields use subfield link names |
| Record | `ID` or `record_ID` |
| Subform record | separate subform `ID` |
| Bulk job | `job_ID` |
| Published component | `privatelink` plus component link names |

Discover identities from application URLs or metadata; locally, `zoho.adminuser` and `zoho.appname` identify the owner and application. Get Fields type `21` identifies subforms. Never substitute display labels.

## Authentication and data centres

Normal REST requests require OAuth 2.0 and must use the `api_domain returned by OAuth`. Verified data-centre coverage is US, EU, IN, AU, JP, CA, SA, CN, and UAE. New integration tasks require a named Creator OAuth connection. Use the exact operation scope and verify API Access permission for the target permission set.

Never expose OAuth tokens or published private links. Configure credentials and private links only through secure deployment configuration.

## Environments and operational limits

Set `environment: development|stage` when targeting those environments; production is the default. Publish APIs are production-only and use private links instead of OAuth.

Normal requests support at most 200 records. The verified rate limit is 50 requests per minute, per endpoint, per public IP. Daily Developer API allowance and operation availability depend on subscription and runtime permissions; validate them at runtime rather than inventing a numeric plan matrix.
