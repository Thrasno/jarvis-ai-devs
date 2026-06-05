# AI Orchestration Ecosystem Specification

> Formato: **OpenSpec**. Capability: `ai-orchestration-ecosystem`.
> Las palabras clave estructurales (`SHALL`, `MUST`, `WHEN`, `THEN`, `AND`, `Scenario`) se mantienen en inglés
> para compatibilidad con el parser de OpenSpec. El contenido descriptivo está en español.
> Cada `### Requirement` incluye al menos un `#### Scenario`.

## Purpose

Definir el comportamiento de un **configurador de infraestructura operativa** que envuelve a los agentes de IA soportados (estrictamente **Claude Code** y **OpenCode**) en un conjunto de *Agent Harnesses* (arneses). El ecosistema convierte la autonomía caótica del modelo en trabajo de ingeniería controlado, aportando dirección, proceso, verificación y límites de seguridad, y reduciendo el consumo de tokens entre un 50 % y un 70 %.

El ecosistema debe resolver tres problemas del uso de modelos crudos:
1. Contexto contaminado (ruido y decisiones mezcladas).
2. Ejecución impredecible (archivos incorrectos o soluciones inventadas).
3. Nula trazabilidad de las decisiones tomadas.

## Requirements

### Requirement: Operational Infrastructure, Not An Installer
El sistema SHALL configurar infraestructura operativa (arneses, manifiestos, registros, hooks) y NOT limitarse a instalar un asistente de chat.

#### Scenario: Inspección del árbol de instalación
- **WHEN** un auditor inspecciona el resultado de la instalación
- **THEN** existe una capa de orquestación con arneses, manifiestos y hooks
- **AND** el sistema no consiste únicamente en el binario del agente base

### Requirement: Token Savings
El sistema SHALL lograr un ahorro de consumo de tokens de entre 50 % y 70 % frente al uso de modelos crudos, y SHALL exponer evidencia o metodología de medición reproducible.

#### Scenario: Medición comparativa de consumo
- **WHEN** se compara el consumo de una tarea ejecutada con el ecosistema frente a la misma tarea con el modelo crudo
- **THEN** el ahorro medido cae dentro del rango 50 %–70 %
- **AND** existe un registro o metodología que permite reproducir la medición

### Requirement: Supported Agents Only
El sistema SHALL restringir su integración exclusivamente a **Claude Code** y **OpenCode**, agentes con capacidades de delegación avanzada.

#### Scenario: Intento de integrar un agente no soportado
- **WHEN** se intenta integrar un agente distinto a Claude Code u OpenCode
- **THEN** el sistema no contempla ni habilita dicha integración

### Requirement: Claude Code Task Tools And Concurrency
El sistema SHALL integrar Claude Code mediante "Task tools", desplegando subagentes asíncronos y concurrentes con un máximo de **5 simultáneos**, permitiendo trabajo en segundo plano.

#### Scenario: Despliegue concurrente dentro del límite
- **WHEN** el orquestador delega varias tareas a subagentes
- **THEN** se ejecutan de forma asíncrona y concurrente
- **AND** nunca hay más de 5 subagentes activos al mismo tiempo
- **AND** el desarrollador puede seguir trabajando mientras se ejecutan en segundo plano

#### Scenario: Exceso de subagentes solicitados
- **WHEN** se solicitan más de 5 subagentes concurrentes
- **THEN** el sistema limita o encola la ejecución para no superar el tope de 5

### Requirement: OpenCode Multi-Mode Overlay Routing
El sistema SHALL integrar OpenCode mediante un esquema de superposición multimodo ("multi-mode overlay") con enrutamiento granular de modelos LLM distintos según la fase del desarrollo.

#### Scenario: Asignación de modelo por fase
- **WHEN** el flujo de trabajo transiciona entre fases en OpenCode
- **THEN** cada fase puede recibir un modelo LLM distinto según el perfil de enrutamiento

### Requirement: Orchestrator Never Implements Code
El orquestador (Tech Lead) SHALL mantener la conversación con el humano, decidir el flujo, controlar las puertas (gates) entre fases y sintetizar resultados, y MUST NOT implementar código directamente.

