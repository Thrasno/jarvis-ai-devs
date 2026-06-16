# Hive Dashboard Guide

The Hive dashboard is an optional browser interface served by `hive-api` when compiled dashboard assets are configured. It observes and administers Hive API data; it does not manage local `hive-daemon` processes.

## Quick path

1. Build `hive-dashboard/`.
2. Make the built `dist` directory available to `hive-api`.
3. Set `DASHBOARD_ASSETS_DIR` to that directory.
4. Start or restart `hive-api`.
5. Open `/dashboard/` on the Hive API host.

## Expected result

- `/dashboard/` returns the dashboard shell.
- API routes such as `/auth`, `/memories`, `/sync`, `/admin`, and `/health` remain API routes.
- Users authenticate through Hive API behavior for the deployed version.

## What the dashboard is for

| Area | Purpose |
|------|---------|
| Overview | Inspect high-level Hive API state when implemented/enabled. |
| Memories | Browse or search shared memory records exposed by API endpoints. |
| Users/Admin | Manage or inspect users when the deployment and role allow it. |
| Audit/sync views | Observe available audit and sync information. |

Some dashboard screens may depend on backend endpoint contracts that are still evolving. Avoid promising unavailable analytics, graph, or governance features unless the current deployment exposes them.

## Security notes

- Serve production dashboard traffic over HTTPS.
- Treat browser sessions and admin access as sensitive.
- Do not expose the dashboard publicly without the intended authentication and reverse-proxy controls.
- The dashboard does not start, stop, or configure local daemon processes.

## Troubleshooting

| Symptom | What to check |
|---------|---------------|
| `/dashboard/` returns 404 | `DASHBOARD_ASSETS_DIR` is unset, invalid, or assets were not built. |
| Assets return 404 | Use `/dashboard/` with trailing slash and confirm asset paths. |
| Login fails | Check API health, credentials, session/JWT settings, and deployment logs. |
| Admin views are missing | Confirm the logged-in user has admin permissions for the deployment. |

## Next step

Use [`../hive-api-dashboard.md`](../hive-api-dashboard.md) for the operator-level overview of how Hive API serves the dashboard. Use [`hive-dashboard/deployment.md`](hive-dashboard/deployment.md) for deployment and VPS details.
