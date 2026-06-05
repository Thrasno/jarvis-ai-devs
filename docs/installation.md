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

## Notes

- Without `JARVIS_INSTALL_VERSION`, the script installs the latest published release.
- Exact release tags remain supported, for example `JARVIS_INSTALL_VERSION=v0.1.0`.
- Set `JARVIS_INSTALL_REPO=owner/repo` to fetch artifacts from a different GitHub repository.
- Windows: installs to `%LOCALAPPDATA%\Programs\jarvis` and adds it to the user PATH automatically.
- Linux: installs to `/usr/local/bin`.
- macOS artifacts are best effort from GoReleaser and are not separately validated by CI.
