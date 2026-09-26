# Installation

## Latest production

Without an override, the installers download the latest production release from GitHub.

**Windows**

```powershell
irm https://raw.githubusercontent.com/Thrasno/jarvis-ai-devs/master/scripts/install.ps1 | iex
```

**Linux**

```bash
curl -sSL https://raw.githubusercontent.com/Thrasno/jarvis-ai-devs/master/scripts/install.sh | bash
```

## Beta channel

Use the mutable `beta` prerelease when a teammate asks you to validate the next release candidate.

**Windows**

```powershell
$env:JARVIS_INSTALL_VERSION = "beta"
irm https://raw.githubusercontent.com/Thrasno/jarvis-ai-devs/master/scripts/install.ps1 | iex
```

**Linux**

```bash
export JARVIS_INSTALL_VERSION=beta
curl -sSL https://raw.githubusercontent.com/Thrasno/jarvis-ai-devs/master/scripts/install.sh | bash
```

## Upgrading an existing machine

Reinstalling or upgrading over a machine that already has a supported agent
configured (Claude Code, OpenCode) does not change that configuration by
itself. Run `jarvis` after installing: its wizard detects the existing
configuration and, as its first step, offers an explicit, consented reset of
the Jarvis-managed parts of it. See
[Configuration reset (upgrading an existing machine)](getting-started.md#configuration-reset-upgrading-an-existing-machine)
in `getting-started.md` for exactly what it lists, what accepting replaces
entirely, and what declining leaves in place.

## Notes

- Without `JARVIS_INSTALL_VERSION`, the script installs the latest published release.
- Exact release tags remain supported, for example `JARVIS_INSTALL_VERSION=v0.1.0`.
- Set `JARVIS_INSTALL_REPO=owner/repo` to fetch artifacts from a different GitHub repository.
- Windows: installs to `%LOCALAPPDATA%\Programs\jarvis` and adds it to the user PATH automatically.
- Linux: installs to `/usr/local/bin`.
- macOS artifacts are best effort from GoReleaser and are not separately validated by CI.
