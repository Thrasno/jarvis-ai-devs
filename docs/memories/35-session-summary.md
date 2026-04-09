# Memory #35: Session summary jarvis-dev

**Type**: session_summary  
**Created**: 2026-04-09 20:24:27  
**Project**: jarvis-dev  
**Scope**: project

---

## Goal
Diseñar ecosistema IA (jarvis-dev) para equipo Conpas: 8 devs, stack Zoho/PHP, memoria compartida cross-team, QA manual mejorado, onboarding progresivo.

## Instructions
- Usuario prioriza hacerlo BIEN vs rápido
- NO usar Engram de base, desarrollar sistema de memoria propio
- Siempre preguntar antes de cada fase, no asumir
- Personalidad Stark aplicada (tono directo, sin filtro)
- API REST convencional (no jerga tipo Engram)
- Sync manual en v1, auto-sync para futuro

## Discoveries

### Arquitectura de Memoria (Decisión Crítica)
- **Hybrid local + central**: Cada dev usa SQLite local, sincroniza a PostgreSQL central (hub-and-spoke)
- **Guardado 100% automático**: IA decide qué guardar sin intervención del usuario
- **Scopes**: `personal` (solo local) vs `project` (sincroniza)
- **Sync manual v1**: Comando `jarvis sync` (auto-sync backlog para v2)
- **Metadata tracking**: `created_by`, `updated_by`, historial de revisiones
- **Resolución conflictos**: Last-write-wins con historial completo

### Realidad Zoho SaaS (Game Changer)
- **NO test runners**: Zoho es SaaS sin ejecución local, tests 100% manuales
- **GGA descartado**: Code review automático no aplica para Zoho
- **Solución**: Fase `sdd-qa` obligatoria con Manual QA Protocol
- **Formato**: Markdown checklist con inputs, steps, expected outputs, edge cases
- **Bloqueante**: No se puede archive sin confirmar que todos los tests pasaron
- **NO se salta con fast-forward**

### Stack y Contexto
- **Team**: 8 devs (3 avanzados, 5 en distintos niveles, algunos nunca usaron IA)
- **Zoho primario**: SaaS para la mayoría del desarrollo
- **PHP**: Cuando volumetría es alta
- **GitLab**: Self-hosted, usado como backup/versionado (NO CI/CD activo)
- **Auth**: GitLab users + Microsoft SSO (Zoho)
- **VPS**: 3 para apps PHP, 1 para GitLab (recursos disponibles para reutilizar)

### Sistema de Onboarding Progresivo (Innovación vs gentle-ai)
- **3 niveles automáticos**: Beginner (0-10 tasks), Intermediate (11-30), Advanced (30+)
- **Nivel 1**: Solo tareas `complexity: low`, max 3 archivos, NO fast-forward
- **Nivel 2**: Tareas `complexity: medium`, max 8 archivos, fast-forward con warnings
- **Nivel 3**: Sin restricciones
- **Tracking**: `user-stats/{username}` en memoria con contador de tareas, QA success rate
- **Override manual**: Admin puede elevar nivel si es necesario

### Sistema de Persona (2 Capas)
- **Layer 1 (Base Inmutable)**: Comportamiento, expertise, skills, workflow rules (definido por Conpas)
- **Layer 2 (User Preset)**: Tono, idioma, analogías, estilo (editable por usuario)
- **Presets**: X cantidad (usuario revelará después) + opción Custom
- **Validación**: Custom debe mantener estructura, no puede tocar Layer 1
- **Para PRD**: Documentar mecánica, no presets específicos

### API REST del Sistema de Memoria
- **Endpoints propuestos**: `/memories`, `/memories/search`, `/sync`, `/memories/{id}/history`, `/memories/{id}/resolve`
- **Auth**: JWT con GitLab tokens
- **Full-text search**: PostgreSQL `tsvector` + GIN index (no ElasticSearch en v1)
- **Tipos**: decision, bugfix, feature, config, gotcha, convention
- **Scopes**: personal, project

## Accomplished

✅ Análisis profundo de gentle-ai (arquitectura, SDD, skills, Engram, PRD structure)
✅ Extracción de template de PRD (2,366 líneas → adaptar a ~800 para jarvis-dev)
✅ Captura de requerimientos Conpas (2 rondas de preguntas/respuestas)
✅ Diseño de arquitectura hybrid local+central para memoria compartida
✅ Diseño de fase `sdd-qa` con Manual QA Protocol (Markdown checklist)
✅ Diseño de sistema de onboarding progresivo (3 niveles automáticos)
✅ Diseño de API REST convencional para sistema de memoria
✅ Propuestas de nombres para sistema (Cortex, Nexus, Bitácora, etc.)
✅ Decisión de desarrollar sistema propio (NO usar Engram de base)

🔲 Escribir PRD completo (pendiente respuestas finales sobre nombre, stack, umbrales)

## Next Steps

1. **Usuario responda pendientes**:
   - Nombre del sistema de memoria (Cortex, Nexus, Bitácora, otro)
   - Stack: PHP vs Go para backend/CLI
   - Confirmar umbrales de niveles onboarding
   
2. **Ejecutar `/sdd-new jarvis-dev-prd`**: Crear PRD completo con SDD workflow

3. **Fase de diseño**: Arquitectura detallada de componentes (API central, CLI local, sync protocol)

4. **Implementación por fases**:
   - MVP 1: Memoria compartida funcionando
   - MVP 2: Fase sdd-qa con Manual QA Protocol
   - MVP 3: Onboarding progresivo
   - MVP 4: Sistema de Persona

## Relevant Files

- **Ninguno todavía**: Proyecto en fase de análisis pre-PRD
- **Siguiente**: `PRD.md` (por crear), `docs/architecture.md`, `docs/api-spec.md`

## Prioridades Confirmadas

1. **A (CRÍTICO)**: Memoria compartida cross-team funcionando YA
2. **C (IMPORTANTE)**: QA/testing manual mejorado (checklist + bloqueante)
3. **B (DESEABLE)**: Estandarización de código (linters, formatters, conventions)

## Deal Breakers Identificados

- ❌ GGA/test runners tradicionales (Zoho es SaaS sin ejecución local)
- ✅ Memoria compartida cross-team (gentle-ai tiene Engram local only)
- ✅ Manual QA Protocol con fase bloqueante (crítico para mejorar calidad)
- ✅ Desarrollar sistema propio vs reutilizar Engram (control total del stack)
