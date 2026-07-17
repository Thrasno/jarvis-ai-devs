# Delta for Memory Delete and Restore Revalidation

## ADDED Requirements

### Requirement: Immutable validation lineage

This validation-only successor MUST treat the predecessor artifacts, implementation, tests, review receipts, and historical report as immutable evidence. It MUST NOT regenerate code or tests, rewrite predecessor artifacts, or alter their classification.

#### Scenario: Predecessor remains unchanged
- GIVEN the predecessor change and its historical FAIL report exist
- WHEN this successor is reviewed or verified
- THEN those artifacts remain byte-for-byte unchanged and are referenced only as evidence.

### Requirement: Native dispatcher and evidence-only review

Every executed phase MUST be authorized by the Gentle AI 2.1.6 native dispatcher using `nextRecommended` and `dependencies`. Review and verify MUST use the native dispatcher and produce receipts. Current-tree implementation MUST be inspected as evidence only; no code regeneration or implementation replay is permitted.

#### Scenario: Dispatcher blocks a phase
- GIVEN native status does not authorize the next phase or required binding
- WHEN the phase is attempted
- THEN the phase stops and records the native status without bypassing it.

#### Scenario: Authorized review and verification
- GIVEN valid dispatcher authorization and review authority
- WHEN review and verify run
- THEN both use the native dispatcher and retain receipts tied to this successor.

### Requirement: Issue-438 verification coverage

Independent verification MUST cover the predecessor semantics: delete/restore behavior, explicit mutation acknowledgements, deleted-only tombstone lookup for restore, terminal mutation rejection, canonical TUI behavior, and MCP least privilege.

#### Scenario: Complete focused evidence
- GIVEN the current implementation and available focused suites
- WHEN verification exercises issue-438 behavior
- THEN it proves explicit acknowledgements, restore tombstone lookup, terminal rejection, TUI behavior, and absence of destructive MCP access.

### Requirement: Honest failure and stop boundaries

Known unrelated environment or pre-existing failures MUST be reported separately and MUST NOT be claimed green. Any product or test defect discovered MUST fail verification and stop this change; remediation is out of scope.

#### Scenario: Unrelated limitation
- GIVEN Windows symlink/persona or rootless-Docker limitations affect available checks
- WHEN results are recorded
- THEN each limitation is reported as not green and is not attributed to issue-438.

#### Scenario: Product or test defect
- GIVEN verification finds a defect attributable to product behavior or tests
- WHEN the defect is confirmed
- THEN verification fails and stops without editing or repairing code or tests.

### Requirement: Zero attributable production and test diff

Success MUST require no production or test file diff attributable to this successor. Only validation artifacts may be added under this change, while predecessor history and semantics remain unchanged.

#### Scenario: Clean validation boundary
- GIVEN verification completes with acceptable evidence and separately reported limitations
- WHEN the final diff is inspected
- THEN no production or test changes are attributable to this successor and closure is allowed only on that basis.
