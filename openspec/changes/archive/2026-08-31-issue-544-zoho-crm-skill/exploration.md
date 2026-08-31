# Exploration: `issue-544-zoho-crm-skill`

## Executive finding

The current repository and GitHub state support moving to proposal work. Issue #544 is open, labelled `status:approved` and `type:feature`, and explicitly records that implementation has not started and must wait for maintainer instruction. Issue #545 is now closed, its three evidence documents are tracked on `master`, and PR #610 merged them; the remaining evidence uncertainties are conservative exclusions rather than product blockers.

No production code or generated user-machine configuration was changed during this exploration.

## Verified GitHub state

Repository identity was verified read-only as `Thrasno/jarvis-ai-devs` (`master`). The working tree was clean at exploration start.

### Issue #544

- Title: `feat(skills): implement zoho-crm against the shared application contract`
- State: `OPEN`
- Labels: `status:approved`, `type:feature`
- Assignees: none
- Milestone: none
- Formal GitHub issue-dependency summary: zero blocked-by and zero blocking relationships
- Textual dependencies in the current body: #542, completed #543 where Deluge applies, and approved #545
- Latest state comment: scope and contract are fully defined; #545 is approved; implementation remains explicitly deferred until requested

The following is the exact current bilingual body fetched from GitHub:

