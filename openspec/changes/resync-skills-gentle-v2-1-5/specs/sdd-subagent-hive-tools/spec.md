# Delta for SDD Subagent Hive Tools

## MODIFIED Requirements

### Requirement: Claude Code SDD Hive Tool Grants

Generated Claude Code SDD phase agents MUST explicitly allow the Hive MCP tools required to retrieve and persist SDD artifacts in `hive`, `hybrid`, `openspec`, or `none` store modes, while preserving phase-specific filesystem permissions. The four-store contract MUST remain exactly `hive | openspec | hybrid | none`.
(Previously: grants covered Hive or hybrid persistence without stating the complete four-store contract.)

#### Scenario: Generated Claude agents include bounded Hive access
- GIVEN Claude Code SDD phase agents are generated
- WHEN an agent definition is rendered
- THEN required Hive access is present where the selected store requires it
- AND memory access does not expand filesystem permissions

### Requirement: OpenCode SDD Hive Tool Grants

Generated OpenCode SDD subagents MUST expose explicit per-subagent Hive access when required, and runtime verification MUST retain grant evidence for all four store modes.
(Previously: verification covered Hive-configured runtimes without the complete store model.)

#### Scenario: Parser preserves grant evidence
- GIVEN generated OpenCode subagents use any supported store mode
- WHEN configuration verification parses them
- THEN per-subagent grant evidence remains testable
- AND `none` and `openspec` do not require Hive persistence

### Requirement: Verification and Doctor Diagnostics

Verifier and doctor MUST detect missing or outdated generated grants, report drift, and explain regeneration without mutating user configuration. Generated files MUST be treated as outputs; source templates and embedded assets remain the edit sources. `AGENTS.md.tmpl` and `CLAUDE.md.tmpl` MUST remain equivalent.
(Previously: diagnostics required regeneration guidance but did not state source-only editing and template parity.)

#### Scenario: Drift is actionable and read-only
- GIVEN generated artifacts lack required grants or differ from source
- WHEN doctor runs
- THEN it reports drift and recommends `jarvis init` or reconfiguration
- AND it does not rewrite user-owned files

### Requirement: Hive Mode Degraded Behavior

Hive and hybrid flows MUST fail clearly when required Hive access is unavailable and MUST NOT silently inline artifact context. Persistence MUST use Hive and preserve four-store semantics; per-phase model rows remain Go-template-owned, persona voice MUST NOT enter technical artifacts, and `.jarvis` is canonical with `.atl` read fallback.
(Previously: degraded behavior only specified missing Hive tools and non-Hive advisory behavior.)

#### Scenario: Required runtime capability is absent
- GIVEN mode is `hive` or `hybrid`
- WHEN required Hive access is unavailable
- THEN the phase reports the missing capability and remedy
- AND it does not silently fall back

#### Scenario: Existing install regenerates safely
- GIVEN an older installation has generated SDD agents
- WHEN verification evaluates it
- THEN regeneration guidance identifies the supported flow
- AND user-owned configuration is not clobbered

## ADDED Requirements

### Requirement: Neutral Status-Core and Review Boundary

The sync MAY introduce only the minimum current-state status-core fields required by updated phases; it MUST NOT import unsupported future fields or the complete authority-bearing status contract. Executors MUST preserve negative boundaries and MUST NOT launch positive review, remediation, transaction, receipt, or authority workflows; #363 owns final routing and complete `jarvis.sdd-status`.

#### Scenario: Status dependency is absent
- GIVEN updated phases do not require status-core data
- WHEN the sync is applied
- THEN no status contract is added
- AND deferred #363/#420/#421/#422 capabilities remain unimplemented

#### Scenario: Executor encounters review work
- GIVEN a phase executor reaches a review or remediation boundary
- WHEN it continues execution
- THEN it records the safe negative boundary
- AND it does not invoke or implement the deferred capability
