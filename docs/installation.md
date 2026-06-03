# Installation

## Latest (production)

**Windows**
```powershell
irm https://raw.githubusercontent.com/Thrasno/jarvis-ai-devs/master/scripts/install.ps1 | iex
```

**Linux**
```bash
curl -sSL https://raw.githubusercontent.com/Thrasno/jarvis-ai-devs/master/scripts/install.sh | bash
```

## Beta (v0.0.1-beta)

**Windows**
```powershell
$env:JARVIS_INSTALL_VERSION = "v0.0.1-beta"
irm https://raw.githubusercontent.com/Thrasno/jarvis-ai-devs/master/scripts/install.ps1 | iex
```

**Linux**
```bash
JARVIS_INSTALL_VERSION=v0.0.1-beta \
curl -sSL https://raw.githubusercontent.com/Thrasno/jarvis-ai-devs/master/scripts/install.sh | bash
```

## Notes

- Without `JARVIS_INSTALL_VERSION`, the script installs the latest published release.
- Set `JARVIS_INSTALL_REPO=owner/repo` to fetch artifacts from a different GitHub repository.
- Windows: installs to `%LOCALAPPDATA%\Programs\jarvis` and adds it to the user PATH automatically.
- Linux: installs to `/usr/local/bin`.
