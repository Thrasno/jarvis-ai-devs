# Zoho CRM Skill

## Purpose

CRM generation MUST be evidence-backed.

## Requirements

### Requirement: Activation, composition, and placement

The skill MUST recognize CRM as host, target, or both and compose. Output MUST stay correctly placed: Deluge `[name].deluge`, Client Script `[name].js`, external code in its requested language/placement.

#### Scenario: CRM-only Deluge

- GIVEN CRM Deluge requested
- WHEN activated
- THEN it loads `zoho-crm` plus `zoho-deluge` and outputs `[name].deluge`.

#### Scenario: Cross-application Deluge

- GIVEN a Deluge request involving CRM and other apps
- WHEN activated
- THEN it loads every involved application skill plus `zoho-deluge` and emits `[name].deluge`.

#### Scenario: CRM Client Script

- GIVEN a CRM Client Script request
- WHEN activated
- THEN it uses JavaScript, emits `[name].js`, and MUST NOT load `zoho-deluge`.

#### Scenario: External runtime

- GIVEN an external-runtime request
- WHEN activated
- THEN it uses the requested language and placement with relevant skills, not Deluge.

### Requirement: Routing and V8 policy

New CRM code MUST use `zoho.crm.v8.*` or REST API V8. The allowlist MUST contain exactly 13 tasks: `createRecord`, `getRecords`, `searchRecords`, `getRecordById`, `updateRecord`, `bulkCreate`, `bulkUpdate`, `getRelatedRecords`, `updateRelatedRecord`, `convertLead`, `upsert`, `attachFile`, and `getFields`.

#### Scenario: Allowlisted operation

- GIVEN an operation is allowlisted
- WHEN routing occurs
- THEN it selects V8 without an API-version question.

#### Scenario: Allowlist miss

- GIVEN an operation is absent from the allowlist
- WHEN routing occurs
- THEN it evaluates standard CRM configuration, Client Script/ZDK, REST V8, COQL, metadata/bulk APIs, verified surfaces, or manual workaround before reporting unsupported.

### Requirement: Legacy and API names

The skill MUST use module/field API names. Existing `zoho.crm.*` modifications MUST ask whether to migrate to V8 or preserve legacy behavior; legacy documentation MUST NOT establish V8 behavior.

#### Scenario: New versus legacy request

- GIVEN new-code and legacy-modification requests
- WHEN guidance is composed
- THEN new code is V8-only and the legacy request receives the choice.

### Requirement: Catalog and runtime authority

The static catalog MAY recognize only standard display/API names from the 21-module, 609-field baseline. Runtime metadata MUST govern permissions, schema, layouts, relationships, required/read-only fields, picklists, operation support, quotas, and policy.

#### Scenario: Catalog and runtime disagreement

- GIVEN a catalog name has differing runtime metadata
- WHEN safety or operation support is evaluated
- THEN it resolves the API name without asserting organization support; runtime metadata wins.

### Requirement: Responses and alternatives

Responses MUST preserve operation boundaries: `getRecords` uses JSON-list handling, `searchRecords` direct iteration, and `getRelatedRecords`, `bulkCreate`, and `bulkUpdate` remain opaque absent evidence. It MUST NOT invent a universal `data` wrapper and MUST explain verified alternatives.

#### Scenario: Response and alternative guidance

- GIVEN standard CRM configuration satisfies custom code or an allowlisted operation is used
- WHEN guidance is generated
- THEN it explains verified alternatives without blocking valid work and applies evidenced operation-specific/opaque handling.

### Requirement: Bounded questions and limits

The skill MUST ask only missing facts that change routing or prevent safe generation. Plan/edition checks MAY establish availability only. Guidance MUST retain structural limits, treat concurrency as qualitative risk, avoid unnecessary parallelism, and omit quotas, credit formulas, capacity estimates, exact concurrency thresholds, and numeric timeouts.

#### Scenario: Ambiguous routing

- GIVEN one valid path remains or paths are equally optimal
- WHEN context is assessed
- THEN it uses and explains the single path; otherwise it recommends one and waits for selection.

### Requirement: Authentication and exclusions

The skill MUST never request or embed secrets, SHOULD prefer named OAuth `conpas_crm`, and MUST document scopes and secure deployment. Unsupported or unevidenced schedule arguments, implicit custom-button arguments, validation signatures/returns, Quick Create behavior, Function API endpoint templates, non-CRM applications, and other unapproved API facts MUST be excluded, not guessed.

#### Scenario: Safe generation boundary

- GIVEN a request includes credentials or an intentionally excluded fact
- WHEN output is composed
- THEN no secret is embedded, and detail is identified as excluded with a safe alternative or clarification.
