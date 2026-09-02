# People Deluge integration tasks

The closed native allowlist contains exactly four tasks:

| Task | Contract |
|---|---|
| `zoho.people.getRecords(form_name, [from_index], [count], [search_criteria], [connection])` | Any accessible form; maximum 200 records. Preserve optional positional order. `searchField` uses a field link name. |
| `zoho.people.create(form_name, record_values, [connection])` | Standard or custom form. Use documented form and field label names. |
| `zoho.people.getRecordById(form_name, record_id, [connection])` | Use the form label name and People record ID. |
| `zoho.people.update(form_name, new_values, [connection])` | `new_values` must include `recordid`; mutation keys use field label names. |

Each execution that receives a response consumes one external call in the host service. New tasks use named OAuth connections; legacy authtoken behavior is not a generation target. The connection parameter is not applicable in Creator and is mandatory in Cliq where the exact task page documents it.

The allowlist excludes specialized leave, attendance, time, cases, LMS, compensation, performance, survey, files, webhook, or HR-process operations. For a miss, follow [routing.md](routing.md); never invent a fifth task or route specialized behavior through generic form CRUD without exact evidence.
