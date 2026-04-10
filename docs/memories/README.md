# Memorias del Proyecto Jarvis-Dev

Este directorio contiene todas las memorias (decisiones, arquitectura, patterns, etc.) exportadas desde Engram para permitir la importación en otro entorno.

## Estructura

```
memories/
  ├── architecture/     # Decisiones de arquitectura técnica
  ├── decision/         # Decisiones de producto/proceso
  ├── pattern/          # Patrones y templates
  ├── discovery/        # Descubrimientos durante análisis
  └── session_summary/  # Resúmenes de sesiones de trabajo
```

## Cómo Importar Memorias

**Cuando Jarvis-Dev esté desarrollado**:
```bash
# Clonar repo
git clone git@github.com:Thrasno/jarvis-dev.git
cd jarvis-dev

# Importar memorias (comando futuro cuando CLI esté listo)
jarvis mem import docs/memories/
```

**Mientras tanto (usando Engram directamente)**:
```bash
# Las memorias están en formato Markdown con frontmatter YAML
# Puedes leerlas manualmente o importarlas a Engram cuando esté disponible el comando
engram sync import docs/memories/
```

## Convención de Nombres

Archivos: `{id}-{slug}.md`
- `id`: ID de la observación en Engram
- `slug`: Descripción corta (kebab-case)

Ejemplo: `37-hive-nombre.md`

## Metadatos (Frontmatter)

Cada memoria incluye:
```yaml
---
id: <observation_id>
type: <architecture|decision|pattern|discovery|session_summary>
project: jarvis-dev
scope: project
session: manual-save-jarvis-dev
created_at: YYYY-MM-DD HH:MM:SS
---
```

## Protocolo de Sincronización

Según memoria #36 (Git Protocol), **SIEMPRE** que hagas push:

1. Exportar memorias actualizadas: `jarvis mem export docs/memories/`
2. Verificar cambios: `git status docs/memories/`
3. Agregar al commit: `git add docs/memories/`
4. Commit descriptivo: `git commit -m "update: memorias - nueva decisión X"`
5. Push: `git push`

Esto asegura que el contexto completo esté disponible en cualquier PC donde se clone el repo.
