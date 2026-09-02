# Zoho Books Deluge Integration Tasks

This closed, official 2026-08-31 index contains seven `zoho.books.*` tasks. Prefer one only when its module, parameters, response, host support, OAuth connection, and operation contract all fit. Integration Tasks are independent from REST v3. Use `conpas_books` and operation-appropriate scopes; never invent a universal response wrapper.

| Task | Contract |
|---|---|
| `zoho.books.getOrganizations` | `getOrganizations(connection)`; the bootstrap task has no `organization_id`. |
| `zoho.books.createRecord` | `createRecord(module_name, org_ID, data_map, connection)`; derive module and mandatory fields from the selected Create operation. |
| `zoho.books.updateRecord` | `updateRecord(module_name, org_ID, record_ID, data_map, books_connection)`; inspect operation-specific replacement semantics. Official syntax says `books_connection`, while its parameter table says `connection`; both label the same trailing argument, not overloads. |
| `zoho.books.getRecords` | `getRecords(module_name, org_ID, search, connection)`; search keys come from the selected List operation. |
| `zoho.books.getRecordsByID` | `getRecordsByID(module_name, org_ID, record_id, connection)`; the official example instead calls `getRecordsById`. Preserve this casing conflict and do not claim case-insensitivity. |
| `zoho.books.markStatus` | `markStatus(module_name, org_ID, record_ID, status, connection)`; only Estimates (`accepted`, `declined`, `sent`) and Invoices (`void`, `sent`) are documented. |
| `zoho.books.getTemplates` | `getTemplates(module_name, org_ID, connection)`; documented modules are Invoices, Salesorders, RetainerInvoices, Estimates, Purchaseorders, and CreditNotes. |

Six tasks require an organization ID; `getOrganizations` does not. A catalog miss proves only that no compatible Integration Task was recognized and does not authorize automatic REST routing.
