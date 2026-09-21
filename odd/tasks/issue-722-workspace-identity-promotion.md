# Issue #722 — Workspace Identity Promotion

## Objective

Implement guarded promotion from a directory-derived local project identity to a later Git-derived canonical identity without losing or duplicating local project state, while absorbing the concrete Project Viewer regression from #721.

## Problem

A workspace first observed without Git can be persisted under directory identity A and later derived as Git identity B. Today the two keys can diverge across memories, sessions, prompts, sync state, governance, aliases, SDD state, and UI filters. Project DTOs also conflate canonical identity with display spelling, which can make valid memories appear missing.

## Why

The split produces a false data-loss signal, weakens project continuity, and can allow both identities to keep receiving writes. A supported promotion must preserve audit history, prevent the retired source from being recreated during normal operation, and keep archive/purge semantics predictable.

## Scope

- Update GitHub issue #722 with the accepted product decisions and #721 regression; close #721 as absorbed, not completed.
- Separate canonical project key from display name in local governance/client/TUI contracts.
- Persist a durable workspace-to-project binding.
- Detect an unambiguous directory→Git identity transition and promote A→B atomically.
- Resolve redirects before supported local writes, imports, and sync ingestion so A cannot be recreated after promotion.
- Reconcile all relevant local project-scoped state, not only memories.
- Preserve identity metadata on archive.
- On local purge, remove every local trace including aliases, redirects, workspace bindings, identity registry state, sync/governance metadata, and applicable SDD state; permit later recreation as a new project.
- Keep Hive API deletion and project administration independent from local purge.
- Carry canonical key/display name through Hive API and dashboard surfaces where necessary for consistent browsing and sync.
- For immutable protected SDD apply progress, fail closed with a clear dependency on #724; do not rewrite hashes, receipts, or signed payloads in #722.

## Out of Scope

- Hive API administrative deletion workflow.
- Rewriting immutable SDD apply progress in place.
- Implementing the general protected-progress supersession flow owned by #724.
- Treating local purge as a cloud deletion or tombstone request.

## Accepted Product Decisions

1. `Project.Key` is the stable canonical technical identity; `Project.Name` is presentation only.
2. All filters, reads, writes, merges, archive, purge, sync, aliases, and SDD coordinates use canonical keys.
3. Archive preserves project identity metadata and redirects.
4. Purge removes all local traces and allows the same project to return later as a new project.
5. Hive API lifecycle is independently administered; remote data may reappear locally until an administrator removes or disables it.
6. #721 is absorbed into #722 as an explicit regression and acceptance case.
7. Protected SDD progress blocks identity promotion until #724 provides safe supersession.

## Constraints

- Preserve boundaries between Hive local memory, Hive API, sync, CLI, and SDD.
- Never rewrite immutable apply-progress payloads or receipts.
- Use atomic SQLite transactions and explicit locks/revalidation for promotion.
- Avoid broad abstractions; prefer explicit Go code.
- Do not edit generated user-machine agent configuration.
- GitHub comments must be bilingual: English first, normative Spanish from Spain.
- No production release is planned before #721/#722 behavior is corrected.

## Testing Mode

- Mode: Standard ODD verification; strict TDD is not enabled by repository policy for non-SDD work.
- Source: `AGENTS.md` requires strict TDD specifically for SDD implementation work.
- Go runner: focused `go test` packages, followed by `go test ./...` and `go vet ./...` at the integrated boundary.
- Frontend runner: resolve from the dashboard package before that work unit; do not invent a command.
- Tests remain mandatory and should be authored with the behavior they protect.

## Delivery Strategy

- Strategy: `ask-on-risk`, resolved to `feature-branch-chain` by the user.
- Aggregate branch: `feat/issue-722-workspace-identity`.
- Forecast: approximately 1,800–2,600 authored changed lines across multiple cohesive work units; generated files excluded.
- Keep each work unit near 400 authored lines when coherent; do not code-golf or omit tests.
- Each work unit closes with a conventional commit and records its commit/slice boundary here.

