# Getting Started with Jarvis Dev

Jarvis Dev is installed and operated through the `jarvis` CLI. The fastest path is: install the binaries, run `jarvis`, let the wizard configure supported agents, then use Hive locally before enabling team sync.

## Quick path

1. Install Jarvis from the release channel in [`installation.md`](installation.md).
2. Run the setup wizard:

   ```bash
   jarvis
   ```

3. If any supported agent is already configured on this machine, the wizard's
   first screen offers a configuration reset before anything else runs. See
   [Configuration reset (upgrading an existing machine)](#configuration-reset-upgrading-an-existing-machine)
   below before answering it.
4. Choose the agent integrations you use, such as Claude Code or OpenCode.
5. Confirm the generated configuration with:

   ```bash
   jarvis verify --provider all
   ```

6. Open local memory tools when the Hive daemon is running:

   ```bash
   jarvis hive
   jarvis timeline --project <project>
   ```

## Configuration reset (upgrading an existing machine)

Before anything else, the wizard detects any already-configured supported
agent (Claude Code, OpenCode) and offers to reset the Jarvis-managed parts of
its configuration. This step exists so a machine that installed an earlier
Jarvis release can be brought fully in line with the current release — for
example, removing agent files or settings a retired Jarvis feature left
behind (see [`maintenance/rdd-experiment-retirement.md`](maintenance/rdd-experiment-retirement.md)
for the concrete case that motivated it).

**What it lists.** The step shows, per detected agent, the exact contract-owned
surfaces a reset would touch:

- **Claude Code (`~/.claude/`):** `settings.json` keys `outputStyle` and
  `statusLine`, plus Jarvis-written `hooks` and `permissions` entries
  identified by content; the `CLAUDE.md` block between Jarvis markers; the
  whole `agents/` directory; the files `sdd-orchestrator.md` and
  `statusline-command.sh`; and the `output-styles/`, `skills/`, and
  `hive-hooks/` directory trees.
- **OpenCode (`~/.config/opencode/`):** the `opencode.json` sections
  `default_agent`, `permission`, `agent`, `mcp.hive`, and `mcp.context7`; the
  `AGENTS.md` block between Jarvis markers; the file `sdd-orchestrator.md`;
  the `skills/` tree; and the Jarvis plugins under `plugins/`.

Everything else in those files and directories — other MCP servers, themes,
keybinds, your own hooks and permissions, and content outside the Jarvis
marker blocks — is preserved either way.

**What accepting replaces entirely.** Two surfaces are entirely Jarvis-owned
rather than partially managed: Claude's whole `agents/` directory and
OpenCode's `agent` section of `opencode.json`. Accepting the reset removes
these completely, including any agent file or entry you added there
yourself — that content is not recoverable from the running configuration
afterward. The wizard's completion summary names the durable backup snapshot
ID it took before making any change, so a user who needs to recover something
can restore that snapshot by hand.

**What declining does.** Declining keeps your current configuration exactly
as it is; nothing described above is touched, and the rest of setup proceeds
unchanged. Leftover files or settings from an earlier release (for example,
retired `review-*` agent files) remain on disk. This is expected, not an
error: `jarvis doctor` reports that kind of residue informationally and
points back at this step, but never deletes it, and neither `jarvis sync`,
`jarvis reconcile`, nor ordinary doctor checks ever perform this reset on
their own. Only this consented wizard step does.

## Expected result

- `jarvis` is available on your PATH.
- The first run launches the full setup wizard.
- Later runs launch the reconfiguration wizard with previous values prefilled.
- Managed agent configuration is generated from Jarvis templates, not hand-authored on the user machine.
- Hive local memory can work without the shared Hive API.

## What Jarvis configures

| Area | Result |
|------|--------|
| CLI | User-facing entrypoint for setup, diagnostics, SDD status, Hive UI, and configuration. |
| Agent setup | Managed configuration for supported agents when selected in the wizard. |
| Hive local memory | Local-first memory access through `hive-daemon`. |
| SDD workflow | Prompt/workflow support for Spec-Driven Development when activated. |
| Team sync | Hive ↔ Hive API synchronization when API credentials and sync settings are configured. |

## First-run checklist

- [ ] Installed from the intended channel: production or `beta`.
- [ ] Ran `jarvis` once from a terminal.
- [ ] Selected the correct agent integrations.
- [ ] Confirmed generated configuration with `jarvis verify --provider all`.
- [ ] Started or confirmed `hive-daemon` before using Hive TUI screens.
- [ ] Used `jarvis timeline --project <project>` with an explicit project name.

## Next step

Read [`cli-reference.md`](cli-reference.md) for command groups, then [`configuration.md`](configuration.md) for local files and environment variables.
