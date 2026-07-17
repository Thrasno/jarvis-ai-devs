# Memory Delete and Restore Final Validation Specification

## Purpose

Define a validation-only successor for issue 438 that preserves historical evidence, proves native authorization, and produces one final auditable verification path.

## Requirements

### Requirement: Immutable Historical Artifacts

The validation process MUST treat issue-438 implementation, tests, prior artifacts, and prior reports as immutable historical evidence.

#### Scenario: Historical evidence remains unchanged

- GIVEN predecessor implementation, tests, and SDD artifacts exist
- WHEN final validation is prepared
- THEN those artifacts MUST remain byte-for-byte unchanged

### Requirement: Successor-Only Scope

The successor MUST make zero production or test edits and MUST NOT replay implementation or remediation.

#### Scenario: Validation changes are limited to successor evidence

- GIVEN the current tree contains issue-438 implementation and tests
- WHEN this validation change is completed
- THEN no production or test path MUST be edited
- AND only successor SDD evidence MAY be added

### Requirement: Native Review and Binding Authorization

Verification MUST use native `nextRecommended` and `dependencies` status, with successor-bound review and binding authorization completed before verification; archive MUST use the native archive gate only.

#### Scenario: Native authorization precedes verification

- GIVEN native status exposes successor dependencies and next-phase authorization
- WHEN review and binding are evaluated
- THEN verification MAY begin only after successor-bound authorization is complete

#### Scenario: Archive is withheld without native authorization

- GIVEN verification has completed but native status does not authorize archive
- WHEN the workflow evaluates the next phase
- THEN archive MUST NOT run
- AND the workflow MUST stop without bypassing native status

### Requirement: Focused Evidence and Honest Failure Classification

The verification evidence MUST identify issue-438-focused and touched paths, distinguish unrelated broad-suite failures, and report issue #441 honestly without classifying either as an issue-438 success or defect.

#### Scenario: Broad and unrelated failures are separated

- GIVEN focused issue-438 evidence and unrelated suite failures are observed
- WHEN the verification report classifies results
- THEN touched evidence MUST be reported separately from broad failures and issue #441
- AND no unrelated failure MAY be claimed green or attributed to issue 438

### Requirement: Write-Once Exact Verification and Native Archive

The verification report MUST be written exactly once, remain unchanged thereafter, and state exact completeness of 5 requirements and 7 scenarios before any native archive action.

#### Scenario: Exact completeness precedes archive

- GIVEN the successor spec has five requirement headings and seven scenario headings
- WHEN verification writes its report
- THEN the report MUST record exactly 5/5 requirements and 7/7 scenarios
- AND native archive MAY occur only afterward when authorized

#### Scenario: Verification report is immutable

- GIVEN the verification report has been written
- WHEN later status or archive processing occurs
- THEN the report MUST NOT be edited, regenerated, or replaced
