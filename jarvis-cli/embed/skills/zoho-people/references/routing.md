# People routing

Route independently by host, target, execution context, requested behavior, API family, operation, plan, and required identifiers.

| Request | Skills and surface |
|---|---|
| People Deluge | Load `zoho-people` and `zoho-deluge`; emit `[name].deluge` at the requested placement. |
| Cross-application Deluge | Load every involved application skill and `zoho-deluge`; emit `[name].deluge`. |
| External runtime | Load every involved application skill; preserve the requested language and placement. |
| Product automation | Load `zoho-people`; configure only officially verified People functionality. |

Check standard People functionality before custom code. For an exact four-task allowlist match, prefer the native task when the host supports its connection semantics. An allowlist miss means only that no native task is verified: evaluate current REST through `invokeUrl`, another documented surface, standard product functionality, or an explicit unsupported result.

Never translate a v1/v2 route into v3 by rewriting its path. Use the exact official operation page, runtime metadata, returned IDs, and exact scopes. Specialized domains must not route through generic form CRUD without operation-level evidence.

Use and explain the single verified path when one remains. When verified paths are equally optimal, recommend one and wait for selection.
