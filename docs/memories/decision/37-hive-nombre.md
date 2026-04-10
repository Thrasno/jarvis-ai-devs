---
id: 37
type: decision
project: jarvis-dev
scope: project
session: manual-save-jarvis-dev
created_at: 2026-04-10 07:00:11
---

# Nombre del sistema de memoria: Hive

**What**: Se decidió el nombre "Hive" para el sistema de memoria compartida del ecosistema Jarvis-Dev

**Why**: Necesitábamos nombre diferenciador para el componente de memoria (Jarvis-Dev = ecosistema completo, Hive = solo memoria)

**Where**: Diseño de arquitectura — componente de memoria

**Decision**: Opción 1 (Simple) — Hive es TODO
- `hive-daemon`: Servicio local (SQLite)
- `hive-api`: Servidor cloud (PostgreSQL)
- `hive` CLI: Comandos del usuario (timeline, sync, export, import)

**Metáfora**: Colmena = mente colmena, memoria compartida cross-team

**Learned**: 
- Usuario rechazó nombres como Cortex, Chronicle, Synapse
- Hive refleja la naturaleza colaborativa del equipo (8 devs)
- Simple es mejor: un solo nombre, detalles internos escondidos
- Usuario NO necesita diferenciar local vs cloud (es transparente)
