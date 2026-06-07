# Desplegar el Hive Dashboard

Esta guía explica cómo publicar el Hive Dashboard en el mismo servidor donde corre `hive-api`, usando una URL pública como `https://hivemem.dev/dashboard/`.

El dashboard no es un servicio backend separado. Es una aplicación web estática que se compila desde `hive-dashboard/` y que `hive-api` sirve bajo `/dashboard/`.

## Ruta rápida

1. Apuntar el dominio al servidor donde corre `hive-api`.
2. Configurar `hive-api` con un `JWT_SECRET` fuerte.
3. Construir/desplegar el contenedor de `hive-api`, que también compila los assets del dashboard.
4. Exponer la aplicación por HTTPS en `https://hivemem.dev/dashboard/`.
5. Abrir el dashboard e iniciar sesión con el flujo de credenciales/token usado por `hive-api`.

Usar la barra final en documentación y marcadores:

```text
https://hivemem.dev/dashboard/
```

`https://hivemem.dev/dashboard` puede redirigir, pero la barra final evita problemas innecesarios con rutas y assets del frontend.

## Qué corre en cada sitio

| Pieza | Dónde corre | Notas |
|-------|-------------|-------|
| `hive-api` | Servidor | Sirve la API y los archivos estáticos del dashboard. |
| Hive Dashboard | Archivos estáticos compilados | En producción no necesita un proceso Node separado. |
| PostgreSQL | Servidor o base de datos gestionada | Lo usa `hive-api`, no el dashboard directamente. |
| Reverse proxy | Borde del servidor | Termina HTTPS y reenvía el tráfico a `hive-api`. |

El dashboard de producción no ejecuta `vite`, `npm` ni un servidor Node. Esas herramientas solo se usan para compilar los archivos estáticos.

## Requisitos previos

Antes de desplegar, confirmar que se tiene:

- Un servidor Linux donde pueda correr `hive-api`.
- Un dominio o subdominio, por ejemplo `hivemem.dev`.
- Acceso DNS para ese dominio.
- Docker y Docker Compose instalados en el servidor.
- Un valor de producción para `JWT_SECRET`.
- La configuración de conexión a base de datos para `hive-api`.

## 1. Apuntar DNS al servidor

Crear o actualizar un registro `A`:

| Campo DNS | Valor |
|-----------|-------|
| Nombre | `hivemem.dev` o el subdominio elegido |
| Tipo | `A` |
| Valor | IP pública del servidor |

Si se usa IPv6, añadir también un registro `AAAA`.

Después de guardar DNS, esperar la propagación. Se puede comprobar desde la máquina local:

```bash
dig hivemem.dev
```

La IP devuelta debería ser la IP pública del servidor.

## 2. Configurar variables de entorno

En el servidor, mantener los secretos fuera del repositorio Git. Una opción común es usar un archivo `.env` de despliegue junto al archivo de Compose.

Ejemplo:

```env
JWT_SECRET=replace-with-a-long-random-production-secret
GIN_MODE=release

# Ejemplo solamente. Usar los valores reales del servidor/base de datos.
DATABASE_URL=postgres://hive:replace-me@postgres:5432/hive?sslmode=disable
```

Reglas importantes:

- `JWT_SECRET` es obligatorio. No usar un valor público por defecto.
- Usar un valor largo y aleatorio para `JWT_SECRET`.
- No commitear el archivo `.env`.
- Mantener privadas las credenciales de base de datos.

Generar un secreto fuerte con:

```bash
openssl rand -base64 48
```

## 3. Construir e iniciar `hive-api`

Desde la raíz del repositorio en el servidor, usar los archivos de despliegue de `hive-api`.

Flujo típico:

```bash
cd /opt/jarvis-dev
git fetch --all
git checkout master
git pull --ff-only
docker compose -f hive-api/deploy/docker-compose.yml up -d --build
```

Esto debería:

1. Compilar el dashboard desde `hive-dashboard/`.
2. Copiar los archivos compilados al runtime image de `hive-api`.
3. Iniciar `hive-api` con `DASHBOARD_ASSETS_DIR` apuntando a esos archivos compilados.
4. Servir el dashboard bajo `/dashboard/`.

Si el despliegue usa otro proyecto Compose o rutas personalizadas, mantener el mismo principio: el proceso runtime de `hive-api` debe conocer la ubicación de los assets compilados mediante `DASHBOARD_ASSETS_DIR`.

