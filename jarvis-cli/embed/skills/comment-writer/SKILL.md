---
name: comment-writer
display_name: "Comment Writer"
description: "Write warm, direct collaboration comments. Trigger: PR feedback, issue replies, reviews, Slack messages, or GitHub comments."
license: Apache-2.0
scope: optional
metadata:
  author: gentleman-programming
  version: "1.0"
---

<!-- Synced from https://raw.githubusercontent.com/Gentleman-Programming/gentle-ai/v1.26.5/internal/assets/skills/comment-writer/SKILL.md -->

## When to Use

Load this skill whenever you write a comment that another human will read.

Use it for:

- GitHub PR or issue comments.
- Review feedback and requested changes.
- Maintainer replies.
- Slack, Discord, or async project updates.

## Voice Rules

| Rule | Requirement |
|------|-------------|
| Be useful fast | Start with the actionable point. Do not recap the whole PR before feedback. |
| Be warm and direct | Sound like a thoughtful teammate, not a corporate bot. |
| Keep it short | Prefer 1 to 3 short paragraphs or a tight bullet list. |
| Explain why | Give the technical reason when asking for a change. |
| Avoid pile-ons | Comment on the highest-value issue, not every tiny preference. |
| Match thread language | Write in the thread or user's language by default: match the language of the target context. If the user explicitly requests a language or tone, follow that request. Use a neutral, professional register unless the target context clearly calls for another one. |
| No em dashes | Use commas, periods, or parentheses instead. |

## Comment Formula

```text
<Direct observation or request>

<Why it matters, only if needed>

<Concrete next action>
```

## Examples

### Request change

```markdown
Good approach overall. I'd split this into a separate commit because it mixes validation logic with UI wiring.

That keeps the reviewer's focus narrower and makes rollback cleaner if the integration fails.
```

### Approve with a note

```markdown
Approved. The scope is clear and the change is well-contained.

For the next PR, add links to the previous and following PRs so the chain stays navigable.
```

### Ask for split

```markdown
This PR exceeds the 400-line budget, so we need to split it or justify `size:exception`.

Suggested order: foundation + tests first, then integration, then docs. That gives each review a clear start and end.
```

## Commands

```bash
# Inspect a PR before writing review feedback
gh pr view <PR_NUMBER> --json title,body,additions,deletions,changedFiles
```
