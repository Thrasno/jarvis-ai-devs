---
id: 32
type: architecture
project: jarvis-dev
scope: project
session: manual-save-jarvis-dev
created_at: 2026-04-09 19:53:26
---

# Requerimientos detallados Conpas - Sesión de análisis completa

**What**: Requerimientos completos del ecosistema jarvis-dev para Conpas basados en sesión de preguntas/respuestas

**Why**: CTO respondió preguntas de análisis pre-PRD — info crítica para diseño de arquitectura

**Key Requirements**:
1. **Infraestructura**: 3 VPS (PHP apps) + 1 VPS (GitLab), BD centralizada
2. **Auth**: GitLab users + Microsoft SSO (Zoho), created_by metadata obligatorio
3. **GitLab/GGA**: NO aplica para Zoho (SaaS sin test runners)
4. **QA/Testing**: Fase sdd-qa con Manual QA Protocol (checklist Markdown, bloqueante)
5. **Onboarding**: 3 niveles automáticos, tracking de progreso
6. **Prioridades**: A=Memoria compartida (CRÍTICO), C=QA (IMPORTANTE), B=Estandarización (DESEABLE)
7. **Sistema de Persona**: 2 capas (Base Inmutable + User Preset)

**Learned**: Zoho SaaS = deal-breaker para test runners tradicionales, Manual QA es la realidad