## 4. Poner HTTPS delante de `hive-api`

Los usuarios deberían acceder al dashboard por HTTPS:

```text
https://hivemem.dev/dashboard/
```

El reverse proxy recibe el tráfico público en los puertos `80` y `443`, y luego lo reenvía al puerto interno de `hive-api`.

### Opción A: ejemplo con Caddy

Caddy es la opción más simple porque puede gestionar certificados TLS automáticamente.

Ejemplo de `Caddyfile`:

```caddyfile
hivemem.dev {
    reverse_proxy 127.0.0.1:8080
}
```

Usar el puerto local real donde escucha `hive-api`. Si Compose mapea `hive-api` a otro puerto del host, reemplazar `8080`.

### Opción B: ejemplo con Nginx

Ejemplo de server block de Nginx:

```nginx
server {
    listen 80;
    server_name hivemem.dev;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

Después configurar TLS con Certbot o con el proceso estándar de certificados.

## 5. Verificar el despliegue

Ejecutar estas comprobaciones desde la máquina local.

### Health de la API

Usar el endpoint de health configurado para `hive-api`:

```bash
curl -i https://hivemem.dev/health
```

Resultado esperado:

- HTTP `200`, o la respuesta saludable documentada para `hive-api`.

### HTML del dashboard

```bash
curl -I https://hivemem.dev/dashboard/
```

Resultado esperado:

- HTTP `200`.
- Un content type compatible con HTML.

### Assets del dashboard

Abrir el dashboard en el navegador:

```text
https://hivemem.dev/dashboard/
```

Resultado esperado:

- Carga la pantalla de login.
- No hay archivos JavaScript o CSS faltantes en la consola del navegador.
- Los errores de login muestran un mensaje visible en vez de una página en blanco.
- Después de iniciar sesión, las vistas read-only cargan usando endpoints de `hive-api`.

## Problemas comunes

### `404` en `/dashboard/`

Causas probables:

- Los assets del dashboard no se compilaron.
- `DASHBOARD_ASSETS_DIR` apunta al directorio incorrecto.
- La imagen del contenedor no se reconstruyó después de traer los cambios del dashboard.

Probar:

```bash
docker compose -f hive-api/deploy/docker-compose.yml up -d --build
```

### El dashboard carga, pero JS/CSS devuelve `404`

Causas probables:

- La URL no tiene la barra final.
- El reverse proxy reescribe mal las rutas.
- El dashboard se compiló con un base path inesperado.

Usar:

```text
https://hivemem.dev/dashboard/
```

Después verificar que el reverse proxy reenvía la ruta completa a `hive-api` sin eliminar `/dashboard`.

### El login funciona localmente pero no en el servidor

Causas probables:

- `JWT_SECRET` incorrecto o el servidor se reinició con otro secreto.
- URL base de API o headers del proxy incorrectos.
- HTTPS no está configurado correctamente.

Comprobar:

- `JWT_SECRET` se mantiene estable entre reinicios.
- El navegador usa `https://hivemem.dev/dashboard/`.
- El reverse proxy configura `X-Forwarded-Proto`.

### El contenedor arranca localmente pero falla en producción

Causas probables:

- Faltan variables de entorno obligatorias.
- La base de datos no es accesible desde el contenedor.
- Permisos de archivos impiden que el proceso runtime lea los assets del dashboard.

Revisar logs:

```bash
docker compose -f hive-api/deploy/docker-compose.yml logs hive-api
```

## Checklist de despliegue

- [ ] DNS apunta `hivemem.dev` al servidor.
- [ ] HTTPS está configurado.
- [ ] `JWT_SECRET` está definido con un valor privado fuerte.
- [ ] La configuración de base de datos está definida.
- [ ] El contenedor de `hive-api` compila correctamente.
- [ ] Los assets del dashboard están incluidos en la imagen runtime.
- [ ] `https://hivemem.dev/dashboard/` devuelve el HTML del dashboard.
- [ ] La pantalla de login carga en el navegador.
- [ ] Las vistas read-only autenticadas cargan correctamente.

## Trabajo de seguridad pendiente

El dashboard puede correr en producción sin dependencias Node en runtime. Aun así, las herramientas de desarrollo/compilación deben mantenerse sanas.

El seguimiento de la remediación de dependencias está en:

```text
https://github.com/Thrasno/jarvis-ai-devs/issues/70
```

Ese follow-up debe hacerse desde una rama o worktree nuevo basado en `master`.
