# Approved Proposal Appendix: Dispositions and Issue #362 Rewrite

## Baseline and Corrected Facts

- **Upstream:** annotated tag `v2.1.5`; tag object `0b4532b5a73c12b7347c1954ef37cb372056c914`; peeled commit `1b5a5f59f74d3f6dab7de01c1603d5ce1b77af17`.
- **Inventory:** 19 invokable skills, 3 separately tracked meta-tooling packages, and 8 `_shared` support files. Jarvis has 18 invokable equivalents; only Hermes delegation is absent and excluded.
- **Version breadth:** v3 applies to `sdd-init`, `sdd-apply`, and `sdd-verify`; other SDD assets remain v2 except `sdd-onboard` v1. The proposal does not describe all SDD skills as v3.
- **Review routing:** phase executors receive safe negative boundaries; any future positive review invocation/routing belongs to the parent/orchestrator and #363, not the executor.
- **Verification preimage:** upstream verification-evidence preimage/GateRequest behavior is authority-bearing and deferred to #420/#366; it is not a neutral verification requirement for this sync.
- **Semantic validity:** #422 is a Jarvis follow-up and is not represented as behavior already present in upstream `sdd-status-contract.md`.
- **Approval:** the user approved this proposal after two clarification rounds.

## Invokable Skill Matrix (19)

Every synchronized row targets tag `v2.1.5`, peeled commit `1b5a5f59f74d3f6dab7de01c1603d5ce1b77af17`.

| Skill | Current Jarvis state | v2.1.5 decision | Required adaptation or exclusion | Destination |
|---|---|---|---|---|
| `branch-pr` | v1.26.5 body-equivalent | No change: conceptually equivalent | Catalog metadata and pinned provenance only | None |
| `chained-pr` | v1.26.5 body-equivalent | No change: conceptually equivalent | Keep review-budget integration; no authority clauses | #366 for authority |
| `cognitive-doc-design` | v1.26.5 body-equivalent | No change: conceptually equivalent | Catalog metadata and pinned provenance only | None |
| `comment-writer` | v1.26.5 plus Jarvis regional behavior | Adopt upstream runtime body | No behavioral adaptation; retain catalog metadata/provenance only. Upstream examples are English. Repository bilingual GitHub policy remains solely in `AGENTS.md`/`CLAUDE.md` | None |
| `go-testing` | Legacy Gentleman.Dots tutorial | Adopt upstream runtime body plus `references/examples.md` | Catalog metadata/provenance only; no Jarvis, Gentleman.Dots, or project-specific paths/examples | None |
| `hermes-ephemeral-delegation` | Absent | Discard / out of scope | Hermes-specific `delegate_task` runtime is unavailable | #363 only if a future cross-platform equivalent is designed |
| `issue-creation` | v1.26.5, partly repository-neutral | Adopt with repository-neutral adaptation | Discover current repository, templates, labels, blank-issue policy, approval workflow, and Discussions capability; never hard-code repository/URL. Language policy remains external | None |
| `judgment-day` | v1.26.5/v1.4 | Discard / out of scope | Do not import transaction, ledger, no-refuter, lineage, or positive routing behavior | #367; dependencies #365/#366 |
| `sdd-explore` | v1.26.5 Hive-adapted | Adopt neutral mechanics | Executor/language/persistence/envelope behavior; no review authority | #363 final routing |
| `sdd-propose` | v1.26.5 Hive-adapted | Adopt neutral mechanics | Question rounds, capabilities, persistence, limits | #363 final routing |
| `sdd-spec` | v1.26.5 + v1.40.2 delta semantics | Adopt neutral mechanics | Capability domains, complete deltas, migrations; Hive/four stores | None |
| `sdd-design` | v1.26.5 Hive-adapted | Adopt neutral mechanics | Threat applicability and planned RED cases only | None |
| `sdd-tasks` | v1.26.5 Hive-adapted | Adopt neutral mechanics | Focused tests, runtime harness, rollback, authored-line/golden treatment; no snapshot/receipt fields | #366/#421 deferred authority |
| `sdd-init` | v1.26.5 Jarvis variant | Adopt compatible v3 mechanics | Testing capabilities, Hive/four stores, `.jarvis` canonical and `.atl` legacy read | #363 final routing/status |
| `sdd-onboard` | v1.26.5 | Adopt neutral mechanics | Artifact-language/walkthrough improvements only | #363 final routing |
| `sdd-apply` | v1.40.2 Hive-adapted | Adopt compatible v3 mechanics | Workspace/task/work-unit evidence and negative “executor never launches review/remediation” boundary; no positive review/remediation commands or required authority fields | #365/#366/#363/#421 |
| `sdd-verify` | v1.40.2 Hive-adapted | Adopt compatible v3 mechanics | Independent spec/runtime evidence only; no preterminal transaction, evidence-preimage, exit-125 authority denial, generation, or receipt requirements | #420; #366 inputs |
| `sdd-archive` | v1.40.2 Hive-adapted | Adopt neutral mechanics | Task/spec reconciliation and current verify gate only; no receipt/reviewGate authority | #420/#366 |
| `work-unit-commits` | v1.26.5 | Adopt neutral evidence mechanics | Include focused tests, runtime evidence, rollback, and authored-line treatment of goldens; defer snapshot identity, receipt validation, and correction transaction clauses | #366/#421 |

