# Memory #33: Decisiones arquitectónicas - Round 2 análisis

**Type**: architecture  
**Created**: 2026-04-09 20:07:11  
**Project**: jarvis-dev  
**Scope**: project

---

**What**: Decisiones de arquitectura tomadas en segunda ronda de preguntas/respuestas

**Why**: Usuario respondió a mis propuestas de approach con decisiones finales

**Where**: Conversación análisis pre-PRD

**Decisiones Confirmadas**:

## 1. Arquitectura de Memoria (HYBRID LOCAL + CENTRAL)
- **Propuesta usuario**: Mix de local + sincronización central
- **Modelo**: Hub-and-spoke
  - Cada dev usa Engram LOCAL (SQLite en su máquina)
  - Sincronización periódica/on-demand a memoria CENTRAL (PostgreSQL o similar)
  - Memoria central = fuente de verdad compartida
- **Beneficios**: Offline-first, performance local, compartición cross-team
- **Pendiente**: Desarrollar approach técnico completo

## 2. Fase sdd-qa
- **Estado**: OK confirmado
- **Pendiente**: Mockup de cómo se muestra lista de pruebas al usuario

## 3. Onboarding Progresivo
- **Decisión**: Opción A (niveles automáticos) APROBADA
- **Pendiente**: Desarrollar mecánica completa

## 4. TDD en Zoho
- **Decisión**: TDD Manual confirmado como approach correcto
- **Razón**: Zoho SaaS sin test runners automatizados

## 5. Sistema de Persona
- **Decisión para PRD**: 
  - Habrá X presets (cantidad/nombres por definir después)
  - Opción "Custom" disponible
  - Custom muestra preset base (cualquiera) editable
  - Sistema valida estructura antes de aplicar
  - Si válido → se aplica
- **Presets reales**: Usuario los revelará más adelante (no son los que propuse)
- **Para PRD**: Solo documentar mecánica, no presets específicos

**Learned**:
- Arquitectura hybrid local+central es más robusta que solo central (offline-first)
- Usuario tiene visión clara de UX (quiere ver mockups antes de aprobar)
- Sistema de validación de presets custom es crítico (evita que usuarios rompan comportamiento base)
