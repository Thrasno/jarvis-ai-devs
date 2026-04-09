# Memory #32: Requerimientos detallados Conpas - Sesión de análisis completa

**Type**: architecture  
**Created**: 2026-04-09 19:53:26  
**Project**: jarvis-dev  
**Scope**: project

---

**What**: Requerimientos completos del ecosistema jarvis-dev para Conpas basados en sesión de preguntas/respuestas

**Why**: CTO respondió preguntas de análisis pre-PRD — info crítica para diseño de arquitectura

**Where**: Conversación 2026-04-09 (post-crash recovery)

**Requerimientos Capturados**:

## 1. Infraestructura y Hosting
- **Recursos existentes**: 3 VPS (PHP apps) + 1 VPS (GitLab self-hosted)
- **Memoria compartida**: BD centralizada (preferencia confirmada)
- **Decisión pendiente**: Approach de deployment (1on1 necesario)

## 2. Autenticación y Permisos
- **Auth actual**: GitLab users + Microsoft SSO (Zoho)
- **Memoria compartida v1**: SIN roles/permisos complejos (todos ven todo)
- **REQUERIMIENTO CRÍTICO**: Cada observation DEBE incluir usuario que la creó
  - Ejemplo: "Se cambió sistema de inventario a multialmacén" → metadata: created_by = "andres"
- **Backlog futuro**: Sistema de permisos/roles si se necesita

## 3. GitLab y Code Review (GGA NO aplica)
- **Realidad Zoho**: SaaS sin ejecución local, tests 100% manuales
- **GitLab uso actual**: Backup de sources + versionado (NO CI/CD activo)
- **GGA descartado**: No aplica para Zoho
- **PROPUESTA USUARIO**: Fase `sdd-qa` ANTES de archive
  - IA analiza código realizado
  - Genera checklist de pruebas MANUALES
  - Incluye inputs de ejemplo + resultado esperado
  - Usuario tacha a medida que pasan
  - BLOQUEANTE: no se puede avanzar sin confirmar que todos los tests pasaron

## 4. Onboarding Progresivo
- **Objetivo**: Ecosistema hace onboarding automático
- **Mecánica**: Tareas pequeñas al principio → tareas grandes cuando ganan confianza
- **Migración**: Obligatoria cuando esté ready (opt-out para quien NO quiera IA)
- **Métricas de adopción**: Backlog futuro (NO versión inicial)

## 5. QA y Testing (CRÍTICO)
- **Realidad**: NO test runners (Zoho SaaS)
- **Flujo deseado**:
  1. IA revisa código generado
  2. IA propone mejoras (si las hay)
  3. IA genera checklist de pruebas MANUALES (inputs, expected outputs, edge cases)
  4. Usuario ejecuta pruebas en Zoho
  5. Usuario confirma pass/fail
  6. BLOQUEANTE: no se puede archive sin confirmación de que todos pasaron
- **Integración SDD**: Posible fase `sdd-qa` obligatoria (NO se salta con fast-forward)
- **Strict TDD**: Usuario NO conoce el concepto (necesita explicación)

## 6. Prioridades (Ordenadas)
1. **A (CRÍTICO)**: Memoria compartida cross-team funcionando YA
2. **C (IMPORTANTE)**: QA/testing automatizado (checklist manual)
3. **B (DESEABLE)**: Estandarización de código (linters, formatters, conventions)

## 7. Licencias
- **Estado**: Claude Teams (8 seats) compradas por 1 año
- **Budget**: Aprobado, sin problemas

## 8. Sistema de Persona (BONUS TRACK)
- **Arquitectura de 2 capas**:
  - **Base INMUTABLE**: Comportamiento, expertise, skills, workflow (definido por Conpas, NO editable)
  - **Capa PERSONALIZABLE**: Cómo HABLA la IA (tono, idioma, estilo)
- **Presets planeados**: Usuario tiene ideas (revelará más adelante)
- **Modo Custom**: Usuario elige preset base → edita MD → sistema aplica (sin tocar base inmutable)

**Learned**:
- Zoho SaaS = deal-breaker para GGA/test runners tradicionales
- Testing MANUAL es la realidad → IA debe generar test plans, no ejecutar tests
- Fase `sdd-qa` bloqueante es KEY para mejorar calidad (prioridad C)
- Memoria compartida requiere tracking de usuario (created_by) pero SIN permisos complejos en v1
- Onboarding progresivo = feature diferenciador vs gentle-ai
- Sistema de Persona de 2 capas = balance entre control (empresa) y flexibilidad (usuario)
