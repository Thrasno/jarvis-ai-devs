# Projects routing and dependencies

Load `zoho-projects` whenever Projects is a host or target. Load `zoho-deluge` only for actual Deluge output. External runtimes retain the requested language and requested placement; Projects does not imply Deluge.

| Request | Route |
|---|---|
| Projects Deluge | Load `zoho-projects` and `zoho-deluge`; emit `[name].deluge` at the requested function placement. |
| Cross-application Deluge | Load every involved application skill and `zoho-deluge`; emit `[name].deluge`. |
| External runtime | Load every involved application skill and use the requested language and requested placement. |
| Documents or attachments using WorkDrive | Load `zoho-projects` and the WorkDrive application skill; apply every operation-specific Projects and WorkDrive scope. |
| CRM account, deal, or contact association | Load `zoho-projects` and the CRM application skill. |
| Another target product | Load Projects plus that target application's skill. |

Before custom code, check standard Projects functionality. State the standard alternative without blocking a valid requested custom implementation. Prefer a closed-catalog native task only when the exact operation, module, host, and named-connection semantics match. Absence from the catalog means only "no native integration task".

For a miss, evaluate exact current REST through `invokeUrl`, another documented surface, standard product functionality, or unsupported behavior. Never force metadata, permissions, documents, attachments, setup, automation, custom modules, lifecycle operations, import/export, or bulk jobs through the nine Deluge tasks.

Route in this authority order:

1. Exact current operation page and exact v3/v3.1 path.
2. Exact Deluge task page for a closed-catalog task.
3. Runtime portal, project, metadata, plan, and permission responses.
4. An explicit legacy page only when legacy behavior was intentionally selected.

If one verified route remains, explain and use it. If several routes are equally suitable, present them, recommend one and wait for selection. Contradictory or unavailable evidence is unsupported until policy or new official evidence resolves it.
