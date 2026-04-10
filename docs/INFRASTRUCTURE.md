# Documento Técnico de Infraestructura — Jarvis-Dev MVP 1

**Versión**: 1.0.0  
**Fecha**: 10 de Abril, 2026  
**Responsable**: Andrés (CTO Conpas)  
**Audiencia**: Equipo técnico / CTO / Infraestructura

---

## TL;DR — Checklist Técnico

**Antes de arrancar desarrollo**, tener listo:

- [ ] **VPS nuevo** (2GB RAM, 2vCPU, 50GB SSD) — USD $12/mes
- [ ] **Dominio DNS** `hive.conpas.dev` → apuntando a IP del VPS
- [ ] **Firewall** configurado (puertos 22, 80, 443 abiertos)
- [ ] **Docker + Docker Compose** instalado en VPS
- [ ] **SSL certificado** (Let's Encrypt via Certbot)
- [ ] **GitLab SSH keys** configuradas (CTO + VPS pueden pull repos)
- [ ] **PostgreSQL 15** running en Docker
- [ ] **Backups automáticos** (daily dumps a storage externo)
- [ ] **Monitoring** (UptimeRobot + log aggregation)

**Tiempo estimado de setup**: 4-6 horas (una tarde)

---

## Tabla de Contenidos

1. [Arquitectura General](#1-arquitectura-general)
2. [Componentes de Infraestructura](#2-componentes-de-infraestructura)
3. [Especificaciones del VPS](#3-especificaciones-del-vps)
4. [Configuración de Red](#4-configuración-de-red)
5. [Setup Day-1](#5-setup-day-1)
6. [Docker Compose Stack](#6-docker-compose-stack)
7. [Base de Datos (PostgreSQL)](#7-base-de-datos-postgresql)
8. [Backups Strategy](#8-backups-strategy)
9. [Monitoring & Alerting](#9-monitoring--alerting)
10. [Disaster Recovery Plan](#10-disaster-recovery-plan)
11. [Security Hardening](#11-security-hardening)
12. [Costos Detallados](#12-costos-detallados)
13. [Maintenance Plan](#13-maintenance-plan)

---

## 1. Arquitectura General

### Vista de Alto Nivel

```
┌──────────────────────────────────────────────────────────────────┐
│                        DEVELOPERS (8)                             │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐           │
│  │ Dev Machine  │  │ Dev Machine  │  │ Dev Machine  │  ...      │
│  │ + Claude AI  │  │ + Claude AI  │  │ + Claude AI  │           │
│  ├──────────────┤  ├──────────────┤  ├──────────────┤           │
│  │ Jarvis CLI   │  │ Jarvis CLI   │  │ Jarvis CLI   │           │
│  │ Hive Daemon  │  │ Hive Daemon  │  │ Hive Daemon  │           │
│  │ (SQLite)     │  │ (SQLite)     │  │ (SQLite)     │           │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘           │
│         │                  │                  │                   │
│         └──────────────────┼──────────────────┘                   │
│                            │ HTTPS (sync)                         │
└────────────────────────────┼──────────────────────────────────────┘
                             ↓
                  ┌──────────────────────┐
                  │  INTERNET / FIREWALL │
                  └──────────┬───────────┘
                             ↓
┌────────────────────────────────────────────────────────────────────┐
│                 VPS: hive.conpas.dev                               │
│  ┌──────────────────────────────────────────────────────────────┐ │
│  │ Docker Compose Stack                                         │ │
│  ├──────────────────────────────────────────────────────────────┤ │
│  │ ┌────────────────┐  ┌──────────────────┐                    │ │
│  │ │   Nginx        │  │  Hive Cloud API  │                    │ │
│  │ │ (Reverse Proxy)│→ │  (Go + Gin)      │                    │ │
│  │ │   + SSL        │  │  Port: 8080      │                    │ │
│  │ └────────────────┘  └─────────┬────────┘                    │ │
│  │                               │                              │ │
│  │                               ↓                              │ │
│  │                   ┌───────────────────────┐                  │ │
│  │                   │   PostgreSQL 15       │                  │ │
│  │                   │   + FTS (tsvector)    │                  │ │
│  │                   │   Port: 5432          │                  │ │
│  │                   │   Volume: /data/pg    │                  │ │
│  │                   └───────────────────────┘                  │ │
│  └──────────────────────────────────────────────────────────────┘ │
│                                                                    │
│  ┌──────────────────────────────────────────────────────────────┐ │
│  │ Backup Cron Job (daily PostgreSQL dumps)                    │ │
│  │ → /backups/hive-YYYY-MM-DD.sql.gz                            │ │
│  │ → S3/Wasabi (offsite)                                        │ │
│  └──────────────────────────────────────────────────────────────┘ │
└────────────────────────────────────────────────────────────────────┘
```

### Flujo de Datos

1. **Developer codea** con Claude AI (8 seats ya pagadas)
2. **Hive Daemon local** (SQLite) guarda memorias automáticamente
3. **Auto-sync** (on save, on session end) → HTTPS a `hive.conpas.dev`
4. **Hive Cloud API** valida JWT, guarda en PostgreSQL
5. **Otros developers** hacen pull → reciben memorias del equipo
6. **Backup diario** → dump PostgreSQL a S3/Wasabi

---

## 2. Componentes de Infraestructura

### Componentes Nuevos (A Crear)

| Componente | Descripción | Stack | Ubicación |
|------------|-------------|-------|-----------|
| **Hive Cloud VPS** | Servidor central para sync de memorias | Ubuntu 22.04 LTS | DigitalOcean / Hetzner |
| **Hive Cloud API** | REST API para auth + sync | Go + Gin + JWT | Docker en VPS |
| **PostgreSQL 15** | Base de datos central | PostgreSQL + FTS | Docker en VPS |
| **Nginx** | Reverse proxy + SSL termination | Nginx 1.24+ | Docker en VPS |
| **Backup Storage** | Almacenamiento externo para dumps | S3/Wasabi | Cloud |

### Componentes Existentes (Reutilizar)

| Componente | Status Actual | Uso en Jarvis-Dev |
|------------|---------------|-------------------|
| **Claude Team (8 seats)** | ✅ Activo, pagado USD $9,600/año | Developers interactúan con Jarvis via Claude |
| **GitLab Self-Hosted** | ✅ Funcionando en VPS propio | Source code repos (Jarvis CLI, Hive daemon) |
| **DNS (conpas.dev)** | ✅ Controlado por Conpas | Crear subdomain `hive.conpas.dev` |
| **VPS PHP Apps (3 existentes)** | ✅ Recursos libres disponibles | NO usar (queremos VPS dedicado para Hive) |

---

## 3. Especificaciones del VPS

### Requerimientos Mínimos

| Recurso | Especificación | Justificación |
|---------|----------------|---------------|
| **RAM** | 2GB | PostgreSQL (1GB) + API Go (256MB) + Nginx (128MB) + OS (512MB) + buffer |
| **vCPU** | 2 cores | API Go concurrente (1 core) + PostgreSQL (1 core) |
| **Disco** | 50GB SSD | PostgreSQL data (10GB inicial, 30GB growth) + backups locales (7 días × 1GB) + OS (5GB) |
| **Bandwidth** | 2TB/mes | Sync requests (8 devs × 100 req/día × 5KB avg = ~12GB/mes) + buffer |
| **Sistema Operativo** | Ubuntu 22.04 LTS | Estable, soporte 5 años, Docker bien soportado |

### Requerimientos Recomendados (Headroom)

| Recurso | Especificación | Justificación |
|---------|----------------|---------------|
| **RAM** | 4GB | Permite crecimiento sin resize (si superamos 10k memorias) |
| **vCPU** | 2 cores | Suficiente (CPU no es bottleneck en este workload) |
| **Disco** | 80GB SSD | Permite 1 año de crecimiento sin resize |

**Recomendación**: Empezar con mínimos (2GB RAM, 50GB), escalar si métricas muestran necesidad.

---

### Proveedores Recomendados

| Proveedor | Plan | RAM | vCPU | Disco | Precio/Mes | Uptime SLA |
|-----------|------|-----|------|-------|------------|------------|
| **DigitalOcean** | Basic Droplet | 2GB | 2 vCPU | 50GB SSD | USD $12 | 99.99% |
| **Hetzner Cloud** | CX21 | 4GB | 2 vCPU | 40GB SSD | €4.15 (~USD $4.50) | 99.9% |
| **Linode** | Shared 2GB | 2GB | 1 vCPU | 50GB SSD | USD $12 | 99.9% |
| **Vultr** | Regular Performance | 2GB | 1 vCPU | 55GB SSD | USD $12 | 99.99% |

**Recomendación**: **Hetzner** (mejor precio/performance) O **DigitalOcean** (si preferís UX más amigable).

---

## 4. Configuración de Red

### DNS

**Subdomain**: `hive.conpas.dev`

**Registro DNS** (Tipo A):
```
hive.conpas.dev.    IN A    <IP_DEL_VPS>
```

**TTL**: 300 segundos (5 minutos, permite cambios rápidos si movemos VPS)

**Validación**:
```bash
dig hive.conpas.dev +short
# Debería retornar: <IP_DEL_VPS>
```

---

### Firewall (UFW — Ubuntu Firewall)

**Puertos a abrir**:

| Puerto | Protocolo | Servicio | Acceso Desde |
|--------|-----------|----------|--------------|
| **22** | TCP | SSH | CTO IP (o VPN) — NO abrir a 0.0.0.0/0 |
| **80** | TCP | HTTP (redirect a HTTPS) | 0.0.0.0/0 (público) |
| **443** | TCP | HTTPS (Hive API) | 0.0.0.0/0 (público) |

**Configuración UFW**:
```bash
# Default: deny incoming, allow outgoing
sudo ufw default deny incoming
sudo ufw default allow outgoing

# SSH (restringir a IP del CTO si es posible)
sudo ufw allow from <CTO_IP> to any port 22 proto tcp

# HTTP + HTTPS (público)
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp

# Activar firewall
sudo ufw enable
sudo ufw status verbose
```

**IMPORTANTE**: PostgreSQL (5432) NO debe ser accesible desde internet. Solo bind a `127.0.0.1` (localhost) dentro del VPS.

---

### SSL Certificado

**Proveedor**: Let's Encrypt (gratuito, auto-renovación)

**Herramienta**: Certbot

**Setup**:
```bash
# Instalar Certbot
sudo apt update
sudo apt install certbot python3-certbot-nginx -y

# Generar certificado (Nginx debe estar running)
sudo certbot --nginx -d hive.conpas.dev

# Auto-renovación (Let's Encrypt certs expiran cada 90 días)
sudo systemctl enable certbot.timer
sudo systemctl start certbot.timer
```

**Validación**:
```bash
# Verificar que cert existe
sudo certbot certificates

# Test de renovación (dry-run)
sudo certbot renew --dry-run
```

**Nginx config snippet** (auto-generado por Certbot):
```nginx
server {
    listen 443 ssl http2;
    server_name hive.conpas.dev;

    ssl_certificate /etc/letsencrypt/live/hive.conpas.dev/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/hive.conpas.dev/privkey.pem;
    
    # Resto de config...
}
```

---

## 5. Setup Day-1

### Checklist de Instalación (4-6 horas)

#### Paso 1: Provisionar VPS (15 min)

- [ ] Crear VPS en DigitalOcean/Hetzner (Ubuntu 22.04 LTS)
- [ ] Configurar SSH key (CTO public key)
- [ ] Anotar IP pública del VPS
- [ ] Crear usuario `jarvis` (no root)

```bash
# SSH como root
ssh root@<VPS_IP>

# Crear usuario jarvis
adduser jarvis
usermod -aG sudo jarvis

# Copiar SSH keys a jarvis
rsync --archive --chown=jarvis:jarvis ~/.ssh /home/jarvis

# Test login
exit
ssh jarvis@<VPS_IP>
```

---

#### Paso 2: Configurar DNS (5 min)

- [ ] Crear registro A: `hive.conpas.dev` → `<VPS_IP>`
- [ ] Validar con `dig hive.conpas.dev`
- [ ] Esperar propagación DNS (5-15 min)

---

#### Paso 3: Instalar Docker (15 min)

```bash
# Update packages
sudo apt update && sudo apt upgrade -y

# Instalar Docker
curl -fsSL https://get.docker.com -o get-docker.sh
sudo sh get-docker.sh

# Agregar usuario jarvis a grupo docker
sudo usermod -aG docker jarvis
newgrp docker

# Validar instalación
docker --version
# Debería mostrar: Docker version 24.x.x

# Instalar Docker Compose
sudo apt install docker-compose-plugin -y
docker compose version
# Debería mostrar: Docker Compose version v2.x.x
```

---

#### Paso 4: Configurar Firewall (10 min)

```bash
# Instalar UFW (si no está instalado)
sudo apt install ufw -y

# Configurar reglas (ver sección 4 arriba)
sudo ufw default deny incoming
sudo ufw default allow outgoing
sudo ufw allow from <CTO_IP> to any port 22 proto tcp
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp

# Activar
sudo ufw enable

# Validar
sudo ufw status verbose
```

---

#### Paso 5: Instalar Nginx + SSL (30 min)

```bash
# Instalar Nginx
sudo apt install nginx -y

# Instalar Certbot
sudo apt install certbot python3-certbot-nginx -y

# Crear config básico Nginx
sudo nano /etc/nginx/sites-available/hive.conpas.dev
```

**Contenido inicial** (antes de SSL):
```nginx
server {
    listen 80;
    server_name hive.conpas.dev;

    location / {
        return 200 "Hive API - Setup in progress\n";
        add_header Content-Type text/plain;
    }
}
```

```bash
# Activar site
sudo ln -s /etc/nginx/sites-available/hive.conpas.dev /etc/nginx/sites-enabled/
sudo nginx -t
sudo systemctl reload nginx

# Obtener certificado SSL
sudo certbot --nginx -d hive.conpas.dev

# Validar
curl https://hive.conpas.dev
# Debería responder sin errores SSL
```

---

#### Paso 6: Setup PostgreSQL (Docker) (30 min)

Crear `docker-compose.yml`:

```bash
mkdir -p /home/jarvis/hive-cloud
cd /home/jarvis/hive-cloud
nano docker-compose.yml
```

**Contenido** (ver sección 6 para versión completa):
```yaml
version: '3.8'

services:
  postgres:
    image: postgres:15-alpine
    container_name: hive-postgres
    environment:
      POSTGRES_DB: hive
      POSTGRES_USER: hive_user
      POSTGRES_PASSWORD: <GENERAR_PASSWORD_FUERTE>
    volumes:
      - ./data/postgres:/var/lib/postgresql/data
    ports:
      - "127.0.0.1:5432:5432"  # Solo localhost
    restart: unless-stopped
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U hive_user -d hive"]
      interval: 10s
      timeout: 5s
      retries: 5
```

```bash
# Generar password fuerte
openssl rand -base64 32

# Editar docker-compose.yml con password generado

# Iniciar PostgreSQL
docker compose up -d postgres

# Validar
docker compose ps
docker compose logs postgres
```

---

#### Paso 7: Configurar Backups (45 min)

Ver sección 8 para detalles completos.

```bash
# Crear directorio backups
mkdir -p /home/jarvis/backups

# Crear script backup
nano /home/jarvis/backup-hive.sh
```

**Script backup** (ver sección 8).

```bash
# Hacer ejecutable
chmod +x /home/jarvis/backup-hive.sh

# Test manual
./backup-hive.sh

# Configurar cron (daily 3am)
crontab -e
# Agregar:
# 0 3 * * * /home/jarvis/backup-hive.sh >> /home/jarvis/backups/backup.log 2>&1
```

---

#### Paso 8: Configurar Monitoring (30 min)

Ver sección 9 para detalles completos.

- [ ] UptimeRobot: monitor HTTPS `hive.conpas.dev/health`
- [ ] Log aggregation: journalctl + retention 7 días
- [ ] Disk usage alert: script cron check

---

#### Paso 9: Deploy Hive Cloud API (60 min)

**NOTA**: Este paso se hace en **Mes 1 Semana 3-4** (después de desarrollar la API).

```bash
cd /home/jarvis/hive-cloud

# Clonar repo (GitLab)
git clone git@gitlab.conpas.dev:jarvis/hive-cloud.git api
cd api

# Build Docker image
docker build -t hive-cloud-api:latest .

# Agregar a docker-compose.yml
nano ../docker-compose.yml
# (ver sección 6 para config completa)

# Deploy
cd ..
docker compose up -d api

# Validar
curl https://hive.conpas.dev/health
# Debería retornar: {"status":"ok"}
```

---

### Validación Final

- [ ] DNS resuelve: `dig hive.conpas.dev` → `<VPS_IP>`
- [ ] HTTPS funciona: `curl https://hive.conpas.dev` → sin errores SSL
- [ ] PostgreSQL running: `docker compose ps | grep postgres`
- [ ] Backups configurados: `ls -lh /home/jarvis/backups/`
- [ ] Monitoring activo: UptimeRobot recibiendo pings
- [ ] Firewall activo: `sudo ufw status` → active

---

## 6. Docker Compose Stack

### Archivo Completo: `docker-compose.yml`

**Ubicación**: `/home/jarvis/hive-cloud/docker-compose.yml`

```yaml
version: '3.8'

services:
  # PostgreSQL Database
  postgres:
    image: postgres:15-alpine
    container_name: hive-postgres
    environment:
      POSTGRES_DB: hive
      POSTGRES_USER: hive_user
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}  # From .env file
      POSTGRES_INITDB_ARGS: "-E UTF8 --locale=en_US.UTF-8"
    volumes:
      - ./data/postgres:/var/lib/postgresql/data
      - ./init-db.sql:/docker-entrypoint-initdb.d/init-db.sql:ro
    ports:
      - "127.0.0.1:5432:5432"  # Only accessible from localhost
    restart: unless-stopped
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U hive_user -d hive"]
      interval: 10s
      timeout: 5s
      retries: 5
    networks:
      - hive-network

  # Hive Cloud API (Go)
  api:
    image: hive-cloud-api:latest
    container_name: hive-api
    environment:
      - ENV=production
      - DB_HOST=postgres
      - DB_PORT=5432
      - DB_NAME=hive
      - DB_USER=hive_user
      - DB_PASSWORD=${POSTGRES_PASSWORD}
      - JWT_SECRET=${JWT_SECRET}  # From .env file
      - PORT=8080
    ports:
      - "127.0.0.1:8080:8080"  # Only accessible via Nginx
    depends_on:
      postgres:
        condition: service_healthy
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "wget", "--quiet", "--tries=1", "--spider", "http://localhost:8080/health"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 40s
    networks:
      - hive-network
    volumes:
      - ./logs:/app/logs

  # Nginx Reverse Proxy
  nginx:
    image: nginx:1.24-alpine
    container_name: hive-nginx
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./nginx/nginx.conf:/etc/nginx/nginx.conf:ro
      - ./nginx/sites-enabled:/etc/nginx/sites-enabled:ro
      - /etc/letsencrypt:/etc/letsencrypt:ro
      - ./logs/nginx:/var/log/nginx
    depends_on:
      - api
    restart: unless-stopped
    networks:
      - hive-network

networks:
  hive-network:
    driver: bridge
```

---

### Archivo `.env`

**Ubicación**: `/home/jarvis/hive-cloud/.env`

```bash
# PostgreSQL
POSTGRES_PASSWORD=<GENERAR_PASSWORD_FUERTE_32_CHARS>

# JWT Secret (para auth tokens)
JWT_SECRET=<GENERAR_SECRET_FUERTE_64_CHARS>
```

**Generar secrets**:
```bash
# PostgreSQL password
openssl rand -base64 32

# JWT secret
openssl rand -base64 64
```

**IMPORTANTE**: 
- `.env` debe estar en `.gitignore` (NO commitear secrets)
- Backup de `.env` en lugar seguro (1Password, Bitwarden, etc.)

---

### Nginx Config

**Ubicación**: `/home/jarvis/hive-cloud/nginx/sites-enabled/hive.conf`

```nginx
# Rate limiting zone
limit_req_zone $binary_remote_addr zone=api_limit:10m rate=60r/m;

# Upstream (Hive API)
upstream hive_api {
    server api:8080;
}

# HTTP → HTTPS redirect
server {
    listen 80;
    server_name hive.conpas.dev;

    location /.well-known/acme-challenge/ {
        root /var/www/certbot;
    }

    location / {
        return 301 https://$server_name$request_uri;
    }
}

# HTTPS server
server {
    listen 443 ssl http2;
    server_name hive.conpas.dev;

    # SSL certificates (managed by Certbot)
    ssl_certificate /etc/letsencrypt/live/hive.conpas.dev/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/hive.conpas.dev/privkey.pem;

    # SSL config (Mozilla Intermediate)
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers 'ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384';
    ssl_prefer_server_ciphers off;

    # Security headers
    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;
    add_header X-Frame-Options "DENY" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-XSS-Protection "1; mode=block" always;

    # Logs
    access_log /var/log/nginx/hive-access.log;
    error_log /var/log/nginx/hive-error.log;

    # Health check (no rate limit)
    location /health {
        proxy_pass http://hive_api;
        access_log off;
    }

    # API endpoints (rate limited)
    location / {
        limit_req zone=api_limit burst=20 nodelay;
        
        proxy_pass http://hive_api;
        proxy_http_version 1.1;
        
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # Timeouts
        proxy_connect_timeout 60s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;
    }
}
```

---

## 7. Base de Datos (PostgreSQL)

### Schema Inicial

**Ubicación**: `/home/jarvis/hive-cloud/init-db.sql`

```sql
-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Users table
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    email VARCHAR(255) UNIQUE NOT NULL,
    name VARCHAR(255) NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Projects table
CREATE TABLE projects (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) UNIQUE NOT NULL,
    description TEXT,
    created_at TIMESTAMP DEFAULT NOW()
);

-- Memories table
CREATE TABLE memories (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    project_id UUID REFERENCES projects(id) ON DELETE CASCADE,
    
    title VARCHAR(500) NOT NULL,
    content TEXT NOT NULL,
    type VARCHAR(50) NOT NULL,  -- architecture, bugfix, feature, etc.
    scope VARCHAR(20) DEFAULT 'project',  -- project, personal
    topic_key VARCHAR(500),
    
    created_at TIMESTAMP DEFAULT NOW(),
    
    -- Full-text search vector
    search_vector tsvector
);

-- Indexes for performance
CREATE INDEX idx_memories_user ON memories(user_id);
CREATE INDEX idx_memories_project ON memories(project_id);
CREATE INDEX idx_memories_type ON memories(type);
CREATE INDEX idx_memories_topic_key ON memories(topic_key);
CREATE INDEX idx_memories_created_at ON memories(created_at DESC);

-- GIN index for full-text search
CREATE INDEX idx_memories_search ON memories USING GIN(search_vector);

-- Trigger to auto-update search_vector
CREATE OR REPLACE FUNCTION memories_search_vector_update() RETURNS trigger AS $$
BEGIN
    NEW.search_vector :=
        setweight(to_tsvector('english', COALESCE(NEW.title, '')), 'A') ||
        setweight(to_tsvector('english', COALESCE(NEW.content, '')), 'B') ||
        setweight(to_tsvector('english', COALESCE(NEW.topic_key, '')), 'C');
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER tsvector_update BEFORE INSERT OR UPDATE
ON memories FOR EACH ROW EXECUTE FUNCTION memories_search_vector_update();

-- Sessions table (for JWT refresh tokens)
CREATE TABLE sessions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    refresh_token VARCHAR(500) UNIQUE NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_sessions_user ON sessions(user_id);
CREATE INDEX idx_sessions_token ON sessions(refresh_token);
```

---

### Conexión a PostgreSQL

**Desde VPS** (para debugging):
```bash
# Conectar a PostgreSQL container
docker exec -it hive-postgres psql -U hive_user -d hive

# Listar tablas
\dt

# Ver memories
SELECT id, title, type, created_at FROM memories LIMIT 10;

# Salir
\q
```

---

### Backup Manual

```bash
# Dump completo
docker exec hive-postgres pg_dump -U hive_user hive > hive-backup-$(date +%Y%m%d).sql

# Dump comprimido
docker exec hive-postgres pg_dump -U hive_user hive | gzip > hive-backup-$(date +%Y%m%d).sql.gz
```

---

### Restore desde Backup

```bash
# Restaurar desde dump
gunzip -c hive-backup-20260410.sql.gz | docker exec -i hive-postgres psql -U hive_user -d hive
```

---

## 8. Backups Strategy

### Política de Backups

| Tipo | Frecuencia | Retención | Ubicación |
|------|------------|-----------|-----------|
| **Local (VPS)** | Daily 3am | 7 días | `/home/jarvis/backups/` |
| **Offsite (S3/Wasabi)** | Daily 4am | 30 días | `s3://conpas-hive-backups/` |
| **Manual (pre-cambios)** | On-demand | Permanente | Etiquetado con descripción |

---

### Script de Backup Automático

**Ubicación**: `/home/jarvis/backup-hive.sh`

```bash
#!/bin/bash
set -euo pipefail

# Config
BACKUP_DIR="/home/jarvis/backups"
RETENTION_DAYS=7
DB_CONTAINER="hive-postgres"
DB_USER="hive_user"
DB_NAME="hive"
TIMESTAMP=$(date +%Y%m%d-%H%M%S)
BACKUP_FILE="hive-${TIMESTAMP}.sql.gz"

# Create backup directory if not exists
mkdir -p "$BACKUP_DIR"

# Dump PostgreSQL
echo "[$(date)] Starting backup: $BACKUP_FILE"
docker exec "$DB_CONTAINER" pg_dump -U "$DB_USER" "$DB_NAME" | gzip > "$BACKUP_DIR/$BACKUP_FILE"

# Verify backup
if [ -f "$BACKUP_DIR/$BACKUP_FILE" ]; then
    SIZE=$(du -h "$BACKUP_DIR/$BACKUP_FILE" | cut -f1)
    echo "[$(date)] Backup completed: $BACKUP_FILE ($SIZE)"
else
    echo "[$(date)] ERROR: Backup failed!"
    exit 1
fi

# Delete old backups (keep last 7 days)
echo "[$(date)] Cleaning old backups (retention: $RETENTION_DAYS days)"
find "$BACKUP_DIR" -name "hive-*.sql.gz" -type f -mtime +$RETENTION_DAYS -delete

# Upload to S3/Wasabi (optional — configurar si se requiere offsite)
# aws s3 cp "$BACKUP_DIR/$BACKUP_FILE" s3://conpas-hive-backups/

echo "[$(date)] Backup process finished"
```

**Hacer ejecutable**:
```bash
chmod +x /home/jarvis/backup-hive.sh
```

---

### Configurar Cron Job

```bash
crontab -e
```

**Agregar línea**:
```cron
# Backup diario a las 3am (horario del servidor)
0 3 * * * /home/jarvis/backup-hive.sh >> /home/jarvis/backups/backup.log 2>&1
```

**Validar cron**:
```bash
crontab -l
```

---

### Offsite Backup (S3/Wasabi) — Opcional

**Si se requiere backup offsite** (recomendado para disaster recovery):

1. **Crear bucket S3/Wasabi**: `conpas-hive-backups`
2. **Instalar AWS CLI**:
   ```bash
   sudo apt install awscli -y
   ```
3. **Configurar credentials**:
   ```bash
   aws configure
   # Access Key: <S3_ACCESS_KEY>
   # Secret Key: <S3_SECRET_KEY>
   # Region: us-east-1 (o el que corresponda)
   ```
4. **Descomentar línea en script**:
   ```bash
   aws s3 cp "$BACKUP_DIR/$BACKUP_FILE" s3://conpas-hive-backups/
   ```

**Costo S3/Wasabi**:
- Wasabi: USD $5.99/TB/mes (mínimo 1TB = USD $5.99/mes)
- S3 Standard: ~USD $0.023/GB/mes (10GB = USD $0.23/mes)

**Recomendación**: S3 si backups <100GB, Wasabi si >100GB.

---

## 9. Monitoring & Alerting

### UptimeRobot (Uptime Monitoring)

**Setup**:
1. Crear cuenta free en https://uptimerobot.com
2. Agregar monitor:
   - **Type**: HTTPS
   - **URL**: `https://hive.conpas.dev/health`
   - **Interval**: 5 minutos
   - **Alert contacts**: Email CTO + Slack (opcional)
3. Validar: Debe retornar 200 OK

**Alertas**:
- Downtime > 2 minutos → Email + SMS al CTO
- SSL expiration < 7 días → Email

---

### Disk Usage Alert

**Script**: `/home/jarvis/check-disk.sh`

```bash
#!/bin/bash
set -euo pipefail

THRESHOLD=80  # Alert si uso de disco > 80%
USAGE=$(df -h / | awk 'NR==2 {print $5}' | sed 's/%//')

if [ "$USAGE" -gt "$THRESHOLD" ]; then
    echo "WARNING: Disk usage is ${USAGE}% (threshold: ${THRESHOLD}%)"
    # Aquí podrías enviar email o Slack notification
    # curl -X POST <SLACK_WEBHOOK> -d "{\"text\":\"Disk usage alert: ${USAGE}%\"}"
fi
```

**Cron job** (daily check):
```cron
0 9 * * * /home/jarvis/check-disk.sh >> /home/jarvis/logs/disk-check.log 2>&1
```

---

### Log Aggregation

**journalctl** (systemd logs):
```bash
# Ver logs de Docker services
sudo journalctl -u docker -f

# Ver logs últimas 24h
sudo journalctl --since "24 hours ago"

# Ver logs de errores
sudo journalctl -p err
```

**Retención de logs** (configurar journald):
```bash
sudo nano /etc/systemd/journald.conf
```

**Editar**:
```ini
[Journal]
SystemMaxUse=500M
MaxRetentionSec=7day
```

**Aplicar**:
```bash
sudo systemctl restart systemd-journald
```

---

### Docker Logs

**Ver logs de containers**:
```bash
# API logs
docker compose logs -f api

# PostgreSQL logs
docker compose logs -f postgres

# Últimas 100 líneas
docker compose logs --tail=100 api
```

---

## 10. Disaster Recovery Plan

### Escenarios de Falla

#### Falla 1: VPS Completo Down (Hardware Failure)

**Impacto**: Hive Cloud offline, devs NO pueden sync (pero SQLite local sigue funcionando)

**Recovery Time Objective (RTO)**: 2 horas  
**Recovery Point Objective (RPO)**: 24 horas (último backup diario)

**Steps**:
1. **Provisionar nuevo VPS** (15 min)
2. **Configurar DNS** apuntando a nuevo IP (5 min, 15 min propagación)
3. **Instalar Docker + dependencias** (20 min)
4. **Restaurar backup más reciente** (30 min):
   ```bash
   # Copiar backup desde S3
   aws s3 cp s3://conpas-hive-backups/hive-20260410-030000.sql.gz .
   
   # Iniciar PostgreSQL
   cd /home/jarvis/hive-cloud
   docker compose up -d postgres
   
   # Restaurar
   gunzip -c hive-20260410-030000.sql.gz | docker exec -i hive-postgres psql -U hive_user -d hive
   ```
5. **Deploy API** (15 min)
6. **Configurar SSL** (15 min)
7. **Validar** (10 min)

**Total**: ~2 horas

---

#### Falla 2: Corrupción de Base de Datos

**Impacto**: Queries fallan, API retorna errores 500

**Recovery**:
1. **Stop API** para prevenir writes adicionales:
   ```bash
   docker compose stop api
   ```
2. **Diagnosticar corrupción**:
   ```bash
   docker exec hive-postgres psql -U hive_user -d hive -c "REINDEX DATABASE hive;"
   ```
3. **Si reindex NO resuelve → Restore desde backup**:
   ```bash
   # Drop DB
   docker exec hive-postgres psql -U hive_user -d postgres -c "DROP DATABASE hive;"
   docker exec hive-postgres psql -U hive_user -d postgres -c "CREATE DATABASE hive;"
   
   # Restore
   gunzip -c /home/jarvis/backups/hive-<LATEST>.sql.gz | docker exec -i hive-postgres psql -U hive_user -d hive
   ```
4. **Restart API**:
   ```bash
   docker compose up -d api
   ```

**RTO**: 30 minutos  
**RPO**: 24 horas (último backup)

---

#### Falla 3: Pérdida de .env (Secrets)

**Impacto**: NO podemos deployar API (falta JWT_SECRET, POSTGRES_PASSWORD)

**Prevención**:
- Backup de `.env` en 1Password/Bitwarden
- Document secrets en lugar seguro (no en git)

**Recovery**:
1. Restaurar `.env` desde 1Password
2. Si NO hay backup → **resetear secrets**:
   - Generar nuevo `JWT_SECRET` → invalida todos los tokens (devs deben re-login)
   - Generar nuevo `POSTGRES_PASSWORD` → actualizar en PostgreSQL:
     ```bash
     docker exec hive-postgres psql -U hive_user -d postgres -c "ALTER USER hive_user PASSWORD 'NEW_PASSWORD';"
     ```

**RTO**: 15 minutos (si hay backup), 1 hora (si hay que resetear)

---

#### Falla 4: SSL Certificado Expirado

**Impacto**: Clients NO pueden conectar (error SSL)

**Prevención**: Certbot auto-renewal (ver sección 4)

**Recovery**:
```bash
# Forzar renovación
sudo certbot renew --force-renewal

# Reload Nginx
docker compose restart nginx
```

**RTO**: 5 minutos

---

### Runbook: Restore Completo desde Zero

**Escenario**: VPS completamente perdido, empezamos desde cero.

**Pre-requisitos**:
- Backup más reciente en S3/Wasabi
- `.env` file respaldado en 1Password

**Steps**:
1. Provisionar nuevo VPS (15 min)
2. Ejecutar setup Day-1 completo (ver sección 5) — 4 horas
3. Restaurar backup PostgreSQL (30 min)
4. Validar health checks (10 min)

**Total RTO**: ~5 horas  
**RPO**: 24 horas (último backup diario)

---

## 11. Security Hardening

### Checklist de Seguridad

- [x] **Firewall activo** (UFW) — Solo 22, 80, 443 abiertos
- [x] **SSH**: Key-based auth only (no passwords)
- [x] **PostgreSQL**: NO expuesto a internet (bind 127.0.0.1)
- [x] **SSL**: Let's Encrypt con auto-renovación
- [x] **Rate limiting**: Nginx (60 req/min por IP)
- [x] **Security headers**: HSTS, X-Frame-Options, etc.
- [ ] **Fail2Ban** (opcional): Ban IPs con intentos SSH fallidos
- [ ] **Docker security**: User namespaces, AppArmor profiles

---

### SSH Hardening (Opcional)

**Editar** `/etc/ssh/sshd_config`:
```bash
sudo nano /etc/ssh/sshd_config
```

**Cambios recomendados**:
```conf
# Deshabilitar root login
PermitRootLogin no

# Solo key-based auth
PasswordAuthentication no
PubkeyAuthentication yes

# Puerto custom (opcional, evita scanners automáticos)
# Port 2222  # Si lo cambias, actualizar UFW

# Timeout
ClientAliveInterval 300
ClientAliveCountMax 2
```

**Aplicar**:
```bash
sudo systemctl reload sshd
```

---

### Fail2Ban (Opcional)

**Instalar**:
```bash
sudo apt install fail2ban -y
```

**Configurar** `/etc/fail2ban/jail.local`:
```ini
[sshd]
enabled = true
port = ssh
logpath = /var/log/auth.log
maxretry = 3
bantime = 3600
```

**Activar**:
```bash
sudo systemctl enable fail2ban
sudo systemctl start fail2ban
```

---

## 12. Costos Detallados

### Costos de Desarrollo (5 meses)

| Item | Cantidad | Precio Unitario | Total |
|------|----------|-----------------|-------|
| **VPS** (Hetzner CX21) | 5 meses | USD $4.50/mes | USD $22.50 |
| **Dominio** (subdominio existente) | 1 | USD $0 | USD $0 |
| **SSL** (Let's Encrypt) | 1 | USD $0 | USD $0 |
| **S3 Backups** (opcional) | 5 meses × 1GB | USD $0.023/GB/mes | USD $0.12 |
| **Tiempo CTO** | 400 horas | USD $0 (parte de salario) | USD $0 |
| **TOTAL DESARROLLO** | — | — | **USD $22.62** |

---

### Costos Operacionales (Año 1 Post-Launch)

| Item | Frecuencia | Precio | Total Anual |
|------|------------|--------|-------------|
| **VPS** (Hetzner CX21) | Mensual | USD $4.50 | USD $54.00 |
| **S3 Backups** (10GB promedio) | Mensual | USD $0.23 | USD $2.76 |
| **Monitoring** (UptimeRobot free) | Anual | USD $0 | USD $0 |
| **SSL** (Let's Encrypt) | Anual | USD $0 | USD $0 |
| **Mantenimiento CTO** | 2-3h/semana | USD $0 (parte de salario) | USD $0 |
| **TOTAL AÑO 1** | — | — | **USD $56.76** |

**Nota**: Si usamos DigitalOcean en vez de Hetzner:
- Desarrollo: USD $60 (5 meses)
- Año 1: USD $144 (12 meses)

---

### Costos Totales (Inversión Inicial + Año 1)

| Escenario | Desarrollo (5 meses) | Año 1 Operación | **TOTAL** |
|-----------|---------------------|-----------------|-----------|
| **Hetzner** (recomendado) | USD $22.62 | USD $56.76 | **USD $79.38** |
| **DigitalOcean** (alternativa) | USD $60.00 | USD $144.00 | **USD $204.00** |

**Comparación con alternativas**:
- Contratar 1 dev adicional: USD $30,000 - $42,000/año
- SaaS Knowledge Management: USD $960 - $1,920/año
- **Jarvis-Dev**: USD $79 - $204/año

---

## 13. Maintenance Plan

### Tareas Semanales (CTO — 30 min)

- [ ] **Revisar logs** de errores (Docker logs + journalctl)
- [ ] **Validar backups** existen (check `/home/jarvis/backups/`)
- [ ] **Monitorear disk usage** (`df -h`)
- [ ] **Check UptimeRobot** dashboard (uptime %)

---

### Tareas Mensuales (CTO — 1 hora)

- [ ] **Actualizar packages** del VPS:
  ```bash
  sudo apt update && sudo apt upgrade -y
  sudo reboot  # Si hay kernel updates
  ```
- [ ] **Revisar logs de Nginx** (rate limiting, 4xx/5xx errors)
- [ ] **Validar SSL expiration** (`sudo certbot certificates`)
- [ ] **Review disk usage trend** (¿estamos creciendo más rápido de lo esperado?)

---

### Tareas Trimestrales (CTO — 2 horas)

- [ ] **Disaster recovery drill** (test restore desde backup)
- [ ] **Security audit**:
  - Review firewall rules
  - Check for CVEs en Docker images (`docker scan hive-cloud-api`)
  - Review Nginx access logs (intentos de ataque?)
- [ ] **Performance review**:
  - PostgreSQL query performance (`EXPLAIN ANALYZE` en queries lentas)
  - API latency (P95, P99)
- [ ] **Capacity planning**:
  - ¿Necesitamos escalar RAM/CPU/Disk?
  - Proyección: A este ritmo, ¿cuándo nos quedamos sin espacio?

---

### Upgrades Planeados

| Componente | Versión Actual | Upgrade Path | Frecuencia |
|------------|----------------|--------------|------------|
| **PostgreSQL** | 15.x | 15.x → 16.x (cuando sea stable) | Anual |
| **Docker** | 24.x | Auto-update via apt | Mensual |
| **Nginx** | 1.24 | 1.24 → 1.26 (LTS) | Semestral |
| **Ubuntu** | 22.04 LTS | 22.04 → 24.04 LTS (Abril 2027) | Cada 2 años |

---

## Anexo A: Troubleshooting Común

### Problema 1: API retorna 502 Bad Gateway

**Síntoma**: `curl https://hive.conpas.dev` → 502

**Diagnóstico**:
```bash
# Check si API container está running
docker compose ps

# Ver logs de API
docker compose logs --tail=50 api

# Ver logs de Nginx
docker compose logs --tail=50 nginx
```

**Causas comunes**:
- API container crashed → `docker compose restart api`
- PostgreSQL no está ready → `docker compose logs postgres`
- Nginx no puede alcanzar API (network issue) → `docker network inspect hive-network`

---

### Problema 2: Devs NO pueden sync (timeout)

**Síntoma**: Jarvis CLI muestra "Sync failed: connection timeout"

**Diagnóstico**:
```bash
# Validar que VPS responde
curl -I https://hive.conpas.dev/health

# Check firewall
sudo ufw status

# Check Nginx logs
docker compose logs nginx | grep -i error
```

**Causas comunes**:
- DNS no resuelve → `dig hive.conpas.dev` (verificar)
- Firewall bloqueando 443 → `sudo ufw allow 443/tcp`
- SSL certificado expirado → `sudo certbot renew`

---

### Problema 3: PostgreSQL lento (queries >1s)

**Diagnóstico**:
```bash
# Conectar a PostgreSQL
docker exec -it hive-postgres psql -U hive_user -d hive

# Ver queries lentas
SELECT * FROM pg_stat_statements ORDER BY total_exec_time DESC LIMIT 10;

# Ver tamaño de tablas
SELECT 
  schemaname, 
  tablename, 
  pg_size_pretty(pg_total_relation_size(schemaname||'.'||tablename)) AS size
FROM pg_tables
WHERE schemaname NOT IN ('pg_catalog', 'information_schema')
ORDER BY pg_total_relation_size(schemaname||'.'||tablename) DESC;
```

**Soluciones**:
- Reindex: `REINDEX INDEX idx_memories_search;`
- Vacuum: `VACUUM ANALYZE memories;`
- Escalar VPS a 4GB RAM si DB >2GB

---

## Anexo B: Escalamiento Futuro

### Cuándo Escalar (Signals)

| Métrica | Threshold | Acción |
|---------|-----------|--------|
| **Disk usage** | >70% | Resize VPS disk (50GB → 80GB) |
| **RAM usage** | >80% sustained | Resize VPS RAM (2GB → 4GB) |
| **API latency P95** | >500ms | Investigate slow queries, add caching |
| **Concurrent users** | >50 | Consider horizontal scaling (load balancer) |

---

### Escalamiento Vertical (Más recursos en mismo VPS)

**Hetzner CX21 → CX31**:
- RAM: 4GB → 8GB
- CPU: 2 vCPU → 2 vCPU
- Disk: 40GB → 80GB
- Costo: EUR 4.15 → EUR 7.49 (~USD $8/mes)

**DigitalOcean 2GB → 4GB**:
- Costo: USD $12 → $24/mes

**Steps**:
1. Backup completo
2. Resize VPS (desde panel proveedor)
3. Restart
4. Validar

**Downtime**: ~5 minutos

---

### Escalamiento Horizontal (Multiple VPS)

**Cuándo**: Si >100 concurrent users O si queremos HA (High Availability)

**Arquitectura**:
```
                   ┌─────────────┐
                   │ Load Balancer│
                   └──────┬───────┘
                          │
          ┌───────────────┼───────────────┐
          │               │               │
      ┌───▼────┐      ┌───▼────┐      ┌───▼────┐
      │ API 1  │      │ API 2  │      │ API 3  │
      └───┬────┘      └───┬────┘      └───┬────┘
          │               │               │
          └───────────────┼───────────────┘
                          │
                   ┌──────▼───────┐
                   │ PostgreSQL   │
                   │ (shared)     │
                   └──────────────┘
```

**Costo adicional**: 
- Load balancer: USD $10/mes
- API VPS adicional: USD $5/mes cada uno

**No necesario para MVP 1** (8 devs NO van a saturar 1 VPS).

---

**FIN DEL DOCUMENTO**

---

**Siguiente paso**: Ejecutar Setup Day-1 cuando CEO apruebe propuesta ejecutiva.
