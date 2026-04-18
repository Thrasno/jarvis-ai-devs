# Jarvis Dev

Jarvis Dev is an open development ecosystem for AI-assisted engineering workflows. It combines a CLI, a local memory daemon, and a central API to help teams keep context, structure execution, and reduce repeated decisions.

## What is included

- **`jarvis-cli/`**: user-facing CLI and workflow entrypoint.
- **`hive-daemon/`**: local service for offline-first memory operations.
- **`hive-api/`**: central API service for shared memory and sync.

## Core capabilities

- **Shared memory model**
  - Local-first storage with optional central synchronization.
  - Timeline-style observations with `personal` and `project` scopes.
- **Structured SDD workflow**
  - Spec-driven phases (`explore`, `propose`, `spec`, `design`, `tasks`, `apply`, `verify`, `archive`).
- **Context-aware project detection**
  - Detects stack and project metadata for better defaults and skill selection.
- **Persona + skill system**
  - Configurable communication/workflow presets and reusable skill packs.

## High-level architecture

```text
jarvis (CLI)
   |
   +--> hive-daemon (local SQLite, offline-first)
   |
   +--> hive-api (central service, PostgreSQL)
```

The CLI can operate with local memory only, or with a hybrid local+central setup.

## Quickstart (development)

### 1) Start API dependencies

```bash
docker compose -f hive-api/deploy/docker-compose.yml up -d
```

### 2) Run services locally

```bash
go run ./cmd/server
```

from `hive-api/`, and:

```bash
go run ./cmd/hive-daemon
```

from `hive-daemon/`.

### 3) Run CLI

```bash
go run ./cmd/jarvis --help
```

from `jarvis-cli/`.

## Installation

### Public installer (recommended when releases exist)

Linux/macOS:

```bash
curl -sSL https://raw.githubusercontent.com/Thrasno/jarvis-ai-devs/master/scripts/install.sh | bash
```

Windows (PowerShell):

```powershell
irm https://raw.githubusercontent.com/Thrasno/jarvis-ai-devs/master/scripts/install.ps1 | iex
```

Optional overrides:

- `JARVIS_INSTALL_REPO=owner/repo` to fetch artifacts from another repository.
- `JARVIS_INSTALL_VERSION=vX.Y.Z` to force a specific release tag (skip `releases/latest`).

If no public releases are published yet, installer scripts will fail fast with explicit guidance. They also retry transient GitHub/CDN 5xx/429 failures with backoff and validate artifact content before extraction to avoid HTML/error-page unpack attempts. In that case, use the from-source path below.

### From source

Build each binary from its module:

- `jarvis-cli/cmd/jarvis`
- `hive-daemon/cmd/hive-daemon`
- `hive-api/cmd/server`

### Release artifacts

The repository includes `.goreleaser.yaml` to package multi-platform binaries for the main CLI and daemon components.

### Single installer contract

After installing binaries (from source or release scripts), the canonical entrypoint is:

```bash
jarvis
```

- First run: launches full setup wizard.
- Re-run: launches reconfiguration wizard with previous values prefilled.
- Advanced commands (`jarvis persona`, `jarvis login`, `jarvis config`, etc.) remain optional power-user paths.

## Project status

This repository is under active development. APIs, commands, and workflow details may evolve as the platform matures.
