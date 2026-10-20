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
| --------- | --------- | --------- | --------- |
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
| `jarvis sdd bind` | Yes | Bind a change to the artifact store chosen in the SDD preflight. | `jarvis sdd bind --change <change> --mode hybrid --json` |
| `jarvis sdd status` | Yes | Show SDD phase status for a change. | `jarvis sdd status <change> --project <project>` |
| `jarvis sdd continue` | Yes | Print next recommended SDD phase. | `jarvis sdd continue <change> --json` |

## `jarvis sdd bind`

`jarvis sdd bind` persists the artifact store chosen in the SDD session preflight as the change's immutable binding. Run it right after the preflight choice, before any `jarvis sdd status` call or artifact write for the change. Otherwise the first `status` call adopts the `hive` default (`initial:default:hive`), and Hive progress written before any OpenSpec progress is inferred as `legacy:hive-progress`.

```bash
jarvis sdd bind --change <change> --mode <hive|openspec|hybrid> [--project <project>] [--json]
```

| Property | Behavior |
| ---------- | ---------- |
| `--change` | Required. The canonical SDD change name. |
| `--mode` | Required. `hive`, `openspec`, or `hybrid`. `none` is never persisted and is rejected, as is any other value; nothing is written. |
| `--project` | Optional. Resolved like every other `jarvis sdd` command (see [SDD project identity](#sdd-project-identity)), so the binding is written under the effective canonical Hive key. |
| Storage | Provenance `initial:preflight`. `hive` is stored in Hive. `openspec` is stored in `openspec/changes/<change>/state.yaml` of the working directory, and the change directory is created when missing. `hybrid` is stored in both. |
| Same mode already bound | Idempotent replay: exit 0, nothing is written, and the stored provenance is kept (for example `initial:default:hive` from an earlier `status`). |
| Different mode | Conflict: the error names the existing and requested modes, the exit code is non-zero, and the stored binding is unchanged. This also applies when the change is unbound but existing apply progress already selects another store. Bindings are immutable; use a new change name for another store. |
| `--json` | Prints `{"project", "change", "mode", "provenance", "persisted"}`. |

For a `none` choice, do not bind. Prefix every `jarvis sdd` call for that change with `JARVIS_SDD_STORE_MODE=none` so `status` does not adopt the `hive` default.

## SDD workspace edit authority

`jarvis sdd status` and `jarvis sdd continue` derive edit authority from the invocation's working directory, not from `--project` or an ambient repository. When that directory is inside a Git worktree, Jarvis uses the canonical worktree root. Otherwise, a canonical non-Git project directory is the only allowed edit root.

`--project` remains a Hive project-identity alias, including names that do not match the directory or Git remote. It never grants edit authority outside the actual working directory. Missing, symlinked, root, home, temporary-root, or otherwise unresolved targets produce planning-only status rather than an editable workspace.

## SDD project identity

`jarvis sdd bind`, `jarvis sdd status`, `jarvis sdd continue`, `jarvis sdd progress`, `jarvis sdd archive`, and `jarvis sdd supersede` resolve the project the same way at their boundary:

- The spelling is canonicalized once: case is folded, and `_`, `.`, ` `, `/`, and `-` runs become a single `-`. `MyProject`, ` MYPROJECT `, and `myproject` are the same project (`myproject`); `my_project`, `My.Project`, and `my-project` are the same project (`my-project`). The `project` field of a `checkpoint` or `upgrade-continuation` request, `jarvis sdd archive --project`, and `jarvis sdd supersede --project` are canonicalized the same way.
- A retired alias resolves to its target. Hive reports the effective project key, and the CLI reads the target's binding and progress and writes new progress under the target, never under the retired key.
- `advance` submits an already signed snapshot, which cannot be re-keyed. Its `snapshot.project` must already be the effective key; a non-canonical spelling is rejected before any binding is read or adopted.
- `jarvis sdd supersede` folds the spelling but does not follow a retired alias: it fails closed with a store-binding redirect error and writes nothing. Supersede needs existing apply progress, and Hive refuses to retire a project that holds apply progress, so this case does not arise through a supported path.

Stored progress signed under a different project than the effective key is a genuine identity mismatch. Status reports `apply-progress` as `blocked:project_mismatch` (not `blocked:invalid`). `checkpoint` returns outcome `blocked`, code `project_mismatch`, and `next.action` `stop_blocked`. `advance` returns outcome `blocked` and code `project_mismatch`; its output has no `next` field. Both exit non-zero. Nothing is written. Stop and reconcile the project identity; do not re-sign or copy progress under another key.

## `jarvis sync`

`jarvis sync` reinstalls exactly what the setup wizard installed on this machine: model assignments, Jarvis-managed MCPs, installer-managed skills, the active persona, and the statusline only when you answered yes to it during installation. It reads the recorded manifest at `~/.jarvis/state.yaml` and the assets embedded in the installed binary; it never inspects the filesystem to guess what you once chose.

```bash
jarvis sync
```

| Property | Behavior |
| ---------- | ---------- |
| Flags | None. `jarvis sync --dry-run` and any other flag are usage errors, and nothing is written. |
| Prompts | None. The run is non-interactive from start to finish. |
| Already current | Zero writes, and the report says `this machine is already current; nothing was changed.` |
| Backup | A snapshot lands in `~/.jarvis/backups/` before the first write. A converged run takes none, because it mutates nothing. |
| Output | Always reports the changed paths, each agent's outcome, and the verification result. |
| Exit code | Non-zero when any configured agent failed to converge, or when verification failed. |
| Skills | Catalog and manifest identity decide lifecycle: recorded catalog skills update, catalog-only non-interactive skills install, unselected interactive skills remain absent, recorded skills this build explicitly retired are safely removed after snapshotting their managed tree, and recorded skills unknown to this build are kept. User skills and `_shared` are never removed. |
| Persona | Replay honors the recorded builtin/user source. Legacy manifests without a persona replay the `argentino` builtin. |

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
