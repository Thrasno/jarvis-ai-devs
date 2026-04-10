---
id: 33
type: architecture
project: jarvis-dev
scope: project
session: manual-save-jarvis-dev
created_at: 2026-04-09 20:07:11
---

# Decisiones arquitectónicas - Round 2 análisis

**What**: Decisiones de arquitectura tomadas en segunda ronda de preguntas/respuestas

**Decisiones Confirmadas**:
1. **Arquitectura Hybrid Local + Central**: Hub-and-spoke (SQLite local + PostgreSQL central)
2. **Fase sdd-qa**: OK confirmado
3. **Onboarding Progresivo**: Opción A (niveles automáticos) APROBADA
4. **TDD en Zoho**: TDD Manual confirmado
5. **Sistema de Persona**: Presets + Custom (validación de estructura)

**Learned**: Hybrid local+central es offline-first, usuario quiere ver mockups antes de aprobar features
