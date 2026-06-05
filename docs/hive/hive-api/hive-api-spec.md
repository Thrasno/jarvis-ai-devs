# Hive-API — Especificación (OpenSpec)

## Purpose

Hive-API es una infraestructura de backend **self-hosted** que actúa como registro de réplica y sincronización de memoria entre múltiples instancias locales de agentes de IA (p. ej. Claude y OpenCode).

Su objetivo es **distribuir el conocimiento histórico y las decisiones arquitectónicas** aprendidas por los agentes a través de distintas máquinas físicas, de forma que el contexto de la IA se recupere instantáneamente cuando un desarrollador cambia de equipo o se incorpora un nuevo miembro, manteniendo el **100% de soberanía de los datos** (no es un SaaS público).

---

## Requirements

### Requirement: Despliegue contenerizado sin orquestadores complejos

El sistema SHALL desplegarse mediante contenedores Docker orquestados con Docker Compose, sin requerir orquestadores complejos (p. ej. Kubernetes), exponiendo un backend en Go y una base de datos PostgreSQL como motor de persistencia relacional.

#### Scenario: Arranque del stack en un VPS

- **WHEN** un operador ejecuta `docker compose up` en un VPS con AlmaLinux o Ubuntu, acceso SSH e IP pública
- **THEN** el sistema SHALL levantar el backend en Go y la instancia de PostgreSQL como servicios contenerizados
- **AND** el backend SHALL quedar disponible para recibir peticiones a través del reverse proxy

#### Scenario: Configuración inyectada por entorno

- **WHEN** el stack arranca
- **THEN** el sistema SHALL leer su configuración desde variables de entorno, incluyendo al menos `HIVE_API_TOKEN`, `HIVE_API_ADMIN_TOKEN` y `HIVE_API_ALLOWED_PROJECTS`
- **AND** SHALL fallar el arranque si falta una variable obligatoria de configuración

### Requirement: Terminación TLS obligatoria vía reverse proxy

