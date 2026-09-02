# Identifiers and runtime metadata

Resolve identity from the exact operation and runtime metadata. Display labels and human keys are secondary unless that operation explicitly accepts them.

| Entity | Canonical distinction |
|---|---|
| Portal | Current REST uses the documented `portal_id`; some Deluge tasks accept a portal name. Never treat portal ID and portal name as interchangeable. |
| Project | Use `project_id` as the project ID when required. A project key/prefix and project name are secondary identifiers, not substitutes. |
| User or contact | The exact endpoint decides among ZPUID, user ID, email, and contact ID. |
| Module or entity | Resolve module/entity ID from Module Meta or the associated-modules operation. |
| Layout or section | Use runtime layout ID and section ID. |
| Field | Use field ID or canonical `column_name`/field key; a display name is non-unique. |
| Option | Use the option ID returned for the field. |
| Work item | Preserve project parentage and the exact phase, issue, task-list, task, log, forum, event, customer, contact, tag, or record ID. Human keys remain secondary. |
| File | Distinguish a Projects attachment ID from a WorkDrive upload resource ID, team-folder ID, folder ID, and resource ID. |
| Automation or bulk | Preserve blueprint, transition, task entity, function, module, and asynchronous job IDs independently. |

Portal and project are progressive prerequisites rather than universal ambient context. Resolve modules, layouts, sections, fields, options, relationships, required/read-only status, permissions, plan, and operation availability at runtime.

Runtime metadata and the exact operation are authoritative. Never infer an accepted identity type from a neighboring endpoint, display name, legacy URL, or another Zoho application. Projects Time Logs are Projects-native; direct ID interoperability with People Time Tracker is unavailable evidence.
