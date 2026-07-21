# Tasks: Unified project derivation and self-healing registration (#452)

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | PR1 ~500-750, PR2 ~150-250, PR3 ~100-180, PR4 ~120-200 |
| 400-line budget risk | High (PR1 only) |
| Chained PRs recommended | Yes |
| Suggested split | PR1 → PR2 → PR3 → PR4 |
| Delivery strategy | force-chained |
| Chain strategy | feature-branch-chain |

Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: feature-branch-chain
400-line budget risk: High

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|------|------|-----------|----------------------|-----------------|-------------------|
| 1 | `hivederive` module: Derive/NormalizePath, typed errors, injectable stat/WSL-marker | PR 1 | `go test ./hivederive/...` | N/A — pure lib, covered by table tests | Revert `hivederive/` + go.mod replace lines |
| 2 | Wire CLI/daemon adapters to `hivederive` | PR 1 | `go test ./jarvis-cli/... ./hive-daemon/...` | Manual: run `jarvis` in a WSL-mounted repo, confirm project name resolves | Revert detector.go/derive.go adapter diffs only |
| 3 | Self-healing `mem_session_summary` + registration escape | PR 2 | `go test ./hive-daemon/internal/mcp/...` | Manual: call `mem_session_summary` with `directory` after `project_unknown` | Revert tools.go directory param + escape block |
| 4 | Marker decoupling (`markerSessionStart`) | PR 3 | `go test ./jarvis-cli/internal/hook/...` | Manual: fresh session → first prompt → confirm nudge fires once | Revert protocol.go constant + events.go marker calls |
| 5 | Loud failure + doc fixes | PR 4 | `go test ./jarvis-cli/... ./hive-daemon/...` | Manual: force PostSessionStart failure, check stderr log line | Revert events.go error handling + embed docs |

Branch naming: tracker `fix/452-project-autodetection` (draft, no-merge). PR1 branch `fix/452-hivederive-module` bases on tracker. PR2 `fix/452-self-heal-registration` bases on PR1 branch. PR3 `fix/452-marker-decoupling` bases on PR2 branch. PR4 `fix/452-loud-failure-docs` bases on PR3 branch. Only tracker merges to `master`.

## Phase 1: PR1 — `hivederive` shared module (base: `fix/452-project-autodetection`)

- [x] 1.1 RED: `hivederive/derive_test.go` table tests — git-remote name, basename fallback, empty dir → `ErrEmptyDir`, unresolvable path → `ErrPathUnresolvable`, sanitized `extractRepoName` (spec: Single Derivation Source of Truth, No Ambient-CWD Derivation, Unresolvable Path Typed Error).
- [x] 1.2 RED: `hivederive/normalize_test.go` table tests — `C:\a\b`, `/mnt/c/a/b`, UNC `\\wsl$\...`, backslashes, injectable stat + WSL-marker fns, native-Windows pass-through (spec: Cross-Platform Path Normalization, Normalization Gating by Runtime).
- [x] 1.3 GREEN: create `hivederive/go.mod` (`github.com/Thrasno/jarvis-ai-devs/hivederive`), implement `Derive`, `NormalizePath`, typed errors (`ErrEmptyDir`, `ErrPathUnresolvable`, `ErrNoDerivableName`), moved `extractRepoName`.
- [x] 1.4 Decision checkpoint: at checkpoint the module-only diff was 479 lines (forecast ~500-750), under 800 → continued. Final PR1 authored diff landed at 822 (565 add / 257 del, go.sum excluded), 22 over budget due to duplicate-code deletions. Committed as PR1a (module, ~479) + PR1b (wiring, ~343) boundary; flagged to orchestrator as a risk.
- [x] 1.5 Wire `jarvis-cli/go.mod` — add `require`+`replace ../hivederive`; update `jarvis-cli/internal/project/detector.go` `DetectProject` to call `hivederive.Derive`, return `""` on error (no ambient cwd, stat guard).
- [x] 1.6 Wire `hive-daemon/go.mod` — add `require`+`replace ../hivederive`; update `hive-daemon/internal/project/derive.go` `DeriveFromDirectory` to call `hivederive.Derive`, map typed errors to internal `"default"` sentinel (preserve existing `!=default` guards).
- [x] 1.7 RED: hook fail-safe test in `jarvis-cli/internal/hook/events_test.go` — derivation error still emits valid JSON, no crash/non-zero exit (spec: Fail-Safe Hook Degradation).
- [x] 1.8 GREEN: `events.go` call sites unchanged — the fail-safe lives in the `DetectProject` adapter (returns `""` on error), so `RunSessionStart`/`RunPromptSubmit`/`RunSubagentStop` already degrade safely without a signature change. `PostSessionStart` error unswallowing is deferred to PR4 per design.
- [x] 1.9 Verify: `go test ./...` and `go vet ./...` GREEN in `jarvis-cli/`, `hive-daemon/`, and `hivederive/`.

