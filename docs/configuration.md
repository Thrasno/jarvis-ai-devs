# Configuration

Jarvis configuration is split between user-level settings, project-level skill metadata, Hive daemon sync settings, and optional Hive API deployment settings. Generated agent configuration is produced from source templates and should not be patched directly.

## Quick path

```bash
jarvis config
jarvis config set api_url https://hive.example.com
jarvis verify --provider all
```

## Local Jarvis files

| Location | Purpose | Notes |
|----------|---------|-------|
| `~/.jarvis/config.yaml` | User-level Jarvis CLI configuration. | Written by setup, reconfiguration, and supported config commands. |
| `~/.jarvis/state.yaml` | Desired-state manifest replayed by `jarvis sync`. | Written by setup and by migration. Not hand-edited. |
| `~/.jarvis/sync.json` | Hive API sync credentials for `hive-daemon`. | Contains secrets; do not commit. |
| `~/.jarvis/backups/` | Lifecycle snapshots, including the pre-apply snapshot every mutating `jarvis sync` run takes. | Recovery point for managed files. |
| `~/.jarvis/personas/<slug>.yaml` | User-defined persona presets. | Optional. |
| `.jarvis/skill-registry.md` | Project-local skill registry. | Intended to be committed and shared with the team. |
| `.jarvis/skills/<skill>/SKILL.md` | Project-local skill copies when installed/configured. | Treat as project workflow assets. |

## Supported `jarvis config set` keys

| Key | Meaning |
|-----|---------|
| `preset` | Active persona preset. |
| `api_url` | Hive API URL used by Jarvis login/config flows. |
| `email` | User email stored in local Jarvis config. |

`version` is managed by the wizard and is read-only from this command. The agent list moved to `state.yaml`; see below.

## Two stores, no shared fields

Jarvis keeps user configuration and replay state in separate files, and they share no field.

| Store | Owns | Read by |
|-------|------|---------|
| `~/.jarvis/config.yaml` | User configuration: `api_url`, `email`, `version`, and other settings you choose. | Every command. |
| `~/.jarvis/state.yaml` | The desired-state manifest: installed agents and their instruction/config paths, installer-managed skill IDs, persona and its source, per-phase model assignments, scope, and the statusline decision. | `jarvis sync`. |

The split is enforced, not conventional: a value readable from one store is absent from the other. That is what makes a replay reproducible — `jarvis sync` has exactly one authority for what to reinstall.

### Migration to `config.yaml` schema 3

The first run of a version that ships this split migrates automatically:

1. The replay fields are written to `~/.jarvis/state.yaml` and flushed to disk.
2. Only then are those keys deleted from `config.yaml` and its `schema_version` set to `3`.
3. A one-line notice is printed, and only after both writes are durable.

The move is one-way and happens once. If the manifest write fails, `config.yaml` is left untouched at its previous schema version and no notice is printed. An existing `state.yaml` is never overwritten by migration. Keys `config.yaml` never owned are preserved exactly as they were.

### The statusline decision is tri-state

`state.yaml` records whether you were asked about the statusline, separately from your answer: never asked, asked and declined, or asked and accepted. `jarvis sync` installs the statusline only in the last case, and never prompts to fill the first one.

## Hive daemon settings

`hive-daemon` is the local service for memory operations. Public CLI screens resolve the daemon URL in this order:

1. Command flag, such as `jarvis hive --daemon-url <url>`.
2. `HIVE_DAEMON_URL`.
3. `HIVE_HTTP_PORT`, mapped to `http://127.0.0.1:<port>`.
4. Default `http://127.0.0.1:7438`.

For Hive API sync, `hive-daemon` can use either environment variables or `~/.jarvis/sync.json`:

| Setting | Purpose |
|---------|---------|
| `HIVE_API_URL` | Hive API base URL. |
| `HIVE_API_EMAIL` | User account email for API sync. |
| `HIVE_API_PASSWORD` | User password/token as expected by the API login flow. |
| `auto_sync` in `sync.json` | Enables automatic background sync when explicitly set. |

Environment variables take precedence at runtime. If you edit file-based settings while `HIVE_API_*` variables are active, restart `hive-daemon` with those variables unset for file values to take effect.

## Hive API deployment settings

When operating `hive-api`, configure it through environment variables or your deployment secret manager.

| Setting | Required | Purpose |
|---------|----------|---------|
| `DATABASE_URL` | Yes | PostgreSQL connection string. |
| `JWT_SECRET` | Yes | Signs Hive API sessions; must be at least 32 characters. |
| `PORT` | No | API port; defaults are deployment-dependent. |
| `GIN_MODE` | No | Use `release` for production deployments. |
| `CORS_ALLOWED_ORIGINS` | No | Allowed browser origins. |
| `DASHBOARD_ASSETS_DIR` | Dashboard only | Enables `/dashboard` when it points to compiled assets. |

## Generated configuration boundary

Generated user-machine files include Claude/OpenCode settings and injected protocol blocks. Fix behavior by changing source templates or running Jarvis flows again, not by editing generated files directly. See [`generated-artifacts.md`](generated-artifacts.md).

## Checklist

- [ ] Secrets are outside the repository.
- [ ] Project `.jarvis/skill-registry.md` is committed when the team should share skill suggestions.
- [ ] Generated agent files are regenerated through Jarvis, not manually patched.
- [ ] Optional dashboard settings are only documented as active when configured.

## Next step

Read [`security-privacy.md`](security-privacy.md) before enabling team sync or deploying Hive API.
