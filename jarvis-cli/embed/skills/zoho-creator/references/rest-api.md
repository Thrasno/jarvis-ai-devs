# Creator REST API v2 and v2.1

API v2 and v2.1 each expose 21 OpenAPI entries representing 23 HTTP operations. Replace `{version}` with `v2` or `v2.1`; normal data identity is `{owner}/{app}` plus a form or report link name.

## Data — 7 operations

| Method and canonical path | Operation | Scope |
|---|---|---|
| `POST /creator/{version}/data/{owner}/{app}/form/{form}` | Add records | `ZohoCreator.form.CREATE` |
| `GET /creator/{version}/data/{owner}/{app}/report/{report}/{record_ID}` | Get record by ID | `ZohoCreator.report.READ` |
| `GET /creator/{version}/data/{owner}/{app}/report/{report}` | Get records | `ZohoCreator.report.READ` |
| `PATCH /creator/{version}/data/{owner}/{app}/report/{report}/{record_ID}` | Update by ID | `ZohoCreator.report.UPDATE` |
| `PATCH /creator/{version}/data/{owner}/{app}/report/{report}` | Update by criteria | `ZohoCreator.report.UPDATE` |
| `DELETE /creator/{version}/data/{owner}/{app}/report/{report}/{record_ID}` | Delete by ID | `ZohoCreator.report.DELETE` |
| `DELETE /creator/{version}/data/{owner}/{app}/report/{report}` | Delete by criteria | `ZohoCreator.report.DELETE` |

## Publish — 3 operations

Publish APIs are production-only and require a sensitive `privatelink`; their verified pages do not declare OAuth.

| Method and canonical path | Operation | Authentication |
|---|---|---|
| `POST /creator/{version}/publish/{owner}/{app}/form/{form}` | Add records | mandatory private link |
| `GET /creator/{version}/publish/{owner}/{app}/report/{report}/{record_ID}` | Get record by ID | mandatory private link |
| `GET /creator/{version}/publish/{owner}/{app}/report/{report}` | Get records | mandatory private link |

## File — 3 operations

| Method and canonical path | Operation | Scope/evidence |
|---|---|---|
| `POST /creator/{version}/data/{owner}/{app}/report/{report}/{record_ID}/{field_link_name}/upload` | Upload | `ZohoCreator.report.CREATE` |
| `GET /creator/{version}/data/{owner}/{app}/report/{report}/{record_ID}/{field_link_name}/download` | Download | `ZohoCreator.report.READ` |
| `GET /creator/{version}/data/{owner}/{app}/report/{report}/{subform_link_name}.{field_link_name}/{subform_record_ID}/download` (operation page)<br>`GET /creator/{version}/data/{owner}/{app}/report/{report}/{subform_link_name}/{field_link_name}/{subform_record_ID}/download` (OpenAPI) | Download from subform | blocked: operation page and OpenAPI disagree on segment syntax |

The subform file-download path is contradictory and must fail closed: the operation page combines `{subform_link_name}.{field_link_name}` into one segment, while OpenAPI uses separate `{subform_link_name}/{field_link_name}` segments. Do not select either documented shape. Image/signature fields allow 10 MB; file/audio/video fields allow 50 MB. Multi-upload fields require one call per file.

## Metadata — 7 operations

| Method and canonical path | Operation | Scope |
|---|---|---|
| `GET /creator/{version}/meta/{owner}/{app}/form/{form}/fields` | Get fields | `ZohoCreator.meta.form.READ` |
| `GET /creator/{version}/meta/{owner}/{app}/forms` | Get forms | `ZohoCreator.meta.application.READ` |
| `GET /creator/{version}/meta/{owner}/{app}/reports` | Get reports | `ZohoCreator.meta.application.READ` |
| `GET /creator/{version}/meta/{owner}/{app}/pages` | Get pages | `ZohoCreator.meta.application.READ` |
| `GET /creator/{version}/meta/{owner}/{app}/sections` | Get sections | `ZohoCreator.meta.application.READ` |
| `GET /creator/{version}/meta/applications` | Get applications | `ZohoCreator.dashboard.READ` |
| `GET /creator/{version}/meta/{account_owner_name}/applications` | Get applications by workspace | `ZohoCreator.dashboard.READ` |

## Bulk read — 3 operations

| Method and canonical path | Operation | Scope |
|---|---|---|
| `POST /creator/{version}/bulk/{owner}/{app}/report/{report}/read` | Create read job | `ZohoCreator.bulk.CREATE` |
| `GET /creator/{version}/bulk/{owner}/{app}/report/{report}/read/{job_ID}` | Get read-job status | `ZohoCreator.bulk.READ` |
| `GET /creator/{version}/bulk/{owner}/{app}/report/{report}/read/{job_ID}/result` | Download read result | `ZohoCreator.bulk.READ` |

Bulk insert is blocked: the v2.1 overview mentions fetch or insert, while the linked operation page and OpenAPI expose only these three bulk-read operations. Preserve `ZohoCreator.bulk.CREATE/READ` with an evidence note because the OAuth overview omits scopes present in the bulk OpenAPI asset.

## Version boundaries

v2.1 record reads add `record_cursor`, `max_records` values 200/500/1000, field-selection controls, JSON/CSV output, and richer lookup, multivalue, and subform representations. Do not apply these controls to v2 pages or v2-backed integration tasks. The downloadable v2.1 OpenAPI metadata labels itself v2/version 2.0; its v2.1 paths are evidence, but that internal label is not.
