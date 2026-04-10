---
id: 36
type: pattern
project: jarvis-dev
scope: project
session: manual-save-jarvis-dev
created_at: 2026-04-09 20:32:04
---

# Regla de Git: Siempre incluir memorias en cada push

**What**: Protocolo obligatorio para commits y push a git

**Why**: Usuario necesita tener TODAS las memorias disponibles en cualquier PC donde clone el proyecto

**Protocolo**:

## Antes de CADA Push
1. Exportar memorias: `jarvis mem export docs/memories/`
2. Verificar cambios: `git status docs/memories/`
3. Agregar: `git add docs/memories/`
4. Commit descriptivo: `git commit -m "update: memorias - descripción"`
5. Push: `git push`

**Beneficios**:
- Contexto completo en cualquier PC
- Historial versionado en git
- Recovery automático si se pierde DB local

**Learned**: Esta regla es CRÍTICA porque las memorias son SQLite local + PostgreSQL central