## Work Units

### WU-01 — Issue authority and regression transfer

- Route: parent inline GitHub mutation; no repository source edit.
- Trigger evidence: GitHub state operations are parent-owned; issue bodies were read and verified.
- [x] Add the accepted canonical-key, purge/archive, Hive API ownership, #724 dependency, and #721 regression decisions to #722 in English and Spanish.
- [x] Add a bilingual absorption comment to #721 and close it as not planned/absorbed, not completed.
- Checks:
  - Read back #722 and confirm the exact added decisions.
  - Read back #721 and confirm closed state plus absorption comment.
- Commit: N/A.

### WU-02 — Canonical local project key and #721 regression

- Route: delegated direct writer.
- Trigger evidence: multi-file write across daemon DTO/query, CLI client, TUI, runner, and tests.
- [x] Add a canonical project key separate from display name to local governance/client DTOs.
- [x] Use the key for memory/timeline filtering and per-project API requests while rendering the display name.
- [x] Ensure merge/archive/purge selections bind canonical keys even when confirmations render display names.
- [x] Add regression coverage for display spelling differing from the stored canonical key.
- Checks:
  - Focused daemon governance tests.
  - Focused CLI hiveclient/hiveui tests.
- Commit: `2c1232b8` (`fix(hive): separate project keys from display names`).
- Authored code lines: 410 (349 additions + 61 deletions); slightly above the ~400 heuristic because DTO, request, rendering, and regression tests form one atomic compatibility unit. No size-only code-golf applied.

### WU-03 — Workspace binding and guarded local promotion

- Route: delegated direct writer.
- Trigger evidence: coordinated schema, repository, validator, derivation, and migration changes across more than four files.
- [x] Add durable workspace→canonical-project binding storage with migration coverage.
- [x] Resolve existing binding before deriving a new identity.
- [x] Detect an unambiguous directory→Git transition.
- [x] Implement atomic, idempotent promotion with source/target revalidation.
- [x] Fail closed when protected SDD apply progress exists, naming #724-compatible recovery.
- Checks:
  - Focused DB/project/validator tests covering first observation, restart, promotion, retry, conflicts, alias/session ordering, provenance, sibling bindings, and protected-progress rollback.
- Commit: `2e361374` (`feat(hive): promote workspace project identities`).
- Authored code lines: 1,273 (1,204 additions + 69 deletions); the coherent transaction/schema/validator unit substantially exceeds the heuristic because three review rounds added required upgrade, rollback, alias, provenance, session, sibling-binding, and restart regression coverage. No size-only code-golf applied.

### WU-04 — Universal ingress resolution and complete local-state migration

- Route: delegated direct writer.
- Trigger evidence: multi-file write across sessions, memories, prompts, imports, passive observations, sync ingestion, and migration inventory.
- [ ] Resolve redirects before every supported local write/import/pull path.
- [ ] Promote/reconcile the complete applicable `ProjectKeyedStates()` inventory.
- [ ] Prevent offline/import/session ingress from recreating retired A after promotion.
- [ ] Preserve relocation provenance needed by sync.
- Checks:
  - Focused DB/import/sync tests for delayed writes, imports, pull ingestion, and retry safety.
- Commit: pending.

### WU-05 — Archive and complete local purge lifecycle

- Route: delegated direct writer.
- Trigger evidence: coordinated DB, governance service/HTTP/client, confirmation, and tests.
- [ ] Preserve bindings, identities, and redirects during archive.
- [ ] Purge all local project-bearing states, identity registry rows, aliases in both directions, workspace bindings, sync state, governance records, and applicable SDD state atomically.
- [ ] Confirm a later observation recreates the same name as a new project.
- [ ] State clearly that local purge does not delete Hive API data.
- Checks:
  - Focused DB/governance/http/client tests for archive retention, purge completeness, idempotency, recreation, and cloud-handoff wording.
