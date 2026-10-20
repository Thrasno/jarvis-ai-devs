# Issue 798 (+797): one project identity at every ingress

Objective: every SDD path resolves the project the same way (spelling fold plus alias) at its ingress, so non-canonical spellings and retired keys never report `blocked:invalid` or write under a retired key; then `jarvis sdd bind` (#797) writes the effective key.
Branches: `fix/798-daemon-identity-ingress` (daemon, PR3), `fix/798-cli-typed-identity` (CLI, PR4, stacked), `feat/797-sdd-bind` (PR5, stacked). Push/PR are the user's decision. Build prohibited. Cross-machine alias propagation is out of scope (issue #801).

## Specs

S1 (issue #798, Expected behaviour, verbatim): "Any spelling that maps to the same canonical Hive key reports the same state (`ready`). The CLI should canonicalize the project once at its boundary (`resolveSddProject` and the checkpoint/advance input, via `hiveclient.CanonicalProjectKey`), as `store_binding` and the daemon already do. As defence in depth, the sources should compare canonical keys. A real identity mismatch should get a specific code, not `blocked:invalid`."

S2-S6: the maintainer scope correction on issue #798 (English section, verbatim):

## Scope correction after investigating sync

The previous comment called the approach definitive. Tracing hive-api sync showed two different problems, so the scope is split.

**What already holds:** promotion is refused when a project has apply-progress (`WorkspacePromotionProtectedError`, `db/project.go:450-461`), so immutable progress is never re-keyed.

**#798 now covers local identity consistency** (one machine resolves identity the same way at every ingress):

1. Apply-progress handlers resolve spelling and alias at ingress and return the effective project key (today `GetGovernanceProject` only folds spelling, `db/project.go:161-175`).
2. Store-binding HTTP routes get the same project gate and alias resolution. Today `handleSDDStoreBindingGet`/`Adopt` skip governance entirely and can write a binding under a retired key (`httpapi/server.go:304-357`, `governance/sdd.go:80-110`). This also protects `jarvis sdd bind` from #797.
3. `AddAlias` gets the apply-progress guard that `MergeGovernanceProjectTx` already has, so a manual alias cannot strand heads, receipts or documents under a retired key (`db/alias.go:105-160`).
4. Pulled immutable apply-progress documents whose project was remapped by an alias are rejected with a specific, diagnosable reason instead of being dropped as a generic invalid document (`db/sync.go:1290-1307`).
5. CLI: spelling canonicalized once at the boundary, `projectidentity.Key` in SDD signatures, comparison against the effective key returned by the daemon, and `project_mismatch` instead of `blocked:invalid`.

Delivery stays two PRs: daemon first (1-4), then CLI (5). #797 follows.

**Moved to a new issue:** propagating aliases across machines. Aliases are local-only, hive-api has no alias table, receipts/heads/bindings never sync, and a teammate who never promoted keeps using the retired key. That is product design for #722, not this bug.


Spec mapping: S2 = item 1 (apply-progress ingress, effective key), S3 = item 2 (store-binding gate), S4 = item 3 (AddAlias guard), S5 = item 4 (diagnosable pull rejection), S6 = item 5 (CLI).

S7: RED tests named in the earlier maintainer comment on #798 (verbatim): "`MyProject`, `my_project` and `myproject` report the same `ready` state; a retired alias key reads and writes the promoted project's progress and binding, and never recreates rows under the retired key; a genuine mismatch reports `project_mismatch` and leaves stored progress unchanged."

Investigation evidence (explore handoff, 2026-10-20): apply-progress HTTP handlers `hive-daemon/internal/httpapi/server.go:449-561` -> `governance/sdd.go:151-264` -> `GetGovernanceProject` (`db/project.go:161-175`, spelling only); store-binding handlers `server.go:304-357` -> `governance/sdd.go:80-110` (no governance gate) -> `db/sdd_store_binding.go:92-110`; `DB.AddAlias` `db/alias.go:105-160` lacks the guard used by `governanceMergeHasImmutableApplyProgress` (`db/project.go:450-461,651-664`); pull validation `db/sync.go:1290-1307`, rejection `1446-1447`, `1003-1022`; MCP tools `sdd_apply_progress_get`, `sdd_apply_evidence_get` (`mcp/tools.go:100-101`) not traced.

## Tasks

- [x] T1 -> S2-S5, S7: daemon PR. RED tests, alias resolution at ingress for apply-progress and store-binding (HTTP, governance, db, MCP SDD tools), effective key in responses (additive field), store-binding project gate, AddAlias guard, specific pull rejection reason. Route: background gentle-ai-worker in `../jarvis-dev-798`. Check: worker RED then GREEN; parent reran `hive-daemon` gofmt + vet + `go test -count=1 ./...` and `jarvis-cli` `go test -count=1 ./...` (both exit 0). Commit: see git log of fix/798-daemon-identity-ingress.
- [ ] T2 -> S2-S5: independent verification + native review (high risk: data, contract). Verify round 1: S4, S5, S7 PASS; blockers B1 (released CLI store-binding decoders reject `project_key`) and B2 (legacy case-only alias resolved differently in and out of a tx) fixed per L7 with tests. Native review: pending.
- [ ] T3 -> S1, S6, S7: CLI PR stacked on T1. Commit: pending.
- [ ] T4 -> #797 maintainer decision: `jarvis sdd bind`, stacked on T3. Commit: pending.

## Log

L1 (user, 2026-10-20): "#798 - Ya llevamos varias iteracciones sobre la resolucion del nombre de proyectos. Necesitamosmos abordar esto de forma definitiva. Cuentame exactamente que tenemos pensado hacer y si realmente crres que esto es la forma de solventar esto de forma definitiva tanto como para hive como para jarvis."
L2 (user, 2026-10-20): chose "Definitivo (recomendado)" scope.
L3 (user, 2026-10-20): after the sync investigation, chose "A en #798 + issue nuevo para B (recomendado)": #798 covers local identity consistency; cross-machine propagation moved to #801.
L4 (orchestrator, 2026-10-20, T1): S7 example corrected: `my_project` folds to `my-project`, not `myproject`; contract v1 unchanged. Fold tests use `MyProject`/` MYPROJECT `/`myproject` -> `myproject` and `my_project`/`My.Project`/`my-project` -> `my-project`; aliases are covered by their own retired-key tests.
L5 (orchestrator, 2026-10-20, T1): store-binding get/adopt get fold + alias resolution (adopt inside the tx) and the blocked-project write gate on a new adopt; identical replay stays idempotent even when blocked; no governance-existence requirement (adopt-before-artifacts is required by #797).
L6 (orchestrator, 2026-10-20, T1): uniform `project_key` on every success and post-resolution error response of the apply-progress and store-binding routes and MCP apply-progress tools; the shared `testdata/sdd-progress/` wire fixtures are regenerated only through the existing `-update` test path.
L7 (parent, 2026-10-20): store-binding responses keep their released closed shape; no `project_key` there because released CLI decoders reject unknown fields. `project_key` stays on apply-progress HTTP routes and MCP.
