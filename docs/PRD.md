# Jarvis PRD Contract

Jarvis is the setup and reconfiguration entrypoint for the complete local development ecosystem. The primary flow is `jarvis`, which configures local Hive, agent prompt injection, skills, and generated agent files. Installer subcommands are not the primary setup path.

Claude MCP registration is owned by the setup flow and documents the user-scoped state contract in `~/.claude.json`:

```bash
claude mcp add --transport stdio --scope user hive -- hive-daemon mcp
claude mcp add --transport http --scope user context7 https://mcp.context7.com/mcp
```

SDD activation is explicit and warning-only unless the user asks for it. The runtime policy modes are `force_sdd`, `force_inline`, and `recommendation_only`. SDD should trigger for phrases such as `use sdd` and `usa sdd`; inline work should remain available for overrides such as `do it inline` and `hacelo directo`.
