# Analytics Deluge integration tasks

Use this closed allowlist only from a verified Deluge-capable host. `zoho-deluge` owns grammar; this skill owns task applicability, Analytics identity, and target constraints. Analytics has no local Deluge runtime.

| Operation | Exact signature | Identity and connection |
|---|---|---|
| Create row | `zoho.reports.createRow(database_name, table_name, data_map, connection)` | Legacy database/workspace name, table name, column names, and a named Analytics OAuth connection. |
| Update rows | `zoho.reports.updateData(database_name, table_name, data_map, criteria, connection)` | Database/table/column names, SQL-like criteria, and a named connection. |
| Delete rows | `zoho.reports.deleteRow(database_name, table_name, criteria, connection)` | Database/table/column names and a named connection. |

There is no Analytics read integration task. This means no Deluge wrapper, not no read capability: evaluate REST view export, asynchronous REST SQL export, the standard connector, or unsupported. CloudSQL remains outside V0 routing.

Before generating a task, require source host, target database/workspace name, table and column names, exact named connection, compatible operation, write confirmation, and known unit implications. Never invent a fourth task, a read wrapper, an ID-based Deluge signature, or REST response semantics.
