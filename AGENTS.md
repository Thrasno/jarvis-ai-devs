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
