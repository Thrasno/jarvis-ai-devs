# Skill Registry — jarvis-dev

**Generated**: 2026-04-10  
**Project**: jarvis-dev  
**Agent Framework**: opencode / claude / gemini

This registry lists all available skills and conventions for this project. Skills provide specialized instructions and workflows for specific tasks.

---

## Available Skills

### Workflow Skills

#### branch-pr
**Description**: PR creation workflow for Agent Teams Lite following the issue-first enforcement system.  
**Trigger**: When creating a pull request, opening a PR, or preparing changes for review.  
**Location**: `~/.config/opencode/skills/branch-pr/SKILL.md`

#### issue-creation
**Description**: Issue creation workflow for Agent Teams Lite following the issue-first enforcement system.  
**Trigger**: When creating a GitHub issue, reporting a bug, or requesting a feature.  
**Location**: `~/.config/opencode/skills/issue-creation/SKILL.md`

#### judgment-day
**Description**: Parallel adversarial review protocol that launches two independent blind judge sub-agents simultaneously to review the same target, synthesizes their findings, applies fixes, and re-judges until both pass or escalates after 2 iterations.  
**Trigger**: When user says "judgment day", "judgment-day", "review adversarial", "dual review", "doble review", "juzgar", "que lo juzguen".  
**Location**: `~/.config/opencode/skills/judgment-day/SKILL.md`

---

### SDD (Spec-Driven Development) Skills

#### sdd-init
**Description**: Initialize Spec-Driven Development context in any project. Detects stack, conventions, testing capabilities, and bootstraps the active persistence backend.  
**Trigger**: When user wants to initialize SDD in a project, or says "sdd init", "iniciar sdd", "openspec init".  
**Location**: `~/.config/opencode/skills/sdd-init/SKILL.md`

#### sdd-explore
**Description**: Explore and investigate ideas before committing to a change.  
**Trigger**: When the orchestrator launches you to think through a feature, investigate the codebase, or clarify requirements.  
**Location**: `~/.config/opencode/skills/sdd-explore/SKILL.md`

#### sdd-propose
**Description**: Create a change proposal with intent, scope, and approach.  
**Trigger**: When the orchestrator launches you to create or update a proposal for a change.  
**Location**: `~/.config/opencode/skills/sdd-propose/SKILL.md`

#### sdd-spec
**Description**: Write specifications with requirements and scenarios (delta specs for changes).  
**Trigger**: When the orchestrator launches you to write or update specs for a change.  
**Location**: `~/.config/opencode/skills/sdd-spec/SKILL.md`

#### sdd-design
**Description**: Create technical design document with architecture decisions and approach.  
**Trigger**: When the orchestrator launches you to write or update the technical design for a change.  
**Location**: `~/.config/opencode/skills/sdd-design/SKILL.md`

#### sdd-tasks
**Description**: Break down a change into an implementation task checklist.  
**Trigger**: When the orchestrator launches you to create or update the task breakdown for a change.  
**Location**: `~/.config/opencode/skills/sdd-tasks/SKILL.md`

#### sdd-apply
**Description**: Implement tasks from the change, writing actual code following the specs and design.  
**Trigger**: When the orchestrator launches you to implement one or more tasks from a change.  
**Location**: `~/.config/opencode/skills/sdd-apply/SKILL.md`

#### sdd-verify
**Description**: Validate that implementation matches specs, design, and tasks.  
**Trigger**: When the orchestrator launches you to verify a completed (or partially completed) change.  
**Location**: `~/.config/opencode/skills/sdd-verify/SKILL.md`

#### sdd-archive
**Description**: Sync delta specs to main specs and archive a completed change.  
**Trigger**: When the orchestrator launches you to archive a change after implementation and verification.  
**Location**: `~/.config/opencode/skills/sdd-archive/SKILL.md`

---

### Technology-Specific Skills

#### go-testing
**Description**: Go testing patterns for Gentleman.Dots, including Bubbletea TUI testing.  
**Trigger**: When writing Go tests, using teatest, or adding test coverage.  
**Location**: `~/.config/opencode/skills/go-testing/SKILL.md`

#### skill-creator
**Description**: Creates new AI agent skills following the Agent Skills spec.  
**Trigger**: When user asks to create a new skill, add agent instructions, or document patterns for AI.  
**Location**: `~/.config/opencode/skills/skill-creator/SKILL.md`

---

## Project Conventions

No project-level convention files detected. If you create `AGENTS.md`, `CLAUDE.md`, `GEMINI.md`, `.cursorrules`, or similar, they will be indexed here.

---

## How to Use

1. **Load a skill**: When your task matches a trigger, use the skill tool to load the full instructions
2. **Check conventions**: Read project-level convention files for coding standards and patterns
3. **Follow workflows**: Skills provide step-by-step workflows — follow them exactly

---

**Last Updated**: 2026-04-10 (via sdd-init)
