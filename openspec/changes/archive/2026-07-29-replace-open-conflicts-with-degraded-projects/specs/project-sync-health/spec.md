# Project Sync Health Specification

## Purpose

Define the canonical server-side projection used to classify projects and compute the administrative degraded-project KPI.

## Requirements

### Requirement: Classify projects from active users' latest attempts

The system MUST evaluate each project from the latest recorded sync attempt for each active portal user associated with that project, considering attempts across all devices. A project MUST be `degraded` when any qualifying latest attempt failed, and MUST be `healthy` when all qualifying latest attempts succeeded.

#### Scenario: A failed latest attempt degrades a project

- GIVEN an enabled user has a successful attempt followed by a failed attempt for the same project
- WHEN the project health projection is computed
- THEN the project status is `degraded`

#### Scenario: An older failure does not override a newer success

- GIVEN an enabled user has an older failed attempt and a newer successful attempt for the same project
- WHEN the projection is computed
- THEN that user's qualifying outcome is success and the project is not degraded because of the older failure

#### Scenario: Multiple active users are aggregated

- GIVEN a project has two enabled users whose latest qualifying outcomes are success and failure
- WHEN the projection is computed
- THEN the project status is `degraded`

### Requirement: Resolve equal timestamps deterministically without device identity

When attempts for the same project and active user have equal timestamps, the system MUST select the outcome using a stable canonical attempt ordering. The ordering MUST be independent of device identity and MUST produce the same result for the same records on every evaluation. Device identity MUST NOT be exposed in the product contract.

#### Scenario: Equal-timestamp attempts have a stable winner

- GIVEN two attempts for one project and active user share a timestamp and have different outcomes
- WHEN the projection is computed repeatedly
- THEN the same canonical attempt wins every time and the resulting project status is unchanged

#### Scenario: Device identity is absent from the product contract

- GIVEN health is returned for a project with attempts reported from multiple devices
- WHEN an API consumer reads the projection
- THEN it receives health status and aggregate values only, with no device identifier or device-based classification field

### Requirement: Exclude non-qualifying projects from KPI totals

Disabled users MUST be excluded immediately. Blocked or quarantined projects MUST be excluded. Projects with no recorded sync attempts MUST be excluded from both the degraded numerator and participating-project denominator; they MUST NOT be classified as healthy, degraded, or unknown for this KPI.

#### Scenario: Disabled users do not affect health

- GIVEN a project has only a failed latest attempt from a disabled user
- WHEN the projection is computed
- THEN that project is excluded from KPI participation

#### Scenario: Blocked projects do not participate

- GIVEN a project has qualifying attempts but is blocked or quarantined
- WHEN KPI totals are computed
- THEN the project is excluded from both totals

#### Scenario: Projects without attempts are excluded

- GIVEN a project has no recorded sync attempts
- WHEN KPI totals are computed
- THEN it is excluded from the denominator and degraded count

### Requirement: Use one projection for rows and totals

The API MUST derive per-project health rows and degraded-project totals from the same canonical projection, and MUST expose the degraded count as a numerator over the participating-project total. The API MUST NOT expose `conflicts.open` or an equivalent compatibility alias.

#### Scenario: Totals match project rows

- GIVEN the projection contains one degraded and two healthy participating projects
- WHEN the overview is returned
- THEN the degraded KPI is `1 / 3` and the rows use the same statuses