```markdown
## Outcome

Implement `zoho-crm` against the canonical application contract in #542 and the approved evidence contract in #545. The evidence gate is resolved, but implementation has not started and must wait for explicit maintainer instruction.

## Implementation state

- Evidence gate: approved.
- Implementation: not started.
- Current action: wait for explicit maintainer instruction.
- Status: keep `status:needs-review`.

## Scope

- Natural-language activation and host/target routing for CRM as host, target, or both.
- CRM Functions and Client Script contexts without treating Deluge and JavaScript as the same language or placement.
- Deterministic routing across the closed V8 Deluge task catalog, Client Script/ZDK, REST V8, COQL, metadata and bulk APIs, standard CRM configuration, and unsupported operations.
- Separation of execution location, capability, transport, authentication, and operation prerequisites.
- Standard CRM alternatives that warn without blocking valid custom work.
- Missing-context questions limited to facts that materially change routing.

An allowlist miss means only that no current V8 task exists. The router must still evaluate standard CRM configuration, Client Script/ZDK, REST V8, COQL, another verified surface, or a documented manual workaround.

## V8 and legacy policy

- All new CRM code uses `zoho.crm.v8.*` or REST API V8.
- Do not ask which API version to use for new code. V8 is fixed.
- Legacy documentation does not prove V8 behaviour.
- Existing `zoho.crm.*` modifications ask whether to migrate to V8 or preserve the requested legacy behaviour.
- Module and field API names are always used.

The closed task allowlist contains exactly: `createRecord`, `getRecords`, `searchRecords`, `getRecordById`, `updateRecord`, `bulkCreate`, `bulkUpdate`, `getRelatedRecords`, `updateRelatedRecord`, `convertLead`, `upsert`, `attachFile`, and `getFields`.

## Catalog and runtime metadata

Use the static catalog for standard module and field display names and API names only. It contains 21 modules and 609 fields across 21 sections and was filtered through official evidence from authenticated CRM V8 metadata captured on 2026-08-29.

Runtime metadata is authoritative for the target organization. The static catalog must not infer write safety, permissions, layouts, relationships, mandatory/read-only behaviour, picklist values, operation support, quotas, or runtime policy.

## Routing and output

- CRM Client Script loads `zoho-crm`, uses JavaScript, and outputs `[name].js`; it never loads `zoho-deluge`.
- CRM Deluge loads `zoho-crm` and `zoho-deluge` and outputs `[name].deluge`.
- Cross-application Deluge loads all involved application skills plus `zoho-deluge`.
- External runtimes use the actual implementation language and relevant application skills.

If one valid path remains, use and explain it. If several equally optimal paths remain, present them, recommend one, and wait for human selection.

## Context, plan, and limits

Ask only for missing facts that change routing or prevent safe generation. Check plan or edition only when it determines whether a capability exists. Do not use it to calculate quotas, credits, concurrency thresholds, or timeouts.

Retain actionable structural limits that shape code. Treat concurrency as an advisory risk and avoid unnecessary parallelism without claiming an exact tenant threshold. Exclude daily allowances, Function/API credit formulas, capacity estimates, exact concurrency thresholds, and numeric execution timeouts.

## Authentication

Never request or embed secrets. Prefer named OAuth connections using `conpas_[target-app]`; the CRM default is `conpas_crm`. Document operation scopes and secure deployment configuration.

## Evidence gate

#545 supplies the approved contexts, closed task catalog, signatures, response boundaries, REST V8 sources, authentication, structural limits, static recognition baseline, runtime-metadata authority, and deterministic exclusions. There are no remaining `TBD` blockers. Approval of #545 does not authorize implementation automatically.

## Acceptance criteria

- [ ] `zoho-crm` and focused references follow #542 and the bundled skill style.
- [ ] Activation and composition cover CRM, cross-application, Client Script, Deluge, and external runtimes.
- [ ] Client Script is isolated from Deluge and emits `.js`; CRM Deluge emits `.deluge`.
- [ ] Routing accepts exactly the 13 approved V8 tasks and routes every miss through verified alternatives.
- [ ] New code is V8-only and legacy modifications use the migrate-or-preserve question.
- [ ] Module and field API names are used consistently.
- [ ] The recognition catalog is integrated without replacing runtime metadata.
- [ ] Responses remain operation-specific without a universal wrapper.
- [ ] Standard product capabilities are checked before code without blocking requested custom work.
- [ ] Plan checks are limited to capability availability.
- [ ] Numeric quotas, credits, exact concurrency thresholds, and numeric timeouts are excluded.
- [ ] Structural limits and advisory concurrency are implemented.
- [ ] Authentication safety, `conpas_crm`, placement, assumptions, and expected outcomes are tested.
- [ ] Contract tests are automated; executable Deluge TDD remains conditional on a verifiable runtime.

## Dependencies

Depends on #542, completed #543 where Deluge applies, and approved #545.

---

## Resultado

Implementar `zoho-crm` conforme al contrato canónico de #542 y al contrato de evidencias aprobado en #545. La puerta de evidencias está resuelta, pero la implementación no ha comenzado y debe esperar una instrucción explícita de un mantenedor.

## Estado de la implementación

- Puerta de evidencias: aprobada.
- Implementación: no iniciada.
- Acción actual: esperar una instrucción explícita.
- Estado: mantener `status:needs-review`.

## Alcance

- Activación mediante lenguaje natural y encaminamiento anfitrión/destino.
- Contextos de CRM Functions y Client Script sin tratar Deluge y JavaScript como el mismo lenguaje o ubicación.
- Encaminamiento determinista entre tareas Deluge V8, Client Script/ZDK, REST V8, COQL, API de metadatos y operaciones masivas, configuración estándar de CRM y operaciones no compatibles.
- Separación entre ubicación, capacidad, transporte, autenticación y requisitos previos.
- Alternativas estándar que advierten sin bloquear trabajo personalizado válido.
- Preguntas limitadas a datos que cambian materialmente el encaminamiento.

La ausencia en la lista permitida solo significa que no existe una tarea V8 actual. Deben seguir evaluándose configuración estándar, Client Script/ZDK, REST V8, COQL, otra superficie verificada o una solución manual documentada.

## Política V8 y código heredado

- Todo el código nuevo utiliza `zoho.crm.v8.*` o REST API V8.
- No se pregunta la versión para código nuevo; V8 está fijada.
- La documentación heredada no demuestra comportamientos V8.
- Al modificar `zoho.crm.*`, se pregunta si debe migrarse o conservarse.
- Siempre se utilizan nombres de API de módulos y campos.

La lista cerrada contiene: `createRecord`, `getRecords`, `searchRecords`, `getRecordById`, `updateRecord`, `bulkCreate`, `bulkUpdate`, `getRelatedRecords`, `updateRelatedRecord`, `convertLead`, `upsert`, `attachFile` y `getFields`.

## Catálogo y metadatos en tiempo de ejecución

El catálogo estático se utiliza únicamente para reconocer nombres visibles y nombres de API. Contiene 21 módulos y 609 campos en 21 secciones y fue filtrado mediante evidencias oficiales a partir de metadatos autenticados de CRM V8 capturados el 29 de agosto de 2026.

Los metadatos en tiempo de ejecución son la autoridad. El catálogo no debe inferir seguridad de escritura, permisos, diseños, relaciones, obligatoriedad o solo lectura, listas de selección, compatibilidad, cuotas ni políticas de ejecución.

## Encaminamiento y salida

- Client Script carga `zoho-crm`, usa JavaScript y genera `[name].js`; nunca carga `zoho-deluge`.
- CRM Deluge carga `zoho-crm` y `zoho-deluge` y genera `[name].deluge`.
- Deluge entre aplicaciones carga todas las skills implicadas más `zoho-deluge`.
- Los entornos externos utilizan su lenguaje real y las skills correspondientes.

Si queda una vía válida, se utiliza y explica. Si quedan varias vías óptimas equivalentes, se presentan, se recomienda una y se espera la elección humana.

## Contexto, plan y límites

Solo se pregunta por datos ausentes que cambien el encaminamiento o impidan una generación segura. El plan o edición solo se comprueba cuando determina la existencia de una capacidad, nunca para calcular cuotas, créditos, concurrencia o tiempos.

Se conservan los límites estructurales que determinan la forma del código. La concurrencia se trata como riesgo consultivo sin afirmar un umbral exacto. Se excluyen asignaciones diarias, fórmulas de créditos, estimaciones de capacidad, umbrales exactos y tiempos de espera numéricos.

## Autenticación

Nunca se solicitan ni incorporan secretos. Se prefieren conexiones OAuth con nombre mediante `conpas_[target-app]`; la predeterminada de CRM es `conpas_crm`. Se documentan scopes y configuración segura.

## Puerta de evidencias

#545 aporta los contextos, tareas, firmas, respuestas, fuentes REST V8, autenticación, límites estructurales, referencia base, autoridad de metadata y exclusiones aprobadas. No quedan bloqueos `TBD`. La aprobación de #545 no autoriza automáticamente la implementación.

## Criterios de aceptación

- [ ] `zoho-crm` y sus referencias siguen #542 y la guía de skills.
- [ ] La activación cubre CRM, trabajo entre aplicaciones, Client Script, Deluge y entornos externos.
- [ ] Client Script está aislado de Deluge y genera `.js`; CRM Deluge genera `.deluge`.
- [ ] El encaminamiento acepta exactamente las 13 tareas V8 y dirige cada ausencia hacia alternativas verificadas.
- [ ] El código nuevo utiliza V8 y el heredado pregunta si migrar o conservar.
- [ ] Los nombres de API se utilizan de forma coherente.
- [ ] El catálogo se integra sin sustituir los metadatos en tiempo de ejecución.
- [ ] Las respuestas se tratan por operación sin contenedor universal.
- [ ] Las capacidades estándar se comprueban antes del código sin bloquear trabajo personalizado.
- [ ] Los planes solo se comprueban para disponibilidad de capacidades.
- [ ] Se excluyen cuotas, créditos, umbrales exactos y tiempos numéricos.
- [ ] Se implementan límites estructurales y avisos de concurrencia.
- [ ] Se prueban seguridad de autenticación, `conpas_crm`, ubicación, supuestos y resultados.
- [ ] Las pruebas de contrato se automatizan; el TDD ejecutable de Deluge depende de un entorno verificable.

## Dependencias

Depende de #542, de #543 cuando se utilice Deluge y de #545 aprobada.
```