El sistema SHALL operar exclusivamente detrás de un reverse proxy (Apache o Nginx) con certificados SSL/TLS (vía Let's Encrypt), de modo que **todo** el tráfico viaje por HTTPS.

#### Scenario: Petición entrante por canal cifrado

- **WHEN** llega una petición a través de HTTPS
- **THEN** el sistema SHALL procesarla normalmente

#### Scenario: Petición entrante sin cifrar

- **WHEN** llega una petición que no proviene de un canal HTTPS
- **THEN** el sistema SHALL rechazar la petición y no procesar su contenido

### Requirement: Estrategia de seguridad Fail-Closed en capas

El sistema SHALL aplicar una estrategia de "Fallo Cerrado" (Fail-Closed) compuesta por capas secuenciales de validación, denegando por defecto cualquier petición que no supere todas las capas aplicables.

#### Scenario: Falla una capa intermedia de validación

- **WHEN** una petición supera una o más capas pero falla en una capa posterior
- **THEN** el sistema SHALL rechazar la petición inmediatamente
- **AND** SHALL NOT continuar evaluando capas posteriores ni ejecutar la operación solicitada

### Requirement: Autenticación máquina-a-servidor mediante Bearer Token

Para la sincronización de nodos locales, el sistema SHALL autenticar al cliente local mediante un Bearer Token administrativo transmitido en la cabecera `Authorization: Bearer <token>`, generado criptográficamente (p. ej. `openssl rand -hex 32`).

#### Scenario: Cliente local con token válido

- **WHEN** un nodo local envía una petición con un Bearer Token criptográficamente correcto en la cabecera `Authorization`
- **THEN** el sistema SHALL aceptar la credencial y avanzar a la validación de allowlist

#### Scenario: Cliente local sin token o con token inválido

- **WHEN** un nodo local envía una petición sin Bearer Token o con un token incorrecto
- **THEN** el sistema SHALL rechazar la petición

### Requirement: Allowlist de proyectos en modo opt-in

El sistema SHALL verificar que el nombre del repositorio/proyecto entrante esté explícitamente dado de alta en `HIVE_API_ALLOWED_PROJECTS` antes de aceptar datos, incluso cuando el Bearer Token sea matemáticamente correcto (opt-in).

#### Scenario: Proyecto presente en la lista blanca

- **WHEN** una petición autenticada referencia un proyecto incluido en `HIVE_API_ALLOWED_PROJECTS`
- **THEN** el sistema SHALL permitir la operación sobre ese proyecto

#### Scenario: Proyecto ausente de la lista blanca

- **WHEN** una petición autenticada referencia un proyecto que NO está en `HIVE_API_ALLOWED_PROJECTS`
- **THEN** el sistema SHALL rechazar la petición HTTP inmediatamente, aun con un token válido

### Requirement: Autenticación humano-a-servidor mediante cookie de sesión firmada

Para el acceso al dashboard web, el sistema SHALL autenticar al desarrollador a partir de su token de acceso y emitir una cookie de sesión firmada criptográficamente con HMAC-256, sin exponer Bearer Tokens en el frontend.

#### Scenario: Inicio de sesión válido en el dashboard

- **WHEN** un desarrollador introduce un token de acceso válido en el navegador
- **THEN** el backend en Go SHALL validar la credencial y emitir una cookie firmada con HMAC-256
- **AND** la cookie SHALL establecerse con los flags `HttpOnly` y `Secure`

#### Scenario: Protección contra robo de sesión

- **WHEN** se establece la cookie de sesión
- **THEN** el flag `HttpOnly` SHALL impedir que JavaScript del cliente lea la cookie
- **AND** el flag `Secure` SHALL impedir su transmisión fuera de HTTPS

#### Scenario: Validación de cookie en cada navegación

- **WHEN** el desarrollador renderiza o navega dentro del dashboard
- **THEN** el servidor SHALL validar la firma y vigencia de la cookie en cada renderizado o navegación

#### Scenario: Expiración del ciclo de vida de la sesión

- **WHEN** han transcurrido más de 8 horas desde la emisión de la cookie
- **THEN** el sistema SHALL considerar la sesión inválida y SHALL requerir un nuevo inicio de sesión

### Requirement: Pantalla de Inicio / Proyectos del dashboard

El dashboard web SHALL ofrecer una pantalla de inicio que muestre, en una cuadrícula (grid), todos los proyectos o repositorios en los que el usuario autenticado tiene datos de memoria compartida.

#### Scenario: Visualización del grid de proyectos

- **WHEN** un usuario autenticado accede a la pantalla de inicio
- **THEN** el sistema SHALL mostrar una cuadrícula con todos los proyectos/repositorios donde el usuario tiene memoria compartida y sesión iniciada

### Requirement: Explorador de memoria (Browser)

El dashboard SHALL incluir un explorador de memoria que permita inspeccionar lo que los agentes han aprendido, con filtrado por categoría, auditoría de cambios y trazabilidad de origen.

#### Scenario: Filtrado por categoría de decisión

- **WHEN** el usuario aplica un filtro por categoría
- **THEN** el sistema SHALL permitir clasificar las memorias por los tipos: Arquitectura, Resolución de Bugs (Bugfixing), Configuraciones, Descubrimientos, Features y Patrones

#### Scenario: Auditoría de un aprendizaje concreto

- **WHEN** el usuario hace clic sobre un aprendizaje (p. ej. "Adoptar arquitectura hexagonal")
- **THEN** el sistema SHALL mostrar el detalle completo de qué se aprendió
- **AND** SHALL mostrar qué archivos exactos del repositorio fueron tocados o creados

#### Scenario: Trazabilidad de origen

- **WHEN** el usuario inspecciona un aprendizaje
- **THEN** el sistema SHALL permitir identificar qué máquina, desarrollador o sesión específica del agente originó dicho aprendizaje

### Requirement: Panel de administrador restringido por token de admin

El dashboard SHALL exponer un panel de administración cuyas capacidades SHALL ser visibles únicamente cuando el usuario se haya autenticado con `HIVE_API_ADMIN_TOKEN`.

#### Scenario: Acceso con token de administrador

- **WHEN** un usuario autenticado con `HIVE_API_ADMIN_TOKEN` accede al dashboard
- **THEN** el sistema SHALL mostrar el panel de administración y sus capacidades

#### Scenario: Acceso sin token de administrador

- **WHEN** un usuario sin `HIVE_API_ADMIN_TOKEN` accede al dashboard
- **THEN** el sistema SHALL NOT mostrar el panel de administración ni sus capacidades

#### Scenario: Monitoreo de salud del sistema

- **WHEN** un administrador consulta el monitoreo de salud
- **THEN** el sistema SHALL mostrar en vivo el estado de la conexión con PostgreSQL y el estado general del backend

#### Scenario: Kill-switch de sincronización de un repositorio

- **WHEN** un administrador pausa o bloquea la sincronización de un repositorio
- **THEN** el sistema SHALL detener de forma inmediata la sincronización de ese repositorio

#### Scenario: Monitor de usuarios activos

- **WHEN** un administrador consulta el monitor de actividad
- **THEN** el sistema SHALL mostrar qué usuarios o máquinas locales están enviando datos en ese momento

#### Scenario: Consulta de audit logs

- **WHEN** un administrador consulta los registros de auditoría
- **THEN** el sistema SHALL mostrar los audit logs internos para rastrear errores y accesos

### Requirement: Motor de autosincronización continua ("Tick")

El cliente local SHALL ejecutar un ciclo de sincronización en segundo plano cada 30 segundos, sin bloquear al agente local mientras espera respuestas de red.

#### Scenario: Disparo periódico del Tick

- **WHEN** transcurren 30 segundos desde el último ciclo
- **THEN** el nodo local SHALL salir de su estado inactivo y comunicarse con Hive-API

#### Scenario: Operación de Push idempotente

- **WHEN** el nodo local ejecuta la fase de Push
- **THEN** el sistema SHALL enviar los nuevos fragmentos ("chunks") de memoria local a PostgreSQL de manera idempotente

#### Scenario: Operación de Pull

- **WHEN** el nodo local ejecuta la fase de Pull
- **THEN** el sistema SHALL descargar las memorias subidas previamente por otras máquinas del equipo

#### Scenario: Operación de Register

- **WHEN** el nodo local ejecuta la fase de Register
- **THEN** el sistema SHALL anotar en un manifiesto el estado de las versiones para consolidar el conocimiento

### Requirement: Backoff exponencial ante fallos de red

El cliente local SHALL aplicar un algoritmo de retroceso exponencial (Backoff) de hasta 5 minutos cuando Hive-API esté caído o la red falle, evitando el "retry-bombing" sobre el proxy.

#### Scenario: Fallo de conexión con el servidor

- **WHEN** un ciclo del Tick falla por caída del servidor o de la red
- **THEN** el sistema SHALL entrar en un estado de espera exponencial de hasta 5 minutos antes de reintentar
- **AND** SHALL NOT bombardear el servidor proxy con reintentos inmediatos

#### Scenario: Continuidad operativa con memoria local

- **WHEN** Hive-API permanece inaccesible
- **THEN** el agente local (Claude u OpenCode) SHALL seguir operando al 100% con su memoria local mientras dure el backoff
