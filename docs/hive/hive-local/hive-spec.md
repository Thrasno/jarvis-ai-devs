# Hive — Especificación (OpenSpec)

> Sistema de memoria persistente *Local-First* para agentes de codificación de IA
> (Claude Code y OpenCode). Binario único en Go, SQLite como fuente de verdad,
> exposición de capacidades vía MCP (stdio).

---

## Purpose

Hive provee a los agentes de IA una memoria persistente que vive en el disco del
desarrollador, eliminando la amnesia entre sesiones. El sistema es 100% local por
defecto y garantiza **degradación elegante**: aunque falle la red o el servidor
remoto, las operaciones de lectura y escritura sobre la memoria local nunca se
interrumpen. Los agentes consumen la memoria mediante MCP; el humano la gobierna
mediante CLI y TUI.

---

## Requirements

### Almacenamiento y Núcleo Local

#### Requirement: Distribución como binario único

El sistema SHALL distribuirse como un único binario compilado en Go, sin
dependencias de ejecución de Node.js, Python o Docker en la máquina local.

##### Scenario: Instalación en una máquina sin tooling adicional

- **WHEN** el desarrollador coloca el binario `hive` en una máquina sin Node.js, Python ni Docker
- **THEN** el sistema SHALL ejecutarse correctamente
- **AND** SHALL no requerir instalar runtimes externos

#### Requirement: SQLite como fuente de verdad

El sistema SHALL almacenar toda la memoria en una base de datos SQLite ubicada en
el disco local, tratándola como la fuente absoluta de la verdad.

##### Scenario: Persistencia entre sesiones

- **WHEN** un agente guarda una observación en una sesión y la sesión termina
- **THEN** el dato SHALL persistir en SQLite en disco
- **AND** SHALL estar disponible para lectura en una sesión posterior

#### Requirement: Degradación elegante sin red

El sistema SHALL seguir sirviendo lecturas y escrituras locales aunque no haya
conexión a internet o el servidor remoto falle.

##### Scenario: Pérdida de conectividad durante una operación

- **WHEN** no hay internet o el servidor remoto está caído
- **AND** un agente intenta leer o escribir una memoria
- **THEN** el sistema SHALL completar la operación contra la base de datos local
- **AND** SHALL no interrumpir el flujo de trabajo del desarrollador

---

### Recuperación de Contexto

#### Requirement: Búsqueda de texto completo con ranking

El sistema SHALL combinar búsqueda de texto completo (FTS5) con ranking de
relevancia BM25 para recuperar fragmentos de contexto.

##### Scenario: Consulta de memoria relevante

- **WHEN** un agente consulta la memoria con una expresión de búsqueda
- **THEN** el sistema SHALL devolver los fragmentos más relevantes según frecuencia y rareza de términos (BM25)
- **AND** SHALL responder en el orden de milisegundos

#### Requirement: Optimización de la ventana de tokens

El sistema SHALL devolver fragmentos de contexto acotados y relevantes para
optimizar el consumo de la ventana de tokens del agente.

##### Scenario: Recuperación de contexto histórico

- **WHEN** el agente solicita contexto de un proyecto, dependencia o patrón usado meses atrás
- **THEN** el sistema SHALL devolver los fragmentos pertinentes
- **AND** SHALL evitar volcar la base de datos completa en la respuesta

---

### Interoperabilidad MCP

#### Requirement: Exposición de capacidades vía MCP por stdio

El sistema SHALL exponer sus capacidades como herramientas MCP usando transporte
estándar (stdio), sin reemplazar al agente.

##### Scenario: Descubrimiento de herramientas por el agente

- **WHEN** Claude Code u OpenCode se conecta a Hive vía MCP
- **THEN** el sistema SHALL anunciar aproximadamente 19 herramientas agrupadas en categorías lógicas
- **AND** el agente SHALL poder invocar la herramienta adecuada de forma transparente

#### Requirement: Herramientas de guardado y actualización

El sistema SHALL exponer `mem_save` para que el agente registre decisiones y
artefactos. Las observaciones históricas (decision, architecture, bugfix,
discovery, preference, config, pattern) son append-only — el agente no puede
sobreescribirlas. Los artefactos vivos (e.g. SDD tasks, apply-progress,
skill-registry) pueden actualizarse via upsert por `topic_key` usando `mem_save`.

`mem_update` no se expone como herramienta MCP: el upsert por `topic_key` cubre
el caso de artefactos vivos sin requerir IDs numéricos.

`mem_delete` no se expone como herramienta MCP: el borrado es exclusivamente
humano, desde CLI/TUI, requiere `reason` explícito, y es siempre soft delete.

##### Scenario: Registro de una decisión

- **WHEN** el agente llama a `mem_save` con una observación
- **THEN** el sistema SHALL persistir la observación en la memoria local

#### Requirement: Herramientas de búsqueda y recuperación

El sistema SHALL exponer herramientas de lectura del historial, incluyendo al
menos `mem_search`, `mem_context` y `mem_timeline`.

##### Scenario: Lectura del historial de un proyecto

- **WHEN** el agente llama a `mem_timeline` o `mem_context`
- **THEN** el sistema SHALL devolver el historial o el contexto solicitado desde la memoria local