#### Scenario: Toda escritura de código proviene de subagentes
- **WHEN** se revisa una sesión completa de trabajo
- **THEN** el orquestador coordina, decide y sintetiza
- **AND** toda modificación de archivos de producto fue realizada por subagentes, no por el orquestador

### Requirement: Delegation Decision
El sistema SHALL decidir arquitectónicamente entre resolver una tarea "inline" (cambios triviales) o delegarla a un subagente cuando requiere explorar o tocar múltiples archivos.

#### Scenario: Cambio trivial resuelto inline
- **WHEN** la tarea es un cambio trivial
- **THEN** se resuelve inline sin delegar

#### Scenario: Cambio no trivial delegado
- **WHEN** la tarea requiere explorar múltiples archivos
- **THEN** se delega a un subagente

### Requirement: Sub-Agent Isolation
Un subagente SHALL iniciar con un contexto totalmente limpio y aislado ("Hoja en Blanco"), recibiendo exclusivamente las instrucciones y el contexto necesarios para su tarea, y MUST NOT arrastrar el historial de chat del orquestador.

#### Scenario: Contexto limpio en el arranque del subagente
- **WHEN** se lanza un subagente nuevo
- **THEN** su contexto contiene únicamente la tarea y el contexto específico necesario
- **AND** no contiene el historial de chat del orquestador

### Requirement: SDD Init Calibration
El sistema SHALL ejecutar un "Paso Cero" que, antes de escribir código, detecte el stack tecnológico, los comandos de prueba y las convenciones existentes, generando un manifiesto YAML.

#### Scenario: Generación del manifiesto de calibración
- **WHEN** se ejecuta la inicialización en un proyecto
- **THEN** se genera un manifiesto YAML con el stack detectado, los comandos de prueba y las convenciones del proyecto

### Requirement: Artifact Store As Source Of Truth
El sistema SHALL mantener el estado del proceso en artefactos físicos recuperables (archivos locales o memoria en base de datos), y el chat MUST NOT ser la fuente de la verdad. El trabajo SHALL sobrevivir al cierre o compactación de la sesión.

#### Scenario: Reanudación tras cierre o compactación
- **WHEN** la sesión se cierra o el contexto se compacta a mitad de una tarea
- **THEN** el trabajo no se pierde
- **AND** la siguiente sesión se reanuda desde el último estado documentado en los artefactos

### Requirement: Compaction Recovery Summary
El sistema SHALL guardar un resumen operativo justo antes de que la ventana de contexto se desborde y compacte.

#### Scenario: Resumen previo a la compactación
- **WHEN** la ventana de contexto está a punto de saturarse y compactar
- **THEN** se guarda un resumen operativo con marca de tiempo previa a la compactación
- **AND** la siguiente sesión inicia con el contexto intacto

### Requirement: Workflow DAG Enforcement
El sistema SHALL obligar a respetar un Grafo Acíclico Dirigido de fases en el orden: Inicialización → Exploración → Propuesta → (Especificación ∥ Diseño en paralelo) → Tareas → Aplicación → Verificación → Archivado, y MUST NOT permitir saltar directamente a programar.

#### Scenario: Intento de saltar a la fase de Aplicación
- **WHEN** se intenta iniciar la fase de Aplicación sin haber completado las fases previas
- **THEN** el sistema bloquea la transición

#### Scenario: Especificación y Diseño en paralelo
- **WHEN** se ha completado la fase de Propuesta
- **THEN** las fases de Especificación y Diseño pueden ejecutarse en paralelo

### Requirement: Artifact Dependency Gates
Cada fase SHALL exigir como entrada obligatoria los artefactos de las fases de las que depende; en particular, Diseño requiere el artefacto de Propuesta, y Tareas requiere los artefactos de Especificación y Diseño.

#### Scenario: Fase de Diseño sin Propuesta
- **WHEN** se intenta iniciar la fase de Diseño y no existe el artefacto de Propuesta
- **THEN** la fase no arranca

