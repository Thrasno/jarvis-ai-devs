# Zoho Books skill evidence dossier

| Field | Value |
|---|---|
| Status | **Evidence artifacts written; REST, resource, published-field, and Deluge snapshot reconciliation completed with one explicit OpenAPI response-shape omission; maintainer review still required before skill implementation** |
| Capture/verification date | **2026-08-31** |
| Related project issues | [#542](https://github.com/Thrasno/jarvis-ai-devs/issues/542), [#605](https://github.com/Thrasno/jarvis-ai-devs/issues/605) |
| REST evidence | [Complete v3 operation catalog](zoho-books-rest-v3.md) |
| Deluge evidence | [Complete seven-task catalog](zoho-books-integration-tasks.md) |
| Standard evidence | [Bounded 20-family capability taxonomy](zoho-books-standard-capabilities.md) |
| Standard resource evidence | [42-resource recognition catalog](zoho-books-standard-resources.md) |
| Standard API-field evidence | [File-local schema and field recognition catalog](zoho-books-standard-fields.md) |

> **Answer first:** the official Books REST v3 snapshot closes at **849 operation records across 42 OpenAPI files**, reconciled one-to-one against **849 non-Overview index entries**. Those same files now close as **42 resource rows, 4,671 component schemas, 2,527 inline roots, 7,198 total roots, and 20,987 unique flattened field/structure rows**. All local schema references resolve, no recursive cycle is present, and all **1,084 published request/response schema references or inline markers** reconcile. OpenAPI publishes no response schema for `get_delivery_challan_attachment`, so no response root exists and its payload shape remains an explicit TBD rather than a fabricated field. The independent official Deluge Books index closes at **7 Integration Tasks**, and the standard-capability taxonomy contains **20 stable intent families**. This is still evidence delivery, not `zoho-books` skill implementation, and the maintainer gate remains open.

## 1. Approved contract

| Topic | Maintainer decision encoded here |
|---|---|
| Catalog order | Complete REST API v3 first. The Deluge Integration Task catalog is independently complete and never inferred from REST. |
| REST closure | Use a dated official OpenAPI ZIP plus every non-Overview entry in the official v3 index. A completeness claim requires a closed reconciliation. |
| REST coverage | Preserve every documented operation, including lifecycle, bulk, attachment, banking, regional/compliance, and other non-V0 surfaces. |
| Task closure | Use only official Deluge task pages and prove closure against an official enumerable index when available. |
| Routing | Standard Books functionality → compatible Integration Task → REST v3 → another verified surface/manual outcome. A task miss never means automatic REST. |
| Standard advice | Advisory and non-blocking when code generation was explicitly requested. |
| Version policy | New REST code uses v3. Integration Tasks remain separate. Legacy code is inspected for old API calls; warn and ask migrate-or-preserve. Without explicit migration approval, modify that function using its existing calls/version. |
| Connection | Target-oriented `conpas_[target-app]`; Books default `conpas_books`; never request or embed secrets. |
| Generation and execution | An explicit request authorizes code generation, including destructive code. Future live destructive/financial execution requires an impact warning and explicit confirmation immediately before execution. |
| Limits | Preserve only structural constraints that change generated code. Exclude volatile quotas, credits, concurrency thresholds, capacity estimates, and timeouts. Check plan/edition only for capability availability. |
| Runtime authority | Organization metadata, settings, plan, region, permissions, providers, and configured connections override static recognition. |
| Resource/field baseline | Use only standard resource groups and field structures published in the verified 42-file OpenAPI snapshot. Preserve file-local schema identity; exclude tenant/custom definitions and values. OpenAPI absence does not prove runtime non-support. |
| Maintenance | Catalog updates happen only by explicit maintainer decision. No scheduler/background automation. Every manual refresh requires dated source capture, hashes, diff, and approval. |

## 2. Readiness summary

| Evidence gate | Result | Status |
|---|---|---|
| Official REST ZIP accessible | `openapi-all.zip`, 42 YAML files, OpenAPI 3.0.0 | Closed |
| REST ZIP integrity | SHA-256 `c6a841bbc81ef882c64b1f2ad4761e350faefba1db7fb36a32daf7112b647559` | Closed |
| REST operation extraction | 849 rows: GET 263, POST 338, PUT 131, DELETE 117 | Closed |
| Official REST index extraction | 849 non-Overview operation entries across the same 42 resource pages; 848 unique resource/fragment identities because one Contacts anchor occurs twice | Closed |
| REST reconciliation | 823 normalized summary/anchor matches plus 26 official wording aliases; 0 unmatched | Closed |
| REST identity defects | 6 duplicated operation IDs and 4 duplicated method/path identities retained and flagged | Closed with explicit defects |
| Per-operation organization handling | 841 operations require `organization_id`; 8 omit it. One omission (`email_sales_receipt`) is anomalous and flagged. | Closed with explicit uncertainty |
| Standard resource extraction | 42 source YAML files → 42 deterministic file-local resource rows; 849 operations: GET 263, POST 338, PUT 131, DELETE 117; 590 file-local distinct paths; 0 resource omissions | Closed |
| Standard schema/root extraction | 4,671 component schemas + 2,527 inline operation request/response/parameter roots = 7,198 roots; request-only 3,029, response-only 2,375, both 1,214, unreferenced 580 | Closed for published roots |
| Standard field flattening | 20,987 flattened property/structure rows and 20,987 unique `(source file, root identity, field path)` identities; 1,023 API names collide across scopes, proving file/root scope is required | Closed for published structures |
| Schema traversal integrity | 0 unresolved local references, 0 recursive cycles, 0 malformed schema records; 3,215 zero-field roots retained in a separate auditable ledger | Closed |
| REST/schema-reference reconciliation | 1,084 published request/response schema references or inline markers from all 849 REST rows checked; 0 mismatches | Closed |
| OpenAPI response-shape omission | `get_delivery_challan_attachment` publishes response description `Returns the file content.` but no response `content` or `schema` | Explicit TBD; no response root or payload shape fabricated |
| Official Deluge enumerable index | 7 linked Books tasks | Closed |
| Deluge page reconciliation | 7 pages inspected, 7 rows emitted, 0 missing | Closed |
| Deluge `updateRecord` parameter inconsistency | Syntax uses `books_connection`; the parameter table labels the same trailing argument `connection` | Explicit unresolved source conflict |
| Deluge callable-casing inconsistency | `getRecordsByID` in note/syntax versus `getRecordsById` in example; the “Fetch record by ID” heading does not resolve the spelling | Explicit unresolved source conflict |
| Standard capability taxonomy | 20 required intent families with official help/developer evidence | Ready as bounded recognition |
| Skill implementation | No runtime skill, embedded asset, generated user configuration, or contract test was created here | Not started; maintainer review required |

## 3. Evidence methodology

### REST snapshot

Official sources:

- `https://www.zoho.com/books/api/v3/openapi-all.zip`
- `https://www.zoho.com/books/api/v3/`

Capture method:

```text
curl --fail --location --silent --show-error --output openapi-all.zip https://www.zoho.com/books/api/v3/openapi-all.zip
curl --fail --location --silent --show-error --output api-index.html https://www.zoho.com/books/api/v3/
sha256sum openapi-all.zip api-index.html
unzip openapi-all.zip -d openapi
```

The downloaded index HTML hash was `ffe642b5172332538a2d5d93f3a0aacf893b1a636f200c609bfaece115b40802`. A bounded Python 3/PyYAML extractor enumerated HTTP method keys under every OpenAPI `paths` object, retained operation metadata and schema references, resolved required parameter references, and parsed the official index with the Python standard-library HTML parser. Reconciliation occurred one-to-one within the same resource. For resource/field closure, the same archive was independently re-downloaded and hash-checked, every component schema and inline operation request/response/parameter root was assigned a file-local identity, request/response reachability was propagated through local references, and nested properties/compositions/arrays/maps were flattened with bounded cycle detection. The temporary extractor and downloaded source remained outside the repository; no downloader, refresh scheduler, generated schema copy, or maintenance automation was added to the repository.

### Reconciliation interpretation

- The index has 849 operational entries after excluding documentation topics and every `Overview` link.
- One Contacts fragment, `verify-a-contact-address`, appears twice and corresponds to two distinct OpenAPI operation rows.
- 26 OpenAPI summary slugs differ from official index anchors through punctuation, possessives, `and`/`&`, or minor official wording. Every alias is listed in the REST catalog.
- Duplicated `operationId` and method/path identities mean none of those fields is globally unique alone. Snapshot identity is `(source resource file, method, path, operationId)`.
- REST closure means that this dated official snapshot and dated index reconcile. It does not promise that later documentation, every country edition, or every live organization has identical availability.

### Deluge tasks

The official [Zoho Books Tasks index](https://www.zoho.com/deluge/help/books-tasks.html) is enumerable and links exactly seven pages. Each page's task name, syntax, parameter table, response evidence, connection notes, modules/statuses where bounded, URL, and uncertainty were inspected independently. No task was created from a corresponding REST operation.

### Standard capabilities

The standard taxonomy uses current official Books help/developer pages. It records stable intent families rather than mirroring dynamic menus. This avoids false global claims where plan, edition, country, gateway, bank feed, permissions, and live settings vary.

### Standard resources and API fields

The [resource catalog](zoho-books-standard-resources.md) contains exactly one row per official source YAML file and never invents a single primary identifier or semantic relationship. The [field catalog](zoho-books-standard-fields.md) preserves schema-name collisions through `(source file, root identity, field path)`, distinguishes request from response reachability, and inventories zero-property roots separately. Structural rows preserve `$ref`, composition, array/item, additional-properties, enum/default, format, nullable, read-only/write-only, and nested-object evidence where published. OpenAPI-published envelope properties such as `custom_fields` are retained as standard API property names, but no tenant-defined custom field name, label, value, or organization configuration is included.

## 4. Scope and product boundaries

### In scope

- Books REST API v3 operations in the dated official ZIP/index.
- Standard OpenAPI-published resource groups, component schemas, inline operation schemas, parameters, and field structures from all 42 files in that ZIP.
- Official `zoho.books.*` Deluge Integration Tasks.
- Standard Books intent recognition, including Books-native expenses, items, projects, and time tracking.
- Extensions/widgets as distinct verified developer surfaces.

### Outside `zoho-books` and the initial pack

- Zoho Inventory, Expense, Billing, Payments, Payroll, BillPay, and other adjacent products.
- Authenticated tenant snapshots, credentials, private organization data, and live execution.
- Tenant-defined custom fields, custom-field values, organization-specific configuration values, and schemas from adjacent-product APIs. The archive's `integration.yml` remains in scope because it is an official Books API resource file, not an adjacent-product API snapshot.
- An exhaustive Books UI, plan matrix, country compliance matrix, gateway list, error-code catalog, or volatile limit catalog.
- Any claim that a similarly named Books-native capability belongs to an adjacent app.

## 5. Deterministic routing

1. Identify host, every target, actual language, execution context, requested business behavior, and whether this is generation or future live execution.
2. Check [standard Books capabilities](zoho-books-standard-capabilities.md). Warn when a standard route may satisfy the intent, but continue an explicit code request.
3. Use the [resource](zoho-books-standard-resources.md) and [API-field](zoho-books-standard-fields.md) catalogs only for static recognition; resolve live availability and writable shape from runtime metadata/settings and the selected operation contract.
4. If actual code is Deluge, evaluate the exact [seven-task catalog](zoho-books-integration-tasks.md). Prefer a task only when every relevant contract fits.
5. If no compatible task exists, independently evaluate [REST v3](zoho-books-rest-v3.md). Never infer REST from task absence alone.
6. Evaluate another verified surface such as an extension/widget, or state a bounded manual/unsupported outcome.
7. If several equally optimal valid paths remain, explain and recommend; human selection remains authoritative.

## 6. Authentication and organization prerequisites

- Use OAuth and the exact selected operation's scope evidence. Do not broaden scopes for convenience.
- Generated Deluge uses the named target connection `conpas_books` by default.
- Never request/embed access tokens, refresh tokens, client secrets, passwords, API keys, or credential-bearing URLs.
- Treat `organization_id` per operation/task. `getOrganizations(connection)` is the explicit task bootstrap without it. REST has eight snapshot operations without the parameter, detailed in the operation catalog.
- Resolve organization IDs, region/domain, feature availability, permissions, and settings from runtime metadata/configuration or ask only when selection/generation requires them.

## 7. Version and legacy behavior

- New REST generation always uses Books API v3.
- Integration Tasks remain `zoho.books.*`; do not relabel them as REST v3 calls.
- Before changing a function containing older Books API calls, identify the current version and warn about migration impact.
- Ask one focused migrate-or-preserve question. Never change versions without explicit confirmation.
- If migration is declined or not approved, preserve and modify the existing call/version in that function. Current v3 evidence must not be presented as proof of legacy behavior.

## 8. Safety contract

Code generation and execution are different authorities:

- Explicit generation requests authorize generation, including destructive operations; do not ask a redundant generation confirmation.
- A future tool or agent about to execute destructive or financially sensitive behavior against live Books data must state the concrete impact and receive explicit confirmation immediately before execution.
- This dossier does not grant live access and implements no executor.

## 9. Maintenance contract

There is no scheduled review, scraper, watcher, or background refresh. A maintainer must explicitly decide to update either independent catalog. A manual refresh must preserve:

1. capture date and source URLs;
2. downloadable artifact and page hashes where available;
3. deterministic extraction method;
4. old/new counts and a reviewed diff;
5. duplicates, aliases, exclusions, conflicts, and unresolved mismatches;
6. explicit maintainer approval before replacing the accepted snapshot.

## 10. Remaining TBDs and conservative treatment

| Item | Why unresolved | Required treatment |
|---|---|---|
| `updateRecord` trailing connection parameter | The official syntax names it `books_connection`, while the same page's parameter table labels it `connection`. | Preserve the literal syntax signature in the catalog and record both labels as placeholders for the same trailing connection argument. Do not present either parameter label as a separate overload. |
| `getRecordsByID` versus `getRecordsById` | One official task page conflicts between syntax/note and example casing. | Preserve `getRecordsByID` from the syntax section as catalog identity, record the conflict, and do not claim case-insensitivity. Reinspect before runtime contract finalization if exact casing is material. |
| REST `email_sales_receipt` organization context | Its OpenAPI operation omits `organization_id` while adjacent sales-receipt operations generally include organization context. | Report `not listed`; do not inject the parameter without new official operation evidence. Runtime selection remains cautious. |
| `get_delivery_challan_attachment` response shape | The REST catalog records `200: no schema published`, matching the OpenAPI `200` response, which publishes only `description: Returns the file content.` with no `content` or `schema`. | Keep the field catalog root inventory honest: no response root exists to flatten. Treat the payload shape as TBD until a future explicitly approved official snapshot publishes it; do not fabricate a binary/string schema. |
| Plan/region/organization availability | Official surfaces are dynamic and capability availability is tenant-specific. | Check runtime plan/edition only for capability availability; runtime organization metadata/settings win. |
| Live destructive/financial behavior | This delivery is generation-only evidence. | Require impact warning plus explicit confirmation only at future execution time. |

No REST index/OpenAPI operation-identity or published-schema-marker mismatch remains unresolved in the 2026-08-31 snapshot. The missing `get_delivery_challan_attachment` response shape remains an explicit upstream omission/TBD.

## 11. Gate checklist

- [x] REST official ZIP captured, hashed, and deterministically extracted.
- [x] Every one of 849 OpenAPI operation records emitted with required evidence fields.
- [x] Every one of 849 official-index non-Overview operation entries reconciled.
- [x] Duplicates, aliases, exclusions, and uncertainties recorded.
- [x] Exactly 42 OpenAPI resource files reconciled to 42 standard resource rows with method totals closed.
- [x] All 4,671 component schemas and 2,527 inline operation roots inventoried with file-local identity and request/response usage.
- [x] All 20,987 unique flattened field/structure identities emitted; zero-field roots, composition/reference facts, and collisions retained.
- [x] All 1,084 published REST request/response schema references or inline markers checked with 0 mismatches; the separate missing response shape retained as an OpenAPI TBD.
- [x] Seven Deluge tasks closed independently against the official enumerable index.
- [x] Twenty standard capability families cited and bounded.
- [x] Adjacent products separated from Books-native capabilities.
- [x] v3 generation, legacy preserve/migrate, connection, secret-safety, and execution-confirmation policies encoded.
- [x] Volatile numeric limits excluded and manual-only maintenance encoded.
- [ ] Maintainer reviews and approves these evidence artifacts before `zoho-books` implementation.

## 12. Source ledger

| ID | Source | Role/status |
|---|---|---|
| P01 | https://github.com/Thrasno/jarvis-ai-devs/issues/542 | Current shared application/language/routing contract, read with `gh` on 2026-08-31 |
| P02 | https://github.com/Thrasno/jarvis-ai-devs/issues/605 | Current Books evidence gate and per-operation organization requirement, read with `gh` on 2026-08-31 |
| R01 | https://www.zoho.com/books/api/v3/openapi-all.zip | Official 42-file OpenAPI snapshot; accessible and hashed |
| R02 | https://www.zoho.com/books/api/v3/ | Official REST v3 index; accessible, hashed, and reconciled |
| R03 | [Standard resource recognition catalog](zoho-books-standard-resources.md) | Deterministic 42-row derivation from R01; operation/method closure and file-local identities |
| R04 | [Standard API-field recognition catalog](zoho-books-standard-fields.md) | Complete published schema/root inventory, flattened fields, zero-field roots, cycles/references, and REST-schema reconciliation from R01 |
| D00 | https://www.zoho.com/deluge/help/books-tasks.html | Official enumerable seven-task index |
| D01–D07 | See [Integration Task source ledger](zoho-books-integration-tasks.md#source-ledger) | Seven exact task pages |
| S01–S20 | See [standard-capability rows](zoho-books-standard-capabilities.md#verified-capability-families) | Official Books help/developer evidence by intent family |
