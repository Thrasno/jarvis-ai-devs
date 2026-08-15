# Proposal: `jarvis sync` replays the last installation's desired state

## Intent

`jarvis sync` is a legacy no-op pointing at `mem_sync`. After a binary upgrade there is no non-interactive way to realign machine-scoped artifacts (instructions, MCPs, skills, persona, models, statusline) with the new embedded assets; the user must re-run the wizard and re-answer settled decisions. Persisted config is almost a complete replay manifest — the one gap is statusline consent (`tui/agent_setup.go:85`). This change redefines `jarvis sync` as the deterministic machine-scoped replay of that manifest against the installed version's assets.

## Scope

### In Scope

- `~/.jarvis/state.yaml`: a desired-state store with its own schema version, plus tri-state statusline consent (decided + enabled).
- One-way migration of replay fields **out of** `config.yaml` (advancing it to `schema_version: 3`). Fields move; they are never copied.
- A read-only planner: fail-closed load/validate, render targets from embedded assets, classify ownership by identity.
- Machine-scoped replay of instructions, Jarvis-managed MCPs, skills, persona, models, statusline, with component order as a tested contract.
- Lifecycle safety: snapshot/diff-measured idempotency, backup from sync's own target list, apply, recovery, post-apply verification, bookkeeping written under lock only when something changed.
- Compatibility notes and user documentation.

### Out of Scope

- Any project-scoped work; `jarvis sync` is machine-scoped only.
- Flags of any kind, `--dry-run` included.
- Provenance markers on skills or the statusline; frontmatter `scope:` or name prefixes as ownership signals.
- Filesystem redetection of installed agents (a manifest with no agents blocks).
- Writing `~/.jarvis/sync.json`; it is read-only for sync. Plaintext password storage stays parked.
- Announcing newly available optional skills.
- The Hive memory domain, and `lifecycle.Engine`.

## Capabilities

### New Capabilities

- `desired-state-manifest`: `state.yaml` schema, statusline tri-state, one-way migration and `config.yaml` v3.
- `sync-replay-planning`: fail-closed load, render targets, identity-based ownership, skill lifecycle rules.
- `sync-replay-application`: component order contract and machine-scoped artifact replay.
- `sync-lifecycle-safety`: snapshot/diff, backup, recovery, verification, bookkeeping.

### Modified Capabilities

- None. No existing spec in `openspec/specs/` covers install, sync, or lifecycle.

## Approach

Separate the **replay manifest** from **user configuration**, then drive a plan-then-apply pipeline from it.

| Stage | Decision |
|---|---|
| Store | New `~/.jarvis/state.yaml`, own schema version, disjoint from `config.yaml`; disjoint fields cannot disagree, so no tie-breaking rule exists |
| Migration | Runs before any early return; notice only after a durable write |
| Ownership | Identity against two lists (embedded catalog, manifest `skills`). The manifest list is never filtered on write — it is the only proof allowing deletion of a skill a later version dropped |
| Managed instruction files | A `CLAUDE.md`/`AGENTS.md` belonging to a manifest-listed agent is a Jarvis target and its whole path is owned. With no Jarvis sentinels present, sync renders fresh and discards the previous content, matching `WriteInstructions` today (`agent/claude.go:350-356`, `agent/opencode.go:445-452`). Product decision: sync matches installer behavior rather than introducing a second ownership rule for the same file. The pre-apply backup is the recovery path |
| Optionality | Membership in `interactiveSkillIDs` (`tui/skills_selection.go:28-33`), never frontmatter |
| MCPs | Derived from embedded assets, nothing persisted, replaced unconditionally |
| Ordering | Persona applied **last**, after every content-injecting component; locked by a test. Jarvis inverts the `gentle-ai` order because `WriteInstructions` is the sole writer of `CLAUDE.md`/`AGENTS.md` (`agent/claude.go:327`, `agent/opencode.go:424`) and rebuilds or patches the whole file, re-injecting the Hive protocol and orchestrator import itself (`claude.go:365-372`). Production already applies persona after the agent loop (`tui/agent_setup.go:264-277`) |
| Idempotency | Measured by snapshot/diff over sync's own path list; the same list feeds backup targets |
| Boundary | `internal/reconcile` as a package only ("is it broken?"); sync answers "is it stale?". Never `lifecycle.Engine` |

Strict TDD: every slice lands test-first, with component order and the zero-diff second run as executable contracts.

Slices (linear: 2←1, 3←2, 4 wraps 3, 5 last): store and migration; read-only planner; machine-scoped replay; lifecycle safety and measured idempotency; compatibility and docs.

## Affected Areas

| Area | Impact | Description |
|---|---|---|
| `jarvis-cli/internal/config/` | Modified | `schema_version: 3`, replay fields removed |
| `jarvis-cli/internal/state/` (new) | New | `state.yaml` load, validate, migrate |
| `jarvis-cli/cmd/jarvis/cmd_sync.go` | Modified | no-op replaced by replay command |
| `jarvis-cli/internal/agent/`, `internal/tui/skills_selection.go` | Modified | render/ownership seams reused by the planner |
| `jarvis-cli/internal/lifecycle/backup.go` | Modified | targets sourced from sync's plan |
| `docs/` | Modified | sync behavior and upgrade notes |

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| Migration loses replay fields on a partial write | Med | Migrate before early return; report only after durable write; backup precedes apply |
| Wrong component order strips just-written content | Med | Order is a tested contract; persona runs after the injectors, matching the sole-writer topology |
| Identity-based ownership deletes a user-authored skill | Med | Skills in neither list are untouchable; manifest list never filtered |
| `InstallStatusline` cannot run without its confirm callback | Med | Resolved in design; tri-state supplies the decision non-interactively |
| Replay is not actually idempotent | Med | Second-run zero-diff measured by snapshot/diff, asserted in tests |
| Blocking on an agent-less manifest strands old installs | Low | Explicit product decision; error names the recovery command |

## Rollback Plan

Per-slice revert. Slice 1 carries the only irreversible step: the `config.yaml` v2→v3 field move. Recovery is the pre-apply backup snapshot (`~/.jarvis/backups/<id>.tar.gz`, checksum-validated on restore). Slices 2–5 are additive to a command that is a no-op today, so reverting them restores current behavior without user-visible data loss.

## Dependencies

- Existing `internal/lifecycle` backup/restore machinery.
- `internal/reconcile` as a package (not `lifecycle.Engine`).
- Embedded assets in `jarvis-cli/embed/` as the sole render source.

## Success Criteria

- [ ] `jarvis sync` after an upgrade brings machine-scoped artifacts to the installed version's assets with no prompts.
- [ ] A second consecutive `jarvis sync` produces a measured zero diff and writes nothing.
- [ ] Statusline consent survives upgrade as a tri-state; "not decided" and "decided-disabled" both leave the statusline untouched.
- [ ] A skill dropped from the catalog but present in the manifest is deleted; a user-authored skill in neither list is untouched.
- [ ] Non-wizard content in shared files is preserved byte-for-byte.
- [ ] Invalid or missing state, and a manifest with no agents, block with an actionable message and write nothing.
