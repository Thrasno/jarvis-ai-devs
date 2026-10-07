# OpenCode permissive permission defaults

## Objective
Minimize permission prompts in every directory, deny secret reads by default, and ask for known destructive operations without installer questions.

## Scope and constraints
Source configuration only; isolated worktree `../jarvis-dev-opencode-permissions`, branch `fix/opencode-permission-defaults`, base `master` at `5ddf342f`. Preserve explicit user permission policies and ordering, including scalar and object policies. Preserve read-only agent edit/task boundaries. User explicitly authorized commit, push, PR, merge and beta release. No local builds or developer configuration changes. Pattern-based shell permissions are not a sandbox and cannot detect arbitrary destructive scripts. Existing restricted installations must not be silently overridden.

## Tasks
- [x] T1 (done): Implement permissive external-directory and agent shell defaults with known destructive-command asks; add regression tests test-first.
- [x] T2 (done): Independently verify focused tests/static checks, inspect source diff, and run native review if enabled.

- [ ] T3 (in_progress): Publish approved issue and PR; integrate latest remote master and verify CI.
- [ ] T4 (pending): Merge green PR and publish fixed beta channel; verify tags and assets.

## Acceptance and checks
- Missing external_directory defaults allow any directory; existing explicit external_directory stays intact.
- Global and generated agent shell defaults allow ordinary actions and ask for known destructive commands; preserve role-specific edit/task boundaries.
- Existing secret read deny rules remain intact; explicit existing policies and order survive repeated render/merge.
- Regression tests observe RED before production edits and GREEN afterwards.
- Focused Go tests and go vet in jarvis-cli; no explicit builds.

## Evidence
Issue #777 created and read back with status:approved under explicit maintainer authorization. Source work-unit commit 7a59b5da; integration commit 603b7278 preserves current master retirement of legacy review agents. Integrated source diff: seven files, 296 additions/26 deletions. Worker uncached agent/agentapply/sync tests and vet passed. Updated native review review-8641953c15d759fe approved and acknowledged; warning: bash allows writes even for edit-denied agents (not a sandbox). Independent full CLI tests/vet pending.

Worker completed seven source/test/doc files (300 additions, 30 deletions). Observed RED then GREEN; focused permission regressions, agent/agentapply/sync tests and go vet passed. git diff --check passed. Original worktree unchanged. Commits and remote delivery now explicitly authorized. Live OpenCode validation not run. Independent verifier found no blockers: scoped tests passed (cached), go vet and diff check passed. Parent uncached `go test ./internal/agent -run 'TestOpenCodePermission' -count=1` passed. Native review review-532ac573ca33503a approved, exact acknowledgement consumed authority. Non-blocking risk advisory at opencode.go:239 and stale coverage_test.go template fixture noted; no correction offered. ASSESS unavailable due to intended-untracked declaration; conservative independent verification fulfilled. Full suite, builds and live OpenCode validation not run.

## Next step
Create approved issue under exact user authorization, commit changes, integrate public/master, run verification and create PR. Migration not added; existing installation can explicitly remove permission policies before replay using updated beta.
