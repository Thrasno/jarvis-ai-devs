# Skill Parity Run Report Template

Use this template for each Gentle AI skill parity maintenance run. Store completed reports in Hive/current Jarvis artifact store or link them from the maintenance PR unless a maintainer explicitly chooses a committed report.

## Run metadata

| Field | Value |
|-------|-------|
| Run ID |  |
| Gentle AI selected reference |  |
| Gentle AI retrieval date |  |
| Jarvis commit or branch |  |
| Last adopted Gentle AI version |  |
| Maintainer |  |
| Report location |  |
| Reference availability | Confirmed / Unavailable / Incomplete |

## Source-of-truth guardrail

- [ ] This run compared Jarvis source templates/assets, not generated user-machine configs.
- [ ] No direct edits were made to `~/.claude/**`, `~/.config/opencode/**`, generated registries, installed `.jarvis/skills/**` copies, or team environments.
- [ ] Any required generated-output check was performed by regenerating from source.

## Inventory summary

| Jarvis path | Upstream path | Scope status | Notes |
|-------------|---------------|--------------|-------|
| `jarvis-cli/embed/skills/<skill>/SKILL.md` | `internal/assets/skills/<skill>/SKILL.md` | In scope / Adapted equivalent / Out of parity / Retired / Investigate |  |

Required scope reminders:

- `go-testing`, `skill-creator`, `skill-improver`, and `skill-registry` are in scope even if source stamps are absent.
- `hive` is Jarvis' adapted equivalent to Gentle AI Engram.
- `qa-checklist` is Jarvis-local and out of Gentle AI parity unless a future upstream equivalent appears.
- `sdd-workflow` is retired/removed because orchestration authority lives in the orchestrator, shared SDD contracts, and phase skills.

## Difference log

| Jarvis path | Difference summary | Decision | Rationale | Owner | Follow-up |
|-------------|--------------------|----------|-----------|-------|-----------|
|  |  | `apply` / `adapt` / `ignore` / `investigate` |  |  |  |

Decision definitions:

- `apply`: copy the upstream change into Jarvis source without semantic adaptation.
- `adapt`: accept upstream intent but translate it for Jarvis, Hive, `.jarvis`, packaging, or path-injected loading.
- `ignore`: leave source unchanged because the difference is intentionally not applicable.
- `investigate`: leave source unchanged until unclear provenance, meaning, or impact is resolved.

## Approval and implementation plan

- [ ] Maintainer approval recorded before skill content or workflow semantics changed.
- [ ] Accepted changes are grouped into reviewable chained PR slices.
- [ ] No full upstream sync was performed.
- [ ] Verification command or review method is recorded for each implemented slice.

## Greenfield note

For this change, no existing-install stale skill cleanup is required because Jarvis is still greenfield. Future non-greenfield runs must reassess stale generated or installed copies through source-driven install, reconfigure, or migration paths.

## Outcome

| Result | Value |
|--------|-------|
| Run status | Complete / Incomplete / Blocked |
| Source changes proposed | Yes / No |
| Source changes implemented | Yes / No |
| Verification completed | Yes / No |
| Remaining investigations |  |
