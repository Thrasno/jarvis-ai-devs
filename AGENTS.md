# AGENTS.md

This file is the operational contract for AI agents working in this repository.

The goal is not to repeat the README. The goal is to make agents work safely, consistently, and with the right architectural context.

## Project Purpose

`jarvis-dev` is an AI-assisted development ecosystem for development teams.

The MVP is the complete ecosystem, not a single service. It includes:

- SDD prompt injection and workflow support.
- Local memory system: Hive.
- Shared team memory backend: Hive API.
- CLI installer and management tool.
- Hive ↔ Hive API synchronization system.
- Diagnostics, doctor checks, and reconfiguration flows.
- Todoist-backed backlog workflow.

Agents must treat this repository as an ecosystem. A change in one component can affect installation, local memory, shared memory, prompt injection, sync, or team workflow.

## Tech Stack

- Language: Go.
- Package manager: Go modules.
- Test runner: `go test ./...`.
- Coverage: `go test -cover ./...`.
- Static checks: `go vet ./...`.
- Formatter: `gofmt`.

## Core Components

### SDD Prompt Injection

Provides structured prompts, skills, and workflow guidance for Spec-Driven Development.

Agents must preserve the distinction between:

- product prompt injection behavior, and
- development-time SDD artifacts used while building this repository.

### Hive

Hive is the local memory system used by the ecosystem.

Expected properties:

- local-first behavior,
- offline resilience,
- durable local storage,
- clear separation between local and shared concerns,
- safe integration with the sync system.

### Hive API

Hive API is the shared team memory backend.

It centralizes project/team knowledge and participates in the Hive ↔ Hive API sync flow.

The intended product behavior is automatic synchronization between local Hive instances and the shared Hive API. Do not confuse this product behavior with assistant memory, Engram storage, or SDD artifact persistence.

### CLI

The CLI installs and manages the ecosystem.

Expected capabilities include:

- installation,
- doctor checks,
- reconfiguration,
- token/config validation,
- local environment checks,
- sync status and diagnostics.

## Architecture Rules

- Preserve boundaries between prompt injection, local memory, shared API, sync, and CLI responsibilities.
- Do not collapse product concepts into development tooling concepts.
- Do not introduce broad abstractions without a concrete architectural reason.
- Prefer boring, explicit Go code over clever indirection.
- Keep user-facing CLI behavior predictable and easy to diagnose.
- Treat install, doctor, and reconfiguration flows as first-class product surfaces.

Before changing architecture, check how the change affects:

- SDD prompt injection,
- local Hive behavior,
- Hive API behavior,
- Hive ↔ Hive API sync,
- CLI installation/configuration,
- diagnostics/doctor flows,
- Todoist backlog integration.

## Generated Artifacts vs Sources

User-machine agent configuration is generated, not hand-edited.

Generated artifacts include:

- `~/.claude/CLAUDE.md`,
- `~/.claude/settings.json`,
- Claude output-style files,
- `~/.config/opencode/opencode.json`,
- injected Hive protocol blocks.

These artifacts are produced by `jarvis init`, `jarvis persona`, and related CLI flows.

Sources of truth are:

- templates and embedded assets in `jarvis-cli/embed/`,
- render/merge adapters in `jarvis-cli/internal/agent/`,
- persona handling in `jarvis-cli/internal/persona/`,
- prompt/runtime contracts in `jarvis-cli/internal/sddruntime/`.

Rules:

- Never fix behavior by editing generated files directly.
- Change the source of truth, then regenerate and verify the rendered output.
- Config emitters must deep-merge; never clobber user-owned keys.
- New content must land in the correct layer and pass the layer/role validator.
- Persona voice must never leak into generated artifacts.

## Development Rules

- Favor correctness, maintainability, and clear concepts over speed.
- Follow standard Go conventions.
- Format Go code with `gofmt`.
- Keep packages cohesive and responsibilities obvious.
- Avoid adding dependencies casually. Justify new dependencies by value, maintenance cost, and operational risk.
- Do not build unless explicitly requested.

## Testing Rules

Strict TDD is expected for SDD implementation work.

When implementing via SDD:

1. Write or update the failing test first.
2. Implement the minimum production code needed to pass.
3. Refactor only after tests are green.
4. Run the relevant test command.

Default verification commands:

```bash
go test ./...
go vet ./...
```

Use coverage when the task requires confidence around behavior:

```bash
go test -cover ./...
```

## SDD Workflow

Use Spec-Driven Development for substantial changes.

Typical phases:

- explore,
- proposal,
- spec,
- design,
- tasks,
- apply,
- verify,
- archive.

Use Engram as the default artifact store unless the user explicitly requests file-based OpenSpec artifacts.

Do not skip planning for architectural, cross-component, or high-risk changes.

## Backlog Management

The backlog is managed through Todoist.

Todoist API token location:

```text
~/.config/tokens
```

Agents may use this token only when explicitly required for Todoist API operations.

Never print, commit, copy, or expose token contents.

## Secrets and Local Configuration

Secrets must stay outside the repository.

Allowed local secret/config locations include:

```text
~/.config/tokens
```

Never commit:

- API tokens,
- `.env` files,
- generated credentials,
- local machine-specific configuration,
- private keys.

## Git and Review Rules

- Use conventional commits.
- Never add AI attribution or `Co-Authored-By` lines.
- Keep changes reviewable.
- If a change is likely to exceed roughly 400 changed lines, split it into reviewable work units or ask for an explicit size exception.
- Keep tests and implementation changes together when they belong to the same behavior.
- GitHub comments, including issue comments, PR comments, and review comments, must be bilingual: English first, followed by normative Spanish from Spain.

## Release Workflow Rules

When the user asks to "saca release beta", "generar beta", or equivalent:

- Use the fixed public beta channel `beta`.
- Do not create a new public beta version unless explicitly requested.
- Use the internal GoReleaser-compatible prerelease tag documented in `docs/release-runbook.md` only as build input.
- Regenerate the GitHub release assets through the Beta Release workflow.
- Verify the `beta` release is recreated as a prerelease with fresh `jarvis` and `hive-daemon` assets.
- Verify tester install commands still work with `JARVIS_INSTALL_VERSION=beta`.
- Follow `docs/release-runbook.md` for exact commands and verification.

When the user asks to "saca release PROD", "generar release de produccion", or equivalent:

- Do not reuse the public `beta` channel or the internal beta prerelease tag.
- Create a new immutable production release tag.
- Choose the next semantic version based on the implemented changes:
  - patch for fixes or safe improvements,
  - minor for meaningful features or product additions,
  - major for breaking changes, migration-heavy changes, or major architecture shifts.
- Inspect recent commits since the last production release before choosing the version.
- Follow `docs/release-runbook.md` for exact commands and verification.

## Forbidden Actions

Agents must not:

- run builds unless explicitly requested,
- expose secrets,
- commit credentials or local config,
- rewrite architecture without an explicit decision,
- treat Hive as the whole MVP,
- confuse Hive sync with assistant memory or SDD artifact storage,
- skip tests after behavior changes,
- add AI attribution to commits.

## Product Direction

This product is built for real team adoption, not experimentation for its own sake.

Prioritize:

- reliability,
- predictable automation,
- clear onboarding,
- low-friction CLI UX,
- useful diagnostics,
- shared knowledge,
- local-first resilience.
