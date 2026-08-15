# Troubleshooting

Start with the safest command: `jarvis doctor --provider all`. It plans remediation without mutating managed files.

## Quick path

```bash
jarvis verify --provider all
jarvis doctor --provider all
jarvis reconcile --provider all --dry-run
```

Only apply repairs after reviewing the plan:

```bash
jarvis reconcile --provider all --yes
```

## Common issues

| Symptom | Likely cause | What to do |
|---------|--------------|------------|
| `jarvis` is not found | Binary is not on PATH or install failed. | Re-run the installer from [`installation.md`](installation.md), then open a new terminal. |
| Setup looks incomplete | Managed files are missing or drifted. | Run `jarvis verify --provider all`, then `jarvis doctor --provider all`. |
| `jarvis timeline` fails immediately | `--project` is required. | Run `jarvis timeline --project <project>`. |
| Hive TUI cannot connect | `hive-daemon` is not running or URL resolution points to the wrong port. | Check `HIVE_DAEMON_URL`, `HIVE_HTTP_PORT`, or use `jarvis hive --daemon-url <url>`. |
| Sync does not happen after install | Installation/reconfiguration is separate from Hive ↔ Hive API sync. | Configure Hive API credentials and enable sync behavior when needed. |
| File-based sync changes are ignored | `HIVE_API_*` environment variables override `~/.jarvis/sync.json`. | Restart `hive-daemon` with env vars unset, or update the env vars instead. |
| Dashboard returns 404 | Dashboard assets are not configured or not built. | Set `DASHBOARD_ASSETS_DIR` to a valid compiled dashboard directory and restart `hive-api`. |
| Dashboard login fails | Invalid credentials, missing server configuration, or expired session. | Check Hive API health, credentials, `JWT_SECRET`, and deployment logs. |
| `hive-daemon` stops at `pending migration restore` on every start | A scheduled restore replaced the database but could not clear its own request. | Follow [Recovery from a stuck pending restore](#recovery-from-a-stuck-pending-restore). |
| `jarvis sync` reports nothing to replay | `~/.jarvis/state.yaml` records no configured agents. | Run `jarvis`. Sync never redetects agents from the filesystem. |
| `jarvis sync` keeps rebuilding an agent you removed | `~/.jarvis/state.yaml` still records it. Sync replays what the manifest records, and never drops an entry because the agent's files went missing — an unobserved record is the only proof that authorizes cleaning it up later. | `jarvis config forget-agent <agent>`. Re-running `jarvis` will not clear it. |
| `jarvis sync accepts no flags` | A flag was passed, including an inherited one such as `--no-tui`. | Run `jarvis sync` with no arguments. Nothing was written. |
| `jarvis sync` names `jarvis login` | Scope is `local+cloud` but `~/.jarvis/sync.json` is missing or unreadable. | Run `jarvis login`. The local replay already completed. |
| `jarvis sync` says converged but something is still wrong | The affected file may not be a tracked path. | See [Known gaps in `jarvis sync`](#known-gaps-in-jarvis-sync). |

## Recovery from a stuck pending restore

A rollback to a migration backup is scheduled as a request file next to the
database and applied by the next `hive-daemon` start, before SQLite is opened.
Once those bytes are back in place the daemon must clear the request. If it
cannot — a full disk, a read-only `~/.jarvis`, or missing permissions — it stops
instead of serving, because serving would let the next start replay the same
restore and discard everything written in between.

The daemon logs the exact file to remove. It is always the database path plus
`.restore-pending`, so with the default database it is:

```bash
rm ~/.jarvis/memory.db.restore-pending
```

Fix the underlying cause first — free disk space or restore write access to
`~/.jarvis` — otherwise the next scheduled restore stops in the same place. Then
start `hive-daemon` again. The restore itself already succeeded, so nothing is
lost by deleting the request.

## `jarvis sync` replaces an unmarked instruction file

Read this before running `jarvis sync` on a machine whose agent instruction files you edited by hand.

A `CLAUDE.md` or `AGENTS.md` belonging to a configured agent is a managed file. If it carries the Jarvis sentinel markers, replay rewrites only the managed sections and leaves everything outside them byte-for-byte intact. **If it carries no sentinel markers at all, it is rendered fresh and its previous content is discarded.** This is the same thing the setup wizard does today; sync does not add the behavior, it inherits it.

The recovery path is the pre-apply snapshot in `~/.jarvis/backups/`, taken before the first write of every run that changes anything:

```bash
jarvis restore --provider claude --snapshot <id> --yes
```

Files outside the recorded instruction targets are never read, modified, or replaced.

## Known gaps in `jarvis sync`

`jarvis sync` is honest about the paths it tracks, and there are three it does not.

1. **`settings.json` is not tracked.** Installing the statusline also merges an entry into `~/.claude/settings.json`, whose final content is a merge with your own bytes rather than a computable desired state. If you remove that entry by hand while every tracked path is current, `jarvis sync` reports converged and does not restore it. Workaround: rerun `jarvis`.
2. **Persona output styles are not replayed.** Changing persona and then upgrading leaves the generated output-style file stale. Workaround: `jarvis persona set <preset>`.
3. **Asset-set bookkeeping is not recorded yet.** The managed-asset digest that would let a run recognize that the installed version ships a different asset set is not produced yet, so an upgrade is detected through the tracked paths themselves rather than through a version comparison.

None of these three block a replay. They mean a converged report is a statement about the tracked paths, not about every byte Jarvis has ever written.

## Recovery workflow for managed agent configuration

1. Diagnose without mutation:

   ```bash
   jarvis verify --provider all
   jarvis doctor --provider all
   ```

2. Create an explicit backup if needed:

   ```bash
   jarvis backup --provider all
   ```

3. Preview repairs:

   ```bash
   jarvis reconcile --provider all --dry-run
   ```

4. Apply only safe owned repairs:

   ```bash
   jarvis reconcile --provider all --yes
   ```

5. Verify again:

   ```bash
   jarvis verify --provider all
   ```

## Important boundary

Do not manually patch generated agent files such as `~/.claude/CLAUDE.md` or `~/.config/opencode/opencode.json`. Change the Jarvis source of truth or rerun `jarvis`/`jarvis persona` so managed output is regenerated consistently.

## Next step

If troubleshooting touches secrets or shared memory, review [`security-privacy.md`](security-privacy.md) before sharing logs.