#### Scenario: Fase de Tareas sin insumos
- **WHEN** se intenta iniciar la fase de Tareas y falta la Especificación o el Diseño
- **THEN** la fase no arranca

### Requirement: Result Contract Envelopes
La comunicación entre fases SHALL realizarse mediante contratos parseables ("Envelopes"). Cada subagente SHALL devolver un envelope con: estado, resumen ejecutivo, ubicación del artefacto, riesgos recomendados y resolución de habilidades.

#### Scenario: Envelope completo en la transición de fase
- **WHEN** un subagente finaliza su fase
- **THEN** devuelve un envelope parseable
- **AND** el envelope contiene estado, resumen ejecutivo, ubicación del artefacto, riesgos recomendados y resolución de habilidades

### Requirement: Operational Process State
Durante la fase de Aplicación, el sistema SHALL guardar el estado operativo por tarea (p. ej. Tarea 1 completada, Tarea 2 en progreso) para que una segunda pasada NOT pise ni duplique el trabajo previo.

#### Scenario: Reanudación de la Aplicación sin duplicar trabajo
- **WHEN** la fase de Aplicación se reinicia tras una interrupción
- **THEN** las tareas ya completadas no se reescriben ni se duplican
- **AND** la ejecución continúa desde la tarea en progreso

### Requirement: Strict TDD Cycle
Cuando el proyecto soporte pruebas, el sistema SHALL obligar al subagente a seguir el ciclo Red → Green → Triangulate → Refactor.

#### Scenario: Ciclo TDD aplicado
- **WHEN** el subagente implementa una tarea en un proyecto con soporte de pruebas
- **THEN** primero escribe una prueba que falla (Red)
- **AND** programa la solución hasta que pasa (Green)
- **AND** triangula casos extremos
- **AND** refactoriza

#### Scenario: Proyecto sin soporte de pruebas
- **WHEN** el proyecto no soporta un framework de pruebas
- **THEN** el ciclo TDD estricto no se exige

### Requirement: Independent Verification With Evidence
El sistema SHALL desplegar un subagente verificador imparcial que NOT confíe en la palabra del programador y SHALL exigir evidencia: comandos de prueba ejecutados con éxito, archivos tocados y confirmación de cumplimiento del alcance del Diseño.

#### Scenario: Cierre de tarea condicionado a evidencia
- **WHEN** una tarea se propone como completada
- **THEN** el verificador exige la salida real de las pruebas que pasaron, la lista de archivos modificados y la confirmación del alcance del Diseño
- **AND** la tarea solo se cierra si la evidencia es verificable

### Requirement: Skill Registry Indexing
El sistema SHALL escanear el repositorio del proyecto y el directorio local del usuario para construir un índice ("Registry") de conocimientos y reglas reutilizables, alojado en un archivo ignorado por Git.

#### Scenario: Índice creado y excluido de Git
- **WHEN** se construye el Skill Registry
- **THEN** existe un archivo de índice de habilidades
- **AND** dicho archivo está listado en `.gitignore` y no se versiona

### Requirement: Skill Digestion Compaction
El sistema SHALL compactar las convenciones en 4 o 5 "Estándares de Proyecto" estrictos y accionables para la tarea del subagente, en lugar de inyectar el volumen completo de reglas al contexto.

#### Scenario: Inyección compactada al subagente
- **WHEN** se prepara el contexto de un subagente que usa habilidades
- **THEN** se inyectan entre 4 y 5 estándares accionables
- **AND** no se inyecta el conjunto completo de reglas

### Requirement: Skill Resolution Feedback
El sistema SHALL retornar un reporte auditable que indique qué habilidades se aplicaron con éxito y cuáles fallaron o requirieron un "fallback".

#### Scenario: Reporte de resolución de habilidades
- **WHEN** finaliza una tarea que utilizó habilidades
- **THEN** se genera un reporte auditable que distingue habilidades aplicadas, fallidas y con fallback

### Requirement: Review Warlock Risk Threshold
El sistema SHALL pausar la ejecución y preguntar al humano cómo entregar el código cuando el cambio supere el umbral de riesgo (más de 400 líneas modificadas o múltiples directorios críticos afectados).

