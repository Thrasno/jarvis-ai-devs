# Enforce verified Deluge loop syntax

## Goal

Prevent the canonical Zoho skills from generating undocumented loop syntax while preserving the ownership boundary between application-neutral Deluge grammar and Creator-specific record iteration.

## Tasks

- [x] Enforce the application-neutral Deluge iteration contract with strict RED/GREEN evidence and commit the work unit.
- [x] Document Creator-only `for each record` applicability and effects with strict RED/GREEN evidence and commit the work unit.
- [x] Run focused and repository-required verification, attempt native review, and record final evidence.

## Constraints

- `zoho-deluge` owns generic grammar for documented `for each` and List-only `for each index` forms.
- `zoho-creator` owns Creator-only `for each record` applicability and effects; do not duplicate it as a universal Deluge form.
- Preserve bounded nested `for each` loops and keep application limits out of the language core.
- Change embedded sources of truth only; never edit generated user-machine agent files.
- Contract tests must inspect required guidance or contextual Deluge code, not globally blacklist explanatory words.
- Follow strict TDD: observe a focused failing test before production documentation changes, then observe it passing.

## Evidence

- Issue: https://github.com/Thrasno/jarvis-ai-devs/issues/725
- Task 1 commit: `5aa1cf2b` (`fix(skills): enforce documented Deluge loops`)
- Task 1 RED: `cd jarvis-cli && go test ./internal/skills -run '^TestCatalogContract_ZohoDeluge' -count=1` failed before the canonical assets were updated.
- Task 1 GREEN: the same focused command passed after the implementation; independent verification repeated it successfully.
- Task 2 commit: `0b5fe3ad` (`fix(skills): scope record iteration to Creator`)
- Task 2 RED: `cd jarvis-cli && go test ./internal/skills -run '^TestZohoCreatorEmbeddedSkill_' -count=1` failed before Creator-owned documentation was updated.
- Task 2 GREEN: the same focused command passed after implementation; independent verification repeated it successfully.
- Final verification: `cd jarvis-cli && go test ./...` passed; `cd jarvis-cli && go vet ./...` passed; `git diff --check master...HEAD` passed; independent committed-diff inspection found no blocker.
- Native review: unavailable. `gentle_review start` with committed range `master...HEAD` failed before lineage creation with `schema-incompatible`; fallback assessment was also unavailable, so writer self-verification plus independent verification satisfied the returned high-risk fallback plan.
