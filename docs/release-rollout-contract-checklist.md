# Release Rollout Contract Checklist

Use this checklist to review the Jarvis release rollout and reconfiguration contract without adding new CLI commands.

## Quick path

1. Confirm the release was announced in an explicit team channel.
2. Confirm developers used the normal install/update channel for the release.
3. Confirm developers reran root `jarvis` after updating.
4. Confirm docs avoid nonexistent release/setup commands.

## Spec proof points

| Requirement | Expected proof point | Current proof |
|-------------|----------------------|---------------|
| Release Announcement Contract | Rollout starts only after a physical notice, Teams post, mailing list, or equivalent team-channel notice. | `README.md` and `docs/setup-recovery.md` require an explicit team channel before rollout is considered started. |
| Ecosystem Pack Invariant | The release channel installs or updates the complete pack, not isolated binaries. | `README.md` states releases include `jarvis` + `hive-daemon` + embedded assets. `scripts/install.sh` downloads both binaries; `scripts/install.ps1` installs both binaries; `.goreleaser.yaml` publishes both artifacts. |
| Post-Update Reconfiguration Entry Point | After update, developers rerun root `jarvis` to reapply embedded assets/config. | `README.md`, `docs/setup-recovery.md`, installer output, and `jarvis-cli/cmd/jarvis/main.go` all point to bare `jarvis` as the wizard entrypoint. |
| Boundary Preservation for Sync Semantics | Release rollout docs describe installation/reconfiguration only. | `README.md`, `docs/setup-recovery.md`, and `docs/PRD.md` keep Hive ↔ Hive API sync separate from release rollout. |

## Source references

- `scripts/install.sh` — downloads `jarvis` and `hive-daemon`, then tells the developer to run `jarvis`.
- `scripts/install.ps1` — installs `jarvis` and `hive-daemon`, then tells the developer to run `jarvis`.
- `.goreleaser.yaml` — defines release artifacts for both `jarvis` and `hive-daemon`.
- `jarvis-cli/cmd/jarvis/main.go` — root command launches the setup/reconfiguration wizard.

## Command drift check

- [x] Canonical command documented for setup/reconfiguration: root `jarvis`.
- [x] Primary release docs avoid nonexistent setup/update/upgrade subcommands.
- [x] Rollout docs do not mix release state with Hive ↔ Hive API sync semantics.
