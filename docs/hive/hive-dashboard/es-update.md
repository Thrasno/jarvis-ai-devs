# Desplegar cambios del dashboard en producción

Esta guía explica cómo publicar nuevo código del dashboard en un VPS que ya tiene el stack de producción en ejecución.

Para la configuración inicial, consultar [es-deployment.md](es-deployment.md).

## Cuándo usar esta guía

Usar esta guía cada vez que haya cambios en el código del frontend del dashboard — TypeScript, CSS, componentes, rutas o cualquier archivo bajo `hive-dashboard/` — que deban publicarse en el VPS.

No usar esta guía para configurar DNS, el reverse proxy, los secretos del `.env` ni para provisionar el servidor por primera vez. Esos pasos son operaciones únicas cubiertas en `es-deployment.md`.

## Por qué `--build` es siempre obligatorio

Los assets del dashboard se compilan dentro de la imagen Docker en tiempo de build. Cuando se ejecuta `docker compose --build`, la etapa `dashboard-builder` (Node 22) compila el frontend Vite/TypeScript y los archivos estáticos resultantes quedan embebidos en la imagen final.

En tiempo de ejecución no hay servidor Vite ni directorio de fuentes montado. El contenedor sirve exactamente lo que fue incorporado durante el build.

Ejecutar `docker compose up -d` sin `--build` reinicia la imagen existente sin cambios. El nuevo código del dashboard en el repositorio no tiene efecto. Siempre se debe pasar `--build` al desplegar cambios del dashboard.

## Lista de comprobación previa

Antes de comenzar:

- [ ] Los cambios están fusionados en `master` en el repositorio remoto.
- [ ] Se dispone de acceso SSH al VPS.
- [ ] No hay ninguna operación crítica en curso en el dashboard (migraciones, sesiones de usuario activas durante un cambio disruptivo).

## Procedimiento de actualización

Ejecutar estos comandos en el VPS, en orden.

### 1. Traer la última versión de master

```bash
cd /opt/hive-api
git fetch origin master
git switch master
git pull --ff-only origin master
```

Confirmar que se está en master y que el pull se completó correctamente:

```bash
git log --oneline -3
```

### 2. Reconstruir y reiniciar

```bash
cd /opt/hive-api/hive-api/deploy
docker compose -f docker-compose.prod.yml up -d --build
```

Esto reconstruye la imagen con los nuevos assets del dashboard y reemplaza el contenedor en ejecución.

### 3. Verificar los contenedores

```bash
docker compose -f docker-compose.prod.yml ps
```

Resultado esperado:

- `postgres` está healthy.
- `api` está running.

### 4. Verificar el dashboard

```bash
curl -I https://hivemem.dev/dashboard/
```

Resultado esperado:

- HTTP `200`.
- `Content-Type` compatible con `text/html`.

Luego abrir `https://hivemem.dev/dashboard/` en un navegador y confirmar que la interfaz actualizada carga correctamente.

## Qué no es necesario rehacer

Una actualización de código no modifica ninguna configuración de infraestructura. Los siguientes elementos no requieren ninguna acción salvo que la infraestructura en sí haya cambiado:

| Elemento | Motivo |
|----------|--------|
| Registros DNS | No se ven afectados por cambios de código. |
| Reverse proxy (Caddy o Nginx) | La configuración no cambia. |
| Certificados TLS | Se gestionan de forma independiente a la aplicación. |
| Secretos en `.env` | No se modifican durante una actualización de código. |
| Datos de PostgreSQL | Persisten en un volumen Docker con nombre; sobreviven al reemplazo del contenedor. |

Solo es necesario modificar estos elementos si el despliegue introduce una nueva variable de entorno, un nuevo puerto o un cambio de configuración.

## Tiempo de inactividad esperado

Compose detiene el contenedor anterior antes de iniciar el reconstruido. Se esperan unos pocos segundos de inactividad durante la transición. Este es el comportamiento normal para un despliegue de instancia única sin balanceador de carga ni configuración blue/green.

## Rollback

Si la nueva versión genera algún problema, revertir al commit anterior que funcionaba correctamente y reconstruir.

### Encontrar el commit anterior que funcionaba

```bash
cd /opt/hive-api
git log --oneline -10
```

### Hacer checkout de ese commit y reconstruir

```bash
git checkout <hash-del-commit-anterior>
cd hive-api/deploy
docker compose -f docker-compose.prod.yml up -d --build
```

Tras confirmar que el rollback es estable, abrir un PR para corregir el problema en master antes de volver a desplegar. No dejar el VPS en un commit detached más tiempo del necesario.

## Revisar logs tras la actualización

```bash
cd /opt/hive-api/hive-api/deploy
docker compose -f docker-compose.prod.yml logs api --tail=50
```

Buscar errores de inicio, fallos en el registro de rutas o problemas con las rutas de assets en la salida del log.

## Errores frecuentes

| Error | Consecuencia |
|-------|--------------|
| Ejecutar `docker compose up -d` sin `--build` | Los assets compilados anteriores permanecen en la imagen; la interfaz actualizada no aparece. |
| Desplegar desde una rama que no es master | Producción ejecuta una versión no probada o incompleta; comprobar con `git branch` antes de desplegar. |
| Ejecutar `docker compose` desde el directorio incorrecto | Compose no puede encontrar `docker-compose.prod.yml`; ejecutar siempre desde `/opt/hive-api/hive-api/deploy` o pasar la ruta completa con `-f`. |
| Olvidar `git pull` antes de `--build` | El build compila la versión anterior del código que ya está en disco. |