- Commit: pending.

### WU-06 — Hive API canonical/display identity compatibility

- Route: delegated direct writer.
- Trigger evidence: API migration plus repositories, services, models, handlers, mocks, and tests.
- [ ] Design canonical/display storage without reviving the unsafe removed API registry semantics.
- [ ] Key project aggregates, filters, audit/health, and sync behavior by canonical identity.
- [ ] Preserve display spelling independently.
- [ ] Maintain compatibility for existing stored project literals and sync clients.
- Checks:
  - Focused Hive API repository/service/handler tests and migrations.
- Commit: pending.

### WU-07 — Dashboard canonical-key browsing

- Route: delegated direct writer.
- Trigger evidence: multi-file TypeScript API/domain/UI tests.
- [ ] Carry canonical key and display name through dashboard DTO/domain models.
- [ ] Query/filter by canonical key and render display name.
- [ ] Add the dashboard side of the #721 regression.
- Checks:
  - Resolve and run the package's documented focused test command.
- Commit: pending.

### WU-08 — Integrated verification and delivery slices

- Route: delegated verification according to native risk assessment after each work-unit commit; parent spot-checks one reported command.
- Trigger evidence: verification commands must route through the verifier under repository orchestration policy.
- [ ] Run `go test ./...`.
- [ ] Run `go vet ./...`.
- [ ] Run the resolved dashboard test command.
- [ ] Verify #721 scenario end to end: A before Git, B after Git, data under both, promotion, one visible B, memories visible exactly once.
- [ ] Record authored line totals and PR slice boundaries.
- Checks: all commands and manual scenario above.
- Commit: pending if integration fixes are needed.

## Acceptance Criteria

- [x] A workspace first observed without Git is durably bound to its canonical project.
- [x] Adding or renaming to a usable Git repository promotes the workspace core state to the Git-derived key when unambiguous; full state inventory remains WU-04.
- [x] Core promotion is atomic and idempotent with source/target revalidation; complete inventory and backup integration remain scoped to WU-04/WU-05.
- [ ] The retired source cannot be recreated by supported session, memory, prompt, import, passive observation, or sync ingress paths.
- [x] Local project DTOs expose canonical key separately from display name; Hive API/dashboard projection remains pending.
- [x] Local TUI renders display names but queries/filters/mutates by canonical key; dashboard remains pending.
- [ ] The complete #721 promotion scenario shows the surviving memories exactly once under B; display-vs-key viewing and core A→B promotion are covered, while universal ingress/state migration remains pending.
- [ ] Archive retains bindings and redirects.
- [ ] Purge removes all local traces and permits later recreation as a new project.
- [ ] Local purge does not claim to delete Hive API data.
- [x] Protected SDD apply progress is never rewritten and blocks promotion with an actionable #724 dependency.
- [ ] Existing project literals remain compatible through migration.
- [ ] Focused checks, `go test ./...`, `go vet ./...`, and dashboard tests pass.

## Progress

