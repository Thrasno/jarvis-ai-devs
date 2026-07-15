## Exploration: resync Jarvis embedded SDD and peripheral skills to gentle-ai v2.1.5

### Current State

Issue #362 is open as `chore(skills): resync SDD & peripheral skills against gentle-ai upstream (→ v1.49.0)`. Its operational intent is still valid, but the target, drift counts, provenance examples, and some exclusions are stale after retargeting to v2.1.5. The issue also predates upstream's native review authorities and newer SDD verification/validity work.

The Jarvis source of truth is `jarvis-cli/embed/`, not installed user files. SDD assets are Hive-adapted and currently preserve four artifact stores (`hive`, `openspec`, `hybrid`, `none`), generated Go-owned model rows, Jarvis runtime status paths, `.jarvis/skill-registry.md` canonical writes with `.atl/` legacy reads, and persona/artifact separation. `AGENTS.md.tmpl` and `CLAUDE.md.tmpl` are parallel generated sources and must remain behaviorally equivalent when touched.

The comparison target is the immutable upstream tag `v2.1.5`, commit `1b5a5f59f74d3f6dab7de01c1603d5ce1b77af17` (the tag object resolves to `0b4532b5a73c12b7347c1954ef37cb372056c914`). Upstream SDD assets are primarily under `internal/assets/skills/`; its shared contract still uses `engram | openspec | hybrid | none`, adds review-ledger/native-authority contracts, and contains v3 SDD skills with materially expanded verification and archive semantics.

### Affected Areas

- `jarvis-cli/embed/skills/sdd-{explore,propose,spec,design,tasks,init,onboard,apply,verify,archive}/` — primary selective resync; provenance and upstream mechanical changes require Hive/Jarvis adaptation.
- `jarvis-cli/embed/skills/{comment-writer,branch-pr,chained-pr,work-unit-commits,issue-creation,cognitive-doc-design}/` — peripheral parity candidates; review/PR language must retain repository policy and avoid importing native review authority.
- `jarvis-cli/embed/skills/_shared/{persistence-contract,hive-convention,sdd-phase-common,skill-resolver}.md` — contract boundary; upstream's Engram terminology and newer review contracts cannot be copied literally.
- `jarvis-cli/embed/skills/skill-registry/` and `jarvis-cli/internal/project/registry.go` — upstream registry changes must preserve canonical `.jarvis/skill-registry.md` and read-only `.atl/` fallback.
- `jarvis-cli/embed/skills/hive/SKILL.md`, `jarvis-cli/embed/hive-protocol.md` — location for Hive-adapted “memory is bookkeeping” semantics; no `engram` leakage.
- `jarvis-cli/embed/templates/{AGENTS.md.tmpl,CLAUDE.md.tmpl}`, persona assets, `jarvis-cli/internal/sddruntime/` — verify source/generated and model-row/persona boundaries; do not hand-edit generated output.
- `jarvis-cli/internal/{sddruntime,skills,agent,project}` contract tests — drift and install/render verification surface.
- OpenSpec bootstrap: no `openspec/config.yaml` or active `openspec/changes/` bootstrap exists in this checkout; only archived changes and main specs are present. The new change directory is therefore a clean addition, not an integration into an existing active change.

### Upstream Change Classification