#### Scenario: Cambio que supera el umbral
- **WHEN** un cambio toca más de 400 líneas o múltiples directorios críticos
- **THEN** el sistema pausa la ejecución
- **AND** solicita al humano la estrategia de entrega

### Requirement: Delivery Strategies
El sistema SHALL ofrecer geometrías de entrega seleccionables: Stacked PRs (PRs encadenadas atómicas), Feature Branch (empuje directo a una rama vacía) y Single PR / Exception (un único PR si el alcance es pequeño o está justificado).

#### Scenario: Selección de geometría de entrega
- **WHEN** el humano elige una geometría de entrega tras una pausa de Review Warlock
- **THEN** el sistema entrega el código según Stacked PRs, Feature Branch o Single PR / Exception, según lo elegido

### Requirement: Delegation Trigger On File Reads
El sistema SHALL delegar a un subagente de Exploración (o iniciar una fase de exploración) cuando una tarea requiera leer 4 o más archivos.

#### Scenario: Lectura de cuatro o más archivos
- **WHEN** una tarea necesita leer 4 o más archivos
- **THEN** se delega la exploración a un subagente o se inicia una fase de exploración

### Requirement: Delegation Trigger On File Edits
El sistema SHALL delegar a un subagente escritor, o exigir una revisión limpia (Fresh Review) antes de completar la tarea, cuando se editen 2 o más archivos no triviales.

#### Scenario: Edición de dos o más archivos no triviales
- **WHEN** una tarea edita 2 o más archivos no triviales
- **THEN** se delega a un subagente escritor o se exige una Fresh Review antes de cerrar la tarea

### Requirement: Working Tree Accident Halt
El sistema SHALL detener la ejecución y lanzar una auditoría en limpio ante accidentes del árbol de trabajo (errores de merge, directorio de trabajo incorrecto o confusión del entorno).

#### Scenario: Conflicto de merge o CWD incorrecto
- **WHEN** se detecta un error de merge, un directorio de trabajo incorrecto o confusión del entorno
- **THEN** el agente detiene la ejecución
- **AND** lanza una auditoría en limpio antes de continuar

### Requirement: Monolithic Session Control
El sistema SHALL forzar una pausa, re-planificación o justificación estricta de la continuidad cuando una sesión acumule demasiada complejidad.

#### Scenario: Sesión con complejidad excesiva
- **WHEN** una sesión acumula complejidad excesiva
- **THEN** el sistema fuerza pausar, re-planificar o justificar estrictamente la continuidad

### Requirement: Code Lifecycle Fresh Audit
El sistema SHALL forzar un Fresh Audit obligatorio antes de comandos destructivos o de integración (commits, pushes, creación de PRs), salvo modificaciones triviales de documentación.

#### Scenario: Integración precedida de auditoría
- **WHEN** se va a ejecutar un commit, push o creación de PR no trivial
- **THEN** el sistema fuerza un Fresh Audit antes de proceder

#### Scenario: Excepción de documentación trivial
- **WHEN** el cambio es una modificación trivial de documentación
- **THEN** el Fresh Audit obligatorio no se exige

### Requirement: sdd-init Command
El comando `/sdd-init` SHALL detectar el framework de pruebas del stack y activar el modo de TDD Estricto cuando sea posible, ejecutándose la primera vez en un proyecto o cuando cambie el framework.

#### Scenario: Detección de framework y activación de TDD
- **WHEN** se ejecuta `/sdd-init` en un proyecto
- **THEN** se detecta el framework de pruebas
- **AND** se activa el modo de TDD Estricto si el framework lo permite

### Requirement: skill-registry refresh Command
El comando `ecosistema skill-registry refresh` SHALL escanear y reconstruir el índice de habilidades, y los startup hooks de Claude y OpenCode SHALL mantener el registro actualizado al abrir el IDE o la CLI.

#### Scenario: Reconstrucción del índice
- **WHEN** se ejecuta `ecosistema skill-registry refresh`
- **THEN** el índice de habilidades se reconstruye

