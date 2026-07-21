# Design: Unified project derivation and self-healing registration (#452)

## Technical Approach

One derivation source of truth in a new committed module `hivederive`, consumed by both binaries via relative `replace`. Typed errors replace the silent `"default"` sentinel; callers map them to fail-safe or honest-failure paths. WSL/Windows normalization runs before `os.Stat`. Self-healing writes extend the provenance-gated escape to `mem_session_summary`. A distinct `markerSessionStart` restores the FIRST ACTION nudge. Delivered as 4 chained PRs onto tracker `fix/452-project-autodetection`.

## Architecture Decisions

### Decision: Approach A committed (shared module via relative `replace`) — A/C gate RESOLVED

**Verdict: A. No blocker found.** Static evidence:

| Evidence | Finding | Impact on relative `replace` |
|---|---|---|
| `.goreleaser.yaml` builds | `dir: jarvis-cli` / `dir: hive-daemon`, per-binary | Relative `replace ../hivederive` resolves to the sibling dir present in checkout |
| `before.hooks` | `go mod tidy -C jarvis-cli`, `-C hive-daemon` | tidy needs the replaced module on disk — it is (full monorepo checkout) |
| `release.yml` / `beta.yml` | `actions/checkout` `fetch-depth: 0` | Entire repo incl. `hivederive/` present during check/tidy/build |
| `ci.yml` | per-module `working-directory`, full checkout, Windows job for jarvis-cli | Relative replace works on Windows (Go normalizes `/` in go.mod) |
| go.sum | local `replace` targets are NOT checksummed | Removes go.sum/network/publish friction |
| `goreleaser check` | validates config only | Indifferent to module graph |

**Choice**: New module `github.com/Thrasno/jarvis-ai-devs/hivederive` at repo root; `require`+`replace ../hivederive` in `jarvis-cli/go.mod` and `hive-daemon/go.mod`. `hive-api` untouched.
**Alternatives**: C (parity copies + cross-module parity test) — kept only as documented fallback if a real release build later disproves this static verdict; B/D rejected (uncommitted go.work / publish friction).
**Rationale**: Only committed+reproducible option that ends parity drift; every release/CI path checks out the full monorepo so the sibling path always resolves.

### Decision: Normalization gating — GOOS + `/proc/version` WSL marker, translate-on-stat-failure as safety net

**Choice**: `NormalizePath` rewrites `C:\x\y`→`/mnt/c/x/y`, UNC `\\wsl$\...`/`\\wsl.localhost\...`→translated, backslashes→slashes ONLY when `runtime.GOOS=="linux"` AND a WSL marker is present (`/proc/version` contains `microsoft`/`WSL`, read once, cached). On native Windows and native Linux, pass through. As a secondary net, `Derive` retries with a normalized path only if the raw `os.Stat` fails.
**Alternatives**: Unconditional rewrite (rejected — misfires on native-Windows daemon per proposal risk); translate-only-on-stat-failure alone (rejected as sole strategy — slower, and a valid `/mnt/c` that coincidentally stats could mask a Windows path).
**Rationale**: Gate keeps native platforms untouched; the WSL marker is the precise signal for the actual bug (Windows path → WSL daemon). Marker detection is injectable for tests so table tests need no real mounts.

### Decision: Pure-function API, `os.Stat`/marker injected

`Derive` and `NormalizePath` are pure string transforms except one stat; the stat fn and WSL-marker fn are package vars (or params on an internal struct) overridable in tests. Table tests exercise synthetic paths deterministically.

## Interfaces / Contracts

```go
// hivederive
var (
    ErrEmptyDir         = errors.New("hivederive: empty directory")
    ErrPathUnresolvable = errors.New("hivederive: path cannot be resolved")
    ErrDefaultOnly      = errors.New("hivederive: no real name derivable")
)
func Derive(dir string) (string, error)        // normalize → stat → git remote → basename → typed err (never "default")
func NormalizePath(dir string) (string, error) // WSL/Windows translation, gated
// unexported: extractRepoName(url string) string (moved here; sanitized [A-Za-z0-9._-])
```

Adapters preserve existing public signatures to keep blast radius small:
- CLI `project.DetectProject(dir) string` → wraps `Derive`; on error returns `""` (hooks treat empty as "no pin / skip register"), never a leaked ambient repo name. Stat guard added; no ambient-cwd git.
- Daemon `project.DeriveFromDirectory(dir) string` → wraps `Derive`; on error returns `"default"` internally so existing `!= "default"` guards keep working; `ResolveEffectiveProject` provenance semantics unchanged.

## Call-site Integration