| Area | Classification | Exploration decision |
|---|---|---|
| `sdd-explore`, `sdd-propose`, `sdd-spec`, `sdd-design`, `sdd-tasks`, `sdd-init`, `sdd-onboard` | Adopt, adapt-to-Hive/Jarvis | Resync the v2.1.5 phase mechanics, provenance, English artifact-language contract, and current persistence rules; retain Jarvis paths, four stores, status context, and Go-owned model injection. |
| `sdd-apply` | Adopt, adapt-to-Hive/Jarvis; defer review authority | Adopt mechanical phase/apply-progress improvements. Defer native review transaction, snapshot, scope, and receipt semantics to #366/#363; do not invent a second authority. |
| `sdd-verify` and `sdd-archive` | Adapt-to-Hive/Jarvis; defer authority work | Adopt clearer strict verification/report behavior only where compatible. Structured verify-result/archive gate, receipts, snapshots, and strict authority belong to #420/#366 and must not be partially ported here. Preserve current `sdd-verify` archive gate and do not add a Judgment-Day-before-archive gate. |
| `comment-writer`, `branch-pr`, `chained-pr`, `work-unit-commits`, `issue-creation`, `cognitive-doc-design` | Adopt selectively; adapt policy | Take documentation/review-load improvements. Keep direct GitHub content English first plus normative Spanish from Spain, and avoid pulling review lifecycle authority from #365–#367. |
| `skill-registry` | Adapt-to-Jarvis | Upstream's larger registry/index behavior is useful, but canonical `.jarvis/skill-registry.md`, `.atl/` legacy read behavior, and installer/runtime contracts remain authoritative. |
| `_shared/review-ledger-contract`, native review transaction material, modern Judgment-Day mechanics | Defer to #365/#366/#367 | These are domain changes, not a mechanical skill sync. #362 should reference them as exclusions/dependencies rather than duplicate their contracts. |
| artifact semantic validity and content-derived state | Defer to #422 | #362 should preserve contract wording needed by #422, but not implement validators or change public state semantics. |
| schema-bound verification/archive authority | Defer to #420 | #420 depends on the resync; it owns the structured authority and strict gate. |
| native remediation lineage | Defer to #421 | #421 depends on #420/#366 and must consume their authorities. |
| CodeGraph lifecycle, unrelated upstream agents/adapters, engram engine adoption | Reject / out of scope | Not part of Jarvis MVP resync and conflicts with explicit product boundaries. |

### Statements in #362 That Become Stale or Ambiguous

1. Every `v1.49.0` reference must become `v2.1.5`, but “latest” must be replaced with the pinned tag and commit above.
2. “~23 minor” and “~9 minor” drift counts are no longer meaningful; replace them with a file-by-file comparison against the pinned tree and a selective scope table.
3. The old provenance table omits upstream changes after v1.49.0 and does not distinguish `internal/assets/skills/` from public `skills/` mirrors.
4. “Mechanical resync” is ambiguous for `sdd-verify`/`sdd-archive`: v2.1.5 includes native review/verification authority concepts that are explicitly owned by #366/#420, so the issue must state that only compatible prompt mechanics are in scope.
5. The “memory is bookkeeping” item must remain Hive-adapted; upstream v2.1.5 says Engram and has upsert/lifecycle semantics that cannot be transplanted without changing product behavior.
6. The acceptance criterion “no `engram` reference leaks into any resynced asset” needs a scope qualifier: upstream provenance/source links and explicitly rejected terminology may mention the upstream name, but shipped runtime/tool semantics must not invoke Engram or use `mcp__engram__*`.
7. The old Judgment-Day exclusion is incomplete: #367 now owns modernization, while #365/#366 own ledger/transaction authority; #363 owns phase wiring. The issue must define sequencing and forbid partial imports.
8. The issue does not mention #420–#422. It must state that #420 and #422 consume the resynced contracts, #421 consumes #420, and none of those follow-ups should be silently implemented in #362.
9. “Go test ./...” and “go vet ./...” remain valid acceptance checks, but the scope must include source-of-truth/render/install drift tests and both template parity checks when templates are touched.
10. The Spanish half must be regenerated from the corrected English contract and follow repository policy: English first, then normative Spanish from Spain. It must not use neutral/Latin American wording as a substitute for the public policy.

### Dependency and Sequencing Findings

Recommended dependency remains `#362 -> #365 -> (#366 and #367) -> #363`, with #420 depending on #362 and #366, #421 depending on #420/#366, and #422 depending on #362/#420. #363 should consume the final phase contracts rather than being implemented concurrently against stale prompt semantics. #365–#367 may proceed in parallel only for clearly isolated review-domain work; their authority names and lifecycle rules must not be guessed in #362. #420–#422 are newer than the original issue and should be linked as downstream consumers/follow-ups, not folded into the resync.

