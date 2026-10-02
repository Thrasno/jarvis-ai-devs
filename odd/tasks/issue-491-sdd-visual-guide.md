# Issue 491 — Illustrated SDD guide

## Objective
Create a Spanish illustrated SDD workflow guide matching the installation guides and publish the personal checklist removal to PR #766.

## Scope and constraints
- Worktree: jarvis-dev-pr-766; branch: docs/issue-491-onboarding-checklist.
- Use existing screenshots in docs/presentaciones/capturas/pantallazosSDD.
- Explain workflow, human approvals, benefits, use cases and limitations using repository-backed commands.
- Preserve unrelated changes, including odd/tasks/issue-491-dashboard-guide-style.md.
- No builds. Passive documentation has no meaningful RED; use structural/content checks instead.
- User explicitly authorized publishing the checklist deletion. Guide publication is to the existing requested PR.

## Tasks
- [ ] T1 Remove personal checklist and publish deletion — in progress.
  Acceptance: only checklist removal and this task record committed; push confirmed.
- [ ] T2 Create, verify and publish illustrated SDD guide — pending exploration.
  Acceptance: installation/Hive API visual style; five existing screenshots with captions/alt; correct phases and human gates; valid local links; no fabricated execution evidence; published HTMLPreview URL.

## Evidence
- Checklist removal already exists locally: docs/issue-491-onboarding-checklist.html, 129 deleted lines.
- No references found under docs to its filename.
- RDD enabled globally; passive documentation exception applies, not source mutation.
- Exploration delegated to gentle-ai-explore.

## Next step
Commit and push T1; reconcile exploration before delegating T2 writer.
