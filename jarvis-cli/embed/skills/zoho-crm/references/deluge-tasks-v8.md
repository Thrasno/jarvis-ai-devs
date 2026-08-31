# CRM V8 Deluge tasks

New code uses `zoho.crm.v8.*` only and module and field API names. When modifying existing `zoho.crm.*`, ask whether to migrate to V8 or preserve legacy behavior. Legacy examples never establish V8 behavior.

The closed V8 task catalog has exactly these 13 tasks:

| Task | Response boundary |
|---|---|
| `createRecord` | Single Map. |
| `getRecords` | Convert to a JSON list with `toJsonList()` before iterating. |
| `searchRecords` | Iterate directly: `for each record in response`. |
| `getRecordById` | Single Map. |
| `updateRecord` | Single Map. |
| `bulkCreate` | Opaque absent verified container evidence. |
| `bulkUpdate` | Opaque absent verified container evidence. |
| `getRelatedRecords` | Opaque absent verified container evidence. |
| `updateRelatedRecord` | Single Map. |
| `convertLead` | Single Map. |
| `upsert` | Single Map. |
| `attachFile` | Single Map; accepts a Deluge FILE. |
| `getFields` | Map containing a fields list. |

`getRelatedRecords`, `bulkCreate`, and `bulkUpdate` opaque handling is mandatory until evidence proves their containers. Never invent a REST-style `data` wrapper or a universal response type.

Use `conpas_crm` explicitly where a connection is supported. For an absent task, route through the verified alternatives in [routing.md](routing.md).