The body contains one stale historical status line (`Status: keep status:needs-review`) that conflicts with the current `status:approved` label and latest approval comment. It is preserved above as verbatim evidence; implementation planning should use the current label/comment state.

### Dependency verification

- #542 is open with `status:needs-review` and remains the canonical shared contract. Its current planning addendum says #544 must join the existing Zoho selection, not merely be added to the catalog.
- #543 is closed with `status:approved`; its delivery comment records merge commit `39e5f892` and confirms the application-neutral Deluge core.
- #545 is closed with `status:approved`; its three documentation files were delivered by PR #610, merged on 2026-08-30 at merge commit `f44f6a79`.
- GitHub's REST issue summary reports no formal dependency edges for #544. The dependencies above are explicit project-contract references in issue text, not GitHub blocked-by links.

## Verified #545 evidence and repository documents

The current dossier at `docs/zoho/zoho-crm-evidence-dossier.md` records 53 successful official claim sources, the approved routing contract, the closed 13-task V8 catalog, REST V8 source groups, runtime metadata authority, authentication policy, structural limits, deterministic exclusions, and the reference architecture. The two catalogs are tracked at:

- `docs/zoho/zoho-crm-standard-modules.md` — 21 standard module rows with canonical display names, API names, and observed aliases.
- `docs/zoho/zoho-crm-standard-fields.md` — 21 matching sections and 609 retained `custom_field: false` field rows with display names, API names, and snapshot data types.

The catalog is recognition-only. It must not infer permissions, write safety, layouts, relationships, mandatory/read-only behavior, picklist values, operation support, quotas, or runtime policy. Authenticated runtime Modules, Fields, Layouts, and Related Lists metadata remains authoritative for the target organization.

The approved 13-task catalog is exactly:

`createRecord`, `getRecords`, `searchRecords`, `getRecordById`, `updateRecord`, `bulkCreate`, `bulkUpdate`, `getRelatedRecords`, `updateRelatedRecord`, `convertLead`, `upsert`, `attachFile`, `getFields`.

Important response boundaries are operation-specific: `getRecords` uses the documented `toJsonList()` handling path, `searchRecords` uses direct iteration, and `getRelatedRecords`, `bulkCreate`, and `bulkUpdate` remain opaque unless new evidence or target-runtime evidence proves a safe container. No universal `data` wrapper may be invented for Deluge tasks.

The evidence dossier deliberately excludes daily allowances, Function/API credit formulas and maxima, exact tenant concurrency thresholds, and numeric execution timeouts. It retains actionable structural limits and qualitative concurrency warnings. Schedule arguments, implicit server-side custom-button arguments, validation-function signatures/returns, Quick Create runtime behavior, and the Function API endpoint template have deterministic exclusions; these are not unresolved product decisions.

One documentation inconsistency is non-blocking: the dossier status/readiness text still says tracked delivery remains under #545, while current GitHub and repository history prove delivery through merged PR #610. The implementation must not treat that stale sentence as a dependency blocker.

## Current source-of-truth architecture

The embedded skill tree is the product source of truth:

- `jarvis-cli/embed/skills/zoho-deluge/SKILL.md` is the application-neutral language core.
- `jarvis-cli/embed/skills/zoho-deluge/references/` carries its focused references and official provenance.
- `jarvis-cli/assets.go` embeds `embed/skills` as `jarvis.SkillsFS`; a new `embed/skills/zoho-crm/` tree is automatically included by the existing `go:embed all:embed/skills` declaration.
- `jarvis-cli/internal/skills/registry.go` discovers only `SKILL.md` files, derives the ID from the directory, and sources display metadata from frontmatter. Supporting references are packaged but not registered as independent skills.
- `jarvis-cli/internal/skills/installer.go` recursively installs the selected skill tree and `_shared/`, writes atomically, skips byte-equivalent files, and refuses destination symlink traversal.
- `jarvis-cli/internal/agent/install.go` reuses the same walker for Claude/OpenCode desired-state rendering and installation, including model-section rendering when applicable.
- `jarvis-cli/internal/agent/claude.go` and `opencode.go` install selected embedded skills into generated user-agent directories; these generated directories are outputs, never sources of truth.
- `jarvis-cli/internal/projectregistry/refresh.go` scans installed skill copies and writes the project-local `.jarvis/skill-registry.md`; it does not install skills.
- `jarvis-cli/internal/skills/diskscan/frontmatter.go` requires a name and trigger for registry discovery. Repository convention uses `Trigger:` inside the frontmatter description rather than a standalone `trigger:` key.
- `jarvis-cli/internal/tui/skills_selection.go` presents only interactive stack-specific prompts. `internal/skills/interactive.go` is the single source of truth for IDs requiring explicit selection; every other non-core skill is auto-installed by the plan.
- `jarvis-cli/internal/project/detector.go` currently recognizes Zoho projects and returns `hive` plus `zoho-deluge`, but this helper has no production caller beyond its tests in the current graph. Selection behavior is controlled by the catalog's interactive-ID set and the wizard plan.

Do not edit `~/.claude/`, `~/.config/opencode/`, generated `.jarvis/skills/` copies, or generated instruction/configuration files. If selection behavior must change, update the embedded source and the selection planner/tests, then let installation regenerate outputs.

## Existing tests and constraints

Relevant existing conventions are:

- `jarvis-cli/internal/skills/catalog_contract_test.go` validates embedded skill frontmatter, application neutrality, official reference packaging, local Markdown link integrity, and the Zoho Deluge regression contract. A CRM skill should add similarly focused contract coverage rather than scrape live Zoho pages.
- `jarvis-cli/internal/skills/registry_test.go` verifies display-name metadata, registry discoverability, unique IDs, and omission of supporting files as registry rows.
- `jarvis-cli/internal/skills/installer_test.go` verifies recursive references, selected/core behavior, idempotency, model-section rendering, and symlink-safe installation.
- `jarvis-cli/internal/skills/diskscan/frontmatter_test.go` verifies both standalone and description-embedded trigger parsing, including sentence boundaries and real skill formats.
- `jarvis-cli/internal/skills/interactive_test.go`, `jarvis-cli/internal/tui/model_test.go`, and `jarvis-cli/internal/tui/nontui_test.go` constrain the interactive-ID set, prompt grouping, selection defaults, and no-TUI behavior.
- Existing skill assets use YAML frontmatter with `name`, `display_name`, description-embedded `Trigger:`, and `scope: optional`; all embedded skill display names must be non-empty and distinct from kebab IDs.
- The repository's Go policy requires focused contract tests and `gofmt`; the repository instructions say not to build unless requested. This exploration therefore did not run tests or builds.
- The repository currently has no root `openspec/config.yaml`, although the embedded OpenSpec convention documents one. There is also a filename mismatch: the explicit user target is `exploration.md`, while current embedded runtime conventions/tests refer to `explore.md`; this exploration honors the explicit target, and proposal/status work should resolve whether the target is intentionally legacy-compatible or needs a path correction.

