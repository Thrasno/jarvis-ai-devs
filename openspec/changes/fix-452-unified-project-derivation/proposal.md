# Proposal: Unified project derivation and self-healing registration (#452)

## Intent

No-git projects (and Windows/WSL path mismatches) silently fail to register with Hive: writes are rejected with `project_unknown` or degrade to a bogus `"default"` project. Two duplicated derivation implementations drift apart, `mem_session_summary` has no self-heal path, a marker collision suppresses the FIRST ACTION nudge, registration failures are swallowed, and embed docs falsely claim `mem_context` registers projects. Fix the root cause: one derivation source of truth, honest failure signals, and self-healing writes.

## Scope

### In Scope
- **PR1 — Unified derivation** (shared logic; typed errors; WSL/Windows normalization before `os.Stat`; no ambient-cwd git; hooks degrade fail-safe).
- **PR2 — Self-healing writes** (`directory` param on `mem_session_summary`; provenance-gated escape mirroring `mem_save`; idempotent registration; filesystem-derived name wins; never register `"default"`).
- **PR3 — Marker decoupling** (`markerSessionStart` constant; first-prompt owned only by `RunPromptSubmit`; restore FIRST ACTION nudge).
- **PR4 — Loud failure + honest docs** (stop swallowing `PostSessionStart` error; log registration failures via existing stderr logger; fix false `mem_context` claims in `embed/` sources).

### Out of Scope
- Adding a structured logs table (none exists; stderr logger only).
- Unifying `detectProjectName` in `cmd_sdd.go` (separate `rev-parse` code path).
- Changing `ResolveEffectiveProject` provenance semantics or the Hive sync protocol.
- Editing generated user config files (change `embed/` sources only).

## Capabilities

### New Capabilities
- `project-derivation`: shared, typed, path-normalized project-name derivation consumed by CLI and daemon.
- `session-registration-self-heal`: `mem_session_summary` self-registers via directory-derived provenance escape.
- `hook-marker-lifecycle`: distinct SessionStart vs first-prompt markers restoring the nudge.

### Modified Capabilities
None.

## Approach

**Decision: Approach A (shared Go module via relative `replace`).** Create `github.com/Thrasno/jarvis-ai-devs/hivederive` with `Derive(dir) (string, error)` returning typed errors (`ErrEmptyDir`, `ErrPathUnresolvable`, `ErrDefaultOnly`) and `NormalizePath(dir)` (translate `C:\...`, `/mnt/c/...`, UNC `\\wsl$\...`, backslashes) applied **before** `os.Stat`. Consume it via relative `replace` in `jarvis-cli/go.mod` and `hive-daemon/go.mod`. It is the only option that ends parity drift while staying committed and reproducible.

**Fallback trigger → Approach C** (parity copies + cross-module parity test): adopt only if GoReleaser/CI verification proves the `replace` directive breaks per-binary release builds. Validate `docs/release-runbook.md` + goreleaser configs in PR1 before committing to A.

Delivery: 4 chained PRs onto tracker `fix/452-project-autodetection` (child PRs target the tracker; only the tracker merges to master). Strict TDD; conventional commits; ≤800 changed lines/PR.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `hivederive/` (new module) | New | Shared `Derive` + `NormalizePath`, typed errors |
| `jarvis-cli/internal/project/detector.go` | Modified | Drop ambient-cwd git; stat guard; typed error |
| `hive-daemon/internal/project/derive.go` | Modified | Normalize before stat; typed error, no `"default"` |
| `hive-daemon/internal/mcp/tools.go` | Modified | `directory` param + escape on session summary |
| `jarvis-cli/internal/hook/events.go` | Modified | 3 call sites; unswallow error; marker decouple |
| `jarvis-cli/internal/hook/marker.go`,`protocol.go` | Modified | `markerSessionStart` constant |
| `hive-daemon/internal/httpapi/server.go` | Modified | Derive call + log registration failures |
| `jarvis-cli/embed/hive-protocol.md`,`embed/skills/hive/SKILL.md` | Modified | Remove false `mem_context` claim |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| `replace` breaks GoReleaser per-binary builds | Med | Verify release configs in PR1; fallback to Approach C |
| Typed error propagates into hook, aborting it | Med | Hooks map errors to safe degraded path, always emit valid JSON |
| Over-eager `C:\`→`/mnt/c/` rewrite on native Windows daemon | Med | Gate on `GOOS`/`/proc/version` WSL marker or translate only on stat failure |
| Cross-OS normalization hard to test | Med | Deterministic table tests with synthetic paths, no real mounts |
| PR1 exceeds review budget | Low | Isolate module surgery; keep marker/docs/logging slices small |

## Rollback Plan

Each PR is independently revertible. Revert PR1 last (module topology). If the shared module destabilizes builds, drop the `replace` and switch to Approach C parity copies without touching PR2–PR4. The tracker branch stays draft/no-merge until all four child PRs are green and integrated.

## Dependencies

- Reuse `projectregistry/root.go` ordering (Abs+Stat-first, 10s timeout) from PR #451.
- No new third-party dependencies.

## Success Criteria

- [ ] No-git and Windows/WSL directories register without `project_unknown` or `"default"`.
- [ ] Single derivation source of truth; no parity-anchor drift.
- [ ] `mem_session_summary` self-heals via `directory`.
- [ ] FIRST ACTION nudge fires exactly once per session.
- [ ] Registration failures are logged, never silently swallowed.
- [ ] Embed docs no longer claim `mem_context` registers projects.
- [ ] `go test ./...` and `go vet ./...` green across all three modules.
