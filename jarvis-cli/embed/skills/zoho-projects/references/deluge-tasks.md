# Closed Projects Deluge task catalog

Use only the nine officially evidenced tasks below. Preserve exact positional order, including omitted optional parameters, and use a named OAuth connection. Legacy authtoken behavior is not a generation target.

| Task | Progressive context | Allowed modules or notes |
|---|---|---|
| `zoho.projects.getPortals([connection])` | Requires neither portal nor project. | Account-root discovery. |
| `zoho.projects.getProjectDetails(portal, [status], [connection])` | Requires portal only. | Project listing/details. |
| `zoho.projects.createProject(portal, [values], [connection])` | Requires portal only. | Project creation. |
| `zoho.projects.getRecords(portal, project_id, module, dataMap/index, range, connection)` | Requires portal plus project. | `milestones`, `taskLists`, `tasks`, `bugs`, `logs`, `comments`. |
| `zoho.projects.getRecordById(portal, project_id, module, [record_id], [connection])` | Requires portal plus project and the selected nested ID. | `milestones`, `tasks`, `bugs`. |
| `zoho.projects.create(portal, project_id, module, data_map, [connection])` | Requires portal plus project. | `milestones`, `taskLists`, `tasks`, `bugs`, `comments`, `logs`. |
| `zoho.projects.update(portal, project_id, module, record_id, data_map, [connection])` | Requires portal plus project and record ID. | `milestones`, `taskLists`, `tasks`, `bugs`, `logs`. |
| `zoho.projects.associateLogs(portal, project_id, module, record_id, values, [connection])` | Requires portal plus project and record ID. | `tasks`, `bugs`. |
| `zoho.projects.updateAssociateLogs(portal, project_id, log_record_id, module, record_id, values, [connection])` | Requires portal plus project, log ID, and record ID. | `tasks`, `bugs`. |

Portal is a progressive prerequisite, not ambient context: `getPortals` has none; project listing and creation use portal only; record and log tasks require portal plus project. Some tasks accept a portal name while current REST operations use the operation's documented portal ID. Do not substitute one form for the other.

The catalog does not cover most metadata, permissions, documents, attachments, setup, automation, custom modules, lifecycle actions, import/export, or bulk operations. Route a miss using [routing.md](routing.md); never invent a task or widen a module allowlist.