## Approaches

1. **Embedded CRM skill pack with focused contract tests** — add `zoho-crm/SKILL.md` and the approved reference files under `jarvis-cli/embed/skills`, integrate CRM with the existing Zoho selection semantics, and extend embedded-skill tests for the exact allowlist, routing, isolation, source integrity, catalog boundaries, and security rules.
   - Pros: follows the current source-of-truth architecture; installer and both agents gain recursive packaging without new installer abstractions; keeps the application/language boundary explicit; bounded for one reviewable PR.
   - Cons: requires careful contract tests for many routing branches; application references are sizeable and must remain concise and progressively loaded.
   - Effort: Medium

2. **Introduce a generic application-skill/router framework first** — create shared runtime models or a new Go routing abstraction before adding CRM content.
   - Pros: could centralize future application metadata if several applications already had concrete implementations.
   - Cons: no existing application-skill implementation justifies the abstraction; expands the first CRM change into architecture work; risks encoding unstable Zoho facts in Go instead of the embedded skill contract.
   - Effort: High

## Recommendation and bounded scope

Use approach 1. The implementation should be limited to the embedded `zoho-crm` skill, its approved focused references, contract tests, and the minimal existing selection integration needed to make the CRM application skill participate in the Zoho pack rather than becoming an unreferenced catalog row. Keep routing, evidence, and product facts in Markdown assets; keep Go changes limited to selection/catalog contracts if tests prove they are necessary.

Concrete non-goals:

- No Deluge interpreter, remote execution runner, live Zoho API tests, credentials, or tenant-specific values.
- No changes to `zoho-deluge` except a separately approved dependency correction.
- No transport-specific skill, universal CRM response wrapper, guessed endpoint/ID/scope/limit, or static catalog authority over runtime metadata.
- No generated user-machine configuration edits.
- No new installer framework, agent adapter, or broad router abstraction.
- No quota/credit formulas, exact concurrency thresholds, or numeric execution-time guarantees.
- No resolution of intentionally excluded schedule/button/validation/Quick Create/Function API facts without new approved evidence.
- No expansion to Zoho Books, People, Projects, Creator, or Analytics implementations.

The review preflight requests a single PR with a 5,000-line budget, but `size:exception` is not approved. The task phase must forecast the authored diff and keep the implementation within the cached budget or stop for the explicit later gate; exploration does not authorize that exception.

## Risks

- The current dossier contains stale delivery wording even though #545 and PR #610 prove delivery; downstream phases must use current GitHub/repository evidence.
- The exact multi-record Deluge response shape is intentionally unresolved for three tasks, so generated guidance must remain opaque there.
- The static 21/609 recognition baseline can be mistaken for tenant schema; runtime metadata checks must remain explicit.
- Adding `zoho-crm` only to embedded assets would allow discovery but could fail the required Zoho application/language composition semantics; selection tests must cover the intended behavior.
- The missing root `openspec/config.yaml` and the `exploration.md` versus `explore.md` convention mismatch may affect downstream phase/status discovery; clarify without creating unrelated artifacts.
- The issue body contains a stale `status:needs-review` instruction that must not override the current approved label and latest comment.

## Business/product questions

No genuinely blocking business or product questions remain before proposal. The issue and approved evidence contract fix V8 for new code, define the 13-task catalog, define host/target/context routing, define output extensions and skill composition, define `conpas_crm`, define runtime-metadata authority, and define the standard-feature warning behavior. Remaining uncertainty is technical evidence handling and is already assigned deterministic exclusions or user questions at generation time.

## Ready for Proposal

Yes. Proceed to `sdd-propose` using this bounded scope. Proposal work should explicitly preserve the current approved GitHub state, treat #545 as delivered, include the concrete non-goals above, and call out the OpenSpec configuration discrepancy without implementing production code.
