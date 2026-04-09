# Jarvis-Dev: Ecosistema IA para Equipo Conpas

> **Estado**: Fase de Análisis Pre-PRD (pausado 2026-04-09)

## Resumen Ejecutivo

Ecosistema de desarrollo asistido por IA diseñado para 8 desarrolladores del equipo Conpas, especializado en stack Zoho/PHP con memoria compartida cross-team y QA manual mejorado.

## Contexto

- **Equipo**: 8 desarrolladores (3 avanzados, 5 en distintos niveles)
- **Stack**: Zoho (SaaS primario), PHP (volumetría alta), GitLab (self-hosted)
- **Licencias**: Claude Teams (8 seats) compradas por 1 año
- **Infraestructura**: 3 VPS (apps PHP), 1 VPS (GitLab)

## Problemas a Resolver

1. **Falta de estandarización**: Cada desarrollador programa "como le da la gana"
2. **QA/Testing débil**: Zoho es SaaS sin test runners, tests 100% manuales
3. **Memoria NO compartida**: Conocimiento en silos, decisiones se pierden
4. **Adopción heterogénea**: 5 devs nunca usaron IA, necesitan onboarding

## Soluciones Diseñadas

### 1. Sistema de Memoria Compartida (Prioridad A - CRÍTICO)

**Arquitectura**: Hybrid Local + Central (hub-and-spoke)
- SQLite local en cada dev (offline-first, performance)
- PostgreSQL central en VPS (fuente de verdad compartida)
- Sincronización manual en v1: `jarvis sync`
- Guardado automático (IA decide qué es digno de memoria)
- Tracking de usuario: `created_by`, `updated_by`, historial completo

**Scopes**:
- `personal`: Solo local, NO sincroniza (preferencias personales)
- `project`: Sincroniza al central (decisiones, bugs, arquitectura)

**Nombres propuestos**: Cortex, Nexus, Bitácora (pendiente decisión)

### 2. Manual QA Protocol (Prioridad C - IMPORTANTE)

**Realidad Zoho**: SaaS sin ejecución local → tests 100% manuales

**Nueva fase SDD**: `sdd-qa` (bloqueante, NO se salta con fast-forward)
1. IA analiza código generado
2. IA propone mejoras (si las hay)
3. IA genera checklist Markdown de pruebas manuales
   - Inputs de ejemplo
   - Pasos a seguir
   - Resultados esperados
   - Edge cases
4. Usuario ejecuta en Zoho, marca pass/fail
5. Si fail → vuelve a `sdd-apply` con detalles
6. Si todo pass → permite `sdd-verify` → `sdd-archive`

### 3. Onboarding Progresivo (Feature diferenciador)

**3 niveles automáticos** basados en tareas completadas:

| Nivel | Rango | Restricciones |
|-------|-------|---------------|
| **Beginner** | 0-10 tasks | Solo `complexity: low`, max 3 archivos, NO fast-forward |
| **Intermediate** | 11-30 tasks | Hasta `complexity: medium`, max 8 archivos, fast-forward con warnings |
| **Advanced** | 30+ tasks | Sin restricciones |

**Tracking**: `user-stats/{username}` con contador, QA success rate, avg duration

### 4. Sistema de Persona (2 Capas)

**Layer 1 (Base Inmutable)**: Comportamiento, expertise, skills, workflow → Definido por Conpas, NO editable

**Layer 2 (User Preset)**: Tono, idioma, analogías, estilo → Editable con validación

**Presets**: X cantidad + opción Custom (usuario edita MD, sistema valida estructura)

## Decisiones Arquitectónicas Clave

| Decisión | Rationale |
|----------|-----------|
| **NO usar Engram de base** | Control total del stack, API REST convencional familiar para equipo PHP |
| **Sync manual v1** | Validar approach antes de automatizar |
| **Guardado automático** | IA suficientemente autónoma, usuario NO debe indicar |
| **PostgreSQL (no ElasticSearch)** | `tsvector` + GIN index suficiente para v1 |
| **Markdown checklist (no TUI)** | Más simple, portable, familiar |
| **TDD Manual (no test runners)** | Zoho SaaS = deal-breaker para testing automatizado |

## Tecnologías Propuestas

**Sistema de Memoria** (pendiente decisión de stack):
- **Opción A**: Backend Laravel 10 (PHP 8.2+), CLI PHP
- **Opción B**: Backend Gin (Go), CLI Go (binario portable)

**Base de datos**:
- Central: PostgreSQL 15+ con FTS (`tsvector` + GIN index)
- Local: SQLite 3

**Auth**: JWT con GitLab tokens

## Estado Actual

### Completado ✅
- Análisis profundo de gentle-ai (referencia arquitectónica)
- Captura de requerimientos (2 rondas de preguntas/respuestas)
- Diseño de arquitectura de memoria compartida
- Diseño de fase `sdd-qa` con Manual QA Protocol
- Diseño de onboarding progresivo
- Propuesta de API REST convencional

### Pendiente 🔲
- Decidir nombre del sistema de memoria
- Decidir stack (PHP vs Go)
- Confirmar umbrales de niveles onboarding
- Escribir PRD completo
- Diseño detallado de componentes
- Implementación (4 MVPs)

## Próximos Pasos

1. **Usuario responda pendientes**:
   - Nombre del sistema (Cortex, Nexus, Bitácora, otro)
   - Stack backend/CLI (PHP vs Go)
   - Umbrales de niveles (OK con 0-10, 11-30, 30+?)

2. **Ejecutar `/sdd-new jarvis-dev-prd`**: PRD completo (~800 líneas)

3. **Fase de implementación**:
   - MVP 1: Memoria compartida funcionando
   - MVP 2: Fase `sdd-qa`
   - MVP 3: Onboarding progresivo
   - MVP 4: Sistema de Persona

## Documentación

- **[Análisis Completo](docs/ANALYSIS.md)**: Documento consolidado con todos los requerimientos y decisiones
- **[Memorias](docs/memories/)**: Observaciones individuales guardadas durante el análisis

## Prioridades

1. **A (CRÍTICO)**: Memoria compartida cross-team funcionando YA
2. **C (IMPORTANTE)**: QA/testing manual mejorado (checklist + bloqueante)
3. **B (DESEABLE)**: Estandarización de código (linters, formatters)

---

**Última actualización**: 2026-04-09  
**Proyecto pausado**: Temporalmente por otras tareas  
**Contacto**: CTO Conpas
