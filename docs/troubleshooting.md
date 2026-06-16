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
