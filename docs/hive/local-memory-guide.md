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

## Guarded memory delete and restore

Use the installed `jarvis hive` TUI for a single-memory delete or restore. This is the only supported human workflow; MCP and agent tools do not expose these operations.

### Quick path

1. Open a project and select an active memory, or press `x` to open Recently Deleted.
2. Press `d` to delete an active memory or `r` to restore a deleted memory.
3. Enter a non-empty reason and the exact confirmation phrase shown by the TUI.

The TUI creates a fresh local backup, re-reads the selected memory identity, and only then asks the daemon to commit the guarded mutation. It never performs direct SQLite or cloud mutation.

### Status meanings

| Status | Meaning |
|---|---|
| Local `committed` | The local tombstone or restore and its journal entry committed atomically. |
| Shared `pending` | Local work is safe, but propagation has not yet been acknowledged. |
| Shared `completed` | The matching sync event was acknowledged. |
| Shared `failed/retryable` | The daemon will retry propagation; do not submit a second mutation. |
| `legacy_unsupported` | The connected daemon cannot prove the required propagation contract. |

Delete and restore controls remain disabled unless the daemon advertises the complete safety contract. There is no hard delete or bulk memory action in this workflow.

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
