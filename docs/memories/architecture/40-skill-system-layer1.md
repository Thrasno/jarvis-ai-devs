---
id: 40
type: architecture
project: jarvis-dev
scope: project
session: manual-save-jarvis-dev
created_at: 2026-04-10 08:17:18
---

# Sistema de Persona - Skill System integrado en Layer 1

**What**: Integración del skill system en Layer 1 de la persona base

**Skills en Layer 1**:
- **Core**: sdd-*, hive
- **Context** (auto-activación): zoho-deluge, phpunit-testing, laravel-architecture, git-workflow, gitlab-integration
- **User** (custom por proyecto): registry_path `.jarvis/skill-registry.md`

**Skill Loading Protocol**:
- on_session_start: Leer registry, cachear compact rules
- on_context_detection: Detectar archivos/comandos, activar skills matching triggers
- on_explicit_request: Forzar skill con `/skill-name`
- fallback: Si registry no existe → usar solo core_skills (degradación graceful)

**Registry Format**: Core Skills, Context Skills (trigger table), Project Skills, Compact Rules

**Learned**: Skills NO hardcoded en Layer 1, registry versionado en git = equipo comparte estándares