| Site | Change | Failure/degradation |
|---|---|---|
| `events.go` RunSessionStart:28 | `canonical=DetectProject`→`""` on err | empty pin (`BuildHiveProtocolText("")`), still register attempt skipped when empty |
| `events.go` RunPromptSubmit:69 | same | prompts attach to caller/empty; hook still emits valid JSON |
| `events.go` RunSubagentStop:161 | same | fail-safe |
| `events.go:36` `_ = PostSessionStart` | capture err; `logger.Log`? (CLI side uses stderr via `fmt`/existing CLI logger) log failure with reason; still non-fatal | hook never aborts |
| `tools.go` memSaveHandler:285 | unchanged (reference impl) | — |
| `tools.go` memSessionSummaryHandler:485 | ADD `directory` field; call `ResolveEffectiveProject`; mirror escape (`derived && Project!="default"`) mirroring 285-311 | on unknown+underived → return validation error (honest) |
| `server.go` handleSessionsCreate:871 | `DeriveFromDirectory` via new module; log when derive yields default/refused | keep `!=default && !=""` gate |

**Implicit registration** (no `ensureProject` exists): registration remains a side effect of the first write row landing in `memories`/`sessions`/`user_prompts` (`DB.KnownProjects` UNION). The provenance-gated escape lets a derived non-`default` name bypass `CodeProjectUnknown` so the very row that references it registers it — idempotent by construction (duplicate session create already treated idempotent at server.go:878).

## Marker Decoupling

Add `markerSessionStart = "session-start"` (protocol.go:24 block). `RunSessionStart` calls a new baseline-marker create using `markerSessionStart` (idempotent, timestamp-preserving) — NOT `markerFirstPrompt`. `RunPromptSubmit` keeps sole ownership of `markerFirstPrompt` via `CreateMarkerExclusive`, so `created==true` on first prompt restores `FirstPromptSystemMessage`. `memoryReminder` still reads `markerFirstPrompt` for session-age. Compaction path (`RunSessionCompact`) untouched — it references no markers.

## Logging

Use daemon `logger.Log` (stderr, `[hive] `). Events:
- CLI: `PostSessionStart` failure → `session-start registration failed: session=%q project=%q: %v`.
- Daemon `Derive` fallback/refusal → `derive: %q unresolved (%v); refusing to register "default"` at server/tools escape sites.
No structured-logs table (none exists; out of scope).

## Testing Strategy (Strict TDD, RED first)

| Layer | What | Approach |
|---|---|---|
| Unit | `Derive`/`NormalizePath` parity: git-remote, basename, empty→ErrEmptyDir, `C:\a\b`, `/mnt/c/a/b`, UNC `\\wsl$\...`, backslashes, unresolvable→ErrPathUnresolvable, sanitization | table-driven; injected stat+WSL-marker fns, `t.TempDir()` for real-stat cases |
| Unit | `extractRepoName` sanitization moved | table |
| Unit | hook JSON fail-safe: derive error → valid JSON, no abort, empty pin | events_test.go |
| Unit | marker: SessionStart writes `session-start` only; first prompt fires nudge once | marker_test.go / events_test.go |
| Integration | daemon mem_session_summary self-heal + escape; sessions create derive | `HIVE_DB_PATH` temp DBs; skip in `-short` if external |
| Cross-module | build/vet each module resolves `replace` | `go test ./... && go vet ./...` per module (CI already matrixed) |

## Threat Matrix

| Threat | Applicable | Safe behavior / RED test |
|---|---|---|
| Prompt injection via crafted git remote URL into pin/context | Yes | `extractRepoName` sanitizes to `[A-Za-z0-9._-]`; `BuildHiveProtocolText` strips `\r\n`. Test crafted URL yields sanitized name |
| Path traversal / arg injection via `directory` into `git` subprocess | Yes | `git remote get-url origin` run with `cmd.Dir=dir` (no dir in args); Abs+Stat-first guard rejects unresolvable before exec. Test `dir` with metacharacters does not reach shell |
| Over-eager path rewrite corrupting native-Windows resolution | Yes | GOOS+WSL-marker gate; RED test native-Windows path unchanged |
| Hook aborts on typed error | Yes | Adapters swallow to fail-safe; RED test asserts valid JSON + exit 0 |
| Ambient-cwd git leaking foreign repo name | Yes | Removed; `Derive("")`→ErrEmptyDir. RED test empty dir never execs git |

## Migration / Rollout

No data migration. Module topology lands in PR1; revert order is PR4→PR1 (PR1 last). If a real release build ever disproves the A verdict, drop `replace` and switch to Approach C without touching PR2–PR4 logic.

## PR sizing forecast

- **PR1 (module + derivation + hook adapters + tests)**: highest risk. Forecast ~500–750 changed lines. **Flag: near budget** — keep marker/docs/logging OUT of PR1; if the parity table tests push it over 800, split derivation-module from hook-adapter wiring into PR1a/PR1b.
- PR2 (self-healing writes), PR3 (marker decoupling), PR4 (loud failure + embed docs): each low, well under 400.

## Open Questions

- [ ] None blocking. A/C gate resolved to A on static evidence; the only residual is the (low) chance a live release build reveals a replace edge case — mitigated by the documented Approach C fallback.
