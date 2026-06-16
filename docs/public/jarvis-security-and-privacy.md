# Jarvis Dev Security and Privacy

Jarvis Dev is designed for teams that need AI assistance without losing control of project knowledge. Local memory, shared memory, and generated configuration are treated as separate concerns.

## Key points

- Local memory is available without the shared server.
- Shared memory is enabled/configured deliberately.
- Secrets should stay outside repositories.
- Production server traffic should use HTTPS.
- Generated developer-machine configuration should be regenerated through Jarvis flows, not manually patched.

## Data model in plain language

| Area | What it means |
|------|---------------|
| Local memory | Knowledge stored on a developer machine for local continuity. |
| Shared memory | Team knowledge synchronized through Hive API when configured. |
| Dashboard | Optional browser view for shared memory operations when enabled. |
| Generated config | Tool configuration created by Jarvis from trusted sources. |

## Security caveats

The login interface may mask passwords on screen, but display masking is not encryption. Real protection comes from secret handling, access controls, HTTPS, and secure deployment practices.

Dashboard access and admin features depend on the deployed Hive API configuration. Companies should review access rules, hosting, and data retention expectations during evaluation.

## Governance questions

- Which repositories may use shared memory?
- Who operates the shared Hive API?
- Who has dashboard/admin access?
- What information should never be saved as memory?
- How are secrets rotated and incidents handled?

## Next step

Use [`jarvis-evaluator-brief.md`](jarvis-evaluator-brief.md) to run a controlled evaluation before broad rollout.
