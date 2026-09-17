# Enforce verified Deluge loop syntax

## Goal

Prevent the canonical Zoho skills from generating undocumented loop syntax while preserving the ownership boundary between application-neutral Deluge grammar and Creator-specific record iteration.

## Tasks

- [x] Enforce the application-neutral Deluge iteration contract with strict RED/GREEN evidence and commit the work unit.
- [ ] Document Creator-only `for each record` applicability and effects with strict RED/GREEN evidence and commit the work unit.
- [ ] Run focused and repository-required verification, complete native review when offered, and record final evidence.

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
- Task 2 commit: pending
- Final verification: pending
- Native review: pending
