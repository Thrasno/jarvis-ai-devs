# Setup Recovery Guide (Manual)

Este documento describe cómo recuperar el entorno cuando `jarvis setup` falla de forma parcial.

> No existe rollback automático en este flujo.

## 1) Identificar el estado parcial

Revisá si existen estos artefactos:

- `~/.jarvis/config.yaml`
- `~/.jarvis/memory.db`
- `~/.jarvis/sync.json`
- Configuración de agentes (Claude/OpenCode) en sus directorios locales

## 2) Restaurar credenciales cloud (si corresponde)

Si elegiste `local-only`, el apply elimina credenciales almacenadas de Hive Cloud (`sync.json`).

Si querés volver a modo cloud:

1. Ejecutá `jarvis setup` nuevamente.
2. Elegí scope `local+cloud`.
3. Autenticá con email/password en el paso de cloud.

## 3) Limpiar estado local roto

Si querés resetear completamente:

```bash
rm -f ~/.jarvis/config.yaml
rm -f ~/.jarvis/sync.json
rm -f ~/.jarvis/memory.db
```

Luego corré de nuevo:

```bash
jarvis setup
```

## 4) Limpiar configuración de agentes (si quedó inconsistente)

Si la configuración de un agente quedó a medio aplicar, eliminá manualmente las entradas MCP/instructions del agente afectado y corré `jarvis setup` otra vez.

## 5) Reintento recomendado

1. Confirmá el scope correcto (`local-only` o `local+cloud`).
2. Verificá credenciales cloud solo si usás `local+cloud`.
3. Aplicá nuevamente y revisá salida final.
