# Skill Registry — jarvis-ai-devs

**Stack**: Unknown
Canonical registry path: `.jarvis/skill-registry.md`

---

## Suggested Skills

- `hive`

---

## Installed Skills

| Skill | Trigger / Description | Scope | Path |
|-------|-----------------------|-------|------|
| Branch & PR | When creating pull requests — PR creation workflow with issue-first enforcement, branch naming, and automated checks | optional | `.jarvis/skills/branch-pr/SKILL.md` |
| Chained PR | When PRs exceed 400 lines, stacked PRs, review slices, or chained PRs are needed — PRs over 400 lines, stacked PRs, and review slices that protect review focus | optional | `.jarvis/skills/chained-pr/SKILL.md` |
| Cognitive Doc Design | When writing guides, READMEs, RFCs, onboarding, architecture, or review-facing docs — Design docs that reduce cognitive load for guides, READMEs, RFCs, onboarding, architecture, and reviews | optional | `.jarvis/skills/cognitive-doc-design/SKILL.md` |
| Comment Writer | When writing PR feedback, issue replies, reviews, Slack messages, or GitHub comments — Write warm, direct collaboration comments for PRs, issues, reviews, and async updates | optional | `.jarvis/skills/comment-writer/SKILL.md` |
| Git Workflow | When using git — Conventional commits, branch naming, no force push to main | optional | `.jarvis/skills/git-workflow/SKILL.md` |
| Go Testing | When writing Go tests, using teatest, or adding test coverage — Go testing patterns including Bubbletea TUI testing | optional | `.jarvis/skills/go-testing/SKILL.md` |
| Hive Memory | Using Hive memory — Persistent memory protocol: when to save, how to search, session summary triggers | core | `.jarvis/skills/hive/SKILL.md` |
| Issue Creation | When creating GitHub issues — GitHub issue creation with bug report and feature request templates | optional | `.jarvis/skills/issue-creation/SKILL.md` |
| Judgment Day | When user says judgment day, review adversarial, dual review — Parallel adversarial review protocol with dual blind judges | optional | `.jarvis/skills/judgment-day/SKILL.md` |
| Laravel Architecture | When writing Laravel code — Laravel conventions: thin controllers, services, repositories, FormRequest validation | optional | `.jarvis/skills/laravel-architecture/SKILL.md` |
| PHPUnit Testing | When writing PHP tests — PHPUnit patterns: AAA structure, factories, one concept per test | optional | `.jarvis/skills/phpunit-testing/SKILL.md` |
| QA Checklist | When user asks for batería de pruebas, checklist de pruebas, qué pruebas debería hacer, QA checklist, or test checklist — On-demand QA checklist and test checklist planning with manual QA and automated test recommendations | optional | `.jarvis/skills/qa-checklist/SKILL.md` |
| SDD Apply | When implementing tasks — Implement tasks following specs and design; supports Strict TDD mode | core | `.jarvis/skills/sdd-apply/SKILL.md` |
| SDD Archive | When archiving changes — Merge delta specs to main specs and close the SDD change cycle | core | `.jarvis/skills/sdd-archive/SKILL.md` |
| SDD Design | When designing architecture — Document architecture decisions and technical approach with rationale | optional | `.jarvis/skills/sdd-design/SKILL.md` |
| SDD Explore | When exploring ideas — Investigate ideas and compare approaches before committing to a change | optional | `.jarvis/skills/sdd-explore/SKILL.md` |
| SDD Init | When initializing SDD — Detect project stack, testing capabilities, and initialize SDD context | core | `.jarvis/skills/sdd-init/SKILL.md` |
| SDD Onboard | When onboarding user through full SDD cycle — Guided end-to-end walkthrough of SDD workflow | optional | `.jarvis/skills/sdd-onboard/SKILL.md` |
| SDD Propose | When creating proposals — Create a structured change proposal with intent, scope, and success criteria | optional | `.jarvis/skills/sdd-propose/SKILL.md` |
| SDD Spec | When writing specs — Write delta requirements and Given/When/Then scenarios for a change | optional | `.jarvis/skills/sdd-spec/SKILL.md` |
| SDD Tasks | When creating task lists — Break down a change into a concrete, ordered implementation checklist | optional | `.jarvis/skills/sdd-tasks/SKILL.md` |
| SDD Verify | When verifying implementation — Verify implementation against specs with structural and behavioral checks | core | `.jarvis/skills/sdd-verify/SKILL.md` |
| Skill Creator | When creating new skills, agent instructions, or documenting AI usage patterns — Create LLM-first skills with valid frontmatter, local references, and concise trigger-rich descriptions | optional | `.jarvis/skills/skill-creator/SKILL.md` |
| Skill Improver | When improving skills, auditing skills, refactoring skills, or checking skill quality — Audit and upgrade existing LLM-first skills against style and safety contracts | optional | `.jarvis/skills/skill-improver/SKILL.md` |
| Skill Registry | When user says update skills, skill registry, or after installing skills — Create or update the skill registry for the current project | optional | `.jarvis/skills/skill-registry/SKILL.md` |
| Work Unit Commits | When planning implementation commits, commit splitting, chained PRs, or work units — Plan commits as reviewable work units with tests and docs kept beside code | optional | `.jarvis/skills/work-unit-commits/SKILL.md` |
| Zoho Deluge | When writing Zoho Deluge scripts — Zoho Deluge scripting conventions: no nested loops, bulk operations, null safety | optional | `.jarvis/skills/zoho-deluge/SKILL.md` |

