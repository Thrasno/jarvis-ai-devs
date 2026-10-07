# OpenCode permissive permission defaults

## Objective
Minimize permission prompts in every directory, deny secret reads by default, and ask for known destructive operations without installer questions.

## Scope and constraints
Source configuration only; isolated worktree `../jarvis-dev-opencode-permissions`, branch `fix/opencode-permission-defaults`, base `master` at `5ddf342f`. Preserve explicit user permission policies and ordering, including scalar and object policies. Preserve read-only agent edit/task boundaries. User explicitly authorized commit, push, PR, merge and beta release. No local builds or developer configuration changes. Pattern-based shell permissions are not a sandbox and cannot detect arbitrary destructive scripts. Existing restricted installations must not be silently overridden.

## Tasks
- [x] T1 (done): Implement permissive external-directory and agent shell defaults with known destructive-command asks; add regression tests test-first.
- [x] T2 (done): Independently verify focused tests/static checks, inspect source diff, and run native review if enabled.

- [x] T3 (done): Publish approved issue and PR; integrate latest remote master and verify CI.
- [x] T4 (done): Merge green PR and publish fixed beta channel; verify tags and assets.

## Acceptance and checks
- Missing external_directory defaults allow any directory; existing explicit external_directory stays intact.
- Global and generated agent shell defaults allow ordinary actions and ask for known destructive commands; preserve role-specific edit/task boundaries.
- Existing secret read deny rules remain intact; explicit existing policies and order survive repeated render/merge.
- Regression tests observe RED before production edits and GREEN afterwards.
- Focused Go tests and go vet in jarvis-cli; no explicit builds.

## Evidence
PR #778 opened with type:feature, closing approved issue #777. Full CLI go test ./... -count=1 and go vet ./... passed. PR head f5325c0f unchanged; 14/14 CI checks success in runs 37652114181 and 37652207519. No required checks reported by gh; all available checks green. Published work-unit commits: 7a59b5da (behavior), 603b7278 (master integration), f5325c0f (ODD evidence).
Issue #777 created and read back with status:approved under explicit maintainer authorization. Source work-unit commit 7a59b5da; integration commit 603b7278 preserves current master retirement of legacy review agents. Integrated source diff: seven files, 296 additions/26 deletions. Worker uncached agent/agentapply/sync tests and vet passed. Updated native review review-8641953c15d759fe approved and acknowledged; warning: bash allows writes even for edit-denied agents (not a sandbox). Independent full CLI tests/vet passed.

Worker completed seven source/test/doc files (300 additions, 30 deletions). Observed RED then GREEN; focused permission regressions, agent/agentapply/sync tests and go vet passed. git diff --check passed. Original worktree unchanged. Commits and remote delivery now explicitly authorized. Live OpenCode validation not run. Independent verifier found no blockers: scoped tests passed (cached), go vet and diff check passed. Parent uncached `go test ./internal/agent -run 'TestOpenCodePermission' -count=1` passed. Native review review-532ac573ca33503a approved, exact acknowledgement consumed authority. Non-blocking risk advisory at opencode.go:239 and stale coverage_test.go template fixture noted; no correction offered. ASSESS unavailable due to intended-untracked declaration; conservative independent verification fulfilled. Full suite, builds and live OpenCode validation not run.

## Next step
All delivery tasks complete. Test PC: install updated beta, back up OpenCode configuration, remove only global/generated-agent permission policies if intentionally adopting new defaults, replay `jarvis sync`, then restart OpenCode. No local configuration was changed by this session.

## Final delivery evidence
PR #778 merged as 3c162aa4de5197d8ff0a9c69dbbe536ae840d7c6. Beta Release run 37653242302 succeeded on that master SHA. Public beta release published 2026-10-07T16:36:49Z, prerelease=true/draft=false. master, beta and v0.0.1-beta align at 3c162aa4. All 19 assets fresh (6 jarvis, 6 hive-daemon, 6 hive, checksums); representative Linux/Windows downloads HTTP 200. Both installer sources resolve beta asset names correctly. macOS runtime and live OpenCode remain unverified. Release https://github.com/Thrasno/jarvis-ai-devs/releases/tag/beta . Migration not added.
