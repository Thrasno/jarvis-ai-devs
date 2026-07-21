# Exploration: fix-452 unified project derivation

Change: `fix-452-unified-project-derivation`
Issue: Thrasno/jarvis-ai-devs#452 — fix(hive): no-git projects fail to register — SessionStart derives "default" and saves fail with project_unknown
Engram artifact: observation 4668 (topic `sdd/fix-452-unified-project-derivation/explore`)

## Current State

- **Two duplicate derivation implementations kept in sync only by "parity anchor" comments**: `jarvis-cli/internal/project/detector.go:86` `DetectProject` (NO stat guard — runs git in the hook's ambient cwd when `dir` is empty) and `hive-daemon/internal/project/derive.go:27` `DeriveFromDirectory` (HAS an `os.Stat` guard that silently returns `"default"` when a Windows-form path cannot be stat'd from WSL — the real-world bug).
- **Registration = writing a row.** There is no explicit `ensureProject`; `DB.KnownProjects` (`hive-daemon/internal/db/project.go:72`) UNIONs memories/sessions/user_prompts. `mem_save` already has a provenance-gated `project_unknown` escape (`hive-daemon/internal/mcp/tools.go:285-311`) that excludes `"default"`.
- **`mem_session_summary` (`tools.go:485`) is the gap**: no `directory` param, no `ResolveEffectiveProject`, no escape — fails hard for the whole session.
- **`mem_context` is read-only** (`tools.go:575`) — the claim in `jarvis-cli/embed/hive-protocol.md:10-11` that it "registers the project" is false.
- **Marker bug**: SessionStart (`jarvis-cli/internal/hook/events.go:31`) and first-prompt (`events.go:78`) both use `markerFirstPrompt`, so the FIRST ACTION nudge never fires.
- Daemon logging is stderr-only via `logger.Log` (`logger.go:10`); there is no logs table.

## Module Layout (key finding)

Three **separate Go modules** (jarvis-cli, hive-daemon, hive-api), shared path prefix, **no go.work, no replace directives, no cross-module imports**, released per-binary via GoReleaser. A truly shared package requires build-topology surgery.

- **Approach A (recommended)**: new shared module consumed via relative `replace` directives — the only choice that actually ends the parity drift while staying committed and reproducible.
- **Approach C (fallback)**: parity-copies plus a cross-module parity test — low-risk fallback if GoReleaser/CI validation of the shared module fails.

The A-vs-C choice is the decision to surface in the proposal.

## Other Findings

- **Blast radius** of making derivation return `(string, error)`: `events.go` x3 (fail-safe hooks — must degrade, not abort), `server.go:871`, `tools.go` x2 (plus the new summary handler). `detectProjectName` in `cmd_sdd.go` is a separate `rev-parse` implementation.
- **#449 / PR #451** was narrow: `projectregistry/root.go` timeout 2s→10s. Reusable prior art: `ResolveRoot` (Abs+Stat+timeout+`rev-parse --show-toplevel`) and `samePath`/`homePathCandidates` GOOS-gated Windows path folding.
- **No `C:\ ↔ /mnt/c/` normalizer exists.** It must live in the shared package and run **before** `os.Stat` on the daemon side (the failing path arrives at the WSL daemon).

## Risks

- Shared-module build/CI/GoReleaser re-validation is the top risk (fallback exists).
- Hooks must stay fail-safe under typed errors.
- Path normalization needs deterministic pure-string table tests, gated to avoid misfiring on Windows-native daemons.
- 4 chained PRs onto `fix/452-project-autodetection`; the shared-module slice is the review-budget risk.

## Next Recommended

`sdd-propose`
