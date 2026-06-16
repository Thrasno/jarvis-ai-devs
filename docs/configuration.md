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
| `~/.jarvis/sync.json` | Hive API sync credentials for `hive-daemon`. | Contains secrets; do not commit. |
| `~/.jarvis/personas/<slug>.yaml` | User-defined persona presets. | Optional. |
| `.jarvis/skill-registry.md` | Project-local skill registry. | Intended to be committed and shared with the team. |
| `.jarvis/skills/<skill>/SKILL.md` | Project-local skill copies when installed/configured. | Treat as project workflow assets. |

## Supported `jarvis config set` keys

| Key | Meaning |
|-----|---------|
| `preset` | Active persona preset. |
| `api_url` | Hive API URL used by Jarvis login/config flows. |
| `email` | User email stored in local Jarvis config. |

`configured_agents` and `version` are managed by the wizard and are read-only from this command.

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
