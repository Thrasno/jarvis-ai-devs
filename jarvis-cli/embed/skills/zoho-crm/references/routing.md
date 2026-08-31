# CRM routing

Route independently by host, every target application, execution context, capability, and requested language.

| Request | Skills and output |
|---|---|
| CRM Deluge | Load `zoho-crm` and `zoho-deluge`; emit `[name].deluge`. |
| Cross-application Deluge | Load every involved application skill and `zoho-deluge`; emit `[name].deluge`. |
| CRM Client Script | Load `zoho-crm`; use JavaScript and emit `[name].js`; do not load Deluge. |
| External runtime | Load relevant application skills and use the requested language and requested placement, not Deluge. |

Warn that standard CRM configuration may satisfy the behavior, without blocking valid custom work. An allowlisted V8 task needs no API-version question. For a miss, evaluate standard CRM configuration, Client Script/ZDK, REST V8, COQL, metadata/bulk APIs, another verified surface, or a manual workaround before reporting unsupported.

Use the single valid path when one remains. When paths are equally optimal, recommend one and wait for selection.