#### Requirement: Herramientas de ciclo de vida de sesión

El sistema SHALL exponer herramientas para detectar el inicio de sesión
(`mem_session_start`) y para registrar resúmenes tras la compactación de la
ventana de contexto.

##### Scenario: Compactación de la ventana de contexto

- **WHEN** el agente compacta su ventana de contexto
- **THEN** el sistema SHALL permitir registrar un resumen de la sesión
- **AND** SHALL marcar el inicio de sesión cuando corresponda

---

### Interfaz de Línea de Comandos (CLI)

#### Requirement: Auto-configuración de integración

El sistema SHALL ofrecer `hive setup <agent>` para auto-configurar la integración
con Claude Code u OpenCode.

##### Scenario: Configuración inicial para un agente

- **WHEN** el desarrollador ejecuta `hive setup claude-code`
- **THEN** el sistema SHALL configurar automáticamente la integración MCP para ese agente

#### Requirement: Operaciones manuales sobre la memoria

El sistema SHALL permitir al humano operar la memoria manualmente mediante
`hive search <query>`, `hive save <title> <msg>`, `hive timeline <obs_id>` y
`hive context`.

##### Scenario: Inyección manual de contexto

- **WHEN** el desarrollador ejecuta `hive save "<title>" "<msg>"`
- **THEN** el sistema SHALL persistir esa memoria como si la hubiera escrito el agente

##### Scenario: Reconstrucción de un historial de decisiones

- **WHEN** el desarrollador ejecuta `hive timeline <obs_id>` o `hive context`
- **THEN** el sistema SHALL reconstruir y mostrar el historial de decisiones del proyecto

#### Requirement: Exportación e importación en JSON

El sistema SHALL permitir extraer e ingestar la base de datos en formato JSON
mediante `hive export` y `hive import`.

##### Scenario: Migración de la memoria

- **WHEN** el desarrollador ejecuta `hive export`
- **THEN** el sistema SHALL producir la base de datos serializada en JSON
- **AND** `hive import` SHALL poder reconstruir la memoria a partir de ese JSON

---

### Sincronización

#### Requirement: Sincronización versionada en Git

El sistema SHALL ofrecer `hive sync` para sincronizar mediante un manifiesto
versionado en Git, comprimiendo la memoria en *chunks* para evitar conflictos de
merge. La sincronización con un servidor en la nube SHALL ser opcional.

##### Scenario: Sincronización entre máquinas vía Git

- **WHEN** el desarrollador ejecuta `hive sync`
- **THEN** el sistema SHALL escribir/leer un manifiesto versionado en Git
- **AND** SHALL fragmentar la memoria en *chunks* para minimizar conflictos de merge

##### Scenario: Servidor remoto no disponible

- **WHEN** la sincronización con la nube está configurada pero el servidor falla
- **THEN** el sistema SHALL mantener intacta la memoria local
- **AND** SHALL no interrumpir el trabajo del desarrollador

---

### Terminal User Interface (TUI)

#### Requirement: Interfaz de terminal para gobernanza

El sistema SHALL lanzar una TUI mediante `hive tui` con tema "Catppuccin Mocha",
navegación tipo VIM (teclas `j`/`k`) y búsqueda interactiva con la tecla `/`.

##### Scenario: Navegación y búsqueda en la TUI

- **WHEN** el desarrollador ejecuta `hive tui` y presiona `j`/`k`
- **THEN** el sistema SHALL desplazar la selección por las memorias
- **AND** al presionar `/` SHALL abrir la búsqueda interactiva

#### Requirement: Detalle y copia al portapapeles

El sistema SHALL permitir perforar al detalle de una memoria con `Enter` y copiar
su contenido al portapapeles del sistema operativo con `c`, usando secuencias
OSC 52.

##### Scenario: Copia del detalle de una memoria

- **WHEN** el desarrollador presiona `Enter` sobre una memoria y luego `c`
- **THEN** el sistema SHALL mostrar el detalle de la memoria
- **AND** SHALL copiar su contenido al portapapeles del SO mediante OSC 52

---

### Privacidad y Seguridad Local

#### Requirement: Filtrado de secretos en tiempo de ejecución

El sistema SHALL impedir que se escriban secretos (p. ej. claves o tokens de AWS)
en SQLite. Cuando la IA detecta contenido sensible, lo etiqueta internamente y el
binario en Go expurga el secreto antes de persistir la observación.

##### Scenario: Guardado de una memoria con un secreto

- **WHEN** una observación etiquetada como privada contiene una clave o token sensible
- **THEN** el binario SHALL detectar la etiqueta y expurgar el secreto en tiempo de ejecución
- **AND** SHALL no escribir el secreto en la base de datos SQLite

#### Requirement: Resguardo cíclico (backups)

El sistema SHALL generar copias de seguridad comprimidas de la base de datos y la
configuración antes de operaciones críticas, limitando la retención (*pruning*)
para no sobrecargar el disco.

##### Scenario: Operación crítica con backup previo

- **WHEN** el sistema va a ejecutar una operación crítica
- **THEN** SHALL generar un backup comprimido del estado antes de proceder
- **AND** SHALL podar (*prune*) backups antiguos según la política de retención