## Phase 2: PR2 — self-healing writes (base: PR1 branch)

- [x] 2.1 RED: `hive-daemon/internal/mcp/session_summary_selfheal_test.go` — `mem_session_summary` with `directory` after `project_unknown` self-heals and proceeds; without `directory` still fails `project_unknown` (spec: Directory Parameter, Self-Heal on project_unknown).
- [x] 2.2 RED: idempotent registration test — repeated calls, same directory, no duplicate/error (spec: Idempotent Registration).
- [x] 2.3 RED: conflict test — derived name overrides stale caller-supplied project name (spec: Filesystem-Derived Name Wins on Conflict).
- [x] 2.4 RED: refusal test — derivation resolving to `"default"` is refused, never registered; underivable path (typed error) does not self-heal. Documented decision: a directory whose basename is literally `"default"` is also refused to preserve the reserved pooling-sentinel guard and strict `mem_save` parity (spec: Never Register "default").
- [x] 2.5 RED: parity test — existing `mem_save` escape (`derived && project!="default"`) unchanged (spec: mem_save Escape Behavior Unchanged).
- [x] 2.6 GREEN: added `directory` field to `memSessionSummaryHandler` (tools.go) + schema; derive via `hivederive.Derive` directly (typed errors, derived-name-wins), mirror the memSaveHandler provenance-gated escape. Deviation from design's "call ResolveEffectiveProject": that helper does not derive when a project name is supplied, so it cannot satisfy the derived-name-wins-on-conflict requirement; direct `Derive` is used and the escape guard is preserved verbatim.
- [x] 2.7 Verify: `go test ./...` and `go vet ./...` GREEN in `hive-daemon/`; gofmt clean.

## Phase 3: PR3 — marker decoupling (base: PR2 branch)

- [ ] 3.1 RED: `jarvis-cli/internal/hook/events_test.go` — `RunSessionStart` writes `markerSessionStart` only, not `markerFirstPrompt` (spec: Distinct SessionStart Marker).
- [ ] 3.2 RED: `RunPromptSubmit` exclusive-creates `markerFirstPrompt`, nudge fires once per real session, not on subsequent prompts (spec: First-Prompt Marker Owned Exclusively, FIRST ACTION Nudge Fires Once).
- [ ] 3.3 RED: compaction-path regression test — first-prompt marker pre-exists after compaction, nudge not re-triggered (spec: Compaction Path Unaffected).
- [ ] 3.4 GREEN: add `markerSessionStart` constant to `jarvis-cli/internal/hook/protocol.go`; update `RunSessionStart` in `events.go` to write it (idempotent, timestamp-preserving); confirm `RunPromptSubmit` sole ownership of `markerFirstPrompt`.
- [ ] 3.5 Verify: `go test ./jarvis-cli/internal/hook/...` and `go vet ./jarvis-cli/...`.

## Phase 4: PR4 — loud failure + honest docs (base: PR3 branch)

- [ ] 4.1 RED: `events_test.go` — `PostSessionStart` error is captured and logged with reason via stderr logger, hook still degrades fail-safe (spec: Registration Failures Are Logged, Never Swallowed).
- [ ] 4.2 RED: daemon test — derive fallback/refusal path logs `derive: %q unresolved (%v); refusing to register "default"`, never falls back to registering `"default"` (spec: No Fallback to "default" Registration).
- [ ] 4.3 GREEN: unswallow `PostSessionStart` error in `events.go:36`, add stderr log line with session/project/reason; add matching log line at `hive-daemon/internal/mcp/server.go` `handleSessionsCreate` (~871).
- [ ] 4.4 Update `embed/hive-protocol.md` and `embed/skills/hive/SKILL.md` — remove claim that `mem_context` registers projects; describe SessionStart hook / self-healing writes as actual registration mechanism (spec: Documentation Reflects Actual Registration Behavior).
- [ ] 4.5 RED/regression: add a doc-claim check (grep-based test or doc assertion) that fails if the old incorrect claim reappears.
- [ ] 4.6 Verify: `go test ./...` and `go vet ./...` across `jarvis-cli/`, `hive-daemon/`.
