# Getting Started with Jarvis Dev

Jarvis Dev is installed and operated through the `jarvis` CLI. The fastest path is: install the binaries, run `jarvis`, let the wizard configure supported agents, then use Hive locally before enabling team sync.

## Quick path

1. Install Jarvis from the release channel in [`installation.md`](installation.md).
2. Run the setup wizard:

   ```bash
   jarvis
   ```

3. Choose the agent integrations you use, such as Claude Code or OpenCode.
4. Confirm the generated configuration with:

   ```bash
   jarvis verify --provider all
   ```

5. Open local memory tools when the Hive daemon is running:

   ```bash
   jarvis hive
   jarvis timeline --project <project>
   ```

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