#### Scenario: Actualización por startup hook
- **WHEN** se abre el IDE o la CLI
- **THEN** el startup hook mantiene el registro de habilidades actualizado

### Requirement: doctor Read-Only Diagnostics
El comando `ecosistema doctor` SHALL ser una herramienta de salud de solo lectura que verifique la presencia de binarios, la integridad de `state.json`, la accesibilidad de la base de datos de memoria persistente y el espacio en disco, y MUST NOT modificar ningún estado.

#### Scenario: Ejecución de diagnósticos sin efectos secundarios
- **WHEN** se ejecuta `ecosistema doctor`
- **THEN** se verifican binarios, integridad de `state.json`, accesibilidad de la BD de memoria y espacio en disco
- **AND** no se modifica ningún archivo ni estado

### Requirement: Phase-Based Model Routing (OpenCode)
En OpenCode, el sistema SHALL asignar modelos LLM por fase: modelos rápidos y económicos para Inicialización, Exploración y Archivado; modelos densos de alto razonamiento para Propuesta, Especificación y Diseño; y modelos especializados en código para Implementación y Verificación.

#### Scenario: Enrutamiento por clase de fase
- **WHEN** el flujo transiciona entre fases en OpenCode
- **THEN** Inicialización, Exploración y Archivado usan modelos rápidos y económicos
- **AND** Propuesta, Especificación y Diseño usan modelos densos de alto razonamiento
- **AND** Implementación y Verificación usan modelos especializados en código

### Requirement: Gemini Forbidden For Implementation
El sistema MUST NOT usar Gemini para la fase de Implementación, debido a su inestabilidad histórica con llamadas a herramientas complejas.

#### Scenario: Perfil de Implementación sin Gemini
- **WHEN** se inspecciona el perfil de enrutamiento de la fase de Implementación
- **THEN** Gemini no figura como modelo asignado a Implementación

### Requirement: SDD Profiles And Tab Toggle (OpenCode)
El sistema SHALL permitir crear perfiles de orquestación (p. ej. `sdd-orchestrator-cheap` y `sdd-orchestrator-premium`) y alternar entre ellos en tiempo de ejecución mediante la tecla `Tab`.

#### Scenario: Alternancia de perfil con Tab
- **WHEN** el desarrollador presiona la tecla `Tab` en OpenCode
- **THEN** el perfil de orquestación activo alterna fluidamente entre los perfiles definidos

### Requirement: Backup Snapshot Format
El sistema SHALL crear instantáneas (snapshots) de los archivos de configuración empaquetadas y comprimidas en formato `tar.gz` ante cualquier actualización de infraestructura, sincronización de memoria o modificación de configuración de los agentes.

#### Scenario: Snapshot al modificar configuración
- **WHEN** se actualiza la infraestructura, se sincroniza la memoria o se modifica la configuración de un agente
- **THEN** se crea un snapshot en formato `tar.gz` de los archivos de configuración

### Requirement: Backup Deduplication
El sistema SHALL evaluar criptográficamente si hubo cambios reales y MUST NOT generar un respaldo cuando la configuración sea idéntica al estado anterior.

#### Scenario: Configuración idéntica no genera respaldo
- **WHEN** se dispara un respaldo y la configuración es idéntica a la del estado anterior
- **THEN** no se genera un nuevo snapshot

### Requirement: Backup Auto-Pruning
El sistema SHALL rotar los respaldos conservando únicamente los 5 más recientes no anclados.

#### Scenario: Rotación al superar el límite
- **WHEN** existen más de 5 respaldos no anclados
- **THEN** se podan los más antiguos hasta conservar solo los 5 más recientes

### Requirement: Backup Pinning Protection
El sistema SHALL permitir anclar (Pin) un respaldo crítico desde la interfaz de terminal mediante la tecla `p`, y un respaldo anclado MUST NOT ser eliminado por la purga automática.

#### Scenario: Respaldo anclado sobrevive a la rotación
- **WHEN** un respaldo se ancla con la tecla `p` y posteriormente se generan más de 5 respaldos nuevos
- **THEN** el respaldo anclado se conserva y no es purgado por el auto-pruning