## Meta-Tooling Matrix

| Package | Decision | Exact adaptation |
|---|---|---|
| `skill-creator` | Adopt upstream body and bundled style guide essentially verbatim | Packaging metadata/provenance only; no `.jarvis` product adaptation. Existing quality-loop may remain optional only if it does not alter upstream contract |
| `skill-improver` | Adopt upstream structure with minimal adaptation | `.jarvis` canonical registry, `.atl` read fallback, Jarvis refresh command, bundled style guide, explicit approval, artifact-language safety |
| `skill-registry` | Adopt with Jarvis/Hive adaptation | Preserve Hive, `.jarvis` canonical write, `.atl` legacy read, installer contracts; design MUST resolve gitignored cache versus shareable/versioned registry contradiction before implementation |

## Shared Contract/Support Matrix (8)

| Support file | Decision | Included now / deferred ownership |
|---|---|---|
| `_shared/SKILL.md` | Discard / no adoption required | Support-only metadata; Jarvis already excludes `_shared` from registration |
| `engram-convention.md` | Adopt with Hive adaptation | Neutral artifact naming and automation-save rules only |
| `openspec-convention.md` | No change: conceptually equivalent | Provenance and compatible neutral deltas only |
| `persistence-contract.md` | Adopt with Hive adaptation | Hive/four-store behavior, response ordering, bookkeeping-not-reply; no authority state |
| `review-ledger-contract.md` | Discard / out of scope | #365 owns ledger/4R/refuter; #366 owns transaction/snapshot/receipt/facade |
| `sdd-phase-common.md` | Adopt selectively | Persistence, skill loading, return order, workload/golden treatment; no receipt/snapshot authority |
| `sdd-status-contract.md` | Selective status-core only if required | Do not import the complete authority-bearing file. A minimal current-state Jarvis status-core may be introduced only if updated phases require it. After prerequisites, #363 MUST finalize, Hive-adapt, install, and drift-test the complete contract aligned to `jarvis.sdd-status`. #422 semantic validity is not claimed as upstream status behavior |
| `skill-resolver.md` | Adopt with Hive/Jarvis adaptation | Hive retrieval, canonical `.jarvis`, `.atl` read fallback, exact path injection |

## Exact Authority Deferral Map

| Deferred fragment | Owner |
|---|---|
| Ledger, 4R, refuter | #365 |
| Transaction, immutable snapshot, receipt, facade | #366 |
| Judgment Day modernization | #367 |
| Final phase routing/integration and complete `jarvis.sdd-status` ownership | #363 |
| Structured verify/archive authority and authority-only verification denial | #420 |
| Remediation lineage/generations | #421 |
| Semantic validity/content-derived state | #422 |

### Explicit Follow-up Gaps

1. **#363 amendment:** own the final complete Jarvis status contract after prerequisites land: adapt upstream status semantics to Hive/OpenSpec/hybrid, align schema identity to `jarvis.sdd-status`, install it across supported agents, wire parent routing, and add source-to-installed drift tests.
2. **#420 amendment:** own or explicitly reject the authority-only verification denial protocol, including upstream exit code `125` and its exact failure envelope. This proposal MUST NOT silently import that protocol.

The deferred upstream authority-only envelope that #420 must decide is:

```yaml
authority_only_failure: true
missing_review_authority: true
substantive_failure: false
command_failed: false
observed_authority_revision: sha256:{observed-authority-revision}
test_exit_code: 125
build_exit_code: 125
```

Both declared commands remain unexecuted in that upstream protocol and hash exact empty output. This is reference material for #420, not a contract adopted by #362.

## Intentional Jarvis Invariants

- Runtime memory is Hive; stores remain `hive | openspec | hybrid | none`.
- `.jarvis/skill-registry.md` is canonical; `.atl/` is legacy read fallback.
- Per-phase model rows remain Go-template-owned.
- Persona voice never enters technical artifacts.
- Generated user-machine files are outputs, not edited sources.
- `AGENTS.md.tmpl` and `CLAUDE.md.tmpl` remain equivalent if repository instructions change.

