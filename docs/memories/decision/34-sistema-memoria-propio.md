---
id: 34
type: decision
project: jarvis-dev
scope: project
session: manual-save-jarvis-dev
created_at: 2026-04-09 20:19:54
---

# Decisiones críticas arquitectura - Sistema de memoria propio

**What**: Decisiones arquitectónicas finales sobre sistema de memoria compartida

**Why**: Usuario rechazó usar Engram como base, quiere desarrollar sistema propio con approach personalizado

**Where**: Análisis pre-PRD

**Decisiones Tomadas**:

## 1. NO usar Engram de base
- **Decisión**: Desarrollar sistema de memoria propio desde cero
- **Razón**: "No tenemos tiempo para esto, hagámoslo bien"
- **Implicación**: Diseñar arquitectura, API, storage, sync desde cero

## 2. Guardado Automático (Proactivo)
- **Decisión**: IA decide AUTOMÁTICAMENTE qué guardar
- **Usuario NO debe indicar**: Sistema es suficientemente autónomo
- **Criterio**: IA evalúa si decisión/cambio es meritorio de memoria

## 3. Sync Manual (v1)
- **Decisión**: Primera versión con sync manual (`jarvis sync`)
- **Auto-sync**: Backlog para v2+
- **Razón**: Iterar rápido, validar approach primero

## 4. API REST Convencional
- **Decisión**: Usuario prefiere lenguaje de API más familiar
- **Rechazado**: Nombres tipo Engram (observations, topic_key, etc.)
- **Pendiente**: Proponer nombres nuevos para endpoints/conceptos

## 5. Nombre del Sistema
- **Pendiente**: Proponer nombres creativos para sistema de memoria

## 6. Tests QA
- **Decisión**: Opción B (Markdown checklist) CONFIRMADA
- **Razón**: Más simple, portable, familiar para equipo

## 7. Onboarding Niveles
- **Decisión**: Sistema de niveles 1-2-3 APROBADO
- **Pendiente**: Confirmar umbrales (0-10, 11-30, 30+)

**Learned**:
- Usuario quiere control total del stack (no depender de Engram existente)
- Prioriza hacerlo bien vs rápido (filosofía correcta)
- Prefiere APIs REST tradicionales (probablemente por background PHP/Zoho)
- Sync manual primero = approach pragmático (validar antes de automatizar)
