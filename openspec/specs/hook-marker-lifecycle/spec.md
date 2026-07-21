# Hook Marker Lifecycle Specification

## Purpose

Decouple the SessionStart marker from the first-prompt marker so the FIRST ACTION nudge reliably fires once per real session, and observable failure signals for registration are honest rather than swallowed.

## Requirements

### Requirement: Distinct SessionStart Marker

`RunSessionStart` MUST write its own dedicated marker (e.g. `markerSessionStart`), distinct from the first-prompt marker.

#### Scenario: SessionStart writes its own marker

- GIVEN a new session starts and `RunSessionStart` executes
- WHEN the hook completes
- THEN a `markerSessionStart` marker is created for the session
- AND the first-prompt marker is NOT created by this hook

### Requirement: First-Prompt Marker Owned Exclusively by RunPromptSubmit

The first-prompt marker MUST be created exclusively by `RunPromptSubmit` using an exclusive-create operation; no other hook MUST create or pre-populate it.

#### Scenario: First user prompt creates the marker exclusively

- GIVEN a session where `RunSessionStart` has already run (creating only `markerSessionStart`)
- WHEN the user submits their first prompt and `RunPromptSubmit` executes
- THEN the first-prompt marker does not yet exist
- AND `RunPromptSubmit`'s exclusive create succeeds (`created == true`)

### Requirement: FIRST ACTION Nudge Fires Once Per Real Session

The FIRST ACTION nudge (`FirstPromptSystemMessage`) MUST fire exactly once, on the first user prompt of a real session.

#### Scenario: Nudge fires on first prompt

- GIVEN a fresh session with `markerSessionStart` set but no first-prompt marker
- WHEN the user's first prompt is submitted
- THEN `RunPromptSubmit` observes `created == true` for the first-prompt marker
- AND the FIRST ACTION nudge message is emitted

#### Scenario: Nudge does not fire again on subsequent prompts

- GIVEN the first-prompt marker was already created by an earlier prompt in the same session
- WHEN a subsequent prompt is submitted
- THEN `RunPromptSubmit` observes `created == false`
- AND the FIRST ACTION nudge is not emitted again

### Requirement: Compaction Path Unaffected

Marker decoupling MUST NOT alter behavior across a compaction event: a post-compaction continuation of the same session MUST NOT be treated as a fresh session requiring a duplicate nudge.

#### Scenario: Compaction does not re-trigger the nudge

- GIVEN a session already had its first-prompt marker created before a compaction event
- WHEN the session continues after compaction and further prompts are submitted
- THEN the first-prompt marker still exists
- AND the FIRST ACTION nudge is not emitted again due to compaction alone

### Requirement: Registration Failures Are Logged, Never Swallowed

`PostSessionStart` and related registration failures MUST be logged with a reason via the existing stderr logger, and MUST NOT be silently discarded.

#### Scenario: PostSessionStart failure is logged

- GIVEN `PostSessionStart` returns an error during `RunSessionStart`
- WHEN the hook handles the error
- THEN the error is logged with its reason via the existing stderr logger
- AND the hook still completes and emits valid JSON output (fail-safe)

### Requirement: No Fallback to "default" Registration

The registration path MUST NOT fall back to registering `"default"` as a project under any failure condition.

#### Scenario: Registration failure does not fall back to default

- GIVEN project registration fails for any reason during a hook or daemon request
- WHEN the failure is handled
- THEN no project named `"default"` is registered as a fallback

### Requirement: Documentation Reflects Actual Registration Behavior

Embedded documentation sources (`embed/hive-protocol.md`, `embed/skills/hive/SKILL.md`) MUST NOT claim that `mem_context` registers the current project, since `mem_context` is read-only.

#### Scenario: Docs no longer claim mem_context registers projects

- GIVEN the embed documentation sources are inspected after this change
- WHEN searched for claims about `mem_context` registering a project
- THEN no such claim is present
- AND the docs accurately describe that registration happens via `SessionStart` / self-healing writes (`mem_save`, `mem_session_summary`)
