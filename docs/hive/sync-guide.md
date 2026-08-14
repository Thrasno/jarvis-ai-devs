# Hive Sync Guide

Hive ↔ Hive API sync is separate from installing or rerunning Jarvis. Installation updates binaries and generated assets; sync moves product memory between local Hive instances and the shared Hive API when configured.

## Quick path

1. Deploy or identify the Hive API endpoint.
2. Configure local sync credentials through Jarvis login/config flows, `~/.jarvis/sync.json`, or `HIVE_API_*` environment variables.
3. Start or restart `hive-daemon`.
4. Enable automatic sync only when the team wants background sync.
5. Use local memory normally; sync should not block local-first work.

## Configuration inputs

| Input | Purpose |
|-------|---------|
| `HIVE_API_URL` | Shared Hive API base URL. |
| `HIVE_API_EMAIL` | User email for API authentication. |
| `HIVE_API_PASSWORD` | User secret for API authentication. |
| `~/.jarvis/sync.json` | File-based sync credentials and optional `auto_sync`. |
| `auto_sync` | Enables background sync when explicitly true/configured. |

Environment variables take precedence over file configuration at runtime. File changes remain overridden until `HIVE_API_*` variables are updated or unset and `hive-daemon` is restarted.

## Recover after changing an account password

1. Open Jarvis → Hive API Config.
2. Enter the new Hive account password and save. This updates the stored daemon credential; it does not change the account password.
3. Restart `hive-daemon` to clear the rejected session and load the new credential.

Test Connection is read-only. A successful test does not update the running daemon or clear its rejected-session latch; save and restart after testing a replacement password.

## Manual vs automatic sync

Memory sync is handled through `hive-daemon` and its tools. Manual sync can be requested through the agent-facing Hive tools when available; `mem_sync` is the only thing that moves memory data. Automatic background sync only runs when explicitly enabled/configured.

## `jarvis sync` is configuration replay, not memory sync

Despite the name, `jarvis sync` never touches Hive memory data. It replays this machine's recorded agent configuration — model assignments, managed MCPs, skills, persona, statusline — from `~/.jarvis/state.yaml`. Every run says so explicitly in its report.

| Concern | Command |
|---------|---------|
| Reapply this machine's recorded configuration after an upgrade | `jarvis sync` |
| Move memory between local Hive and Hive API | `mem_sync`, or automatic daemon sync when enabled |

One overlap is worth knowing: when the manifest records `local+cloud` scope and `~/.jarvis/sync.json` is missing or unreadable, `jarvis sync` reports that `jarvis login` is needed for the cloud portion and still completes the local replay. It reads that file at most once and never writes, creates, or repairs it; credentials belong to `jarvis login`.

## Installation is not sync

Do not tell users that reinstalling or rerunning Jarvis synchronizes memory. The release/install flow updates the ecosystem pack and regenerates managed configuration. Hive ↔ Hive API sync is a separate product behavior that depends on daemon configuration, credentials, and sync enablement.

## Failure behavior

When Hive API is unavailable, local Hive memory should remain usable. Sync should retry according to daemon behavior and avoid blocking local reads/writes.

## Checklist

- [ ] Team has agreed which projects may sync shared memory.
- [ ] Hive API URL and credentials are configured outside git.
- [ ] `hive-daemon` was restarted after sync configuration changes.
- [ ] `HIVE_API_*` env vars are not unintentionally overriding `sync.json`.
- [ ] Users understand local memory works without shared sync.

## Next step

Operators should read [`api-operator-guide.md`](api-operator-guide.md). Users should read [`local-memory-guide.md`](local-memory-guide.md).
