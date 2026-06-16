# Jarvis CLI Reference

The public CLI entrypoint is `jarvis`. Run it without arguments for setup or reconfiguration; use subcommands for diagnostics, Hive views, SDD status, and advanced configuration.

> This reference is intentionally brief. It lists command groups and common examples, but not every flag is fully audited here.

## Quick path

```bash
jarvis
jarvis verify --provider all
jarvis doctor --provider all
jarvis hive
jarvis timeline --project <project>
```

## Command groups

| Command | Public? | Purpose | Example |
|---------|---------|---------|---------|
| `jarvis` | Yes | Launch setup or reconfiguration wizard. | `jarvis` |
| `jarvis --no-tui` | Yes | Use non-TUI setup prompts. | `jarvis --no-tui` |
| `jarvis verify` | Yes | Verify managed runtime integrity. | `jarvis verify --provider all` |
| `jarvis doctor` | Yes | Plan remediation without mutating files. | `jarvis doctor --provider opencode` |
| `jarvis reconcile` | Yes, advanced | Apply safe managed repairs for owned drift. | `jarvis reconcile --provider all --dry-run` |
| `jarvis backup` | Yes, advanced | Create a lifecycle backup snapshot. | `jarvis backup --provider claude` |
| `jarvis restore` | Yes, advanced | Restore managed assets from a snapshot. | `jarvis restore --provider claude --snapshot <id> --yes` |
| `jarvis uninstall` | Yes, advanced | Remove managed lifecycle assets. | `jarvis uninstall --provider all --dry-run` |
| `jarvis config` | Yes | View current Jarvis configuration. | `jarvis config` |
| `jarvis config set` | Yes | Set supported keys: `preset`, `api_url`, `email`. | `jarvis config set api_url https://hive.example.com` |
| `jarvis login` | Yes | Re-authenticate with Hive API credentials. | `jarvis login` |
| `jarvis persona set` | Yes | Change active persona preset and regenerate managed config. | `jarvis persona set default` |
| `jarvis init` | Yes | Create or refresh project `.jarvis/skill-registry.md`. | `jarvis init` |
| `jarvis skill-registry refresh` | Yes | Refresh project-local skill registry. | `jarvis skill-registry refresh --cwd .` |
| `jarvis hive` | Yes | Open Hive governance TUI using live daemon data. | `jarvis hive` |
| `jarvis hive import-engram` | Advanced | Preview or execute local Engram-to-Hive import. | `jarvis hive import-engram --dry-run` |
| `jarvis timeline` | Yes | Open Hive timeline TUI for a project. Requires `--project`. | `jarvis timeline --project jarvis-ai-devs` |
| `jarvis sync` | Legacy/no-op | Prints sync guidance; sync is handled through Hive daemon tools. | `jarvis sync` |
| `jarvis sdd status` | Yes | Show SDD phase status for a change. | `jarvis sdd status <change> --project <project>` |
| `jarvis sdd continue` | Yes | Print next recommended SDD phase. | `jarvis sdd continue <change> --json` |

## Hive daemon URL resolution

`jarvis hive` and `jarvis timeline --project <project>` talk to `hive-daemon` over HTTP.

Resolution order:

1. `--daemon-url` when the command supports it.
2. `HIVE_DAEMON_URL`.
3. `HIVE_HTTP_PORT`, resolved as `http://127.0.0.1:<port>`.
4. Default: `http://127.0.0.1:7438`.

Examples:

```bash
HIVE_DAEMON_URL=http://127.0.0.1:7438 jarvis hive
HIVE_HTTP_PORT=7439 jarvis timeline --project my-project
jarvis hive --daemon-url http://127.0.0.1:7438
```

## Public commands vs internal hooks

The hidden `jarvis hook ...` command group is called by generated Claude Code hooks. It is an internal integration surface, not a user workflow. Do not call it manually unless you are diagnosing hook integration behavior.

## Expected result

- Read-only commands such as `verify` and `doctor` do not mutate managed files.
- Mutating lifecycle commands require explicit confirmation flags such as `--yes`.
- `jarvis timeline` fails clearly when `--project` is omitted.

## Next step

Use [`troubleshooting.md`](troubleshooting.md) when a command fails or generated configuration drifts.
