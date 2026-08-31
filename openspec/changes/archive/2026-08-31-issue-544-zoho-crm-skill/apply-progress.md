# Apply Progress: Add the Zoho CRM Skill

## Status

success — all seven tasks are complete in Strict TDD mode. The maintainer-approved `size:exception` is recorded for this single-PR work unit.

## Completed Tasks

- [x] 1.1 CRM embedded-content contract tests
- [x] 1.2 Interactive and grouped Zoho selection tests
- [x] 2.1 Embedded CRM skill and focused references
- [x] 2.2 Recognition-only 21-module / 609-field catalogs
- [x] 2.3 Existing interactive-selection and Zoho-prompt wiring
- [x] 3.1 Content/link/installation refactor and contract verification
- [x] 3.2 Focused and full `jarvis-cli` Go test verification

## TDD Cycle Evidence

| Task | Test file | Layer | Safety net | RED | GREEN | Triangulate | Refactor |
|---|---|---|---|---|---|---|---|
| 1.1 | `internal/skills/zoho_crm_contract_test.go` | Unit | N/A (new file) | `go test ./internal/skills -run 'TestZohoCRMEmbeddedSkill\|TestIsInteractive'` exited 1: CRM assets were absent | Same command exited 0 after assets | Activation has four routes; catalog, response, safety, installer, and links use independent assertions | Local references and recognition-only catalog headers retained; focused test exited 0 |
| 1.2 | `internal/skills/interactive_test.go`, `internal/tui/model_test.go` | Unit | Existing focused root invocation exposed that the repository root is not a Go module; module-scoped tests were used | `go test ./internal/tui -run TestBuildSkillSelectionPlan_OnlyPromptsStackSpecificSkills` exited 1: old Zoho prompt label | Focused TUI command exited 0 after grouped prompt wiring | Defaults, two toggles, and non-Zoho isolation are asserted | `gofmt` then focused TUI test exited 0 |
| 2.1 | `internal/skills/zoho_crm_contract_test.go` | Unit | N/A (new assets) | Contract test was RED before any CRM asset existed | `go test ./internal/skills -run TestZohoCRMEmbeddedSkill` exited 0 | Independent routing, V8, response, security, and installation contracts | Installed source links now stay inside the skill tree |
| 2.2 | `internal/skills/zoho_crm_contract_test.go` | Unit | N/A (new assets) | Catalog contract was RED while catalog assets were absent | Focused CRM contract test exited 0 | Test counts 21 module rows, 21 field sections, and 609 field rows | Catalog introductions restrict content to recognition and defer to runtime metadata |
| 2.3 | `internal/skills/interactive_test.go`, `internal/tui/model_test.go` | Unit | Existing source covered by focused selection tests | Interactive and grouped-prompt assertions were RED before selection wiring | Skills and TUI focused commands exited 0 | Toggling both directions proves grouping does not change `branch-pr` | No abstraction added; existing prompt loop remains the authority |
| 3.1 | `internal/skills/zoho_crm_contract_test.go` | Unit | Focused contract suite green | Link and content assertions were present before implementation | Focused CRM contract test exited 0 | Recursive installation and local-link assertions are separate from content assertions | `gofmt` and focused suites remained green |
| 3.2 | Existing focused and package suites | Unit | Package suites passed before final full run | N/A — verification task reuses prior RED contracts | `go test ./internal/skills ./internal/tui` and `go test ./...` exited 0 in `jarvis-cli` | Focused contract, selection, package, and full-module execution cover different paths | No behavior refactor required after green |

## Work Unit Evidence

| Work unit | Focused test command and exact result | Runtime harness | Rollback boundary |
|---|---|---|---|
| CRM content contracts | `go test ./internal/skills -run TestZohoCRMEmbeddedSkill` — exit 0 | N/A: static embedded Markdown has no live Zoho runtime in scope; `InstallSelected` provides the in-process packaging boundary | Remove `jarvis-cli/embed/skills/zoho-crm/` and `jarvis-cli/internal/skills/zoho_crm_contract_test.go` |
| Recognition catalogs | `go test ./internal/skills -run TestZohoCRMEmbeddedSkill` — exit 0; verifies 21 modules and 609 fields | N/A: catalogs are static recognition data and runtime metadata remains external authority | Remove the two CRM catalog references and catalog assertions |
| Zoho selection | `go test ./internal/tui -run 'TestBuildSkillSelectionPlan_OnlyPromptsStackSpecificSkills\|TestZohoSkillPrompt_TogglesCRMAndDelugeTogether\|TestViewSkills_DoesNotLeakLargeCatalog'` — exit 0 | N/A: the existing planner runs in process; no agent installation or user-machine configuration was invoked | Revert `interactive.go`, `skills_selection.go`, and their focused tests |

