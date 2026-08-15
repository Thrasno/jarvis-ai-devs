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
| `jarvis config forget-agent` | Yes | Remove one agent from `~/.jarvis/state.yaml` so `jarvis sync` stops managing its files. Deletes no files. | `jarvis config forget-agent opencode` |
| `jarvis login` | Yes | Re-authenticate with Hive API credentials. | `jarvis login` |
| `jarvis persona set` | Yes | Change active persona preset and regenerate managed config. | `jarvis persona set default` |
| `jarvis init` | Yes | Create or refresh project `.jarvis/skill-registry.md`. | `jarvis init` |
| `jarvis skill-registry refresh` | Yes | Refresh project-local skill registry. | `jarvis skill-registry refresh --cwd .` |
| `jarvis hive` | Yes | Open Hive governance TUI using live daemon data. | `jarvis hive` |
| `jarvis hive import-engram` | Advanced | Preview or execute local Engram-to-Hive import. | `jarvis hive import-engram --dry-run` |
| `jarvis timeline` | Yes | Open Hive timeline TUI for a project. Requires `--project`. | `jarvis timeline --project jarvis-ai-devs` |
| `jarvis sync` | Yes | Replay this machine's recorded configuration. Takes no flags. | `jarvis sync` |
| `jarvis sdd status` | Yes | Show SDD phase status for a change. | `jarvis sdd status <change> --project <project>` |
| `jarvis sdd continue` | Yes | Print next recommended SDD phase. | `jarvis sdd continue <change> --json` |

## `jarvis sync`

`jarvis sync` reinstalls exactly what the setup wizard installed on this machine: model assignments, Jarvis-managed MCPs, installer-managed skills, the active persona, and the statusline only when you answered yes to it during installation. It reads the recorded manifest at `~/.jarvis/state.yaml` and the assets embedded in the installed binary; it never inspects the filesystem to guess what you once chose.

```bash
jarvis sync
```

| Property | Behavior |
|----------|----------|
| Flags | None. `jarvis sync --dry-run` and any other flag are usage errors, and nothing is written. |
| Prompts | None. The run is non-interactive from start to finish. |
| Already current | Zero writes, and the report says `this machine is already current; nothing was changed.` |
| Backup | A snapshot lands in `~/.jarvis/backups/` before the first write. A converged run takes none, because it mutates nothing. |
| Output | Always reports the changed paths, each agent's outcome, and the verification result. |
| Exit code | Non-zero when any configured agent failed to converge, or when verification failed. |

There is no `--dry-run` on purpose. Replay is the whole command, and describing changes without making them would require a second path through the applier that could drift from the real one.

### `sync` versus `reconcile`

| Command | Question it answers | Source of the answer |
|---------|---------------------|----------------------|
| `jarvis reconcile` | Is my managed configuration broken? | What `jarvis doctor` observes on disk. |
| `jarvis sync` | Is my managed configuration stale? | The recorded manifest plus the installed version's embedded assets. |

Neither command synchronizes Hive memory. The agent-facing `mem_sync` tool remains the only thing that moves memory data between local Hive and Hive API; `jarvis sync` never touches it, and says so in every report.

### Partial scope and recovery

- **Nothing recorded to replay.** A manifest with no configured agents blocks the run and names `jarvis` as the recovery command. Nothing is written.
- **Cloud portion unavailable.** When the manifest records `local+cloud` scope and `~/.jarvis/sync.json` is missing or unreadable, the run reports `jarvis login` for the cloud portion and still replays the local configuration. The cloud portion never aborts a local replay.

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
