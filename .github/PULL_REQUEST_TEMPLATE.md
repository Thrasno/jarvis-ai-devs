<!--
Every pull request must close an issue that already has the status:approved label.
Cada pull request debe cerrar una issue que ya tenga la etiqueta status:approved.
-->

## Linked Issue

Closes #

<!-- Replace the placeholder with the approved issue number, for example: Closes #42. -->
<!-- Sustituye el marcador por el número de la issue aprobada, por ejemplo: Closes #42. -->

## Issue Vinculada

La referencia anterior debe usar `Closes`, `Fixes` o `Resolves` y apuntar a una issue con `status:approved`.

## PR Type

Select exactly one option and apply the matching label to the pull request.

- [ ] `type:bug` - Bug fix
- [ ] `type:feature` - New or changed product behaviour
- [ ] `type:chore` - Documentation, refactoring, tests, build, CI, or maintenance

## Tipo de Pull Request

Las tres casillas anteriores son también las opciones normativas para esta sección. Marca exactamente una y aplica la etiqueta correspondiente a la pull request.

- `type:bug` - Corrección de un error
- `type:feature` - Comportamiento de producto nuevo o modificado
- `type:chore` - Documentación, refactorización, pruebas, compilación, CI o mantenimiento

## Summary

<!-- Explain what changed and why in 1-3 concise points. -->

## Resumen

<!-- Explica qué ha cambiado y por qué en uno o tres puntos concisos. -->

## Changes

| File or area | Change |
|--------------|--------|
| `path/to/file` | Brief description |

## Cambios

| Archivo o área | Cambio |
|----------------|--------|
| `ruta/al/archivo` | Descripción breve |

## Verification

Run the checks relevant to every affected Go module. Record additional manual or component-specific validation below.

```bash
go test ./...
go vet ./...
```

- [ ] Relevant tests pass in each affected module
- [ ] `go vet ./...` passes in each affected module
- [ ] Go files are formatted with `gofmt`
- [ ] Other affected components or scripts were validated as applicable

## Verificación

Ejecuta las comprobaciones pertinentes en cada módulo de Go afectado. Indica a continuación cualquier validación manual o específica de un componente.

- [ ] Las pruebas pertinentes se superan en cada módulo afectado
- [ ] `go vet ./...` se supera en cada módulo afectado
- [ ] Los archivos de Go tienen el formato de `gofmt`
- [ ] Se han validado los demás componentes o scripts afectados cuando corresponde

<!-- Describe commands, scenarios, and results. / Describe los comandos, escenarios y resultados. -->

## Contributor Checklist

- [ ] This pull request closes an issue with `status:approved`
- [ ] Exactly one `type:*` label is applied
- [ ] Commits follow Conventional Commits
- [ ] Commits contain no `Co-Authored-By` trailers or other AI attribution
- [ ] Documentation is updated when behaviour or workflows change
- [ ] Generated user configuration was not edited directly; its source template or asset was changed instead

## Lista de Comprobación de la Persona Contribuidora

- [ ] Esta pull request cierra una issue con `status:approved`
- [ ] Se ha aplicado exactamente una etiqueta `type:*`
- [ ] Los commits siguen Conventional Commits
- [ ] Los commits no contienen líneas `Co-Authored-By` ni ninguna otra atribución a IA
- [ ] La documentación se ha actualizado cuando ha cambiado el comportamiento o el flujo de trabajo
- [ ] La configuración de usuario generada no se ha editado directamente; se ha modificado su plantilla o recurso de origen

## Reviewer Notes

<!-- State review focus, risks, rollout concerns, or intentional exclusions. -->

## Notas Para la Revisión

<!-- Indica el foco de la revisión, los riesgos, las consideraciones de despliegue o las exclusiones intencionadas. -->
