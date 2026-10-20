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

S8 (issue #797, Expected, verbatim):

## Expected
The preflight choice reaches the CLI explicitly (e.g. an initial-mode flag, or a mandatory env prefix enforced by orchestrator and skills). A read-only-looking `status` call must not silently persist the default when the user chose otherwise.

S9 (maintainer decision on issue #797, verbatim):

## Maintainer decision

**Approved approach:**

- Add an explicit adoption surface: `jarvis sdd bind --change <c> --mode hive|openspec|hybrid`, persisted with provenance `initial:preflight`.
  - An identical existing binding is an idempotent replay.
  - A different existing binding returns a clear conflict; it never overwrites.
- The orchestrator and SDD skills call `bind` right after the preflight choice, before any `status` call or artifact write. This closes both paths: the silent `initial:default:hive` adoption and the `legacy:hive-progress` inference.
- Update `persistence-contract.md` so the preflight choice reaches the CLI through `bind`.

**Out of scope:** changing `status` so it stops adopting the default for an unbound change. It changes the documented contract and is not needed once `bind` exists. Open a separate issue if it becomes necessary.

**Ordering:** implement after #798. `bind` receives a project identity and must use the canonical Hive key; otherwise a non-canonical spelling would be written into an immutable binding.

**RED tests first:** `bind --mode hybrid` followed by `status` without `JARVIS_SDD_STORE_MODE` keeps `hybrid`; identical replay succeeds; conflicting mode is rejected and leaves the stored binding unchanged; `bind` before any Hive progress write prevents `legacy:hive-progress` inference.

---

## Decisión de mantenimiento

**Enfoque aprobado:**

- Añadir una forma explícita de adopción: `jarvis sdd bind --change <c> --mode hive|openspec|hybrid`, que se persiste con procedencia `initial:preflight`.
  - Un vínculo existente idéntico se trata como repetición idempotente.
  - Un vínculo existente distinto devuelve un conflicto claro; nunca se sobrescribe.
- El orquestador y las skills de SDD invocan `bind` justo después de la elección del preflight, antes de cualquier llamada a `status` o escritura de artefactos. Así se cierran ambos caminos: la adopción silenciosa de `initial:default:hive` y la inferencia `legacy:hive-progress`.
- Actualizar `persistence-contract.md` para que la elección del preflight llegue a la CLI mediante `bind`.

**Fuera de alcance:** modificar `status` para que deje de adoptar el valor por defecto en un cambio sin vincular. Altera el contrato documentado y no es necesario una vez exista `bind`. Si llegara a hacer falta, se abrirá una issue aparte.

**Orden:** implementar después de #798. `bind` recibe una identidad de proyecto y debe usar la clave canónica de Hive; de lo contrario, una forma no canónica del nombre quedaría grabada en un vínculo inmutable.

**Tests en rojo primero:** `bind --mode hybrid` seguido de `status` sin `JARVIS_SDD_STORE_MODE` mantiene `hybrid`; la repetición idéntica se acepta; un modo en conflicto se rechaza y deja intacto el vínculo almacenado; ejecutar `bind` antes de cualquier escritura de progreso en Hive evita la inferencia `legacy:hive-progress`.

## Tasks

- [x] T1 -> S2-S5, S7: daemon PR. RED tests, alias resolution at ingress for apply-progress and store-binding (HTTP, governance, db, MCP SDD tools), effective key in responses (additive field), store-binding project gate, AddAlias guard, specific pull rejection reason. Route: background gentle-ai-worker in `../jarvis-dev-798`. Check: worker RED then GREEN; parent reran `hive-daemon` gofmt + vet + `go test -count=1 ./...` and `jarvis-cli` `go test -count=1 ./...` (both exit 0). Commit: see git log of fix/798-daemon-identity-ingress.
- [x] T2 -> S2-S5: independent verification + native review (high risk: data, contract). Verify round 1: S4, S5, S7 PASS; blockers B1 (released CLI store-binding decoders reject `project_key`) and B2 (legacy case-only alias resolved differently in and out of a tx) fixed per L7 with tests. Commit `5414e027`. Native review: assess medium, not due (under budget).
- [x] T3 -> S1, S6, S7: CLI PR stacked on T1. Daemon follow-up for change-list and artifact routes (L11): `d725cdf0`. Verify: S1 PASS, S7 PASS, S6 blocker (advance adopted a binding before reporting project_mismatch) fixed with an unbound no-write test. Checks: `jarvis-cli` gofmt + vet + test and `hive-daemon` test exit 0. Commits: `5adcfdbf`, `c99b5c88`. Native review: assess medium, not due.
- [x] T4 -> S8, S9: `jarvis sdd bind`, stacked on T3. Verify: S8 PASS, S9 PASS; finding F1 (failed hybrid bind left empty `openspec/` dirs), N3 (orchestrator status-before-bind warning) and N5 (lost adoption race) fixed test-first (L19-L21). Commits: `b0504f97`, `12cdf486`.

## Log

L1 (user, 2026-10-20): "#798 - Ya llevamos varias iteracciones sobre la resolucion del nombre de proyectos. Necesitamosmos abordar esto de forma definitiva. Cuentame exactamente que tenemos pensado hacer y si realmente crres que esto es la forma de solventar esto de forma definitiva tanto como para hive como para jarvis."
L2 (user, 2026-10-20): chose "Definitivo (recomendado)" scope.
L3 (user, 2026-10-20): after the sync investigation, chose "A en #798 + issue nuevo para B (recomendado)": #798 covers local identity consistency; cross-machine propagation moved to #801.
L4 (orchestrator, 2026-10-20, T1): S7 example corrected: `my_project` folds to `my-project`, not `myproject`; contract v1 unchanged. Fold tests use `MyProject`/` MYPROJECT `/`myproject` -> `myproject` and `my_project`/`My.Project`/`my-project` -> `my-project`; aliases are covered by their own retired-key tests.
L5 (orchestrator, 2026-10-20, T1): store-binding get/adopt get fold + alias resolution (adopt inside the tx) and the blocked-project write gate on a new adopt; identical replay stays idempotent even when blocked; no governance-existence requirement (adopt-before-artifacts is required by #797).
L6 (orchestrator, 2026-10-20, T1): uniform `project_key` on every success and post-resolution error response of the apply-progress and store-binding routes and MCP apply-progress tools; the shared `testdata/sdd-progress/` wire fixtures are regenerated only through the existing `-update` test path.
L7 (parent, 2026-10-20): store-binding responses keep their released closed shape; no `project_key` there because released CLI decoders reject unknown fields. `project_key` stays on apply-progress HTTP routes and MCP.
L8 (worker, 2026-10-20, T3): store-binding redirect. `hiveclient` keeps the released closed decoders and still rejects a binding whose project differs from the canonical request, but reports an otherwise valid binding under a different canonical key as a typed `SDDStoreBindingRedirectError` (effective key = `binding.project`). `resolveSddStoreBindingAt` accepts the redirect only after the apply-progress route returns the same `project_key` for the requested key, then resolves again under the effective key; an unconfirmed redirect (including older daemons that omit `project_key`) is `project_mismatch`. A redirected first adoption already lands under the target, so the replay only reads it back.
L9 (worker, 2026-10-20, T3): `HiveSource.FetchArtifacts` reads the guarded head before the artifact rows and reads the rows under the head's effective key, because the daemon artifact and change-list routes fold spelling but do not resolve aliases. Implicit change discovery (`status`/`continue` without a change name) under a retired key therefore still fails at the daemon list route; an explicit change name works.
L10 (worker, 2026-10-20, T3): `advance` submits a signed snapshot that cannot be re-keyed; its project is canonicalized only to resolve the binding, and a snapshot whose project differs from the effective key returns `project_mismatch` without writing. `checkpoint`/`upgrade-continuation` rewrite the request project to the effective key before planning. `project_mismatch` is `blocked` with `next.action` `stop_blocked`; status reports `blocked:project_mismatch`.
L11 (parent, 2026-10-20): "Q1: yes. The daemon change-list and artifact routes will resolve aliases in a separate daemon commit on fix/798-daemon-identity-ingress (another worker, in parallel). Do not touch hive-daemon. After it lands, status/continue without a change name must work under a retired key; keep your code ready for it (no CLI-side workaround)."
L12 (parent, 2026-10-20): "Q2: yes. `jarvis sdd archive --project` folds the spelling through the same boundary as status/continue/progress, instead of rejecting non-canonical spellings."
L13 (parent, 2026-10-20): "Q3: no. `jarvis sdd supersede` stays fail-closed for retired keys. Supersede requires existing apply progress, and both promotion and AddAlias now refuse to retire a project that holds apply progress, so a retired-key supersede cannot arise from a supported path; following a redirect would widen a high-risk flow for no reachable case."
L14 (worker, 2026-10-20, T4): `bind` is `sddbinding.LegacyResolver.Bind`, which shares the resolution lock and the read-decide path of `ResolveAndAdopt` (refactored into `decideLocked` + `adopt`; `ResolveAndAdopt` behaviour unchanged). A persisted binding, or the binding that protected progress already selects, always wins: the same mode is an idempotent replay that writes nothing and keeps the stored provenance (for example `initial:default:hive` from an earlier status), and a different mode returns `*sddbinding.ConflictError` without writing. A legacy inference that matches the requested mode is adopted with its `legacy:*` provenance; only an unbound change without protected progress gets `initial:preflight`. Mode equality, not provenance equality, defines "identical".
L15 (worker, 2026-10-20, T4): OpenSpec persistence for `bind` follows the existing owners: `hive` in Hive only, `openspec` in `openspec/changes/<change>/state.yaml` of the working-directory workspace only, `hybrid` in both (Hive first, then the mirror). Because `bind` runs before any artifact write and the file store requires an existing change directory, `Bind` creates `openspec/changes/<change>/` for `openspec`/`hybrid` only, under the lock, after the decision (never on a conflict), and revalidates the coordinate after creating it; `hive` never creates `openspec/`. The persistence contract already allows `openspec/` creation for those two modes.
L16 (worker, 2026-10-20, T4): CLI `jarvis sdd bind --change <c> --mode hive|openspec|hybrid [--project <p>] [--json]` (no `--root`; the workspace is the canonical working directory, as for status). `--mode none` and unknown modes are rejected before any daemon request or write. The project goes through `resolveSddProject`, and the store-binding redirect goes through `followSddStoreBindingRedirect`, now shared with `resolveSddStoreBindingAt`, so a retired alias binds under the confirmed effective key. `--json` prints `{project, change, mode, provenance, persisted}` with the effective key; there is no created/replayed flag because a redirected first adoption is read back on the retry.
L17 (parent, 2026-10-20): accepted: a `none` change is never bound; assets tell agents to prefix every `jarvis sdd` call for that change with `JARVIS_SDD_STORE_MODE=none` so status does not adopt the `hive` default. Recorded in `persistence-contract.md`.
L18 (worker, 2026-10-20, T4): assets. The orchestrator gains `#### Preflight Store Binding` (bind right after the artifact-store choice, before any status/continue call, phase launch or artifact write; replay on recorded decisions; any non-zero exit stops before status) and lists `bind` first in the dispatcher block. `sdd-phase-common.md` makes phase skills run the same `bind` before their own first status call or artifact write unless forwarded status JSON already shows `storeBinding.persisted: true`; `sdd-propose` keeps the `state.yaml` that `bind` wrote. No `internal/config` policy test pinned wording that contradicts the new rule, so none was changed; status without preflight stays an inspection path, out of scope per S9.
L19 (worker, 2026-10-20, T4 verify F1): a failed `bind` leaves the workspace as it was. `Bind` no longer creates the OpenSpec change directory before adopting: it creates the missing levels below the workspace only when the local copy is written (after Hive accepted the adoption for `hybrid`; `openspec` has no Hive step), records the levels this call created, and removes them deepest first when that write fails. A level that is no longer an empty directory is kept, because it holds content `bind` did not write. If Hive accepted a `hybrid` binding but the local write failed, `PartialAdoptionError` still applies and rerunning `bind` (not `status`, which never creates the directory) writes the mirror.
L20 (worker, 2026-10-20, T4 verify N5): a lost race is resolved, not just reworded. When the Hive adoption returns `binding_conflict` because another worktree adopted the change between the read and the adoption, `Bind` re-runs the locked decision once against the binding that won. A different mode then returns the usual `*ConflictError` (same `bound to X ... requested Y` message as a stored binding); the same mode is a replay, and for `hybrid` it writes the local mirror with the winner's provenance (replaying the Hive copy, as `ResolveAndAdopt` does). `docs/troubleshooting.md` and `docs/cli-reference.md` describe this.
L21 (worker, 2026-10-20, T4 verify N3): the orchestrator's Preflight Store Binding section says not to run `jarvis sdd status` on a new change before `bind`, because status adopts the default store; the status-without-preflight exception covers inspecting existing changes. Existing pinned phrases are unchanged; the guidance contract test pins the new line.
