# Embedded Skill Upstream Sync Specification

## Purpose

Synchronize approved gentle-ai v2.1.5 mechanics into existing Jarvis assets without adding an invokable skill or importing unavailable authority.

## Requirements

### Requirement: Pinned Provenance and Disposition Ledger

Every synchronized row MUST record tag `v2.1.5`, tag object `0b4532b5a73c12b7347c1954ef37cb372056c914`, peeled commit `1b5a5f59f74d3f6dab7de01c1603d5ce1b77af17`, disposition, and exact adaptation or deferral rationale.

#### Scenario: Inventory is reviewable
- GIVEN 19 invokable skills, 3 meta-packages, and 8 support files are assessed
- WHEN the sync record is reviewed
- THEN every row has provenance and a disposition
- AND Hermes delegation and Judgment Day are explicitly excluded

### Requirement: Exact Adoption Boundaries

The sync MUST apply these exact dispositions: unchanged metadata-only `branch-pr`, `chained-pr`, `cognitive-doc-design`; upstream-body `comment-writer`; upstream body plus examples `go-testing`; repository-neutral `issue-creation`; discard `hermes-ephemeral-delegation` and `judgment-day`; neutral mechanics for `sdd-explore`, `sdd-propose`, `sdd-spec`, `sdd-design`, `sdd-tasks`, `sdd-onboard`, `sdd-archive`, and `work-unit-commits`; compatible v3 mechanics for `sdd-init`, `sdd-apply`, and `sdd-verify`. Meta-packages are: `skill-creator` upstream body/style guide, `skill-improver` with `.jarvis`/`.atl` adaptation, and `skill-registry` with Hive adaptation. Shared files are: discard `_shared/SKILL.md` and `review-ledger-contract.md`; preserve `openspec-convention.md`; treat upstream `engram-convention.md` as a source disposition whose Jarvis target is the intentionally shipped `_shared/hive-convention.md`, porting only product-neutral concepts adapted to Hive tools and semantics; Hive-adapt `persistence-contract.md` and `skill-resolver.md`; selectively adopt neutral `sdd-phase-common.md`; and conditionally adopt status-core only from `sdd-status-contract.md`. Jarvis MUST NOT create or ship `_shared/engram-convention.md`. No new invokable skill SHALL be added.

#### Scenario: Adaptation is not inferred
- GIVEN a row has no explicit adaptation in the approved matrix
- WHEN its asset is synchronized
- THEN only provenance/catalog metadata changes behavior
- AND repository policy or persona voice is not copied into the generic body

### Requirement: Authority Deferral and Non-Goals

The change MUST NOT specify or ship positive ledger, transaction, snapshot, receipt, reviewGate, Judgment Day, remediation, generation, semantic-validity, or structured verification/archive authority. Ownership remains #365, #366, #367, #421, #422, #420, and final routing/status remains #363. CodeGraph lifecycle, Engram adoption, generated user-machine edits, and Hermes delegation are non-goals.

#### Scenario: Deferred fragment is encountered
- GIVEN an upstream fragment requires deferred authority
- WHEN the sync is authored
- THEN it is excluded or reduced to a safe negative executor boundary
- AND its owner issue is recorded without implementing the capability

### Requirement: Registry Contradiction Decision

Before implementation, the change MUST record an explicit design decision resolving whether the registry cache is gitignored or shareable/versioned; the specification MUST NOT invent the answer. The selected decision MUST preserve `.jarvis/skill-registry.md` as canonical and `.atl/` as legacy read fallback.

#### Scenario: Contradiction remains unresolved
- GIVEN cache policy and shareability have not been decided
- WHEN implementation planning is evaluated
- THEN the sync is blocked from silently choosing either policy
- AND the unresolved decision is surfaced for approval

### Requirement: Source-to-Installation Verification

The implementation MUST verify source contracts, full CLI installation and regeneration, template parity, and generated-output drift. Authored changes forecast at 900–1,300 lines MUST carry the approved `size:exception`; generated goldens remain included in complete verification.

#### Scenario: Regeneration is complete
- GIVEN embedded sources and an existing installation
- WHEN CLI install and regeneration verification run
- THEN source and installed outputs agree across supported agents
- AND focused checks, `go test ./...`, and `go vet ./...` are recorded