## Proposed Bilingual Rewrite Plan for Issue #362

### English

**Title:** `chore(skills): selectively resync Jarvis SDD and peripheral skills to gentle-ai v2.1.5`

**Scope:** Compare Jarvis embedded sources with annotated tag `v2.1.5` at peeled commit `1b5a5f59f74d3f6dab7de01c1603d5ce1b77af17`. Synchronize approved generic and SDD mechanics into existing Jarvis assets; keep generic bodies upstream-generic unless an explicit capability adaptation is required; preserve Hive/Jarvis invariants; and exclude unsupported authority protocols. No new invokable skill is added.

**Acceptance criteria:**
- [ ] Every skill/meta-tool/support row records disposition, pinned provenance, and exact adaptation/defer rationale.
- [ ] `comment-writer`, `go-testing`, and `skill-creator` retain upstream runtime behavior with packaging metadata only; `issue-creation` is repository-neutral and capability-driven.
- [ ] SDD/shared assets contain neutral mechanics and safe negative executor boundaries, but no positive transaction/ledger/snapshot/receipt/reviewGate/remediation/generation/archive-authority contract.
- [ ] Hive, four stores, `.jarvis`/`.atl`, Go-owned rows, persona separation, and template parity remain intact.
- [ ] Source assets and full CLI installation/regeneration are verified; relevant focused checks, `go test ./...`, and `go vet ./...` pass during implementation verification.
- [ ] The single PR records the approved `size:exception` when authored changes exceed 600 lines.

**Exclusions/destinations:** #365 ledger/4R/refuter; #366 transaction/snapshot/receipt/facade; #367 Judgment Day; #363 final routing and complete `jarvis.sdd-status`; #420 structured verify/archive authority plus the exit-125 authority-only denial decision; #421 remediation lineage; #422 semantic validity; CodeGraph lifecycle; Engram adoption; generated user-machine edits; Hermes delegation.

**Proposed issue amendments:** amend #363 with final status-contract adaptation/install/drift-test ownership; amend #420 with explicit ownership or rejection of exit `125` plus the exact authority-only verification failure envelope.

### Español (España)

**Título:** `chore(skills): resincronizar selectivamente las skills SDD y periféricas de Jarvis con gentle-ai v2.1.5`

**Alcance:** Comparar las fuentes embebidas de Jarvis con la etiqueta anotada `v2.1.5` en el commit resuelto `1b5a5f59f74d3f6dab7de01c1603d5ce1b77af17`. Sincronizar las mecánicas genéricas y SDD aprobadas en los assets existentes de Jarvis; mantener genéricos los cuerpos upstream salvo adaptación explícita de capacidad; preservar los invariantes Hive/Jarvis; y excluir protocolos de autoridad aún no implementados. No se añade ninguna skill invocable nueva.

**Criterios de aceptación:**
- [ ] Cada fila de skill, metaherramienta o soporte registra disposición, procedencia fijada y motivo exacto de adaptación o aplazamiento.
- [ ] `comment-writer`, `go-testing` y `skill-creator` conservan el comportamiento runtime upstream con metadatos de empaquetado únicamente; `issue-creation` es neutral respecto al repositorio y se basa en capacidades detectadas.
- [ ] Los assets SDD/compartidos contienen mecánicas neutrales y límites negativos seguros para ejecutores, pero ningún contrato positivo de transacción, ledger, snapshot, receipt, reviewGate, remediación, generación o autoridad de archivo.
- [ ] Se conservan Hive, los cuatro almacenes, `.jarvis`/`.atl`, las filas propiedad de Go, la separación de persona y la paridad de plantillas.
- [ ] Se verifican los assets fuente y la instalación/regeneración completa mediante CLI; durante la verificación de implementación pasan las comprobaciones focalizadas, `go test ./...` y `go vet ./...`.
- [ ] El PR único registra la excepción de tamaño aprobada cuando los cambios de autoría superen 600 líneas.

**Exclusiones/destinos:** #365 ledger/4R/refuter; #366 transacción/snapshot/receipt/fachada; #367 Judgment Day; #363 routing final y contrato completo `jarvis.sdd-status`; #420 autoridad estructurada de verificación/archivo y decisión sobre denegación de autoridad con salida 125; #421 linaje de remediación; #422 validez semántica; ciclo de vida de CodeGraph; adopción de Engram; archivos generados de usuario; delegación Hermes.

**Enmiendas propuestas:** ampliar #363 con la propiedad final de adaptación, instalación y pruebas de deriva del contrato de estado; ampliar #420 con la propiedad o rechazo explícito de la salida `125` y del envelope exacto de fallo de verificación por ausencia de autoridad.
