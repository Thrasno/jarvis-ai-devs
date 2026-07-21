# Verify Report: fix-452-unified-project-derivation

**Change**: fix-452-unified-project-derivation
**Mode**: Strict TDD, hybrid artifact store
**Branch verified**: fix/452-pr4-loud-failure (full chain: PR1+PR2+PR3+PR4)
**Verdict**: PASS

## Completeness

- Tasks: 28/28 checked in tasks.md across 4 phases (PR1 hivederive module, PR2 self-heal, PR3 marker decoupling, PR4 loud failure + docs). Spot-checked apply-progress claims against actual git diff: `git diff --stat 8fc8cf23 fix/452-pr4-loud-failure` shows exactly the 10 files (+ tasks.md = 11) and the reported +284/-28 shape for PR4. Claims verified truthful.
- Ghost-project compensation (commit f880d4cd) confirmed reverted at 630fdfa9 — accepted deviation, follow-up tracked as #453. Self-heal spec scenarios (session-registration-self-heal) all pass without it per live test run.

## Test Execution (real, this session)

| Module | `go test ./... -count=1` | `go vet ./...` |
|---|---|---|
| hivederive | PASS (ok, 0.004s) | clean |
| jarvis-cli | PASS (23 packages ok) | clean |
| hive-daemon | PASS (11 packages ok) | clean |

`gofmt -l` on all three module trees: pre-existing unformatted files unrelated to this change (agent/skills/sync test helpers); none of the PR1–PR4 touched files appear in the gofmt-dirty list — consistent with the "gofmt clean" claims in apply-progress.

`doc_claim_test.go` (root `jarvis_test` package in `jarvis-cli`): `TestHiveProtocol_NoFalseRegistrationClaim` and `TestHiveSkill_NoFalseRegistrationClaim` both PASS — real content assertions (absence of false claim + presence of "SessionStart hook" text), not tautologies.

## Spec Compliance Matrix

### project-derivation (7 requirements)

| Requirement / Scenario | Test | Result |
|---|---|---|
| Single Derivation Source of Truth — git remote name | `hivederive.TestDerive` | PASS |
| Single Derivation Source of Truth — no-remote basename | `hivederive.TestDerive` | PASS |
| No Ambient-CWD Derivation — empty dir → ErrEmptyDir | `hivederive.TestDerive` | PASS |
| Unresolvable Path Typed Error | `hivederive.TestDerive` | PASS |
| Cross-Platform Path Normalization (Windows/WSL/UNC/backslash) | `hivederive.TestNormalizePath_WSLGate`, `TestDerive_NormalizesBeforeStatOnWSL` | PASS |
| Normalization Gating by Runtime (native Windows/Linux passthrough) | `TestNormalizePath_NativeWindowsPassthrough`, `TestNormalizePath_NativeLinuxPassthrough` | PASS |
| Fail-Safe Hook Degradation | `hook.TestRunSessionStart_DerivationError_FailsSafe`, `TestRunPromptSubmit_DerivationError_FailsSafe` | PASS |

### session-registration-self-heal (6 requirements)

| Requirement / Scenario | Test | Result |
|---|---|---|
| Directory Parameter accepted | `TestMemSessionSummary_ProjectUnknown_WithGitDirectory_SelfHeals` | PASS |
| Self-Heal on project_unknown (success + no-directory-still-fails) | `TestMemSessionSummary_ProjectUnknown_WithGitDirectory_SelfHeals`, `TestMemSessionSummary_ProjectUnknown_NoDirectory_StillFails` | PASS |
| Idempotent Registration | `TestMemSessionSummary_SelfHeal_Idempotent_SecondCallAlreadyKnown` | PASS |
| Filesystem-Derived Name Wins on Conflict | `TestMemSessionSummary_DerivedName_WinsOverStaleCallerProject` | PASS |
| Never Register "default" (typed-error path + literal-basename-"default" path) | `TestMemSessionSummary_UnderivableDirectory_DoesNotSelfHeal`, `TestMemSessionSummary_DirectoryBasenameDefault_Refused` | PASS |
| mem_save Escape Behavior Unchanged | `TestMemSave_ProvenanceEscape_Parity_Unchanged` | PASS |

