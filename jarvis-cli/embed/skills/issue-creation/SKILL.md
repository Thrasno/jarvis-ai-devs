---
name: issue-creation
display_name: "Issue Creation"
description: "Create repository-aware issues with issue-first checks. Trigger: When creating a GitHub issue, reporting a bug, or requesting a feature."
license: Apache-2.0
scope: optional
metadata:
  author: gentleman-programming
  version: "1.0"
---

<!-- gentle-ai v2.1.5 selective sync; repository-neutral adaptation -->
<!-- Synced from https://raw.githubusercontent.com/Gentleman-Programming/gentle-ai/v1.26.5/internal/assets/skills/issue-creation/SKILL.md; adapted for Jarvis packaging. -->
<!-- Create Gentle AI issues with issue-first checks, adapted without repository coupling. -->

## Activation Contract

Use when creating, triaging, or helping a contributor file an issue.

## Hard Rules

- Discover the current repository before writing or invoking `gh`.
- Inspect issue templates, configuration, labels, blank-issue policy, approval workflow, and Discussions availability before assuming any workflow.
- Search existing issues for duplicates before creation.
- Use the repository's available template or a concise structured body when templates are unavailable.
- Do not hard-code a repository URL, labels, approval state, or language policy.

## Decision Gates

| Situation | Action |
|---|---|
| Duplicate exists | Link it and do not create another issue. |
| Matching template exists | Use it and complete required fields. |
| Questions have an enabled discussion space | Direct the question there. |
| No template or policy exists | Create a clear issue with problem, impact, reproduction or proposal, and acceptance criteria. |

## Execution Steps

1. Identify the repository and inspect its issue configuration and existing labels.
2. Search for duplicates using the user’s terminology and related technical terms.
3. Select the applicable template and write reproducible, outcome-focused content.
4. Apply only labels and workflow steps verified in the current repository.
5. Return the created issue URL or explain why creation was not appropriate.

## Output Contract

Report repository discovery, duplicate check, template used, issue URL when created, and any repository-specific workflow requirement discovered.
