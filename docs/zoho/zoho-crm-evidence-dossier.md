# Zoho CRM Skill Evidence Dossier

| Field | Value |
|---|---|
| Status | **Evidence gate approved; tracked documentation delivery remains under #545; #544 awaits explicit implementation instruction** |
| Verification date | **2026-08-29** |
| Related issues | [#542](https://github.com/Thrasno/jarvis-ai-devs/issues/542), [#545](https://github.com/Thrasno/jarvis-ai-devs/issues/545) → [#544](https://github.com/Thrasno/jarvis-ai-devs/issues/544) |
| Audience | Maintainers completing #545 and implementers of `zoho-crm` in #544 |
| Evidence policy | Zoho product claims require inspected official Zoho documentation; project contracts require project provenance; V8 API claims require a V8 page |

> **Readiness:** the routing, V8-only generation policy, closed 13-task catalog, REST V8 source groups, reference split, automated-test contract, and standard module/field recognition baseline are complete. The evidence gate is approved, these documents still require tracked delivery under #545, and #544 may begin only on explicit implementation instruction. The independent factual gaps in section 18 retain their conservative, deterministic exclusions.

## 1. Approved contract at a glance

These decisions are deterministic project policy. They do not remain open for #544.

| Topic | Approved decision |
|---|---|
| Task catalog | The 13 `zoho.crm.v8.*` tasks in section 8 are the current closed catalog. A missing capability has no current V8 integration task and routes to REST V8, another verified surface, or a manual workaround until a maintainer changes the catalog. |
| API generation | Generate V8 only for new code. When modifying existing `zoho.crm.*` code, ask whether to migrate to V8 or preserve legacy behavior. Legacy documentation never proves V8 behavior. |
| API identity | Always use module and field API names. This includes task maps, REST payloads, COQL, and search criteria. Never generate field labels as the integration contract. |
| Query collection | `fields` is the only valid query-collection key for `getRecords`, `getRecordById`, and `getRelatedRecords`. |
| Connections | Prefer a named connection for every Deluge task or API call where supported. Use target-oriented names: `conpas_[target-app]`; CRM defaults to `conpas_crm`. Never request or embed secrets. |
| Responses | Document each response from its official operation page. There is no universal CRM task response type. |
| REST | The REST V8 source groups, endpoint families, scope families, and async lifecycles in section 9 are approved. |
| Metadata and catalogs | The static knowledge base recognizes standard module and field display/API names only. It is an official-evidence-filtered baseline derived from authenticated CRM V8 metadata captured on 2026-08-29; it is not globally exhaustive. Runtime metadata remains authoritative for the target organization. The catalog does not establish write safety, permissions, layouts, relationships, quotas, or runtime policy. |
| References and tests | The reference split in section 17 and automated contract tests in section 15 are approved. |
| Limits and quotas | Encode only actionable structural limits that determine code shape. Treat concurrency as an advisory failure risk without claiming a tenant threshold. Per-day request quotas, Function-credit formulas/maxima, CRM API-credit formulas/maxima, and numeric execution timeouts are researched but excluded from the future skill contract; use the qualitative call-optimization rule in section 12 instead. |
| Maintenance | Updates occur only when a maintainer chooses. No release cadence, periodic review schedule, or catalog-version scheme is imposed. Provenance remains attached to facts. |

### Approved routing principles

- Host application, every target application, and exact execution context are independent routing inputs.
- CRM Client Script loads `zoho-crm`, uses JavaScript, and outputs `.js`; it does not load `zoho-deluge`.
- CRM Deluge loads `zoho-crm` and `zoho-deluge` and outputs `.deluge`.
- Cross-application Deluge loads every involved application skill plus `zoho-deluge`.
- Standard CRM configuration is checked before code. Its warning is advisory, not a blocker to valid user-requested custom work.
- When multiple equally optimal valid paths remain, present them, recommend one with reasons, and wait for the human choice.

## 2. Scope and readiness

### #545 tracked delivery before #544 implementation

- This evidence dossier and its official-source register.
- The approved reference-set design.
- The completed recognition catalogs: [standard modules](zoho-crm-standard-modules.md) and [standard fields](zoho-crm-standard-fields.md).
- The approved catalog scope: an official-evidence-filtered standard baseline plus authoritative runtime metadata, without a globally exhaustive claim.
- The other factual gaps in section 18 now have evidence-bounded deterministic exclusions and require no additional policy choice.

### #544 owns on explicit implementation instruction

- The concise `zoho-crm/SKILL.md` runtime contract.
- The approved local references.
- Activation, routing, output, security, and uncertainty behavior.
- Automated contract tests.

### Non-goals

- Implementing `zoho-crm` or changing `zoho-deluge` in this dossier task.
- Certifying a customer's edition, permissions, layouts, fields, connections, or enabled features.
- Treating reachability, issue text, legacy examples, or historical API examples as V8 proof.
- Handling access tokens, refresh tokens, client secrets, passwords, API keys, or tenant-specific values.
- Duplicating exhaustive volatile error, comparator, ZDK-method, or tenant-customization catalogs where runtime metadata or official references own the detail.

## 3. Mental model and outputs

| Axis | Question | Example |
|---|---|---|
| Host | Where does execution begin? | CRM workflow function |
| Targets | Which products are used? | CRM and Books |
| Context | What invokes the behavior? | Workflow instant action |
| Capability | What must happen? | Create a CRM record |
| Surface | How is it reached? | V8 task, REST, COQL, ZDK |
| Authentication | Which principal authorizes it? | Named OAuth connection |
| Prerequisites | Which operation facts are required? | API names, layout, record ID |

Do not collapse these axes. “Runs in CRM,” “uses Deluge,” “calls REST,” and “uses OAuth” describe different properties.

| Actual language | Artifact | Skill loading |
|---|---|---|
| Deluge | `[name].deluge` | Every host/target application skill + `zoho-deluge` |
| CRM Client Script | `[name].js` | `zoho-crm`; never `zoho-deluge` |
| External language | Language-native extension | Target application skills and the actual language skill when available |

## 4. CRM Functions execution contexts

CRM Functions are server-side Deluge. Creating, updating, or deleting them requires **Manage Extensibility**; users with workflow/module customization permissions may only view and associate existing functions [S30–S31]. Functions created for one location cannot be reused in another location [S31].

| Context | Configuration | Invocation and inputs | Runtime and availability | Standard alternative | Source/status |
|---|---|---|---|---|---|
| Workflow function | Setup → Automation → Workflow Rules → Instant/Time-Based Action → Function; select, gallery, or write Deluge | Workflow criteria invoke it. Argument mapping accepts CRM fields, custom values, and Execution Info. Previous values are available only on edit; not for multi-line, lookup, subform, or multi-select lookup fields. Merge source is available only for Find and Merge Duplicates. | Up to **1 instant and 5 time-based functions/rule**. Native action appears in Enterprise/Ultimate; Standard/Professional Functions are extension-only. | Workflow field update, task, email, webhook | [S31, S43–S44] **Confirmed** |
| Standalone function | Setup → Developer Hub → Functions → Create New Function → Standalone | Explicit arguments map CRM fields or custom values. Invoked by supported callers or exposed through the Function API. | Context-specific edition gate is not separately stated. Overall Functions: unavailable in Free; Standard/Professional extension-only; native availability in Enterprise/Ultimate. | Direct V8 API when the function adds no domain behavior | [S30–S31, S43–S44] **Confirmed; plan scope noted** |
| Schedule function | Functions/Schedules configuration | Recurrence invokes the function. The inspected official pages confirm association but do not expose an exact schedule argument map. **Deterministic exclusion:** generate no schedule arguments unless the mapping is copied from the target CRM configuration or a newly inspected official source. | Enterprise **30 schedules/org**; Ultimate **50/org**. | Time-based workflow action when semantically sufficient | [S31, S43–S44]; A02, A10 **Partially confirmed; mapping excluded** |
| Custom-button function | Function associated with a CRM custom button | User click invokes server-side Deluge. The inspected official pages confirm association but do not expose a universal server-side button context or argument map. **Deterministic exclusion:** do not generate implicit button arguments; require the target button's configured mappings or new official evidence. | Custom buttons: Enterprise **50/module**; Ultimate **250/module**. | Native action or Client Script button when browser-local behavior fits | [S31, S43–S44]; A03, A11 **Partially confirmed; mapping excluded** |
| Related-list function | Record → Add Related List → Custom Functions; choose gallery or create Deluge | Arguments are declared and mapped while adding the related list; the function returns CRM's related-list row structure. | Current matrix: Professional **3/module**, Enterprise **5/module**, Ultimate **10/module**. | Existing related-list data/actions | [S43–S45] **Confirmed; source conflict noted in section 19** |
| Validation-rule function | Associate a function from Validation Rules | Association is confirmed, but the inspected official source for exact inputs and return semantics remains inaccessible. **Deterministic exclusion:** do not generate a validation function signature or return contract without target-UI mappings or newly inspected official evidence. | Function-backed limits: Enterprise **3/layout**, Ultimate **5/layout**. | Declarative validation rule | [S31, S43–S44]; A05, A12 **Partially confirmed; contract excluded** |
| Function API | Standalone function → Settings → REST API | Supports GET/POST. POST uses form-data key `arguments` with an input JSON object; arguments may instead be URL encoded. OAuth 2.0 is the documented mode for access within the organization; API-key exposure is documented for third parties but is rejected by this project's secret-safety policy. The API name is system-generated and shown in Settings → REST API. | The inspected official page does not publish an exact endpoint template. **Deterministic exclusion:** copy the generated OAuth endpoint from that function's REST API settings; never synthesize a hostname, path, API version, API key, or credential-bearing URL. | Direct V8 endpoint | [S31]; A13 **Authentication and invocation confirmed; endpoint template excluded** |

## 5. Client Script context matrix

Client Script is browser-side JavaScript. Configure it under **Setup → Developer Hub → Client Script** by choosing category, module, page, layout, and event. Create/clone/edit pages can also open the setup from the page itself [S32–S33, S46].

| Page/context | Invocation model | Documented event inputs/cancellation |
|---|---|---|
| Create/Clone/Edit — standard | Page `onLoad`, `onChange`, `onSave`; field `onChange`, `onType`; subform row events; custom-button click | Page `onChange`: `field_name`; field `onChange`: `value`; load/save: no argument documented. `return false` from `onSave` prevents save. |
| Create/Clone/Edit — Canvas | Page load/change/save; field events; mandatory-form load; canvas button/icon/text click; subform events; custom button | Same primary page/field arguments. |
| Detail — Standard | `onLoad`; field `onBeforeUpdate`; mandatory-form events; Blueprint `beforeTransition`; tag, subform, Notes-related-list, and custom-button events | Update: `value`; mandatory save: `value`; mandatory load: `field`; Blueprint: `transition`; tag: `tag`, `added`, `removed`. Supported pre-events can return false. |
| Detail — Canvas | Standard-detail events plus canvas button/icon/text clicks | Same documented detail arguments; click events have no argument documented. |
| Create/Edit Wizard | Wizard load/change/transition/before-transition/before-save; field/subform events; custom button | `field_name`, `screen`, or `source_screen` and `target_screen`; field change receives `value`. Supported before-events can return false. |
| List — Standard | `onCustomViewLoad`, `onBeforeCustomViewChange`, custom-button click | Both view events receive `custom_view`. |
| List — Canvas | `onLoad`, `onBeforeCustomViewChange`, canvas button/icon/text click | Detailed arguments beyond the custom-view context are not documented in the argument table. |
| Quick Create popup | Page load/change/save; field change/type | Current event page supports it; the current FAQ says it is unsupported. Preserve the contradiction and exclude Quick Create from the future runtime contract. |
| Command | Category **Commands**; user invokes globally through command palette or personal shortcut | No event argument contract documented. Maximum **30 commands**. |
| Client Script custom button | Create only from the Buttons page; edit later from Client Script setup | Button click with page/record context through Client/ZDK APIs; no universal positional argument. |

### Shared Client Script constraints

- Available in **Professional, Enterprise, and Ultimate** [S32].
- Maximum **30 scripts/page**, **30 commands**, and **5 static resources/page** [S47–S49].
- A separate script is required for every module layout [S32, S46, S49].
- Only documented events are supported; custom events cannot be created [S49].
- ZDK Web API calls contribute to API usage; apply the qualitative call-optimization rule in section 12 without estimating credits [S32, S50].
- Third-party calls require the domain in Trusted Domains [S48–S49].

## 6. Standard CRM capabilities to check before code

The router warns when standard configuration can satisfy the requirement, but continues with valid custom work unless the user chooses the standard path.

| Capability | What it replaces when sufficient | Current availability/limit | Source |
|---|---|---|---|
| Workflow rules | Custom trigger/orchestration code for CRM record events and criteria | All editions with edition-specific rule limits; scheduled actions unavailable in Free | [S31, S44] |
| Workflow field updates | A function whose only behavior sets fixed or mapped CRM fields | Standard–Ultimate: **5 field updates/action** | [S44] |
| Workflow tasks/email | A function that only creates a follow-up task or sends a standard templated notification | Available with edition-specific constraints; do not encode per-day quotas | [S44] |
| Formula fields | Deterministic values derived solely from record fields | Professional **15**, Enterprise **20**, Ultimate **25** | [S19, S44] |
| Rollup summary | Read-related-records → aggregate → update-parent code | Professional **2/module**, Enterprise **10/module**, Ultimate **15/module** | [S19, S44] |
| Validation rules | Server-side criteria validation that does not require immediate browser interaction | Professional **5/layout**, Enterprise **10/layout**, Ultimate **25/layout** | [S16–S17, S44] |
| Layout rules | Supported conditional field/section/subform visibility or mandatory behavior | Enterprise/Ultimate **10/layout**. V8 record APIs explicitly support only the Set Mandatory Field action. | [S16–S17, S44] |
| Blueprint | Custom state-machine code for controlled transitions, required data, and transition actions | Professional **3**, Enterprise **50**, Ultimate **100** | [S21, S33, S44] |
| Custom buttons | An external UI trigger; the underlying action may still be Deluge or Client Script | Enterprise **50/module**, Ultimate **250/module** | [S32–S33, S44] |
| Workflow webhooks | A function whose only role serializes data and calls one external endpoint | Available with edition-specific constraints; optimize calls and do not claim a per-day quota | [S44] |
| Custom schedules | An external scheduler for CRM-owned recurring Deluge work | Enterprise **30/org**, Ultimate **50/org** | [S43–S44] |

Required warning: “CRM may provide a standard configuration for this behavior; verify edition and organization setup. I will continue with the requested custom implementation unless you choose the standard path.”

## 7. Capability and transport matrix

| Surface | Best fit | Authentication | Evidence/status |
|---|---|---|---|
| CRM V8 integration task | One of the 13 closed-catalog operations from a supported Deluge host | Explicit `conpas_crm` by project policy | [S01–S14] **Approved** |
| REST API V8 | External runtime, no task match, or REST-only feature | OAuth scope from exact endpoint | [S15–S29, S36] **Approved** |
| COQL | Advanced read/query, aggregates, or lookup joins | COQL + module read; fields scope if metadata included | [S18] **Approved with confirmed structural limits; contradictory per-call ceilings excluded** |
| Bulk Read | Large asynchronous export/backup | Bulk read + module read | [S23–S24, S51] **Approved** |
| Bulk Write | Large asynchronous CSV insert/update/upsert | Bulk + module operation scopes | [S25–S26, S52–S53] **Approved** |
| Metadata APIs | Tenant module/field/layout/related-list discovery | Settings scopes | [S19–S21, S36] **Approved** |
| Attachments/ZFS | Record attachments versus file/image-upload fields | Module attachment scope or Files scope | [S27–S29] **Approved** |
| Client Script/ZDK/ZRC | Immediate CRM browser behavior | Logged-in CRM context; managed connections where applicable | [S32–S33, S46–S50] **Approved where facts are resolved** |
| Standard configuration | Declarative CRM behavior | Product permissions | Section 6 |

Capability is not transport. “Query records” can mean a V8 task, Records API, Search API, COQL, Bulk Read, or ZDK depending on context and prerequisites.

## 8. Closed CRM V8 Deluge task catalog

### Generation rules shared by all 13 tasks

- Use the exact `zoho.crm.v8.*` name and documented positional order [S01–S14].
- Every task accepts a trailing connection. Although official pages make it mandatory in Cliq and optional in other hosts, generated project code passes `"conpas_crm"` explicitly.
- Normative task-page text permits `null` for an unused preceding optional parameter. It also permits an empty map for an unused `options_map` positional placeholder. Examples need not demonstrate the rule for it to apply.
- An empty options map `{}` means “no options supplied.” It is **not** `{"trigger":[]}`: an empty trigger list suppresses CRM automations.
- User-supplied page/per-page values override the deterministic defaults below.
- Document and handle responses from the individual task page; never assign one response type to all tasks.

| Task | Exact signature | Deterministic project form | Response contract | Source |
|---|---|---|---|---|
| `createRecord` | `createRecord(module_name, record_details, options_map, connection)` | `zoho.crm.v8.createRecord(module, values, {}, "conpas_crm")` when the options slot is only positional | Single Map on documented success/error paths | [S02] |
| `getRecords` | `getRecords(module_name, query_value, page, per_page, connection)` | `zoho.crm.v8.getRecords(module, {"fields":field_api_names}, 1, 200, "conpas_crm")` | Page labels the variable `KEY-VALUE` and shows one record, but its only multi-record handling converts the response with `toJsonList()` before iteration. Exact wire/container shape is not proved. Safe generated handling is `records = response.toJsonList(); for each record in records`; do not access an invented `data` key. | [S03] |
| `searchRecords` | `searchRecords(module_name, criteria, page, per_page, search_value, connection)` | `zoho.crm.v8.searchRecords(module, criteria_using_api_names, 1, 200, null, "conpas_crm")` | Page labels the variable `KEY-VALUE` and shows one record, while its documented multi-result snippet iterates `response` directly. Exact wire/container shape is not proved. Safe generated handling is direct `for each record in response`; do not call `get("data")` or force `toJsonList()`. | [S04] |
| `getRecordById` | `getRecordById(module_name, record_ID, query_value, connection)` | `zoho.crm.v8.getRecordById(module, record_id, {"fields":field_api_names}, "conpas_crm")` | Single Map; documented failures are Maps | [S05] |
| `updateRecord` | `updateRecord(module_name, record_ID, record_value, options_map, connection)` | `zoho.crm.v8.updateRecord(module, record_id, values, {}, "conpas_crm")` when options are only positional | Single Map | [S06] |
| `bulkCreate` | `bulkCreate(module_name, records_value, options_map, connection)` | `zoho.crm.v8.bulkCreate(module, records, {}, "conpas_crm")` when options are only positional | Page labels the variable `KEY-VALUE` but prints adjacent per-record result maps without a valid outer delimiter and gives no conversion/iteration snippet. Exact outer container and safe generic iteration remain unproved. Return or log the opaque response; do not generate wrapper access or iteration. | [S07] |
| `bulkUpdate` | `bulkUpdate(module_name, records_value, options_map, connection)` | `zoho.crm.v8.bulkUpdate(module, records, {}, "conpas_crm")` when options are only positional | Page labels the variable `KEY-VALUE` but prints adjacent per-record result maps without a valid outer delimiter and gives no conversion/iteration snippet. Exact outer container and safe generic iteration remain unproved. Return or log the opaque response; do not generate wrapper access or iteration. | [S08] |
| `getRelatedRecords` | `getRelatedRecords(relation_name, parent_module_name, record_id, query_value, page, per_page, connection)` | `zoho.crm.v8.getRelatedRecords(relation, parent_module, record_id, {"fields":field_api_names}, 1, 200, "conpas_crm")` | Page labels the variable `KEY-VALUE` and gives only a single-record sample despite multi-record semantics; it provides no conversion/iteration snippet. Exact outer container and safe generic iteration remain unproved. Return or log the opaque response; do not generate wrapper access or iteration. | [S09] |
| `updateRelatedRecord` | `updateRelatedRecord(sub_module, sub_module_record_id, parent_module, parent_module_record_id, values, connection)` | Same order with explicit `"conpas_crm"` | Single Map | [S10] |
| `convertLead` | `convertLead(lead_id, values, connection)` | `zoho.crm.v8.convertLead(lead_id, null, "conpas_crm")` when no values map is supplied | Single Map | [S11] |
| `upsert` | `upsert(module, values, duplicate_check, connection)` | `zoho.crm.v8.upsert(module, values, null, "conpas_crm")` when no duplicate-check value is supplied | Single Map | [S12] |
| `attachFile` | `attachFile(module, record_id, file_object, connection)` | Same order with a Deluge FILE and explicit `"conpas_crm"` | Single Map | [S13, S27] |
| `getFields` | `getFields(module_name, connection)` | `zoho.crm.v8.getFields(module, "conpas_crm")` | Map containing a `fields` List | [S14] |

Generated response handling is operation-specific: use `toJsonList()` only for `getRecords`, iterate the response directly only for `searchRecords`, and keep `getRelatedRecords`, `bulkCreate`, and `bulkUpdate` opaque unless target-runtime evidence or a newly inspected official source proves their containers. Never invent a REST-style `data` wrapper for a Deluge task.

### `attachFile` resource boundary

The task accepts a Deluge **FILE**. REST attachment-link support must not be generalized to this task. V8 attachment coverage confirms these 21 API resource identifiers [S13, S27]:

`leads`, `accounts`, `contacts`, `deals`, `campaigns`, `tasks`, `cases`, `events`, `calls`, `solutions`, `products`, `vendors`, `pricebooks`, `quotes`, `salesorders`, `purchaseorders`, `invoices`, `custom`, `appointments`, `services`, `notes`.

### Closed-catalog routing

| Capability absent from catalog | Current route |
|---|---|
| Delete record | Verify and use the exact REST V8 delete operation, another standard surface, or a manual workaround. |
| COQL | `POST /crm/v8/coql` [S18]. |
| Bulk Read | `/crm/bulk/v8/read` lifecycle [S23–S24, S51]. |
| Bulk Write | Upload + `/crm/bulk/v8/write` lifecycle [S25–S26, S52–S53]. |

An allowlist miss means **no current V8 integration task**. It does not mean “always use REST”; routing still evaluates standard configuration, Client Script/ZDK/ZRC, API, or a documented manual workaround.

### Legacy policy

- New code uses `zoho.crm.v8.*` only.
- When asked to modify existing `zoho.crm.*` code, ask one focused question: migrate to V8 or preserve legacy behavior?
- If preservation is chosen, legacy code may be maintained within that explicit scope.
- Legacy signatures, examples, response shapes, linked v2/v3 pages, and labels never populate a V8 contract [S35–S41].
- Obsolete issue aliases remain rejected: `upsertRecord` → `upsert`, `bulkCreateRecords` → `bulkCreate`, `bulkUpdateRecords` → `bulkUpdate`.

## 9. Approved REST API V8 source groups

`{api-domain}` is data-center dependent. Use a documented placeholder or an explicitly supplied regional domain; never infer one.

| Group | Endpoint/lifecycle | Required facts and scopes | Pagination/response boundary | Source |
|---|---|---|---|---|
| Records read | `GET /crm/v8/{module}` or `/{record_id}` | Module API name; record ID for one; `fields`; module READ/ALL | `data` + `info`; endpoint-specific pagination | [S15] |
| Records create | `POST /crm/v8/{module}` | Module/field API names, mandatory fields, layout/trigger as needed; module CREATE/WRITE/ALL | Up to 100 synchronous records; ordered per-record results; partial HTTP 207 | [S16] |
| Records update | `PUT /crm/v8/{module}` or `/{record_id}` | IDs, field API names, optional concurrency/feature execution; module UPDATE/WRITE/ALL | Up to 100 synchronous records; structured per-record results | [S17] |
| Search | `GET /crm/v8/{module}/search` | One search mode; API names by project policy; module READ + `ZohoSearch.securesearch.READ` | `data` + `info`; indexing delay documented | [S22] |
| Related records | `GET /crm/v8/{module}/{record_id}/{related_list_api_name}` | Parent module, ID, related-list API name, `fields`; module READ/ALL | `data` + `info`; page token after first 2,000 | [S20] |
| Modules metadata | `GET /crm/v8/settings/modules` | Settings/modules READ or broader settings scope | Organization-specific module capability and API identity | [S21] |
| Fields metadata | `GET /crm/v8/settings/fields?module={module}` | Module API name; fields/settings scope | All layouts; not layout-specific mandatory fields/picklists | [S19] |
| Layouts metadata | `GET /crm/v8/settings/layouts?module={module}` | Module API name; layouts/settings scope | Layout sections, fields, visibility, and profiles | [S36] |
| COQL | `POST /crm/v8/coql` | `select_query`; API names; COQL + module read; fields scope if metadata included | `data` + `info`; confirmed structural limits only, with contradictory per-call record/field ceilings omitted | [S18, S50] |
| Bulk Read | `POST /crm/bulk/v8/read` → status/callback → ZIP result | Module, fields/criteria, job ID; bulk read + module read | Async CSV/ICS | [S23–S24, S51] |
| Bulk Write | ZIP CSV upload → `POST /crm/bulk/v8/write` → status/callback → result | File ID, operation, module, mappings, `find_by`; bulk + module operation scopes | Async result ZIP with row status/error | [S25–S26, S52–S53] |
| Attachments | `GET/POST /crm/v8/{module}/{record_id}/Attachments` | Module, record ID, file or link for REST upload, fields for list; module + attachment scope | One file or one link per upload call | [S27–S28] |
| ZFS files | `POST /crm/v8/files` | Multipart files; `ZohoCRM.Files.CREATE` | Encrypted file ID for file/image fields | [S29] |

## 10. Per-operation prerequisites

| Family | Required facts before generation |
|---|---|
| Any | Host, targets, exact context, capability, V8 surface |
| Records | Module and field API names, mandatory values, layout when relevant |
| Specific record | Module API name and record ID |
| Create/update/upsert | Automation triggers, layout/feature execution, and duplicate identity where applicable |
| Search | Search mode, API-name criteria, operators, and pagination intent |
| Related records | Parent module/ID, related-list API name, related record ID for update |
| COQL | Base module, selected API names, joins, criteria, order, expected volume |
| Bulk Read | Module, fields, criteria/view, callback versus polling, consumer |
| Bulk Write | Operation, ZIP/CSV, file ID, module, mappings, layout, `find_by`, callback/polling |
| Attachments/files | Module, record ID, source FILE/file, attachment versus file-upload field |
| Client Script | Module, layout, page, event/command/button, field, cancellation semantics |

Discover missing tenant identities through metadata or ask. Never translate labels, invent IDs, or fabricate production values.

## 11. Authentication and connection model

1. Prefer a named Zoho OAuth connection for every supported Deluge task or authenticated `invokeUrl` [S34].
2. Use the target-oriented default `conpas_[target-app]`; for CRM use `conpas_crm`.
3. Select scopes from the exact operation page; do not broaden scopes for convenience.
4. Configure OAuth clients, regional callback, sharing, production/sandbox target, and authorization in deployment UI.
5. Generated Deluge references only the connection link name.
6. Never request or embed access tokens, refresh tokens, client secrets, API keys, passwords, or credential-bearing URLs.

| Capability | Scope family |
|---|---|
| Records | `ZohoCRM.modules...` operation scopes [S15–S17] |
| Search | Module READ + `ZohoSearch.securesearch.READ` [S22] |
| COQL | `ZohoCRM.coql.READ` + module READ; fields scope if metadata included [S18] |
| Metadata | `ZohoCRM.settings.modules/fields/layouts...` [S19, S21, S36] |
| Bulk Read | `ZohoCRM.bulk.read` + module READ [S24] |
| Bulk Write | `ZohoCRM.bulk.CREATE/ALL` + module operation scope [S26] |
| Attachments | Module + attachment operation scope [S27–S28] |
| ZFS | `ZohoCRM.Files.CREATE` [S29] |

## 12. Actionable structural limits and excluded quota domains

### Runtime call-optimization policy

- Batch, paginate, and select only needed fields within the actionable structural limits below. Avoid redundant reads, repeated metadata fetches, and unnecessary calls inside loops.
- Parallel calls can fail because CRM enforces concurrency controls. Treat concurrency as an advisory operational risk: avoid unnecessary parallelism, bound work conservatively, and handle retryable failures. Never claim the user's exact threshold because it depends on licensing and tenant conditions.
- Do not estimate remaining daily capacity, Function credits, CRM API credits, or execution duration. The agent cannot reliably measure those values.

### Researched but excluded from skill contract

| Evidence area | Classification and required treatment | Source |
|---|---|---|
| Per-day request allowances | **Researched but excluded from skill contract.** Licensing-dependent daily allowances must not appear in runtime references, implementation checklists, or generated guidance. Apply only the qualitative call-optimization policy above. | [S43–S44, S50] |
| CRM Function credits | **Researched but excluded from skill contract.** Do not encode formulas, maxima, or the contradictory maximum-credit evidence. | [S43–S44] |
| CRM API credits | **Researched but excluded from skill contract.** Do not encode formulas, maxima, per-operation credit tiers, or estimates of remaining capacity. | [S50] |
| Numeric execution timeouts | **Researched but excluded from skill contract.** Do not encode timeout values or estimate whether generated work will finish within them. | [S32, S43, S49] |
| Exact concurrency thresholds | **Researched but excluded from skill contract.** Retain only the advisory failure risk above; do not infer or claim a tenant-specific threshold. | [S50] |

### Records, search, COQL, bulk, and files

| Surface | Confirmed scoped limits | Source |
|---|---|---|
| Get Records | **200/request**; ordinary pages to **2,000**; page-token traversal to **100,000**; token **24h**, user/parameter-bound; **50 fields**. | [S15] |
| Related Records | **200/request**; ordinary pages to **2,000**, then user-bound page token. | [S20] |
| Search | **200/request**, **2,000 total**, **10 criteria**, `in` up to **100 values**. | [S22] |
| COQL | Overall pagination to **100,000**; **2 joins**; `in/not in` **100 values**; ORDER BY **10 fields**; GROUP BY **4**; **5 aggregate** and **5 dynamic-formula fields**. Contradictory per-call record/field ceilings are intentionally omitted. | [S18] |
| Bulk Read | **200,000 records/job**; page max **500**; **200 select fields**; **25 criteria**; `in/not_in` **20 values**; result available for **1 day**; ICS **20,000/batch**. | [S24, S51] |
| Bulk Write | **25,000 records**, **200 columns**, ZIP **25MB**; one ZIP/request; parent-child ZIP may contain multiple CSVs; result available for **7 days**. | [S25–S26, S52–S53] |
| REST attachment upload | One file or one attachment link per upload call. | [S27] |
| ZFS | **10 files/call**, **20MB/file**. | [S29] |
| Client Script | **30 scripts/page**, **30 commands**, **5 static resources/page**, separate script/layout, documented event set only. | [S32, S47–S49] |

## 13. Approved standard module and field recognition baseline

### Catalog contract

The static knowledge base exists only to recognize standard Zoho CRM module and field names and API names:

- [Standard module baseline](zoho-crm-standard-modules.md)
- [Standard field baseline](zoho-crm-standard-fields.md)

It does not establish write safety, permissions, layouts, relationships, operation support, mandatory or read-only behavior, picklist values, quotas, or runtime policy. Those concerns remain outside the catalog and must not be inferred from its rows.

The catalogs were generated from authenticated CRM V8 Modules and Fields Metadata on **2026-08-29**, then filtered against the registered official standard CRM evidence [S13, S19, S21, S27–S28]. The result is an official-evidence-filtered standard baseline, not a globally exhaustive or organization-independent catalog. Authenticated runtime metadata remains authoritative for the target organization.

### Identity and filtering rules

- Module API name is authoritative. Official canonical display names replace customized tenant labels where the registered evidence proves the canonical name; useful observed aliases are explicitly labeled.
- Field identity is `(module_api_name, field_api_name)`. Field API name is authoritative; display labels are recognition hints that may be tenant-localized or renamed.
- Only field rows marked `custom_field: false` in the authenticated snapshot are retained. Data type is retained because it was already available and useful for recognition.
- `generated_type != custom` and `generated_type == default` are not evidence that a module is standard.
- Tenant-specific custom modules, extension- or tenant-generated entities, custom subforms, linking modules, field trackers, web tabs, integration surfaces, dashboards, and other organization-specific entries are excluded.

### Final coverage

| Measure | Final count |
|---|---:|
| Retained modules | 21 |
| Retained snapshot module types | 21 `default` |
| Field sections | 21 |
| Retained `custom_field: false` rows | 609 |
| Appendix entries for retained standard modules rejected by Fields API | 0 |

Every retained module has exactly one field section. No rejected tenant-specific surface remains in the appendix. The catalog deliberately excludes examples such as `CustomModule5001`, `CustomModule5002`, custom subforms such as `Subformulario_2`, linking modules such as `Services_X_Users__s`, field trackers such as `DealHistory`, web tabs such as `Informes_Analytcis`, and integration or dashboard surfaces such as `HubSpot`, `Zoho_Books`, and `Analytics`.

### Runtime authority

Use authenticated Modules and Fields Metadata for the target organization [S19, S21]. Fetch layout and related-list metadata separately when an operation requires those runtime facts [S20, S36]. Catalog recognition never substitutes for runtime validation.

## 14. Routing examples

| Scenario | Valid surfaces | Deterministic treatment |
|---|---|---|
| Update one CRM record in workflow | Workflow field update; V8 `updateRecord`; REST V8 | Warn about field update; prefer task when the custom path is still requested and its contract fits. |
| Validate before save | Validation/layout rule; Client Script | Prefer declarative rule if sufficient; otherwise Client Script `.js`. |
| Read one record in Deluge | V8 `getRecordById`; REST V8 | Prefer the task with `fields` and `conpas_crm` when it fits. |
| Cross-module filtered read | Search; COQL; Client Script ZDK | Use COQL for verified joins; if paths remain equally optimal, explain both and wait. |
| Export a large dataset | COQL; Bulk Read | Recommend async Bulk Read when volume/lifecycle requires it. |
| Import a large CSV | Synchronous `bulkCreate`/`bulkUpdate`; Bulk Write | Explain that the task and Bulk API are different systems; prefer Bulk Write for async CSV jobs. |
| Attach a Deluge FILE | V8 `attachFile`; REST upload | Task accepts FILE only; REST may accept a file or link. |
| Populate file-upload field | ZFS + Records API | Upload to ZFS, then use encrypted file ID. Do not model as record attachment. |
| Delete in Deluge | REST V8 or verified standard/manual surface | Closed task catalog has no delete task; verify the exact REST operation before generation. |
| Client Script calling CRM | ZDK CRM API/ZRC | JavaScript only; never choose Deluge merely because CRM is host. |
| CRM Deluge writing to Books | CRM standard surface; Books task/REST | Load CRM + Books + Deluge and default the target connection to `conpas_books`. |

## 15. Output and automated-test contracts

| Item | Deluge | Client Script |
|---|---|---|
| File | `[name].deluge` | `[name].js` |
| Placement | Exact function/workflow/schedule/button/related-list placement | Exact module/layout/page/event/command/button |
| Inputs | Argument and API-name mappings | Event arguments, field/page APIs, return/cancel behavior |
| Authentication | Explicit named connection and exact scopes | Logged-in context; ZDK/ZRC/connection configuration if used |
| Assumptions | Host, targets, context, V8, module/layout IDs | Module, layout, page, event |
| Standard warning | Required | Required |
| Security | No secret values | No secret values |

### Approved automated contract tests

| Group | Required proof |
|---|---|
| Activation/composition | Correct application skills and language skill load for CRM, cross-app, and external hosts |
| Language isolation | Client Script never activates Deluge and always outputs `.js` |
| Missing context | Ask only the missing routing fact; never repeat supplied facts |
| Path choice | Select the sole valid path; present and await choice among equally optimal paths |
| Standard warning | Warn without blocking valid custom work |
| Closed task catalog | Accept exactly 13 V8 names; route every miss away from task generation |
| Legacy isolation | New code is V8; legacy modification asks migrate/preserve; legacy never proves V8 |
| Deterministic task form | `fields`, API names, positional placeholders, explicit target connection, user pagination override |
| Automation options | `{}` positional placeholder is never confused with `{"trigger":[]}` |
| Response safety | No universal response type; operation-specific conversion/iteration |
| Prerequisites | Missing API identity, layout, record, relation, trigger, or job facts block generation |
| Source integrity | Every fact maps to official URL, date, and evidence status |
| Output/security | Placement, mappings, connection, assumptions, tests, and no secrets |
| Factual gaps | Unresolved facts are omitted from runtime references or validated before encoding |

Ordinary tests store source IDs and expected contracts; they do not scrape external pages. Executable Deluge tests require an available verifiable runtime.

## 16. Error and uncertainty behavior

### Stop and ask

- Missing host/target/context changes routing or skill loading.
- Language is ambiguous between Deluge and Client Script.
- Modifying legacy `zoho.crm.*` requires the migrate-versus-preserve decision.
- Tenant API identity, record/job identity, automation side effects, or security principal cannot be discovered.
- Multiple equally optimal paths remain.

### Stop without requesting secrets

- The path requires a token, key, secret, password, or credential URL in chat/code.
- The user requests hard-coded credentials.
- Required scopes cannot be configured through a named connection or deployment secret store.

### Factual-gap rule

Research gaps may remain in this dossier as review notes. They must not become uncertainty text inside the future runtime contract: omit the unresolved fact or validate it before encoding. Never invent task names, response wrappers, API names, IDs, scopes, limits, plans, ZDK methods, events, or placements.

## 17. Approved `zoho-crm` reference architecture

```text
zoho-crm/
├── SKILL.md
└── references/
    ├── routing.md
    ├── execution-contexts.md
    ├── deluge-tasks-v8.md
    ├── rest-v8.md
    ├── client-script.md
    ├── zoho-crm-standard-modules.md
    ├── zoho-crm-standard-fields.md
    ├── metadata-and-prerequisites.md
    ├── authentication.md
    ├── standard-capabilities.md
    ├── uncertainty-and-errors.md
    └── sources.md
```

| File | Responsibility |
|---|---|
| `SKILL.md` | Activation, hard rules, routing gates, execution steps, output contract |
| `routing.md` | Host/target/context matrix and human-choice gate |
| `execution-contexts.md` | Verified CRM Functions and Client Script contexts and actionable structural counts; no numeric execution timeouts |
| `deluge-tasks-v8.md` | Closed 13-task catalog, deterministic forms, response boundaries, legacy policy |
| `rest-v8.md` | Approved endpoint/scope/lifecycle groups, actionable structural limits, and qualitative call optimization |
| `client-script.md` | JavaScript pages/events/commands/buttons and actionable structural count constraints; no timeout |
| `zoho-crm-standard-modules.md` | Official-evidence-filtered module display/API-name recognition baseline; runtime metadata remains authoritative |
| `zoho-crm-standard-fields.md` | Per-module field display/API-name and data-type recognition baseline containing only snapshot rows marked `custom_field: false` |
| `metadata-and-prerequisites.md` | Runtime tenant discovery and operation inputs |
| `authentication.md` | `conpas_[target-app]`, exact scopes, and secret safety |
| `standard-capabilities.md` | Declarative alternatives and edition gates |
| `uncertainty-and-errors.md` | Stop/ask/no-guess behavior; unresolved facts omitted from runtime contract |
| `sources.md` | Official provenance URLs and evidence status; no catalog-version scheme |

The future `SKILL.md` remains concise and imperative; detailed facts live in focused references.

## 18. Gate status before #544

Settled policies are intentionally absent from this checklist.

- [x] Complete and reconcile the official-evidence-filtered standard module and field recognition catalogs: 21 modules, 21 field sections, 609 field rows, and no retained-module appendix errors.
- [x] Reinspect the five multi-record V8 task pages. `getRecords` has a documented `toJsonList()` handling path and `searchRecords` has documented direct iteration. Exact outer containers remain contradictory or unproved; related/bulk handling is deterministically excluded as specified in section 8.
- [x] Preserve the Quick Create contradiction and deterministically exclude Quick Create from the runtime contract.
- [x] Recheck schedule-function sources; exact argument mapping remains unavailable and is deterministically excluded.
- [x] Recheck server-side custom-button sources; a universal context/argument map remains unavailable and is deterministically excluded.
- [x] Recheck validation-function sources; exact arguments and return contract remain unavailable and are deterministically excluded.
- [x] Confirm Function API method, argument transport, API-name source, and OAuth expectation; the endpoint template remains unpublished in inspected sources and must be copied from the target function's REST API settings rather than synthesized.

The catalog blocker is resolved and the evidence gate is approved. These documents still require tracked delivery under #545, and #544 may begin only on explicit implementation instruction. The other factual gaps above remain conservatively excluded and create no runtime ambiguity.

## 19. Official-source conflicts and defects

| Claim group | Actual defect | Required treatment |
|---|---|---|
| Multi-record task responses | All five pages label responses `KEY-VALUE`; `getRecords` requires `toJsonList()` in its handling snippet, `searchRecords` iterates the response directly, and related/bulk pages do not publish a valid outer delimiter or handling snippet. | Encode only the two documented handling paths. Keep related/bulk responses opaque and never invent a shared wrapper or REST-style `data` key. |
| COQL limits | COQL error text and resolution disagree about per-call record/field ceilings, while separate API-credit evidence describes other LIMIT ranges. | Keep only confirmed structural limits; omit contradictory per-call record/field ceilings and all credit tiers from the skill contract. |
| Quick Create | The current Client Script events page explicitly documents Quick Create events, while the current FAQ explicitly says Quick Create is unsupported. | Preserve the conflict and omit Quick Create from the runtime contract; do not choose either runtime behavior. |
| Function maximum credits | Enterprise and Ultimate formula/maximum statements conflict. | **Researched but excluded from skill contract.** Preserve only this provenance note; it is not a blocker and must not enter runtime references. |
| Bulk Read V8 example | V8 create-job page contains a V7 `download_url` sample. | Treat sample URL as historical; use the V8 endpoint contract. |
| Related-list editions | Older related-list page says Enterprise maximum five; current edition matrix adds Professional three and Ultimate ten. | Use current matrix for availability; use older page only for setup behavior. |
| Client Script creation guide | Setup guide demonstrates only Create/Clone/Edit while overview/events list more pages. | Treat it as a walkthrough, not a closed page catalog. |
| `attachFile` coverage link | Task page delegates coverage to an unversioned attachment page with older examples. | Use direct V8 attachment coverage [S27]; task input remains FILE-only. |
| Legacy pages | Current legacy pages link old API generations and differ from V8. | Reject as V8 proof. |
| Schedule/button/validation pages | Dedicated and linked official URLs were empty or 404; S31 confirms association but does not expose the exact contracts. | Omit schedule arguments, implicit server-side button arguments, and validation signatures/returns unless target configuration or new official evidence supplies them. |
| Function REST API endpoint | S31 confirms GET/POST, argument transport, system-generated API name, and OAuth 2.0 for organization use, but does not publish the endpoint template; the dedicated guessed page is 404. | Use only the OAuth endpoint displayed in the target function's Settings → REST API screen. Never synthesize the URL or generate API-key authentication. |

## 20. Provenance registers

### Project-contract register

| ID | Public project URL | Contract role |
|---|---|---|
| P01 | https://github.com/Thrasno/jarvis-ai-devs/issues/542 | Application/language ownership and routing contract |
| P02 | https://github.com/Thrasno/jarvis-ai-devs/issues/545 | Dossier scope, catalog ownership, provenance, and gate |
| P03 | https://github.com/Thrasno/jarvis-ai-devs/issues/544 | Skill implementation boundary and dependency on #545 |

### Official Zoho source register

All 53 successful claim sources below were opened and inspected on **2026-08-28**. This continuation pass inspected **18 official Zoho URLs**: ten registered claim sources were reinspected (S03, S04, S07–S09, S30–S33, and S46), two official Zoho sitemaps were inspected only for source discovery, and six accesses were empty/404 (including a repeat of A05 and an unavailable help sitemap). The dossier therefore remains at **53 successful claim sources** and now records **13 unique relevant non-evidentiary attempts**. Accessibility does not by itself validate a claim.

| ID | Official URL | Topic/version | Status |
|---|---|---|---|
| S01 | https://www.zoho.com/deluge/help/crm-integration-tasks-V8.html | CRM V8 task index | Accessible; confirmed 13-item source catalog |
| S02 | https://www.zoho.com/deluge/help/crm/create-record-V8.html | `v8.createRecord` | Accessible; confirmed |
| S03 | https://www.zoho.com/deluge/help/crm/get-records-V8.html | `v8.getRecords` | Accessible; exact outer shape unproved; `toJsonList()` handling confirmed |
| S04 | https://www.zoho.com/deluge/help/crm/search-records-V8.html | `v8.searchRecords` | Accessible; exact outer shape unproved; direct iteration confirmed |
| S05 | https://www.zoho.com/deluge/help/crm/get-record-by-id-V8.html | `v8.getRecordById` | Accessible; confirmed |
| S06 | https://www.zoho.com/deluge/help/crm/update-record-V8.html | `v8.updateRecord` | Accessible; confirmed |
| S07 | https://www.zoho.com/deluge/help/crm/bulk-create-records-V8.html | `v8.bulkCreate` | Accessible; adjacent per-record maps do not prove outer container; no generic handling encoded |
| S08 | https://www.zoho.com/deluge/help/crm/bulk-update-records-V8.html | `v8.bulkUpdate` | Accessible; adjacent per-record maps do not prove outer container; no generic handling encoded |
| S09 | https://www.zoho.com/deluge/help/crm/get-related-records-V8.html | `v8.getRelatedRecords` | Accessible; single-record sample does not prove multi-record outer container; no generic handling encoded |
| S10 | https://www.zoho.com/deluge/help/crm/update-related-record-V8.html | `v8.updateRelatedRecord` | Accessible; confirmed |
| S11 | https://www.zoho.com/deluge/help/crm/convert-lead-V8.html | `v8.convertLead` | Accessible; confirmed |
| S12 | https://www.zoho.com/deluge/help/crm/upsert-record-V8.html | `v8.upsert` | Accessible; confirmed |
| S13 | https://www.zoho.com/deluge/help/crm/attach-file-V8.html | `v8.attachFile` | Accessible; FILE input confirmed |
| S14 | https://www.zoho.com/deluge/help/crm/get-fields-V8.html | `v8.getFields` | Accessible; Map containing fields List |
| S15 | https://www.zoho.com/crm/developer/docs/api/v8/get-records.html | Records read V8 | Accessible; confirmed |
| S16 | https://www.zoho.com/crm/developer/docs/api/v8/insert-records.html | Records insert V8 | Accessible; confirmed |
| S17 | https://www.zoho.com/crm/developer/docs/api/v8/update-records.html | Records update V8 | Accessible; confirmed |
| S18 | https://www.zoho.com/crm/developer/docs/api/v8/Get-Records-through-COQL-Query.html | COQL V8 | Accessible; limit conflict noted |
| S19 | https://www.zoho.com/crm/developer/docs/api/v8/field-meta.html | Field metadata V8 | Accessible; confirmed |
| S20 | https://www.zoho.com/crm/developer/docs/api/v8/get-related-records.html | Related records V8 | Accessible; confirmed |
| S21 | https://www.zoho.com/crm/developer/docs/api/v8/modules-api.html | Module metadata V8 | Accessible; confirmed |
| S22 | https://www.zoho.com/crm/developer/docs/api/v8/search-records.html | Search V8 | Accessible; confirmed |
| S23 | https://www.zoho.com/crm/developer/docs/api/v8/bulk-read/overview.html | Bulk Read V8 overview | Accessible; confirmed |
| S24 | https://www.zoho.com/crm/developer/docs/api/v8/bulk-read/create-job.html | Bulk Read V8 create job | Accessible; V7 sample defect noted |
| S25 | https://www.zoho.com/crm/developer/docs/api/v8/bulk-write/overview.html | Bulk Write V8 overview | Accessible; confirmed |
| S26 | https://www.zoho.com/crm/developer/docs/api/v8/bulk-write/create-job.html | Bulk Write V8 create job | Accessible; confirmed |
| S27 | https://www.zoho.com/crm/developer/docs/api/v8/upload-attachment.html | Attachment upload V8 | Accessible; 21 resources; unknown size researched but excluded from skill contract |
| S28 | https://www.zoho.com/crm/developer/docs/api/v8/get-attachments.html | Attachment list V8 | Accessible; confirmed |
| S29 | https://www.zoho.com/crm/developer/docs/api/v8/upload-files-to-zfs.html | ZFS V8 | Accessible; confirmed |
| S30 | https://www.zoho.com/crm/developer/docs/functions/ | CRM Functions overview | Accessible; context authority, not V8 example authority |
| S31 | https://www.zoho.com/crm/developer/docs/functions/set-up-functions.html | Function setup/API/workflow mapping | Accessible; association, methods, arguments, generated API name, and authentication modes confirmed; endpoint template absent |
| S32 | https://www.zoho.com/crm/developer/docs/client-script/overview.html | Client Script overview | Accessible; confirmed |
| S33 | https://www.zoho.com/crm/developer/docs/client-script/client-script-events.html | Client Script events | Accessible; Quick Create conflict noted |
| S34 | https://www.zoho.com/deluge/help/connections.html | Named Deluge connections | Accessible; confirmed |
| S35 | https://www.zoho.com/deluge/help/crm/get-record-by-id.html | Legacy `getRecordById` | Accessible; rejected as V8 proof |
| S36 | https://www.zoho.com/crm/developer/docs/api/v8/layouts-meta.html | Layout metadata V8 | Accessible; confirmed |
| S37 | https://www.zoho.com/deluge/help/crm/get-records.html | Legacy `getRecords` | Accessible; rejected as V8 proof |
| S38 | https://www.zoho.com/deluge/help/crm/create-record.html | Legacy `createRecord` | Accessible; rejected as V8 proof |
| S39 | https://www.zoho.com/deluge/help/crm/update-record.html | Legacy `updateRecord` | Accessible; rejected as V8 proof |
| S40 | https://www.zoho.com/deluge/help/crm/search-records.html | Legacy `searchRecords` | Accessible; rejected as V8 proof |
| S41 | https://www.zoho.com/deluge/help/crm/upsert-record.html | Legacy `upsert` | Accessible; rejected as V8 proof |
| S42 | https://www.zoho.com/deluge/help/release-notes.html | 2026-04-15 CRM V8 task release | Accessible; confirmed publication date |
| S43 | https://www.zoho.com/crm/developer/docs/functions/functions-limits.html | Function credits/runtime | Accessible; quota and timeout evidence researched but excluded from skill contract |
| S44 | https://www.zoho.com/crm/complete-feature-list.html | Edition comparison and standard capabilities | Accessible; confirmed current matrix |
| S45 | https://www.zoho.com/crm/help/customization/related-lists.html | Related-list function setup | Accessible; older availability conflict noted |
| S46 | https://www.zoho.com/crm/developer/docs/client-script/creation.html | Client Script creation | Accessible; partial walkthrough |
| S47 | https://www.zoho.com/crm/developer/docs/client-script/commands.html | Client Script commands | Accessible; confirmed |
| S48 | https://www.zoho.com/crm/developer/docs/client-script/client-script-best-practices.html | Event arguments/script limits | Accessible; confirmed |
| S49 | https://www.zoho.com/crm/developer/docs/client-script/FAQs.html | Client Script FAQ | Accessible; Quick Create conflict noted |
| S50 | https://www.zoho.com/crm/developer/docs/api/v8/api-limits.html | API credits/concurrency/sub-concurrency | Accessible; quota and exact-threshold evidence researched but excluded from skill contract; concurrency risk retained qualitatively |
| S51 | https://www.zoho.com/crm/developer/docs/api/v8/bulk-read/limitations.html | Bulk Read V8 limits | Accessible; confirmed |
| S52 | https://www.zoho.com/crm/developer/docs/api/v8/bulk-write/limitations.html | Bulk Write V8 limits | Accessible; confirmed |
| S53 | https://www.zoho.com/crm/developer/docs/api/v8/bulk-write/upload-file.html | Bulk Write V8 upload | Accessible; confirmed |

### Non-evidentiary access attempts

These URLs were opened but returned empty content or 404. They support no product claim and are excluded from the successful-source count.

| ID | Attempted URL | Intended topic | Result |
|---|---|---|---|
| A01 | https://help.zoho.com/portal/en/kb/crm/automate-business-processes/workflow-management/articles/workflow-rules | Dedicated workflow article | Empty response; standard actions instead sourced from S44 |
| A02 | https://www.zoho.com/crm/developer/docs/functions/schedules.html | Function schedules | 404 |
| A03 | https://www.zoho.com/crm/developer/docs/functions/custom-buttons.html | Function custom buttons | 404 |
| A04 | https://www.zoho.com/crm/developer/docs/api/v8/get-fields.html | Guessed field page | 404; S19 is authoritative |
| A05 | https://help.zoho.com/portal/kb/articles/create-validation-rules-using-functions | Validation functions | Empty response |
| A06 | https://help.zoho.com/portal/en/kb/crm/customize-crm-account/customizing-page-layouts/articles/create-conditional-layouts | Layout rules article | Empty response; facts sourced from S16–S17, S33, S44 |
| A07 | https://help.zoho.com/portal/en/kb/crm/process-management/blueprint/articles/blueprint-an-overview | Blueprint article | Empty response; facts sourced from S21, S33, S44 |
| A08 | https://help.zoho.com/portal/en/kb/crm/customize-crm-account/custom-links-and-buttons/articles/custom-buttons | Custom buttons article | Empty response; facts sourced from S32–S33, S44 |
| A09 | https://help.zoho.com/portal/en/kb/crm/customize-crm-account/customizing-fields/articles/create-formula-fields | Formula fields article | Empty response; facts sourced from S19, S44 |
| A10 | https://www.zoho.com/crm/help/automation/custom-schedules.html | Linked schedule-function setup | Empty response; S31 confirms association only |
| A11 | https://www.zoho.com/crm/help/customization/custom-buttons.html | Linked server-side custom-button setup | Empty response; S31 confirms association only |
| A12 | https://help.zoho.com/portal/en/kb/crm/automate-business-processes/validation-rules/articles/create-validation-rules-using-functions | Validation-function arguments and return | Empty response; no contract inferred |
| A13 | https://www.zoho.com/crm/developer/docs/functions/functions-api.html | Guessed dedicated Function API endpoint page | 404; exact endpoint template remains unpublished in inspected sources |

## 21. Maintainer-directed maintenance

- Keep official provenance beside each durable fact.
- Change the closed task catalog, routing policy, security contract, plan gates, or approved catalog contract only through an explicit maintainer decision.
- When a maintainer chooses to refresh a fact, inspect the exact affected official operation pages and preserve any unresolved source conflict.
- Reachability checks and claim validation remain separate; blocked URLs do not prove feature removal.
- Historical examples may explain legacy behavior only when labeled; they never establish a V8 contract.

### Maintainer checklist for #544 handoff

- [x] Keep actionable records/search pagination and field/criteria caps.
- [x] Keep confirmed COQL structural limits and omit contradictory per-call record/field ceilings.
- [x] Keep Bulk Read/Write job, file, row, column, and lifecycle constraints that shape code.
- [x] Keep ZFS/file constraints and attachment one-file-or-link behavior.
- [x] Keep actionable Client Script structural counts and omit numeric execution timeouts.
- [x] Keep concurrency as an advisory failure risk without claiming the user's threshold.
- [x] Exclude per-day quotas, Function-credit formulas/maxima, CRM API-credit formulas/maxima, and numeric execution timeouts from runtime references and implementation checklists.
- [x] Keep the two recognition catalogs reconciled at 21 modules, 21 field sections, 609 field rows, and zero retained-module appendix errors; use runtime metadata as organization authority.

The evidence gate is approved. These documents still require tracked delivery under #545, and #544 may begin only on explicit implementation instruction.
