# Security and Privacy

Jarvis is designed around local-first memory and explicit team sharing. Local Hive data, shared Hive API data, assistant memory systems, and generated agent configuration are separate concerns.

## Quick path

- Keep secrets out of git.
- Treat `~/.jarvis/sync.json` and `HIVE_API_*` variables as sensitive.
- Enable Hive API sync only when the team has agreed what projects may share memory.
- Use HTTPS termination for production Hive API/dashboard deployments.
- Regenerate managed agent configuration through Jarvis instead of patching generated files.

## Data locations

| Data | Location | Privacy note |
|------|----------|--------------|
| Jarvis CLI config | `~/.jarvis/config.yaml` | Local user settings. |
| Hive sync credentials | `~/.jarvis/sync.json` or `HIVE_API_*` env vars | Sensitive; do not commit or print. |
| Hive local memory | Local Hive storage managed by `hive-daemon` | Local-first; available without Hive API. |
| Project skill registry | `.jarvis/skill-registry.md` | Intended to be shared with the repository when useful. |
| Generated agent files | `~/.claude/**`, `~/.config/opencode/**` | Generated from Jarvis sources; not source of truth. |
| Hive API shared memory | PostgreSQL behind `hive-api` | Shared team data when sync is enabled/configured. |

## Local vs shared memory

Hive local memory is the product memory stored and served by the Jarvis ecosystem. Hive API is the shared backend that can receive synchronized team memory. Neither should be confused with assistant memory systems such as Engram used by a development agent during repo work.

Installing or rerunning Jarvis does not automatically mean memory has synchronized with Hive API. Sync requires API configuration and enabled sync behavior.

## Tokens and secrets

- Never commit `.env` files, passwords, API tokens, private keys, or generated credentials.
- Prefer deployment secret managers for Hive API settings such as `DATABASE_URL` and `JWT_SECRET`.
- Use `JWT_SECRET` values with at least 32 characters.
- Rotate secrets through the deployment process when exposure is suspected.

## Dashboard and authentication caveats

The Hive dashboard is served by `hive-api` when enabled with compiled assets. Production deployments should terminate TLS at the reverse proxy before forwarding traffic to the API.

The dashboard and login behavior depend on the current Hive API implementation and deployment settings. Treat browser sessions, API credentials, and admin access as sensitive. Do not expose the dashboard on an untrusted network without HTTPS and access controls.

## Password display masking caveat

The Jarvis login UI masks typed passwords with bullet characters for display. This prevents casual shoulder-surfing in the terminal UI; it is not cryptographic protection and does not replace secure storage, transport encryption, or secret handling.

## Checklist

- [ ] No secrets are committed.
- [ ] Hive API runs behind HTTPS in production.
- [ ] `JWT_SECRET` is strong and managed outside git.
- [ ] Project allowlisting and team sharing expectations are clear before sync.
- [ ] Logs are reviewed before sharing externally.

## Next step

For generated-file boundaries, read [`generated-artifacts.md`](generated-artifacts.md).
