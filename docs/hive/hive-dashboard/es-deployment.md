# Desplegar el Hive Dashboard en un VPS

Esta guía explica cómo desplegar el Hive Dashboard en el mismo VPS donde ya corre `hive-api`.

La URL de producción debería ser:

```text
https://hivemem.dev/dashboard/
```

Usar la barra final. Evita problemas innecesarios con assets y rutas del frontend.

## Qué se está desplegando

El dashboard no es un proceso servidor separado. Es un frontend estático compilado desde `hive-dashboard/` y servido por `hive-api` bajo `/dashboard/`.

En producción:

| Componente | Rol |
|------------|-----|
| `postgres` | Almacena los datos de Hive API. |
| `api` | Ejecuta `hive-api` y sirve `/dashboard/`. |
| Reverse proxy | Termina HTTPS y reenvía tráfico a `127.0.0.1:8080`. |

El contenedor de producción no ejecuta Vite ni un servidor Node. Node se usa solo durante el build de Docker para generar archivos estáticos del dashboard.

## Archivos recomendados para producción

Usar el Compose de producción:

```text
hive-api/deploy/docker-compose.prod.yml
```

Usar el archivo de entorno de ejemplo como plantilla:

```text
hive-api/deploy/.env.prod.example
```

En el VPS, copiarlo a `.env` y reemplazar todos los valores:

```bash
cd /opt/hive-api/hive-api/deploy
cp .env.prod.example .env
```

No commitear nunca el `.env` real.

## 1. Preparar DNS

Apuntar `hivemem.dev` a la IP pública del VPS.

Comprobarlo desde la máquina local:

```bash
dig hivemem.dev
```

La IP devuelta debería coincidir con el VPS.

## 2. Preparar secretos

Editar el `.env` del servidor:

```bash
cd /opt/hive-api/hive-api/deploy
nano .env
```

Configurar al menos:

```env
POSTGRES_USER=hive
POSTGRES_PASSWORD=replace-with-a-real-password
POSTGRES_DB=hivedb
DATABASE_URL=postgres://hive:replace-with-a-real-password@postgres:5432/hivedb?sslmode=disable
JWT_SECRET=replace-with-a-real-secret
PORT=8080
GIN_MODE=release
CORS_ALLOWED_ORIGINS=https://hivemem.dev
```

Si la contraseña de base de datos contiene caracteres especiales de URL, codificarlos en `DATABASE_URL`. Prestar especial atención a `@`, `:`, `/`, `?`, `#` y `%`.

Generar un `JWT_SECRET` fuerte con:

```bash
openssl rand -base64 48
```

## 3. Traer el código actualizado

En el VPS:

```bash
cd /opt/hive-api
git fetch origin master
git switch master
git pull --ff-only origin master
```

Esto actualiza el repositorio a la versión que contiene el dashboard y el Compose de producción.

## 4. Validar Compose antes de arrancar

```bash
cd /opt/hive-api/hive-api/deploy
docker compose -f docker-compose.prod.yml config > /tmp/hive-compose-prod.yml
```

Si el comando termina correctamente, Compose puede leer el archivo e interpolar los valores del `.env`.

## 5. Construir y reiniciar

```bash
cd /opt/hive-api/hive-api/deploy
docker compose -f docker-compose.prod.yml up -d --build
```

Esto construye `hive-api`, compila los assets del dashboard e inicia los contenedores.

## 6. Verificar contenedores

```bash
docker compose -f docker-compose.prod.yml ps
```

Resultado esperado:

- `postgres` está healthy.
- `api` está running.
- `api` expone `127.0.0.1:8080->8080/tcp`, no `0.0.0.0:8080`.

## 7. Configurar reverse proxy HTTPS

El dominio público debe apuntar al reverse proxy, y el proxy debe reenviar las peticiones a `127.0.0.1:8080`.

### Ejemplo con Caddy

```caddyfile
hivemem.dev {
    reverse_proxy 127.0.0.1:8080
}
```

### Ejemplo con Nginx

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

Después de configurar Nginx, añadir TLS con Certbot o el proceso habitual de certificados.

## 8. Verificar el dashboard

```bash
curl -I https://hivemem.dev/dashboard/
```

Resultado esperado:

- HTTP `200`.
- `Content-Type` compatible con `text/html`.

Luego abrir:

```text
https://hivemem.dev/dashboard/
```

Resultado esperado:

- Carga la pantalla de login.
- Los assets JavaScript y CSS cargan sin `404`.
- Las vistas autenticadas del dashboard cargan después de iniciar sesión.

## Solución de problemas

### `/dashboard/` devuelve 404

Reconstruir el stack de producción:

```bash
docker compose -f docker-compose.prod.yml up -d --build
```

Después revisar logs:

```bash
docker compose -f docker-compose.prod.yml logs api
```

### CSS o JavaScript devuelve 404

Usar la URL con barra final:

```text
https://hivemem.dev/dashboard/
```

Confirmar también que el reverse proxy no elimina `/dashboard` de la ruta reenviada.

### Compose falla antes de arrancar

Ejecutar:

```bash
docker compose -f docker-compose.prod.yml config
```

La mayoría de fallos aquí significan que falta una variable obligatoria en `.env`.

## Follow-up

El issue #70 registra la remediación de auditoría de dependencias de desarrollo del dashboard. No bloquea este despliegue de producción porque la imagen runtime sirve archivos estáticos compilados y no ejecuta Node.
