# Hive Local Memory Guide

Hive local memory is the local-first memory layer used by Jarvis. It lets agent workflows remember project decisions and context without requiring the shared Hive API to be available.

## Quick path

1. Start or confirm `hive-daemon` is running.
2. Open the local Hive TUI:

   ```bash
   jarvis hive
   ```

3. Browse a project timeline:

   ```bash
   jarvis timeline --project <project>
   ```

## Expected result

- Local memory operations continue even when the network or Hive API is unavailable.
- The daemon serves local HTTP endpoints for Jarvis UI commands.
- Timeline browsing requires an explicit `--project` value.

## Daemon URL resolution

Jarvis resolves the daemon URL in this order:

1. Command flag where available: `--daemon-url`.
2. `HIVE_DAEMON_URL`.
3. `HIVE_HTTP_PORT`, mapped to `http://127.0.0.1:<port>`.
4. Default `http://127.0.0.1:7438`.

## Local memory vs assistant memory

Hive is the product memory layer in Jarvis Dev. It is not the same as assistant memory systems such as Engram that may be used by a coding agent outside the product. Product docs should describe Hive/Hive API behavior only when discussing Jarvis memory.

## What belongs in Hive

| Good fit | Why |
|----------|-----|
| Architecture decisions | Future agents and teammates can recover the reason behind choices. |
| Bugfix root causes | Prevents repeat investigation. |
| Project conventions | Keeps generated work aligned with team rules. |
| SDD artifacts | Preserves workflow state across sessions when Hive is the selected store. |

## Checklist

- [ ] `hive-daemon` is reachable on the expected URL.
- [ ] The project name is explicit when using timeline views.
- [ ] Sensitive data is not intentionally saved as memory.
- [ ] Shared sync is configured only when the team wants central memory.

## Next step

Read [`sync-guide.md`](sync-guide.md) before enabling Hive ↔ Hive API synchronization.
