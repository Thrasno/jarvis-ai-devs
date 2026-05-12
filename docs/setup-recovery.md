# Setup Recovery + Managed Lifecycle Guide

Este documento cubre recuperación de setup y la semántica final del lifecycle managed en v1 (Claude Code + OpenCode).

## Alcance v1 y reglas duras

- Providers soportados: `claude`, `opencode`, `all`.
- Overwrite policy: **solo** dentro de boundaries explícitamente managed.
- `mem_sync`: fuera de scope (no participa de verify/doctor/reconcile/backup/restore/uninstall).
- Cualquier mutación managed (`reconcile`, `uninstall`) exige backup previo y verificación posterior.

## Comandos lifecycle (v1)

### `jarvis verify --provider claude|opencode|all`
- Read-only.
- Valida contrato, ledger y artefactos managed.
- Clasifica drift en `owned`, `non-owned`, `unknown`.

### `jarvis doctor --provider ...`
- Read-only.
- Ejecuta verify + plan de remediación.
- Emite un plan determinístico por provider con `reason_code`, `class`, `safety_class`, `safe_to_auto_apply`, `backup_needed` y `next_action`.
- Marca pasos como `auto-safe` solo cuando apuntan a un asset managed owned con mapping explícito; lo ambiguo/non-owned/unknown queda como `manual-required`.

### `jarvis reconcile --provider ... --yes` (o `--dry-run`)
- `--dry-run` renderiza el mismo plan real derivado de `doctor` y no escribe backups ni muta archivos.
- `--yes` aplica solo pasos `safe_to_auto_apply=true`, `safety_class=auto-safe` y con `asset_id` owned resoluble.
- Crea snapshot antes del primer write cuando hay acciones elegibles.
- Ejecuta post-verify después de aplicar; si falla, devuelve error estructurado con acción sugerida de restore.
- Si solo hay pasos manuales/non-owned/unknown, no crea backup ni aplica cambios; reporta `manual_required` para guiar reparación humana.

#### Campos de diagnóstico

- `reason_code`: taxonomía estable de causa (`managed_artifact_missing`, `managed_artifact_boundary_invalid`, `non_owned_drift`, `unknown_drift`, `manual_invariant_drift`, `provider_schema_mismatch`).
- `class`: clasificación funcional del drift (`owned` o `manual-required`).
- `safety_class`: elegibilidad de mutación (`auto-safe`, `manual-required`, `non-owned`).
- `safe_to_auto_apply`: `true` solo para fixes managed owned que Jarvis puede aplicar automáticamente.
- `next_action`: guía humana explícita para aplicar, revisar o reparar manualmente.

### `jarvis backup --provider ...`
- Ejecuta snapshot explícito para assets managed del provider.
- Guarda archive comprimido + manifest versionado con checksums.

### `jarvis restore --provider ... --snapshot <id>`
- Valida manifest + checksums + allowed roots (`~/.claude`, `~/.config/opencode`, `~/.jarvis`).
- Bloquea path traversal/roots no permitidos antes de escribir.

### `jarvis uninstall --provider ... --yes` (o `--dry-run`)
- v1 **no** soporta `--soft` ni `--purge`.
- Requiere backup previo y post-verify.
- En modo `all`, limpia ledger managed al finalizar correctamente.

## Error envelope esperado

Los comandos lifecycle devuelven errores estructurados con:
- `code`
- `asset`
- `scope`
- `stage`
- `next_action`

Esto permite automatizar handling en CI/scripts sin parseo frágil de strings libres.

## Rollout / recuperación de setup parcial

1. **Diagnóstico inicial**: correr `jarvis verify --provider all`.
2. **Plan**: correr `jarvis doctor --provider all` y revisar pasos.
3. **Backup preventivo extra (opcional)**: `jarvis backup --provider all`.
4. **Reparación**: `jarvis reconcile --provider all --yes`.
5. **Si algo sale mal**: `jarvis restore --provider <p> --snapshot <id>` y volver a verificar.

## Limpieza manual extrema (último recurso)

Si necesitás reset completo local:

### Linux/macOS (Bash)

```bash
rm -f ~/.jarvis/config.yaml
rm -f ~/.jarvis/sync.json
rm -f ~/.jarvis/memory.db
```

### Windows (PowerShell)

```powershell
Remove-Item -Force "$HOME/.jarvis/config.yaml" -ErrorAction SilentlyContinue
Remove-Item -Force "$HOME/.jarvis/sync.json" -ErrorAction SilentlyContinue
Remove-Item -Force "$HOME/.jarvis/memory.db" -ErrorAction SilentlyContinue
```

Luego:

```bash
jarvis setup
jarvis verify --provider all
```
