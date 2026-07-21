# Project Derivation Specification

## Purpose

A single, typed, path-normalized project-name derivation used by both the CLI and the daemon, replacing two drifting implementations and the silent `"default"` fallback.

## Requirements

### Requirement: Single Derivation Source of Truth

The system MUST expose one shared `Derive(dir) (string, error)` implementation consumed by both `jarvis-cli` and `hive-daemon`, and MUST NOT maintain a second parallel derivation implementation for the same code path.

#### Scenario: Git remote name

- GIVEN a directory `dir` that is a git repo with `origin` remote `git@github.com:org/repo.git`
- WHEN `Derive(dir)` is called
- THEN it returns `("repo", nil)`

#### Scenario: No-remote basename fallback

- GIVEN a directory `dir` that is a git repo with no `origin` remote (or not a git repo), basename `myproj`
- WHEN `Derive(dir)` is called
- THEN it returns `("myproj", nil)`

### Requirement: No Ambient-CWD Derivation

The system MUST NOT run git commands or derive a project name from the process's ambient working directory when the caller-supplied directory is empty or absent.

#### Scenario: Empty directory yields typed error, never ambient cwd

- GIVEN `dir` is an empty string
- WHEN `Derive(dir)` is called
- THEN it returns `("", ErrEmptyDir)`
- AND no git command is executed against the process's ambient cwd
- AND the result is never `"default"`

### Requirement: Unresolvable Path Typed Error

The system MUST return a typed error, never a bare `"default"` string, when a supplied directory cannot be resolved or stat'd after normalization.

#### Scenario: Unresolvable path

- GIVEN `dir` is a path that does not exist and cannot be stat'd after normalization
- WHEN `Derive(dir)` is called
- THEN it returns `("", ErrPathUnresolvable)`
- AND the returned name is never `"default"`

### Requirement: Cross-Platform Path Normalization Before Stat

The system MUST normalize Windows and WSL path forms — `C:\...`, `/mnt/c/...`, UNC `\\wsl$\...`, and backslash-separated paths — into a form resolvable on the current runtime BEFORE calling `os.Stat`, so a path produced on one OS can still be derived when received by a daemon running on another.

#### Scenario: Windows-style path normalizes on a WSL daemon

- GIVEN a WSL-hosted daemon receives directory `C:\Users\dev\project`
- WHEN `Derive(dir)` is called
- THEN the path is normalized to `/mnt/c/Users/dev/project` before `os.Stat`
- AND if that path exists, derivation proceeds and returns a valid project name

#### Scenario: UNC WSL path normalizes correctly

- GIVEN directory `\\wsl$\Ubuntu\home\dev\project`
- WHEN `Derive(dir)` is called
- THEN the path is normalized to its POSIX equivalent before `os.Stat`
- AND derivation proceeds without a stat failure caused purely by path form

#### Scenario: Backslash-separated path on POSIX runtime

- GIVEN directory `project\subdir` supplied with backslashes on a POSIX runtime
- WHEN `Derive(dir)` is called
- THEN backslashes are normalized to the runtime path separator before `os.Stat`

### Requirement: Normalization Gating by Runtime

The system MUST gate Windows/WSL path rewriting so a native-Windows daemon does not incorrectly rewrite legitimate native Windows paths (e.g. via `GOOS` or a WSL marker such as `/proc/version`, or by only translating when the untranslated path fails to stat).

#### Scenario: Native Windows daemon does not mis-rewrite

- GIVEN the daemon runs natively on Windows (`GOOS=windows`, no WSL marker present)
- WHEN `Derive(dir)` is called with a valid native Windows path `C:\Users\dev\project`
- THEN the path is used as-is (or only rewritten after a failed native stat) and is NOT mistranslated into a `/mnt/c/...` form that would fail to resolve

### Requirement: Fail-Safe Hook Degradation

Hooks that consume derivation MUST always emit valid JSON output, even when derivation returns a typed error; a derivation error MUST degrade to a safe path and MUST NOT abort the hook.

#### Scenario: Derivation error does not abort hook

- GIVEN a hook calls the derivation function and it returns a typed error
- WHEN the hook completes
- THEN the hook still emits valid JSON output
- AND the hook process does not crash or exit non-zero due to the derivation error alone
