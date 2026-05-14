# Skill Quality Loop

Use this loop only when the skill is complex enough to justify extra checks.

1. **Intent capture** — Write the repeated user/agent request in plain language.
2. **Trigger check** — Verify the frontmatter description contains the phrases that should load the skill.
3. **Negative trigger check** — Name at least one nearby request that should not load it.
4. **Output check** — Define the sections or artifacts the skill must return.
5. **Dry run** — Mentally simulate one realistic prompt and tighten instructions that are vague or verbose.

Keep iteration evidence short. If the loop needs fixtures, prompts, or schemas, place them under `assets/` and reference them locally.