- 2026-09-21: User authorized GitHub issue updates plus complete implementation.
- 2026-09-21: Created aggregate branch `feat/issue-722-workspace-identity` from clean `public/master` at `a315902a`.
- 2026-09-21: User selected #724 dependency for protected SDD progress.
- 2026-09-21: User selected feature-branch chain delivery.
- 2026-09-21: Repository mapping identified six major implementation surfaces and the incomplete current merge/purge inventory.
- 2026-09-21: Updated #722 with the accepted bilingual lifecycle decisions and explicit #721/#724 relationships.
- 2026-09-21: Closed #721 as `NOT_PLANNED` because it is absorbed into #722; the bilingual closing comment states that implementation is still pending.
- 2026-09-21: WU-02 writer added canonical/display separation and focused tests, but independent verification rejected the first candidate: display-name confirmation phrases violate daemon canonical-coordinate confirmation contracts, and batch self-merge compares a canonical source against an unresolved display target.
- 2026-09-21: Corrected both WU-02 defects. Independent re-verification passed the complete eight-file diff, and the parent spot-check passed.
- 2026-09-21: Committed WU-02 as `2c1232b8`; native committed-candidate assessment was unavailable due a schema-incompatible native response, so the risk plan failed closed to high.
- 2026-09-21: Independent committed-range verification of `public/master..2c1232b8` passed all focused checks with no blocking findings. WU-02 is complete.
- 2026-09-21: WU-03 first candidate passed focused tests but failed independent verification on four gaps: upgrade did not adopt historical directory identity, session mismatch could be detected after a committed promotion, only the initiating workspace binding moved, and a nonempty unusable Git origin could misclassify basename fallback as Git-derived.
- 2026-09-21: Three bounded correction/re-verification rounds fixed alias persistence, session ordering, basename provenance, sibling bindings, protected rollback, and historical upgrade compatibility. Final independent verification passed with no blocking or medium findings; parent spot-check passed.
- 2026-09-21: Committed WU-03 as `2e361374`. Independent committed-range verification of `2c1232b8..2e361374` passed with no blocking or medium findings.
- 2026-09-21: Native RDD review could not start: after one correctly rejected abbreviated base, fresh full-SHA committed-only negotiation repeatedly surfaced expired consent state for an empty-workspace projection and created no lineage. The occurrence was added bilingually to upstream issue `Gentleman-Programming/gentle-ai#4754`; independent verification remains the accepted WU-03 gate.

## Verification Evidence

- Branch base: `public/master` and local `master` both `a315902a87252add3ddcfb92d9a7b98258f56a19`; divergence `0 0`.
- #722 readback confirmed the accepted-decisions section, #721 absorption, and #724 dependency.
- #721 readback confirmed `CLOSED` with `stateReason: NOT_PLANNED` and the exact bilingual absorption comment at `https://github.com/Thrasno/jarvis-ai-devs/issues/721#issuecomment-5757241865`.
- WU-02 writer checks passed: `cd hive-daemon && go test ./internal/db`; `cd jarvis-cli && go test ./internal/hiveclient ./internal/hiveui`.
- Independent WU-02 verification initially failed on two contract defects: canonical request coordinates paired with display-name confirmation phrases, and unresolved batch target display names bypassing canonical self-merge validation.
- Post-correction independent verification passed with no blocking findings.
- Parent spot-check `cd jarvis-cli && go test ./internal/hiveclient ./internal/hiveui` passed.
- `git diff --check` passed for all eight WU-02 files.
- Independent committed-range verification passed: `cd hive-daemon && go test ./internal/db`, `cd jarvis-cli && go test ./internal/hiveclient ./internal/hiveui`, and `git diff --check public/master..2c1232b8`.
- WU-03 writer checks passed in every round; earlier independent failures were retained and corrected rather than waived.
- Final independent WU-03 pre-commit verification passed: `cd hivederive && go test ./...`, daemon DB/project/httpapi/mcp focused suites, and `git diff --check`.
- Parent spot-check `cd hive-daemon && go test ./internal/project` passed.
- Independent committed-range verification passed for `2c1232b8841e468623e434f8119f2af6b6257789..2e361374773d0e4e7a0f81776cdb42363f3d7c58`: clean tree, source inspection, focused package tests, and diff checks; no blocking or medium findings. The verifier noted no fresh race/stress/full-ecosystem run, schema-upgrade behavior was mainly source-inspected, and CodeGraph timed out.
- Native RDD produced no lineage, verdict, or receipt because the consent-binding defect blocked START. Upstream occurrence: `https://github.com/Gentleman-Programming/gentle-ai/issues/4754#issuecomment-5758905286`.
- No integrated test suite has been run yet.

## Next Step

Begin WU-04 universal ingress resolution and complete local-state migration from the verified WU-03 boundary.