## Verification Summary

- `go test ./internal/skills ./internal/tui` — exit 0.
- `go test ./...` — exit 0 from the affected `jarvis-cli` Go module.
- `git diff --check` — exit 0.
- The repository root contains multiple Go modules and no root `go.mod`; root-level `go test ./...` is not a valid workspace command. No build command was run.
- Changed assets are source-of-truth embedded content only. No generated user-machine configuration, live Zoho call, credential, tenant assertion, shell/process workflow, generic router/installer abstraction, or unrelated `zoho-deluge` change was introduced.

## Delivery Boundary

- Delivery mode: single PR, `size:exception` explicitly accepted by the maintainer.
- Scope: embedded CRM skill assets, recognition-only catalogs, contract tests, and the two existing selection authorities.
- Review budget: expected within the approved 5,000-line exception budget.

## Focused Remediation: Verify Assertion Coverage

The native unmanaged remediation attempt `verify-assertion-remediation` was authorized after failed verification evidence `sha256:2038ce473282c8841b317977097959f030dbc091ee02f2ef97738a33ecf8df53`. All original tasks remain complete; this test-only correction adds the four missing contract assertions without changing production Go, embedded CRM content, catalogs, selection behavior, specs, design, or task checkboxes.

### Remediation TDD Cycle Evidence

| Work unit | Test file | Layer | Safety net | RED | GREEN | Triangulate | Refactor |
|---|---|---|---|---|---|---|---|
| `verify-assertion-remediation` | `jarvis-cli/internal/skills/zoho_crm_contract_test.go` | Unit/content contract | `go test ./internal/skills -run '^TestZohoCRMEmbeddedSkill'` — exit 0 before the edit | Coverage-closure exception: the four assertions were written first and were expected to pass because the verified Markdown clauses already existed; no false failing RED was claimed | `go test -v ./internal/skills -run '^TestZohoCRMEmbeddedSkill_EnforcesRoutingAlternativesAndSafeClarification$'` — exit 0; 1 test with 4 passing subtests | Four independent scenario clauses across routing, standard capabilities, and uncertainty assets | `gofmt -w internal/skills/zoho_crm_contract_test.go`; no production refactor was needed; focused test remained green |

### Work Unit Evidence

| Work unit | Focused test command and exact result | Runtime harness | Rollback boundary |
|---|---|---|---|
| `verify-assertion-remediation` | `go test -v ./internal/skills -run '^TestZohoCRMEmbeddedSkill_EnforcesRoutingAlternativesAndSafeClarification$'` — exit 0; 1 test and 4 subtests passed | N/A: the boundary is embedded static Markdown exercised through the in-process `SkillsFS`; no live Zoho runtime is in scope | Revert only `TestZohoCRMEmbeddedSkill_EnforcesRoutingAlternativesAndSafeClarification` in `jarvis-cli/internal/skills/zoho_crm_contract_test.go`; no unrelated behavior is removed |

### Exact Test Evidence

- Baseline: `go test ./internal/skills -run '^TestZohoCRMEmbeddedSkill'` — exit 0.
- Focused: `go test -v ./internal/skills -run '^TestZohoCRMEmbeddedSkill_EnforcesRoutingAlternativesAndSafeClarification$'` — exit 0; assertions passed for no API-version question, non-blocking standard alternatives, recommendation before waiting, and safe clarification/alternative.
- Affected package: `go test ./internal/skills` — exit 0.

```yaml
schema: gentle-ai.remediation-result/v1
status: success
mode: unmanaged
lineage_id: null
generation: null
fix_batch: verify-assertion-remediation
failed_evidence_revision: sha256:2038ce473282c8841b317977097959f030dbc091ee02f2ef97738a33ecf8df53
changed_lines_budget: 100
```
```json
{"schema":"gentle-ai.remediation-evidence/v1","mode":"unmanaged","lineage_id":null,"generation":null,"fix_batch":"verify-assertion-remediation","failed_evidence_revision":"sha256:2038ce473282c8841b317977097959f030dbc091ee02f2ef97738a33ecf8df53","focused_test":{"command":"go test -v ./internal/skills -run '^TestZohoCRMEmbeddedSkill_EnforcesRoutingAlternativesAndSafeClarification$'","exit_code":0,"tests":1,"subtests":4},"affected_package_test":{"command":"go test ./internal/skills","exit_code":0},"runtime_harness":{"result":"N/A","reason":"Static embedded Markdown is exercised through in-process SkillsFS; no live Zoho runtime boundary exists."},"rollback_boundary":"jarvis-cli/internal/skills/zoho_crm_contract_test.go: TestZohoCRMEmbeddedSkill_EnforcesRoutingAlternativesAndSafeClarification"}
```
