<!-- Synced from https://raw.githubusercontent.com/Gentleman-Programming/gentle-ai/v1.26.5/internal/assets/skills/sdd-verify/references/report-format.md -->
<!-- Upstream commit: 5f73974b39ae2b9b525ef465b3642030c5f2ce6c; adapted for Jarvis/Hive runtime wording. -->
# SDD Verify Report Format

## Compliance Statuses

- ✅ `COMPLIANT`: covering test exists and passed.
- ❌ `FAILING`: covering test exists but failed.
- ❌ `UNTESTED`: no covering test found.
- ⚠️ `PARTIAL`: test passes but covers only part of the scenario.
- 🔍 `static-reviewed`: no test runner exists; the changed code was read against the scenario and the loaded project skills' rules. It is never a test-backed `COMPLIANT` or `PASS`.
- 👤 `operator-attested`: the scenario depends on an `[operator]` task the developer acknowledged in chat; it is developer attestation, never a test-backed PASS.
- ⏳ `pending-operator`: the scenario depends on an unchecked `[operator]` task; not CRITICAL, but archive waits for the acknowledgement.

## Report Template

~~~markdown
## Verification Report

**Change**: {change-name}
**Version**: {spec version or N/A}
**Mode**: {Strict TDD | Standard}
**Verification mode**: {runtime | static}

### Completeness
| Metric | Value |
|--------|-------|
| Tasks total | {N} |
| Tasks complete | {N} |
| Tasks incomplete | {N} |

### Build & Tests Execution
**Build**: ✅ Passed / ❌ Failed
```text
{build command and relevant output}
```

**Tests**: ✅ {N} passed / ❌ {N} failed / ⚠️ {N} skipped
```text
{test command and failure details}
```

**Coverage** (optional, warn-only): {N}% / threshold: {N}% → ✅ Above / ⚠️ Below / ➖ Not run

In static mode, replace this section with `**Runtime**: ➖ no test runner: static review only` and run no commands.

### Spec Compliance Matrix
| Requirement | Scenario | Test | Result |
|-------------|----------|------|--------|
| {REQ-01} | {Scenario} | `{file} > {test}` | ✅ COMPLIANT |
| {REQ-02} | {Scenario} | (none found) | ❌ UNTESTED |
| {REQ-03} | {Scenario} | static review of `{file}` | 🔍 static-reviewed |

**Compliance summary**: {N}/{total} scenarios compliant ({N} static-reviewed, {N} operator-attested)

### Correctness (Static Evidence)
| Requirement | Status | Notes |
|------------|--------|-------|
| {Req name} | ✅ Implemented | {brief note} |

### Coherence (Design)
| Decision | Followed? | Notes |
|----------|-----------|-------|
| {Decision} | ✅ Yes | |

### Issues Found
**CRITICAL**: {list or None}
**WARNING**: {list or None}
**SUGGESTION**: {list or None}

## Verdict

**{PASS / PASS WITH WARNINGS / FAIL}**

## Critical Findings

{0 or positive integer}

## Blockers

{None | blocker list}
~~~

## Active archive contract

The final three `##` sections are the only active archive decision fields. Emit them exactly once with the exact headings `## Verdict`, `## Critical Findings`, and `## Blockers`.

Archive-ready reports must use one of these exact verdict values:

- `**PASS — archive ready.**`
- `**PASS WITH WARNINGS — archive ready.**`

Emit `archive ready` only when `Critical Findings` is `0` and `Blockers` is `None`. `None`, `**None**`, and `_None_` normalize to the same no-blocker value. Do not emit `archive ready` for `FAIL`, nonzero critical findings, or a non-None blocker value.

The consumer ignores historical narrative and does not fall back to archived-style, YAML, or prose reports. Missing or duplicate active headings, missing fields, or a missing marker are invalid and must be regenerated with `sdd-verify` (`regenerate_with_sdd_verify`).

When Strict TDD is active in runtime mode, insert the TDD compliance and quality metrics sections from `../strict-tdd-verify.md`, plus the optional audit sections when they ran.
