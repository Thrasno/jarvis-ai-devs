# Syncing User Visibility Specification

## Requirements

### Requirement: Calculate the active-user synchronization KPI

The system MUST expose `SYNCING USERS · 24H` as qualifying active users over all active users. Each user's latest completed attempt wins; later failure overrides earlier success. Only `ended_at` counts in inclusive UTC `[now - 24 hours, now]`; future timestamps excluded.

#### Scenario: Latest failure wins
- GIVEN an active user has an earlier success and later failed completed attempt
- WHEN calculated
- THEN the user is excluded

#### Scenario: Inclusive boundary
- GIVEN the latest completed attempt ended exactly 24 hours before UTC `now` and succeeded
- WHEN calculated
- THEN the user qualifies

#### Scenario: Future completion
- GIVEN the latest completed attempt ended after captured UTC `now`
- WHEN calculated
- THEN the user does not qualify

#### Scenario: Empty denominator
- GIVEN there are no active users
- WHEN rendered
- THEN it renders exactly `0 / 0` without a percentage

### Requirement: Use canonical identity and bounded queries

Visibility MUST use retained attempts whose canonical `portal_user_id` matches a user. It MUST NOT infer identity from daemon, device, email, source, or audit actor data. Queries MUST be fixed-number set-based, independent of user count.

#### Scenario: Unresolved identity
- GIVEN an attempt lacks `portal_user_id` but has device or email data
- WHEN projected
- THEN the attempt contributes to neither projection

#### Scenario: Fixed query count
- GIVEN User Management returns any number of users
- WHEN loaded
- THEN repository query count does not grow per user

### Requirement: Project complete User Management sync context

User Management MUST retain every user, including inactive users, and expose account status separately from sync status: `Last 24h`, `Inactive`, or `Never`. For active accounts, mapping MUST be deterministic: `Never` if no successful attempt exists in 90-day history; otherwise `Last 24h` iff the latest completed attempt succeeded with `ended_at` in inclusive UTC `[now - 24 hours, now]`; otherwise `Inactive` when a completed attempt exists but that attempt failed, is outside the window, or is future. Inactive account status forces sync status `Inactive`. Attempts without `ended_at` drive neither status nor `Last sync`. `Last sync` is the latest successful `ended_at`, independently of status. A projection failure is operational `unknown`, rendered `Unavailable`; it does not redefine the three successful-projection statuses.

#### Scenario: Inactive account precedence
- GIVEN an inactive user has sync history
- WHEN returned
- THEN the user remains visible, account status is inactive, and sync status `Inactive`

#### Scenario: Older success and newer failure
- GIVEN a user has an older success and newer failed completed attempt
- WHEN returned
- THEN `Last sync` is the older success and status `Inactive`

#### Scenario: Old retained success
- GIVEN an active user has a successful attempt older than 24 hours but retained within 90 days
- WHEN returned
- THEN sync status is `Inactive` and `Last sync` is that timestamp

#### Scenario: No retained success
- GIVEN an active user has no successful attempt in retained 90-day history
- WHEN returned
- THEN sync status and `Last sync` are `Never`

#### Scenario: Recent successful completion
- GIVEN the latest completed canonical attempt succeeded exactly 24 hours before UTC `now`
- WHEN returned
- THEN sync status is `Last 24h` and `Last sync` is its `ended_at`

#### Scenario: Incomplete attempt
- GIVEN a newer attempt has no `ended_at` and an older retained success exists
- WHEN returned
- THEN status and `Last sync` use the completed success only

### Requirement: Preserve authorization and event boundaries

The KPI and sync context MUST remain admin-authorized. The system MUST NOT alter account activation, roles, or Audit Log semantics, or expose event details in User Management.

#### Scenario: Non-admin access
- GIVEN an authenticated member requests either projection
- WHEN authorized
- THEN access is denied and no projection data is returned

#### Scenario: Summary, not events
- GIVEN a user has multiple sync attempts
- WHEN displayed
- THEN only defined sync summary fields are shown
