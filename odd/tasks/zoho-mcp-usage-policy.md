# Zoho MCP usage policy

## Objective
Forbid generated agents from using Zoho MCP servers unless the user explicitly asks for them by name in the current request. Issue #782.

## Scope and constraints
Source only: `jarvis-cli/internal/config/layer1.md` and its tests. Layer1 renders into both Claude `CLAUDE.md` and OpenCode `AGENTS.md`. Branch `feat/782-zoho-mcp-usage-policy`. No permission-based enforcement (server names are user-chosen). No builds, no local developer configuration changes.

## Tasks
- [x] T1 (done): Add the Zoho MCP usage policy to Layer1 test-first; verify it reaches Layer1 of both agents and never Layer2. Commit `5f57df80`; RED observed, `go test ./...` and `go vet ./...` green; native review `review-e69e69871905aa1d` approved and acknowledged (one non-blocking suggestion on short test anchors).
- [x] T2 (done): Verify focused tests and static checks; publish PR closing #782.

## Acceptance and checks
- Policy phrases present in `Layer1Content()`; Layer Boundary Rule remains the final section.
- Policy lands in the Layer1 region of both rendered instruction files.
- `go test ./internal/config/...` and `go vet ./...` pass.

## Evidence
- Issue: #782 (`status:approved`, `type:feature`).
- T1 commit: `5f57df80`.
- T2: this commit; PR closes #782.
