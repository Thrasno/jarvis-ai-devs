# Issue 800: secret deny rules block ordinary source files

Objective: stop the generated secret-protection deny rules from blocking ordinary source files named `*token*`, `*secret*`, `*credential*`, while keeping real secret locations protected for read and edit in Claude Code and OpenCode.
Branch: `fix/800-secret-deny-rules`. Delivery: one PR for issue #800; push and PR creation are the user's decision. Build prohibited; checks: `cd jarvis-cli && go test ./...`, `go vet ./...`, `gofmt`.

## Specs

S1 (issue #800, Expected behaviour): "Agents can read and write ordinary source files whose names contain `token`, `secret` or `credential`. Actual secret-bearing locations (`.env*`, `.ssh/`, private keys, credential stores, `secrets/` directories, etc.) remain protected."

S2-S6 quote the maintainer decisions comment on issue #800 verbatim:

## Maintainer decisions

**Approved scope (single PR):**

1. **A2.** Remove the substring wildcards `*token*`, `*secret*`, `*credential*` (and their `**/` variants) from Claude Code and OpenCode. Keep the location rules (`.env*`, `.ssh/`, `secrets/`, `secret/`, `tokens/`, `token/`, `credentials/`, `credential/`, `*.pem`, `*.key`, `id_rsa*`, `id_ed25519*`) and add explicit secret-shaped names: `.npmrc`, `.netrc`, `.pypirc`, `credentials.json`, `.aws/credentials`, `.config/gh/hosts.yml`, `*.p12`, `*.pfx`, `*.token`. A generic `config.json` is **not** added: it is a common application config name and would reintroduce this bug.
2. **B1.** Also deny writes to the same locations: `Edit(...)` in Claude Code (covers Edit/Write/MultiEdit) and `edit` in OpenCode. Deletion is **not** blocked: neither agent exposes a delete permission, and `Bash(rm ...)` patterns are trivially bypassed.
3. Losing coverage of loose, free-named files (e.g. `my_api_token_backup.txt`) is accepted.
4. No follow-up issues for C1/C2 or D1.
5. `.env.example`: OpenCode gets an explicit allow for `.env.example` and `.env.sample`. Claude Code keeps `.env.*` denied (`deny` beats `allow`); document the asymmetry.

**Mandatory requirements:**

- Migrate existing installs: deep-merge does not remove retired literals. Add explicit cleanup following the `removeObsoleteClaudeForcePushDenies` precedent, plus the OpenCode equivalent.
- Keep retired literals in `jarvis-cli/internal/sddruntime/reset_inventory.go`.
- Claude Code / OpenCode parity.
- Document that these rules are a convenience guardrail, not a security boundary: shell access and deletion are not covered.
- RED tests first: `Tokenizer.php` / `AuthTokenService.ts` not denied for read or edit; `.env`, `id_rsa`, `secrets/x` and the new names denied for read and edit; migration of a settings file carrying the retired wildcards.


Spec mapping: S2 = decision 1 (A2 list), S3 = decision 2 (B1 write denial), S4 = decision 5 (`.env.example` asymmetry), S5 = mandatory migration + reset inventory + parity, S6 = docs + RED tests.

Sources of truth named by the issue: `jarvis-cli/internal/agent/claude.go` (`MergeGeneratedConfig`), `jarvis-cli/embed/templates/opencode.json.tmpl`, `jarvis-cli/internal/sddruntime/reset_inventory.go`; tests `internal/agent/claude_test.go`, `opencode_test.go`, `integration_test.go`, `coverage_test.go`. Migration precedent: `removeObsoleteClaudeForcePushDenies`.

## Tasks

- [x] T1 -> S1-S6: RED tests, then A2 list + B1 edit denial for Claude Code and OpenCode, migration of retired literals for both agents, reset inventory, docs. Route: background gentle-ai-worker in this worktree. Check: worker RED (6 failing tests) then GREEN; `cd jarvis-cli && gofmt -l . && go vet ./... && go test ./...` passed. Commit: `508c334a` (single behavior, ~1000 lines mostly tests; slice exceeds 400 by design, Claude/OpenCode parity must land together).
- [ ] T2 -> S1-S6: independent verification (high risk: security, user config migration) and native review. Route: gentle-ai-verify, then RDD. Verify: S1-S6 PASS, no blockers (OpenCode 1.18.29 and Claude Code 2.1.296 semantics checked from local binaries; differential master vs branch clean). Open maintainer decisions: Claude home-directory paths (`~/`) not covered; Claude case-insensitive directory rules (`token/`, `secret/`, `credential/`) also deny PascalCase namespace folders such as `src/Security/Token/`. Native review: pending.

## Log

L1 (user, 2026-10-20): "#800 - Ok." approving the plan: A2 + B1 + migration as decided by the maintainer, one PR.
L2 (user, 2026-10-20): "Vamos con ellas" launching wave 1 (#799 inline, #800 worker, hive-api sync check).
