# hive-daemon — Project Instructions

## Development Rules

- NEVER build after changes (use existing test commands instead)
- Run tests: `go test ./...` from `hive-daemon/`
- Lint: `golangci-lint run ./...` from `hive-daemon/`
- Installer-managed binary path: `/usr/local/bin/hive-daemon` (Unix) or `%LOCALAPPDATA%\Programs\jarvis\hive-daemon.exe` (Windows)
- jarvis-cli runtime resolution order: installer-managed path → `PATH` → legacy `$GOPATH/bin` / `~/go/bin`
- DB in production: `~/.jarvis/memory.db`
- DB in tests: `HIVE_DB_PATH` env var (temp dir per test)
