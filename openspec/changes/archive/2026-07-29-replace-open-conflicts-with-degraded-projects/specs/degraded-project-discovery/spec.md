# Degraded Project Discovery Specification

## Purpose

Define the Dashboard presentation and URL-backed discovery flow for participating projects classified as degraded.

## Requirements

### Requirement: Present the degraded-project KPI

The Dashboard MUST replace the `OPEN CONFLICTS` KPI with the exact label `DEGRADED PROJECTS` and render its value as `N / total`, using the canonical API numerator and participating-project denominator. It MUST NOT present historical `sync_conflict` events as open work.

#### Scenario: KPI displays canonical totals

- GIVEN the API reports two degraded projects among five participating projects
- WHEN the Overview renders
- THEN it displays `DEGRADED PROJECTS` with `2 / 5`

#### Scenario: Historical events retain event semantics

- GIVEN historical `sync_conflict` audit records exist
- WHEN audit history is displayed
- THEN the records remain queryable and are described as events, not open conflicts

### Requirement: Support URL-backed degraded filtering

The Dashboard MUST support the canonical route `/dashboard/projects?health=degraded`. The filter MUST be restored on direct navigation, refresh, shared URLs, and browser back/forward navigation. Filtered results MUST be inspectable and the view MUST provide an explicit empty state when no projects are degraded.

#### Scenario: Direct URL loads filtered projects

- GIVEN the browser opens `/dashboard/projects?health=degraded`
- WHEN the Projects view loads
- THEN only degraded projects are shown and the degraded filter is visibly active

#### Scenario: Browser navigation restores filter state

- GIVEN a user navigates between filtered and unfiltered project URLs
- WHEN the user uses browser back or forward
- THEN the Projects view matches the URL's `health` query state

#### Scenario: No degraded projects has an empty state

- GIVEN the API returns zero degraded participating projects
- WHEN the degraded route is opened
- THEN the view shows an explicit empty state and no project rows

### Requirement: Preserve access boundaries and accessible inspection

The Dashboard MUST preserve existing authorization boundaries for project data. Filter controls and result states MUST remain accessible and MUST NOT rely on nested interactive elements.

#### Scenario: Unauthorized data is not revealed by filtering

- GIVEN a user lacks access to a project
- WHEN the degraded filter is applied
- THEN that project is not returned or rendered
