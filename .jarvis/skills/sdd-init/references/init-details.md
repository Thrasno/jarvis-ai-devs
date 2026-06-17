<!-- Synced from https://raw.githubusercontent.com/Gentleman-Programming/gentle-ai/v1.26.5/internal/assets/skills/sdd-init/references/init-details.md -->
<!-- Upstream commit: 5f73974b39ae2b9b525ef465b3642030c5f2ce6c; adapted for Jarvis/Hive runtime wording. -->
# SDD Init Details

## Testing Capability Checklist

- Test runner: `package.json` scripts/deps, `pyproject.toml`, `pytest.ini`, `go.mod`, `Cargo.toml`, `Makefile`.
- Test layers: unit runner; integration libraries (`testing-library`, `httpx`, `httptest`, `WebApplicationFactory`); E2E tools (`playwright`, `cypress`, `selenium`, `chromedp`).
- Coverage: `vitest --coverage`, `jest --coverage`, `c8`, `pytest-cov`, `go test -cover`, `coverlet`.
- Quality: linter, type checker, formatter commands.

## Skill Registry Scan Rules

- Write the generated project registry to `.jarvis/skill-registry.md`; `.atl/skill-registry.md` is only a legacy read fallback if retained by runtime compatibility code.
- Render the registry as index-first and path-first: every skill row includes Skill, Trigger / Description, Scope, and Path.
- For Jarvis built-in skills, copy embedded skill files into `.jarvis/skills/` and render project-local paths such as `.jarvis/skills/<skill>/SKILL.md`; do not render unresolved embedded-relative paths like `go-testing/SKILL.md`.
- Scan user skills in known provider global skill directories and project skills in workspace skill directories.
- Skip `sdd-*`, `_shared`, and `skill-registry`; deduplicate by skill name, preferring project-level skills over user-level skills.
- Read each selected `SKILL.md`; if it exceeds 200 lines, focus on frontmatter plus Critical Patterns / Rules sections.
- Extract `name`, trigger text from `description`, scope, and exact `SKILL.md` path.
- Treat compact rules as optional transitional metadata; runtime prompt injection uses exact skill paths as the primary contract.
- Scan project convention files: `agents.md`, `AGENTS.md`, project-level `CLAUDE.md`, `.cursorrules`, `GEMINI.md`, and `copilot-instructions.md`.
- For index files such as `AGENTS.md`, extract referenced file paths and include both the index and referenced files in the registry.

## LLM-First Skill Criteria

- Treat skills as runtime instruction contracts, not human documentation.
- Required structure: frontmatter, Activation Contract, Hard Rules, Decision Gates, Execution Steps, Output Contract, References.
- Keep `description` quoted, one physical line, trigger-first, and no longer than 250 characters.
- Target 180-450 body tokens; move examples, schemas, edge cases, and background into local `references/` or `assets/`.
- References must be local files and stable relative to the skill directory when possible.
- Quality gates: hard rules are observable, decision gates cover real forks, output contract states exactly what to return, and references resolve locally.

## Hive Saves

```text
mem_save title/topic_key: sdd-init/{project}
type: architecture
content: detected project context markdown
capture_prompt: false when available

mem_save title/topic_key: sdd/{project}/testing-capabilities
type: config
content: testing capabilities markdown
capture_prompt: false when available

mem_save title/topic_key: skill-registry
type: config
content: registry markdown
capture_prompt: false when available
```

## OpenSpec Skeleton

```text
openspec/
├── config.yaml
├── specs/
└── changes/
    └── archive/
```

`config.yaml` should include concise context, `strict_tdd`, testing capabilities, and phase rules for proposal/spec/design/tasks/apply/verify/archive. Keep `context:` under 10 lines.

## Testing Capabilities Format

```markdown
## Testing Capabilities

**Strict TDD Mode**: {enabled/disabled}
**Detected**: {date}

### Test Runner
- Command: `{command}`
- Framework: {name}

### Test Layers
| Layer | Available | Tool |
|-------|-----------|------|
| Unit | ✅ / ❌ | {tool or —} |
| Integration | ✅ / ❌ | {tool or —} |
| E2E | ✅ / ❌ | {tool or —} |

### Coverage
- Available: ✅ / ❌
- Command: `{command or —}`

### Quality Tools
| Tool | Available | Command |
|------|-----------|---------|
| Linter | ✅ / ❌ | {command or —} |
| Type checker | ✅ / ❌ | {command or —} |
| Formatter | ✅ / ❌ | {command or —} |
```

## Output Templates

For each mode, include project, stack, persistence, Strict TDD Mode, Testing Capabilities table, artifacts created/saved, limitations where relevant, and next steps. Hive mode must mention local/non-shareable limitations; none mode must recommend enabling persistence.