### hook-marker-lifecycle (7 requirements)

| Requirement / Scenario | Test | Result |
|---|---|---|
| Distinct SessionStart Marker | `TestRunSessionStart_WritesSessionStartMarkerOnly` | PASS |
| First-Prompt Marker Owned Exclusively by RunPromptSubmit | `TestRunSessionStart_WritesSessionStartMarkerOnly`, `TestFirstActionNudge_FiresAfterSessionStart` | PASS |
| FIRST ACTION Nudge Fires Once (fires + does-not-refire) | `TestFirstActionNudge_FiresAfterSessionStart`, `TestRunPromptSubmit_SubsequentPrompt_ReturnsEmpty` | PASS |
| Compaction Path Unaffected | `TestCompaction_DoesNotRetriggerNudge` | PASS |
| Registration Failures Are Logged, Never Swallowed | `TestRunSessionStart_PostFailure_LoggedNeverSwallowed`, `TestRunSessionStart_PostSucceeds_NoFailureLog` | PASS |
| No Fallback to "default" Registration | `TestPostSessions_UnresolvableDirectory_LogsRefusalNeverRegistersDefault` | PASS |
| Documentation Reflects Actual Registration Behavior | `TestHiveProtocol_NoFalseRegistrationClaim`, `TestHiveSkill_NoFalseRegistrationClaim` | PASS |

**Total**: 20 requirements, 20 pass with covering, behavior-asserting tests (spot-read: no tautologies, no ghost loops, no assertion-without-production-call patterns found in the reviewed test files).

## Issue #452 Acceptance Criteria (via `gh issue view 452`)

| Criterion | Status | Evidence |
|---|---|---|
| Fresh no-git project registers automatically at SessionStart; mem_save succeeds without mem_session_start | MET | hivederive basename fallback + SessionStart wiring (PR1); pre-existing mem_save escape untouched |
| Empty/unusable directory never derives "default" nor leaks ambient repo name; failure logged | MET | `ErrEmptyDir`/`ErrPathUnresolvable` typed errors (PR1) + daemon refusal log (PR4, `TestPostSessions_UnresolvableDirectory_LogsRefusalNeverRegistersDefault`) |
| mem_session_summary accepts directory and self-registers on project_unknown | MET | PR2, full self-heal test suite |
| Derivation logic in one shared package with parity tests (git-remote, no-remote basename, empty-dir, WSL paths) | MET | `hivederive` module, `derive_test.go` + `normalize_test.go` |
| Visible FIRST ACTION nudge fires on first user prompt of a real session | MET | PR3, `TestFirstActionNudge_FiresAfterSessionStart` |
| Docs no longer claim mem_context registers the project | MET | PR4, `doc_claim_test.go` regression test |

All 6 acceptance criteria satisfied by code + passing tests.

## Design Coherence

One documented, disclosed deviation from design.md: `memSessionSummaryHandler` calls `hivederive.Derive` directly instead of the design's `ResolveEffectiveProject`, because that helper does not derive when a project name is supplied and therefore cannot satisfy "derived name wins on conflict." Justification is sound and the escape guard (`derived && project != "default"`) is preserved verbatim, matching `mem_save` parity — WARNING-level, does not break any spec requirement.

## Issues

**CRITICAL**: None.

**WARNING**:
- Design deviation on `ResolveEffectiveProject` vs. direct `hivederive.Derive` in `memSessionSummaryHandler` (justified, spec-compliant, documented in tasks.md 2.6).
- PR1 authored diff landed at 822 lines vs. forecast 500–750 (documented in tasks.md 1.4 as a known, already-flagged risk from the apply phase).

**SUGGESTION**:
- Ghost-project compensation follow-up is tracked as external issue #453; no action needed in this change.

## Final Verdict: PASS
