# SDD Subagent Hive Tools Specification

## Purpose

Define required Hive MCP tool access, verification, regeneration guidance, and degraded behavior for SDD subagents in Claude Code and OpenCode.

## Requirements

### Requirement: Claude Code SDD Hive Tool Grants

Generated Claude Code SDD phase agents MUST explicitly allow the Hive MCP tools required to retrieve and persist SDD artifacts in Hive or hybrid artifact store modes.

#### Scenario: Generated Claude agents include Hive tools

- GIVEN Claude Code SDD phase agents are generated
- WHEN an SDD phase agent definition is rendered
- THEN its tool allowlist MUST include the required Hive MCP memory tools
- AND the grant MUST apply consistently to every generated SDD phase agent

#### Scenario: Existing file permissions remain bounded

- GIVEN Hive MCP tools are added to a Claude SDD phase agent
- WHEN the generated agent has read-only or phase-specific filesystem permissions
- THEN Hive memory access MUST NOT expand filesystem edit permissions

### Requirement: OpenCode SDD Hive Tool Grants

Generated OpenCode SDD subagents MUST explicitly declare per-subagent Hive MCP tool access when Hive or hybrid artifact storage can require subagent memory operations.

#### Scenario: Generated OpenCode subagents expose Hive tools

- GIVEN OpenCode configuration is generated with SDD subagents
- WHEN Hive MCP is configured for the runtime
- THEN each SDD subagent MUST include explicit Hive MCP tool access using the runtime's supported tool naming or pattern semantics

#### Scenario: OpenCode parser preserves grant evidence

- GIVEN an existing OpenCode configuration contains generated SDD subagents
- WHEN runtime verification parses the configuration
- THEN the observed model MUST retain enough per-subagent grant evidence to verify Hive MCP access

### Requirement: Verification and Doctor Diagnostics

The verifier and doctor MUST detect missing or outdated generated SDD subagent Hive grants and SHOULD provide actionable regeneration guidance without mutating user configuration.

#### Scenario: Missing grant is reported as drift

- GIVEN a generated SDD subagent is present without required Hive MCP access
- WHEN verification or doctor checks run
- THEN the result MUST report the missing grant as generated artifact drift
- AND it MUST recommend rerunning `jarvis init` or the supported reconfiguration flow

#### Scenario: Doctor remains read-only

- GIVEN doctor detects outdated generated Claude or OpenCode artifacts
- WHEN it reports the problem
- THEN it MUST NOT silently rewrite user configuration
- AND it MUST explain the regeneration action needed

### Requirement: Existing Install Regeneration Guidance

The system MUST guide existing installations to regenerate generated agent artifacts through normal init or reconfiguration flows.

#### Scenario: Existing install has old generated artifacts

- GIVEN a user has generated SDD agents from an older Jarvis version
- WHEN verification or doctor evaluates the install
- THEN the user MUST receive clear guidance that regeneration is required
- AND the guidance MUST identify that user-owned configuration is not being clobbered

### Requirement: Hive Mode Degraded Behavior

Hive and hybrid SDD flows MUST fail clearly when the active runtime cannot expose required Hive MCP tools to SDD subagents; they MUST NOT silently fall back to inline artifact context.

#### Scenario: Runtime cannot expose Hive tools

- GIVEN artifact store mode is Hive or hybrid
- WHEN an SDD subagent cannot access required Hive MCP tools
- THEN the phase MUST fail with an actionable degraded-mode message
- AND the message MUST name the missing capability and regeneration or configuration remedy

#### Scenario: Non-Hive modes are unaffected

- GIVEN artifact store mode does not require Hive MCP access
- WHEN SDD phase verification runs
- THEN missing subagent Hive MCP grants MAY be reported as advisory drift only when generated artifacts are present
- AND the phase MUST NOT require Hive persistence behavior
