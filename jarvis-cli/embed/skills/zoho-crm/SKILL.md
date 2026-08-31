---
name: zoho-crm
display_name: "Zoho CRM"
description: "Trigger: writing, reviewing, debugging, or planning Zoho CRM automation, CRM Deluge, Client Script, CRM REST V8, COQL, metadata, records, modules, or fields. Evidence-backed CRM routing and safety guidance."
scope: optional
---

# Zoho CRM

Use this skill for CRM product facts. Load `zoho-deluge` only when the requested runtime is Deluge; it supplies the language rules while this skill supplies CRM routing, API names, and safety boundaries.

## Activation Contract

- CRM Deluge loads `zoho-crm` and `zoho-deluge`, and emits `[name].deluge` at the requested CRM function placement.
- Cross-application Deluge loads every involved application skill and `zoho-deluge`, and emits `[name].deluge`.
- CRM Client Script loads `zoho-crm`, uses JavaScript, emits `[name].js`, and MUST NOT load `zoho-deluge`.
- External runtimes keep their requested language and placement; they are not Deluge by implication.

## Hard Rules

- New CRM integration code uses `zoho.crm.v8.*` or REST API V8. Existing `zoho.crm.*` modifications require the user to choose migration to V8 or preservation of legacy behavior.
- Use module and field API names. Display names and static catalog rows are recognition hints, never tenant authority.
- Prefer a named OAuth connection; CRM defaults to `conpas_crm`. Never request, expose, or embed secrets.
- Runtime metadata decides permissions, schema, layouts, relationships, required/read-only fields, picklists, operation support, quotas, and policy.
- Do not invent task names, response wrappers, endpoint templates, API names, limits, plans, or tenant capabilities.

## Decision Gates

1. Identify host, every target application, execution context, requested language, and capability. Ask only for a missing fact that changes routing or prevents safe generation.
2. Warn when standard CRM configuration may satisfy the request; continue with valid requested custom work unless the user chooses the standard path.
3. Use an allowlisted V8 task when its operation fits. For a miss, evaluate verified alternatives before reporting unsupported.
4. If one valid path remains, explain and use it. If paths are equally optimal, recommend one and wait for selection.

## Output Contract

- State the selected surface, placement, language, module/field API names, runtime facts still required, and authentication scope family.
- Keep Deluge response handling operation-specific. Do not invent a universal `data` wrapper.
- Keep unsupported or unevidenced details excluded and offer a safe clarification or verified alternative.

## Reference Routing

| Load when the work involves | Reference |
|---|---|
| Host, target, context, placement, or surface choice | [references/routing.md](references/routing.md) |
| CRM Functions and placement constraints | [references/execution-contexts.md](references/execution-contexts.md) |
| V8 Deluge task selection, responses, or legacy code | [references/deluge-tasks-v8.md](references/deluge-tasks-v8.md) |
| REST V8, COQL, metadata, bulk APIs, or structural limits | [references/rest-v8.md](references/rest-v8.md) |
| Browser-local CRM behavior | [references/client-script.md](references/client-script.md) |
| Module/field recognition | [references/zoho-crm-standard-modules.md](references/zoho-crm-standard-modules.md), [references/zoho-crm-standard-fields.md](references/zoho-crm-standard-fields.md) |
| Runtime prerequisites | [references/metadata-and-prerequisites.md](references/metadata-and-prerequisites.md) |
| OAuth connections and scopes | [references/authentication.md](references/authentication.md) |
| Declarative alternatives | [references/standard-capabilities.md](references/standard-capabilities.md) |
| Missing facts, exclusions, or safe errors | [references/uncertainty-and-errors.md](references/uncertainty-and-errors.md) |
| Provenance | [references/sources.md](references/sources.md) |
