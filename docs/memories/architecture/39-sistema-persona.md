---
id: 39
type: architecture
project: jarvis-dev
scope: project
session: manual-save-jarvis-dev
created_at: 2026-04-10 08:13:10
---

# Sistema de Persona (2 Capas) - Diseño completo

**What**: Sistema de personalización de IA con 2 capas (Base Inmutable + User Preset)

**Design**:

## Layer 1 (Base Inmutable)
- NO editable, opaco (usuario no lo ve)
- Contenido: behavior, expertise, philosophy, workflow_rules, teaching_approach

## Layer 2 (User Preset)
- Editable por usuario
- 7 presets: argentino, neutra, tony-stark, yoda, sargento, asturiano, galleguinho
- Template custom: tone, communication_style, feedback_style, code_preferences, phrases

**Validación**: `jarvis persona validate` (solo Layer 2)

**Comando**: `jarvis persona set <nombre>` (guardado en ~/.jarvis/persona-preset.yaml)

**Learned**: Layer 1 opaco evita confusión, skills específicos (Zoho Deluge) son contextuales (NO en Layer 1)
