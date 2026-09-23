# Issue 761 follow-up: recurring Windows startup flake

Objective: Identify and eliminate the cause of the intermittent Windows `TestOpenCodeHiveTemplate_CoalescesCreatedAndKeepsPromptIndependent` failure, without suppressing lifecycle assertions or publishing beta while master CI is red.

Evidence: PR #763 passed Windows twice at SHA `7ab603f7`, but master merge `d638506a` failed in run 35923094377: initial session-start request absent after 10.32 s, `child terminated: exit status 1; stderr:` empty. The helper cancels Node before reporting that status, so it does not prove a spontaneous Node exit. Issue #761 reopened, remains `status:approved`.

Scope: diagnostic-first changes to `jarvis-cli/internal/agent/opencode_lifecycle_test.go`. Change `.github/workflows/ci.yml` or the embedded template only if concrete evidence warrants it. Preserve distinct session evidence, coalescing, prompt independence, retry ordering and bounded waits. No generated local config, local builds, release trigger, or invented root cause.

Testing: strict TDD enabled by `openspec/config.yaml` (`testing.strict_tdd: true`). RED/GREEN using `cd jarvis-cli && go test ./internal/agent -run 'TestOpenCodeHiveTemplate_' -count=1`; verify `cd jarvis-cli && go test -race ./internal/agent -run 'TestOpenCodeHiveTemplate_' -count=1`, `cd jarvis-cli && go test ./...`, `cd jarvis-cli && go vet ./...`; Windows CI on PR is decisive for cross-platform behavior, but one green run does not prove a flaky gone.

Delivery: `ask-on-risk`; initial instrumented test-harness step forecast 25–45 authored diff lines, later root fix unknown until evidence. Base/first boundary `d638506a`. Route: delegated exploration completed (4+ files); single scoped worker writes test harness with strict TDD. If the accumulated change approaches 400 authored lines, decide chaining before further commits. Native assessment at work-unit boundary; follow selected review route. Branch `fix/issue-761-windows-startup-root-cause`. Beta release follows runbook only if final master CI is green.

## Tasks

- [ ] T1 (in progress; diagnostics only): Added behavioral RED tests for cancellation vs spontaneous exit and runner stage/HTTP request diagnostics in the Go harness, without altering the 10 s deadline. Writer reports focused, race, full CLI tests and vet green; independent verification in progress. No root cause is established yet.
- [ ] T2 (pending): Use observed stage evidence from Windows PR CI to identify a specific failure mechanism and implement the minimal fix with TDD; no assumption that diagnostics alone solve the flake.
- [ ] T3 (pending): Verify PR and master CI, then invoke Beta Release only on green master; validate tags, prerelease assets and installer commands per `docs/release-runbook.md`.

## Evidence and next step

Read-only mapper found cancellation obscures process-exit attribution; the source template's governance request and lifecycle delivery are each bounded to 1 s, while CI does not pin Node in Go jobs. These are hypotheses, not root-cause proof. T1 writer changed only `jarvis-cli/internal/agent/opencode_lifecycle_test.go` (+90/-8 lines), observed RED from missing stage diagnostics, then GREEN on focused/race/full CLI tests and vet. Parent ran the two new attribution tests 10 times (green). Independent verification pending. Next: review T1, commit diagnostic unit if sound and expose it to Windows CI to decide T2. Beta blocked.
