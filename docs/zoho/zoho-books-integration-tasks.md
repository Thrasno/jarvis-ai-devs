# Zoho Books Deluge Integration Task catalog

> **Readiness: CLOSED as a seven-page catalog on 2026-08-31.** The official [Zoho Books Tasks index](https://www.zoho.com/deluge/help/books-tasks.html) enumerates seven tasks, and all seven linked official task pages were inspected. This closure is independent of the REST v3 catalog. Two official internal inconsistencies remain explicit below.

## Contract at a glance

- Integration Tasks are `zoho.books.*` Deluge calls, not REST endpoints and not inferred from REST.
- Prefer a task only when its documented module, parameters, response, host support, OAuth connection, and selected operation all fit.
- A task miss means only that the task catalog has no match. Continue routing through REST v3, another verified surface, or a manual outcome; do not default automatically.
- Generated code uses the target-oriented named OAuth connection `conpas_books`. Never request or embed tokens, client secrets, passwords, or authenticated tenant data.
- The task pages state that a Zoho OAuth connection with appropriate scopes is mandatory for new integration tasks. Exact scopes depend on the corresponding Books API operation and must be selected from that official operation evidence.
- `organization_id` is task-specific: six tasks require it; `getOrganizations(connection)` does not.

## Complete task catalog

| Task | Official signature | Parameters and prerequisites | Return evidence | Connection support | Context and uncertainty | Source |
|---|---|---|---|---|---|---|
| `zoho.books.getOrganizations` | `getOrganizations(connection)` | Named Books connection. No `organization_id`. | `KEY-VALUE`; success sample contains `code`, `message`, and an `organizations` list. | Explicit trailing `connection`; OAuth connection required for new tasks. | Bootstrap organization discovery for the connection principal. No module input. | [Official page](https://www.zoho.com/deluge/help/books/get-organizations.html), accessed 2026-08-31 |
| `zoho.books.createRecord` | `createRecord(module_name, org_ID, data_map, connection)` | Module supported by the corresponding Books **Create** API; organization ID; map containing that operation's mandatory API fields. | `KEY-VALUE`; sample contains `code`, `message`, and a module-specific object. There is no universal object key across modules. | Explicit trailing `connection`; relevant create scope required. | Module eligibility and mandatory fields come from the selected REST operation, not from a static inferred allowlist. | [Official page](https://www.zoho.com/deluge/help/books/create-record.html), accessed 2026-08-31 |
| `zoho.books.updateRecord` | `updateRecord(module_name, org_ID, record_ID, data_map, books_connection)` | Module supported by the corresponding Books **Update** API; organization ID; record ID; update map. | `KEY-VALUE`; sample contains `code`, `message`, and a module-specific object. | The syntax names the trailing argument `books_connection`, while the parameter table labels it `connection`; a relevant update scope is required. | **Parameter-name conflict:** preserve the literal syntax signature above, but treat both names as documentation placeholders for the same trailing connection argument. The page also warns that records/items omitted from the submitted update collection may be deleted in its invoice example. Treat replacement semantics as operation-specific and inspect the target API before generation. | [Official page](https://www.zoho.com/deluge/help/books/update-record.html), accessed 2026-08-31 |
| `zoho.books.getRecords` | `getRecords(module_name, org_ID, search, connection)` | List-capable module; organization ID; `search` as KEY-VALUE or TEXT. Empty map/text requests an unfiltered list. TEXT values containing spaces must be URL encoded. | `KEY-VALUE`; sample contains `code`, `message`, a module-named list, and `page_context`. | Explicit trailing `connection`; relevant list/read scope required. | Query keys come from the selected Books List operation. KEY-VALUE search is documented for all supporting services except Zoho Creator; TEXT is the cross-service form. | [Official page](https://www.zoho.com/deluge/help/books/fetch-records.html), accessed 2026-08-31 |
| `zoho.books.getRecordsByID` | `getRecordsByID(module_name, org_ID, record_id, connection)` | Get-capable module; organization ID; record ID. | `KEY-VALUE`; sample contains `code`, `message`, and a module-specific object. | Explicit trailing `connection`; relevant read scope required. | **Casing conflict:** the note and syntax use `getRecordsByID`; the example calls `getRecordsById`. The page heading is “Fetch record by ID” and does not resolve the callable spelling. This catalog preserves the syntax-section spelling as canonical and records the example spelling as unresolved official inconsistency; it does not claim case-insensitivity. | [Official page](https://www.zoho.com/deluge/help/books/fetch-record-by-id.html), accessed 2026-08-31 |
| `zoho.books.markStatus` | `markStatus(module_name, org_ID, record_ID, status, connection)` | Organization ID and record ID. Exact closed task matrix: `Estimates` → `accepted`, `declined`, `sent`; `Invoices` → `void`, `sent`. | `KEY-VALUE`; samples expose `code` and `message`, including operation-specific failure messages. | Explicit trailing `connection`; relevant status-operation scope required. | This is a lifecycle action, not generic update CRUD. No other module/status is authorized by this task page. | [Official page](https://www.zoho.com/deluge/help/books/mark-status.html), accessed 2026-08-31 |
| `zoho.books.getTemplates` | `getTemplates(module_name, org_ID, connection)` | Organization ID. Exact documented modules: `Invoices`, `Salesorders`, `RetainerInvoices`, `Estimates`, `Purchaseorders`, `CreditNotes`. | `KEY-VALUE`; success sample contains `code`, `message`, and a `templates` list with template metadata. | Explicit trailing `connection`; relevant template-list/read scope required. | Preserve official module spellings for task generation; do not derive additional modules from REST template endpoints. | [Official page](https://www.zoho.com/deluge/help/books/get-templates.html), accessed 2026-08-31 |

## Routing and safety

1. Check verified standard Books functionality first. The recommendation is advisory and never blocks explicitly requested code generation.
2. Prefer a compatible Integration Task when all documented conditions fit.
3. Otherwise evaluate REST v3 independently.
4. Then evaluate another verified surface or state the manual/unsupported outcome.

An explicit generation request authorizes generation, including code for destructive operations. It does **not** authorize future live execution. Immediately before any future destructive or financially sensitive live action, provide a clear impact warning and obtain explicit confirmation.

## Closure proof

| Measure | Count/result |
|---|---:|
| Official index entries | 7 |
| Linked task pages inspected | 7 |
| Catalog rows | 7 |
| Missing index tasks | 0 |
| Tasks requiring `organization_id` | 6 |
| Bootstrap tasks without `organization_id` | 1 |
| Unresolved official inconsistencies | 2 (`updateRecord` trailing parameter label; `getRecordsByID` versus example `getRecordsById`) |

## Evidence boundaries

- The official enumerable task index proves current page-set closure; it does not prove every host product exposes the same connection UI or that every plan/region enables every underlying Books operation.
- Task return values are operation/module-specific KEY-VALUE structures. Never invent one universal response wrapper.
- Volatile numeric quotas, credits, concurrency thresholds, capacity estimates, and timeouts are intentionally excluded. Only structural facts that change generated code belong in the future runtime contract.
- Catalog refreshes occur only after an explicit maintainer decision. A refresh requires a dated capture, hashes where downloadable artifacts exist, a diff, and approval; there is no scheduled/background update process.

## Source ledger

All sources are official Zoho Deluge documentation accessed on **2026-08-31**.

| ID | URL | Evidence |
|---|---|---|
| D00 | https://www.zoho.com/deluge/help/books-tasks.html | Enumerable seven-task index and backend-request note |
| D01 | https://www.zoho.com/deluge/help/books/get-organizations.html | Exact bootstrap signature, return, connection |
| D02 | https://www.zoho.com/deluge/help/books/create-record.html | Exact create signature, parameters, return |
| D03 | https://www.zoho.com/deluge/help/books/update-record.html | Exact update signature, parameters, replacement warning, return |
| D04 | https://www.zoho.com/deluge/help/books/fetch-records.html | Exact list signature, search forms, return |
| D05 | https://www.zoho.com/deluge/help/books/fetch-record-by-id.html | Exact get-by-ID page and casing conflict |
| D06 | https://www.zoho.com/deluge/help/books/mark-status.html | Exact module/status matrix and return |
| D07 | https://www.zoho.com/deluge/help/books/get-templates.html | Exact template-module matrix and return |
