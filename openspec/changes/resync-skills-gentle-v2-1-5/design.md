# Design: Selectively Resync Embedded Skills to gentle-ai v2.1.5

## Technical Approach

Port approved `v2.1.5` mechanics into source-of-truth `jarvis-cli/embed/**`. Tests define contracts first; generated user files and installed `.jarvis/skills/**` are verification targets only.

## Architecture Decisions

| Decision | Choice / rationale |
|---|---|
| Registry | `.jarvis/skill-registry.md` remains the canonical auto-refreshed per-machine cache, gitignored by default. Share tracked skill sources/conventions; `--no-gitignore` is explicit override. Preserve canonical write, `.atl` fallback/import warning, refresh hooks, and untracking. Runtime policy is unchanged. |
| Extraction | Replace approved generic bodies; transplant SDD/shared sections. Exclude ledger/4R/refuter, transaction, snapshot, receipt, reviewGate, remediation/generation, exit-125, semantic authority, and positive review routing. |
| Authority | Do not add `sdd-status-contract.md`; keep current Go-backed `jarvis sdd status` fields. `internal/sddruntime` remains model-row owner. |

## Disposition / File Routing

`docs/maintenance/skill-parity-run-gentle-ai-v2.1.5.md` records tag, tag object `0b4532b5a73c12b7347c1954ef37cb372056c914`, commit `1b5a5f59f74d3f6dab7de01c1603d5ce1b77af17`, disposition, and rationale for every row.

| 19 invokable rows | Action |
|---|---|
| `branch-pr`, `chained-pr`, `cognitive-doc-design` | No body change; provenance/catalog assertions only. |
| `comment-writer` | Replace `embed/skills/comment-writer/SKILL.md` with English upstream body; packaging stamp only. |
| `go-testing` | Replace `SKILL.md`; create `references/examples.md`; remove project examples. |
| `issue-creation` | Discover repo/templates/config/labels/approval/Discussions; no fixed repo/URL/policy/language. |
| `hermes-ephemeral-delegation`, `judgment-day` | Exclude absent Hermes; leave Judgment Day unchanged; defer #363/#367. |
| `sdd-explore`, `sdd-propose` | Modify executor/language/persistence; question/capability/limit mechanics. |
| `sdd-spec`, `sdd-design` | Modify complete deltas/migrations; applicable-threat/RED mechanics. |
| `sdd-tasks`, `sdd-onboard` | Modify test/runtime/rollback/line rules; artifact-language walkthrough. |
| `sdd-init`, `sdd-apply`, `sdd-verify` | Adapt v3 testing/stores/migration, bounded work-unit evidence, independent evidence; keep negative no-review/remediation boundary. |
| `sdd-archive` | Modify task/spec/current-verify reconciliation only. |
| `work-unit-commits` | Add focused result, runtime-or-N/A, rollback, authored-line rules; exclude authority. |

| 3 meta rows | Action |
|---|---|
| `skill-creator` | Replace `SKILL.md`; create `references/skill-style-guide.md`; retain optional `quality-loop.md`. |
| `skill-improver` | Modify for guide, approval, artifact language, canonical/fallback paths, refresh. |
| `skill-registry` | Modify local-cache/Hive contract; runtime unchanged. |

| 8 shared rows | Action |
|---|---|
| `_shared/SKILL.md`, `review-ledger-contract.md` | Exclude; create neither. |
| `hive-convention.md` | Adapt neutral upstream naming to Hive. |
| `openspec-convention.md` | Provenance/compatible deltas only. |
| `persistence-contract.md`, `skill-resolver.md` | Modify for Hive/four stores and exact canonical/fallback paths. |
| `sdd-phase-common.md` | Modify loading/persistence/ordering/workload/goldens only. |
| `sdd-status-contract.md` | Defer complete contract to #363; create no file. |

## Hive Grant Requirement Routing

| Spec requirement | Sources to preserve/change | Focused RED tests |
|---|---|---|
| Claude/OpenCode bounded grants; four stores | `internal/agent/sdd_phase_agents.go`, `opencode.go`, `embed/templates/{claude-sdd-agent.md.tmpl,opencode.json.tmpl}`, `internal/sddruntime/store_contract.go`. Preserve phase read/write/edit/bash permissions; add only five exact Hive tools, never wildcards/filesystem rights. `hive`/`hybrid` require grants; `openspec`/`none` require no Hive persistence. | `agent/{claude_sdd_agents,opencode}_test.go`, `sddruntime/store_contract_test.go`: all phases × four stores; exact tools, unchanged permissions, non-Hive modes. |
| Verifier drift/evidence | `agent/{runtime.go,opencode_parse.go}` observe exact grant evidence; `sddruntime/{verify.go,verify_opencode.go}` classify missing/outdated grants as FAIL for `hive`/`hybrid`, advisory for `openspec`/`none`, with `jarvis init`/reconfiguration guidance. Verification reads only. | `agent/{runtime_opencode_parse,runtime_integration}_test.go`, `sddruntime/{verify,verify_opencode}_test.go`: missing, stale, exact-deny/ask, all stores, no writes. |
| Doctor drift/guidance | `lifecycle/engine.go` creates `ReadOnly:true` plans; `agent/lifecycle_adapter.go` supplies observations. Doctor never calls `Apply`/rewrites config and names regeneration/no-clobber guidance. | `lifecycle/engine_verify_test.go`, `agent/lifecycle_adapter_test.go`: both providers, missing/outdated grants, read-only plan, guidance, unchanged temp-tree bytes. |
| Degraded execution | `_shared/{sdd-phase-common,persistence-contract}.md` and every SDD phase contract: `hive`/`hybrid` return `blocked`, name missing Hive capability/remedy, never inline artifact content or broaden permissions; `openspec`/`none` continue without Hive persistence. | `skills/catalog_contract_test.go`: four-mode table, explicit blocked envelope, forbidden inline fallback/permission broadening, non-Hive continuation. |

## Public Documentation

| File | Wording |
|---|---|
| `docs/configuration.md` | Auto-refreshed local cache, gitignored; share tracked sources/conventions, not registry. |
| `docs/security-privacy.md` | Local metadata may contain machine paths; do not commit by default; distinguish shareable sources. |
| `docs/generated-artifacts.md` | Registry is generated; embedded/project skills and conventions are sources; remove committable claim; document override. |

## Testing / Sequence / Rollback

Strict TDD units: (1) 19/3/8 provenance/authority contracts; (2) four-store grant/permission RED then rendering; (3) verifier; (4) doctor; (5) degradation; (6) docs; (7) both-agent regeneration. Keep tests/rollback together. Never mutate generated user config; use `t.TempDir()`. If `AGENTS.md.tmpl`/`CLAUDE.md.tmpl` change, require parity; otherwise assert untouched. Finish with `go test ./...`, `go vet ./...`. Full rollback reverts the approved `size:exception` PR and regenerates restored sources.

## Threat Matrix

| Boundary | Applicability / RED cases |
|---|---|
| Documentation-like paths | N/A: no executable classification. |
| Git repository selection | Applicable: relative, absolute, explicit `git -C`, nested cwd, unavailable repo, wrong-repo isolation; fail without mutation. |
| Commit/push/PR | N/A: none added. |

## Traceability / Migration

Grant table rows map respectively to spec requirements **Claude Code SDD Hive Tool Grants**, **OpenCode SDD Hive Tool Grants**, **Verification and Doctor Diagnostics**, and **Hive Mode Degraded Behavior**. Registry migration remains canonical `.jarvis` creation with warned `.atl` import/fallback and default untracking. No open questions.
