# Creator integration tasks v2

The closed allowlist contains exactly five `zoho.creator.*` v2 wrappers. New calls require a named Creator OAuth connection. Every execution generates a backend API request; cross-product or Creator-to-Creator calls may consume both the target Creator Developer API allowance and the source host External Calls allowance.

| Exact signature | Unit and scope |
|---|---|
| `zoho.creator.getRecords(owner_name, app_link_name, report_link_name, criteria, from_index, limit, connection)` | Report; maximum 200; `ZohoCreator.report.READ`. |
| `zoho.creator.getRecordById(owner_name, app_link_name, report_link_name, record_id, connection)` | Report and record ID; `ZohoCreator.report.READ`. |
| `zoho.creator.createRecord(owner_name, app_link_name, form_link_name, input_values, other_params, connection)` | Form; map or list; maximum 200; `ZohoCreator.form.CREATE`. |
| `zoho.creator.updateRecords(owner_name, app_link_name, report_link_name, criteria, new_input_values, other_api_params, connection)` | Report and criteria; `ZohoCreator.report.UPDATE`. |
| `zoho.creator.updateRecord(owner_name, app_link_name, report_link_name, record_id, new_input_values, other_api_params, connection)` | Report and record ID; `ZohoCreator.report.UPDATE`. |

There is no Creator delete integration task. Route deletion to native `delete from`, REST DELETE, or unsupported.

Integration tasks remain v2 wrappers and must not inherit v2.1-only controls such as record cursors, expanded read sizes, field selection, output formats, or `skip_workflow`. Do not infer REST response shapes or undocumented workflow behavior for these tasks.