### Recommended Issue Update (do not apply here)

**Title:** `chore(skills): selectively resync Jarvis SDD and peripheral skills to gentle-ai v2.1.5`

**Scope:** Compare the Jarvis embedded source-of-truth assets with upstream tag `v2.1.5` at commit `1b5a5f59f74d3f6dab7de01c1603d5ce1b77af17`; selectively adopt mechanical SDD/peripheral improvements; adapt all persistence, Hive, registry, template, runtime, persona, and repository-language semantics; record per-skill dispositions and provenance.

**Acceptance criteria:**

- [ ] Every in-scope asset has a pinned v2.1.5 tag+commit provenance record and an explicit adopt/adapt/defer/reject disposition.
- [ ] Hive tool semantics, `capture_prompt:false` automation behavior, four artifact stores, Jarvis runtime paths, `.jarvis` canonical registry plus `.atl` legacy read, Go-template model rows, persona/artifact separation, and generated-config source-of-truth rules remain intact.
- [ ] No shipped skill invokes or instructs `engram`; Hive-adapted memory-bookkeeping behavior is present and does not replace the user-facing answer.
- [ ] Review authority, Judgment-Day modernization, schema-bound verification, remediation lineage, and semantic validity are linked to #365/#366/#367/#420/#421/#422/#363 and not reimplemented.
- [ ] Source assets, rendered/install contracts, skill catalog, SDD runtime contracts, and `AGENTS.md.tmpl`/`CLAUDE.md.tmpl` parity checks pass; `go test ./...` and `go vet ./...` pass.
- [ ] Issue documentation is English first followed by normative Spanish from Spain.

**Explicit exclusions:** native review ledger/transaction/snapshot/receipt authorities; 4R/Judgment-Day modernization; strict verify-result/archive gate; remediation generations; content-derived artifact validators; CodeGraph lifecycle; Engram adoption; generated user-machine files; GitHub issue mutation during implementation.

### Effort and Review Budget

The raw one-to-one diff for 16 comparable `SKILL.md` files is approximately 514 additions and 520 deletions before shared contracts, references, tests, provenance comments, or bilingual issue text. A selective Jarvis-adapted patch can likely be held around 350–550 authored changed lines if it excludes authority work and limits peripheral changes. The 600-line single-PR preference is realistic but tight; it becomes high risk if shared contracts, rendered goldens, or all v2.1.5 registry/review assets are imported wholesale. Keep the PR focused on source assets plus focused contract tests, and treat any authority-bearing additions as explicit follow-up issues.

### Recommendation

Proceed to proposal only after retargeting #362 around a pinned upstream snapshot and selective dispositions. The safe implementation is a source-asset resync with explicit Hive/Jarvis adaptation, not a wholesale upstream copy. Keep #362 as the contract prerequisite for #420/#422 and the phase-language prerequisite for #363, while leaving review authority to #365–#367 and later verification/remediation work to #420/#421.

### Risks

- Wholesale copying v2.1.5 would import Engram semantics, native review authorities, or unsupported artifact states into Jarvis.
- `sdd-verify`/`sdd-archive` changes can accidentally create a second archive authority or alter the existing verify gate before #420 is ready.
- Registry and template changes can break installed output while source files remain apparently correct; render/install contract tests are mandatory.
- The 600-line budget can be exceeded by generated goldens or bilingual issue churn; keep generated snapshots out of authored scope where possible and split authority work.
- Current repository has no active OpenSpec bootstrap/config; downstream phases must not assume one exists beyond this change artifact.

### Ready for Proposal

Yes. The orchestrator should first update the issue specification (without editing it in this exploration), then run proposal/design with the pinned upstream commit, per-skill disposition table, and the 600-line single-PR guard as explicit constraints.
