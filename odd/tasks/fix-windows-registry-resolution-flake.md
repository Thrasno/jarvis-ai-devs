# Fix Windows registry resolution flake

## Goal

Remove the flaky duplicate Git worktree-root resolution used by lifecycle registry-quality checks so Windows CI does not report a false missing project registry when Git startup exceeds two seconds.

## Tasks

- [x] Confirm the failing CI assertion and identify the duplicated two-second resolver.
- [x] Reuse the canonical project-registry root resolver and keep nested-worktree behavior covered.
- [x] Run focused and repository-required verification, review the diff, then commit and push to `master`.

## Constraints

- Preserve warning-grade registry-quality behavior when the supplied path is not a Git worktree.
- Do not change generated local agent configuration.
- Keep the fix scoped to lifecycle registry-quality resolution and its tests.