---

## Compact Rules (Transitional Metadata)

Compact rules are compatibility metadata; the skill index path rows above are the primary instruction contract.

- **branch-pr**: Check for an issue first, create a review-focused PR from a focused branch, keep a clean diff, and include verification evidence.
- **chained-pr**: Split work over 400 lines into stacked PRs or chained review slices, each with one focused goal, tests, and a clear rollback boundary.
- **cognitive-doc-design**: Structure docs around audience, task, and decision points; reduce reader cognitive load with clear headings, examples, and review-ready summaries.
- **comment-writer**: Write warm and direct comments: state the decision, explain the reason, give one actionable next step, and avoid vague praise or blame.
- **git-workflow**: Load when: When using git.
- **go-testing**: Use gofmt, targeted go test cycles, and go vet for confidence.
- **hive**: Search memory for past context and save significant discoveries.
- **issue-creation**: Search existing issues first, define the problem, clear scope, acceptance criteria, and labels before creating the GitHub issue.
- **judgment-day**: Load when: When user says judgment day, review adversarial, dual review.
- **laravel-architecture**: Load when: When writing Laravel code.
- **phpunit-testing**: Load when: When writing PHP tests.
- **qa-checklist**: Load when: When user asks for batería de pruebas, checklist de pruebas, qué pruebas debería hacer, QA checklist, or test checklist.
- **sdd-apply**: Follow assigned tasks, specs, design, and Strict TDD when enabled.
- **sdd-archive**: Archive completed changes by syncing accepted spec deltas.
- **sdd-design**: Record architecture decisions, alternatives, and affected files.
- **sdd-explore**: Clarify goals and tradeoffs before committing to a change.
- **sdd-init**: Detect stack, testing capabilities, and project conventions.
- **sdd-onboard**: Load when: When onboarding user through full SDD cycle.
- **sdd-propose**: Define intent, scope, risks, and success criteria before implementation.
- **sdd-spec**: Write behavior as requirements with concrete scenarios.
- **sdd-tasks**: Break implementation into reviewable, testable work units.
- **sdd-verify**: Run verification against specs, tasks, and project test commands.
- **skill-creator**: Create trigger-rich LLM skills with valid frontmatter and local references.
- **skill-improver**: Audit existing skills against the style guide, report quality and safety gaps, and require explicit user approval before changing any skill file.
- **skill-registry**: Refresh the registry when installed skills or conventions change.
- **work-unit-commits**: Plan commits as reviewable work units, keep tests and docs with code, avoid mixed concerns, and preserve a clean review narrative.
- **zoho-deluge**: Load when: When writing Zoho Deluge scripts.

---

## Project Conventions

- Generated sections are deterministic; customize only from `## Custom Skills` onward.
- Keep `.jarvis/skill-registry.md` committed so the team resolves the same skills.
- Built-in skill paths point at project-local `.jarvis/skills/<skill>/SKILL.md` copies generated by `jarvis init`.
- Re-run `jarvis init` after changing stack or installed skill metadata.

---

## Custom Skills

<!-- Add your project-specific skills here -->
