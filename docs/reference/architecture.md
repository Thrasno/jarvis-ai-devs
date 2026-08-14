# Jarvis Dev Architecture Reference

Jarvis Dev is an ecosystem for AI-assisted development teams. The MVP is the complete workflow: CLI, local memory, shared memory, SDD support, diagnostics, installer, dashboard, and backlog integration when configured.

## Quick map

```text
Developer
   |
   v
jarvis-cli
   |--------> generated agent configuration
   |--------> SDD prompt/workflow support
   |--------> lifecycle diagnostics
   |
   v
hive-daemon  <---- local-first memory
   |
   v
hive-api     <---- shared team memory when configured
   |
   v
hive-dashboard when enabled
```

## Components

| Component | Responsibility | Boundary |
|-----------|----------------|----------|
| `jarvis-cli` | User-facing setup, reconfiguration, diagnostics, persona/skill workflows, SDD status, Hive UI entrypoints. | Does not make generated user-machine files the source of truth. |
| `hive-daemon` | Local service for offline-first memory operations and daemon HTTP access. | Local memory must remain useful without Hive API. |
| `hive-api` | Central API for shared team memory and sync. | Shared backend, not a replacement for local memory. |
| `hive-dashboard` | Static dashboard served by Hive API when enabled. | Observes/administers API data; does not manage local daemons. |
| SDD workflow support | Structured phases and artifact-driven implementation flow. | Product SDD artifacts are distinct from assistant memory/Engram. |
| Installer/release flow | Delivers ecosystem binaries and embedded assets. | Install/reconfigure is separate from memory sync. |
| Todoist backlog workflow | Backlog integration when configured. | Operational workflow, not the memory layer. |

## Key boundaries

### Product memory vs assistant memory

Hive and Hive API are Jarvis product memory. Assistant memory systems used by coding agents during repository work are not Jarvis product memory and should not be described as Hive sync.

### Generated artifacts vs source

Generated user-machine files are outputs. Sources of truth live in `jarvis-cli/embed/`, `jarvis-cli/internal/agent/`, `jarvis-cli/internal/persona/`, and `jarvis-cli/internal/sddruntime/`.

### Installation vs sync

Installing or rerunning Jarvis updates binaries and regenerates configuration. Hive ↔ Hive API sync is a separate behavior controlled by daemon configuration and credentials.

### Command boundary: `sync`, `reconcile`, and memory sync

Three commands are easy to confuse; each has one authority and one question.

| Command | Question | Authority | Scope |
|---------|----------|-----------|-------|
| `jarvis reconcile` | Is managed configuration broken? | Doctor observations of the current filesystem. | Owned drift repair. |
| `jarvis sync` | Is managed configuration stale? | `~/.jarvis/state.yaml` plus the installed version's embedded assets. | Local machine artifacts only. |
| `mem_sync` | Is memory data out of date? | `hive-daemon` and Hive API credentials. | Product memory data. |

`jarvis sync` is machine-scoped configuration replay. It never reaches Hive memory synchronization, and that boundary is proven by an import-closure test over the command's call graph rather than by convention.

### Dashboard vs daemon

The dashboard is served by Hive API when enabled. It does not start, stop, or configure local `hive-daemon` processes.

## Design principles

- Preserve local-first behavior.
- Keep shared sync explicit and diagnosable.
- Prefer clear CLI behavior over hidden magic.
- Keep generated configuration reproducible.
- Use SDD for substantial or risky changes.
- Keep human review batches small enough to understand.

## Next step

Use [`../getting-started.md`](../getting-started.md) for onboarding and [`../generated-artifacts.md`](../generated-artifacts.md) for regeneration rules.
