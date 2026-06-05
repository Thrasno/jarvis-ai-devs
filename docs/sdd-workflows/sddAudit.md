# Documento de Especificación Técnica para Auditoría
## Ecosistema de Orquestación y Flujo de Trabajo para IA (Agent Harnesses)

> **Naturaleza del documento:** Especificación técnica de referencia para auditoría externa.
> Este documento traduce el PRD del producto a **requisitos auditables**. Para cada requisito se indica:
> el comportamiento esperado, la **evidencia** que el auditor debe encontrar, el **método de verificación**
> y los **criterios de aceptación (PASA / FALLA)**. El objetivo es que el auditor pueda emitir un dictamen
> sin necesidad de consultas adicionales al equipo de desarrollo.

---

## 0. Cómo usar este documento

- Cada requisito tiene un **identificador único** (p. ej. `REQ-ORQ-01`) para referenciarlo en el informe de auditoría.
- La columna **Evidencia esperada** describe el artefacto físico, log, archivo de configuración o comportamiento observable que demuestra el cumplimiento.
- La columna **Verificación** describe la acción concreta que el auditor debe ejecutar (inspección de archivo, ejecución de comando, prueba funcional, revisión de código).
- **Criterio de aceptación:** un requisito se marca **PASA** sólo si *toda* la evidencia esperada está presente y es coherente; en caso contrario, **FALLA** (con observación) o **N/A** (no aplica, justificado).
- Sección 14 incluye una **matriz de trazabilidad** PRD → Requisito → Estado para consolidar el dictamen.

### Convenciones de severidad para hallazgos
| Severidad | Significado |
|-----------|-------------|
| **Crítica** | Compromete la integridad de datos, la seguridad o la trazabilidad del trabajo. |
| **Mayor** | Incumple un control nuclear del proceso (DAG, delegación, verificación). |
| **Menor** | Desviación que no rompe el flujo pero degrada la calidad o el ahorro de recursos. |
| **Observación** | Mejora recomendada sin incumplimiento formal. |

---

## 1. Alcance de la auditoría

### 1.1 Objeto auditado
El **ecosistema de orquestación** que envuelve a agentes de IA (Claude Code y OpenCode) en un conjunto de *Agent Harnesses* (arneses), imponiendo dirección, proceso, verificación y límites de seguridad sobre la ejecución del modelo.

### 1.2 Dentro del alcance
- Configuración e integración con los agentes soportados (Sección 3).
- Catálogo completo de arneses operativos y su comportamiento observable (Secciones 4–9).
- Motor de disparadores de delegación (Sección 10).
- Herramientas CLI de gobernanza y diagnóstico (Sección 11).
- Enrutamiento multimodelo y perfiles SDD (Sección 12).
- Subsistema de respaldo, recuperación y seguridad (Sección 13).

### 1.3 Fuera del alcance (salvo indicación contraria)
- Calidad intrínseca de los modelos LLM de terceros.
- Infraestructura de red corporativa ajena al ecosistema.
- Código de aplicación generado *por* los agentes (se audita el **proceso**, no el producto de cada tarea).

### 1.4 Objetivo declarado a verificar
El ecosistema debe transformar la autonomía del modelo en "trabajo de ingeniería rigurosamente controlado", resolviendo tres problemas:
1. **Contexto contaminado** (ruido y decisiones mezcladas).
2. **Ejecución impredecible** (archivos incorrectos / soluciones inventadas).
3. **Nula trazabilidad** de las decisiones.

Adicionalmente debe lograr un **ahorro de tokens del 50 %–70 %** (ver `REQ-GEN-02`).

---

## 2. Resumen del sistema bajo auditoría

El ecosistema **no es un instalador de asistentes**, sino un **configurador de infraestructura operativa**. Su núcleo está compuesto por decenas de arneses interconectados que:
- Mantienen el estado en **artefactos físicos**, no en el chat.
- Imponen un **grafo acíclico dirigido (DAG)** de fases obligatorias.
- Aíslan a los subagentes en contextos limpios.
- Exigen **verificación con evidencia** antes de dar una tarea por cerrada.
- Protegen la configuración con **respaldos versionados** y recuperables.

---

## 3. Requisitos generales del ecosistema

| ID | Requisito | Evidencia esperada | Verificación | Criterio de aceptación |
|----|-----------|--------------------|--------------|------------------------|
| **REQ-GEN-01** | El sistema configura infraestructura operativa, no instala simples asistentes. | Estructura de configuración (manifiestos, hooks, registros) presente; no se limita a un binario de chat. | Inspección del árbol de instalación. | PASA si existe la capa de orquestación/arneses, no sólo el agente base. |
| **REQ-GEN-02** | Ahorro de tokens entre **50 % y 70 %** frente a uso de modelos crudos. | Métricas/registros comparativos de consumo de tokens (baseline crudo vs. ecosistema), o metodología documentada de medición. | Revisión de logs de consumo y/o reporte de benchmark. | PASA si existe evidencia reproducible de ahorro en el rango declarado; FALLA si no hay medición. |
| **REQ-GEN-03** | El sistema mitiga los tres problemas críticos (contexto contaminado, ejecución impredecible, nula trazabilidad). | Mapeo de cada problema a un arnés concreto que lo resuelve. | Trazar Aislamiento→contexto, DAG/Delegación→ejecución, Artifact Store/Result Contract→trazabilidad. | PASA si los tres problemas tienen control asociado y operativo. |
| **REQ-GEN-04** | La integración se restringe a agentes con delegación avanzada (**únicamente Claude Code y OpenCode**). | Configuración rechaza o no contempla agentes fuera de los dos soportados. | Inspección de la matriz de compatibilidad y de los conectores instalados. | PASA si sólo se integran los dos agentes especificados. |

---

## 4. Compatibilidad y arquitectura de agentes

### 4.1 Claude Code
| ID | Requisito | Evidencia esperada | Verificación | Criterio de aceptación |
|----|-----------|--------------------|--------------|------------------------|
| **REQ-AGT-01** | Integración mediante **Task tools**. | Configuración que declara el uso de Task tools para el despliegue de subagentes. | Inspección de la config del agente. | PASA si los subagentes se invocan vía Task tools. |
| **REQ-AGT-02** | Despliegue de subagentes **asíncronos y concurrentes**, con **máximo 5 simultáneos**. | Límite de concurrencia = 5 configurado y respetado; capacidad de ejecución en segundo plano. | Prueba funcional: lanzar >5 subagentes y confirmar que el sistema limita/encola a 5. | PASA si el tope de 5 concurrentes se respeta y la ejecución es asíncrona. |

### 4.2 OpenCode
| ID | Requisito | Evidencia esperada | Verificación | Criterio de aceptación |
|----|-----------|--------------------|--------------|------------------------|
| **REQ-AGT-03** | Delegación vía **multi-mode overlay** (superposición multimodo). | Definición de modos superpuestos en la configuración de OpenCode. | Inspección de configuración de modos. | PASA si existe el esquema de superposición multimodo. |
| **REQ-AGT-04** | **Enrutamiento granular** de modelos LLM distintos según la fase del desarrollo. | Mapa fase→modelo configurado (ver Sección 12). | Inspección de perfiles de enrutamiento. | PASA si cada fase puede asignar un modelo distinto. |

---

## 5. Arneses de Orquestación y Aislamiento (Grupo A)

| ID | Arnés | Comportamiento esperado | Evidencia esperada | Verificación | Criterio de aceptación |
|----|-------|-------------------------|--------------------|--------------|------------------------|
| **REQ-ORQ-01** | **Orchestrator (Tech Lead)** | Mantiene la conversación con el humano, decide el flujo, controla las *gates* entre fases y sintetiza resultados. **Nunca implementa código directamente.** | Logs/artefactos donde el orquestador coordina pero no edita archivos de producto. | Revisar trazas de una sesión completa: confirmar que toda escritura de código proviene de subagentes. | PASA si el orquestador no ejecuta cambios de código directos. **(Hallazgo Crítico si el orquestador escribe código).** |
| **REQ-ORQ-02** | **Delegation** | Decide arquitectónicamente entre resolución *inline* (cambios triviales) o **delegación a subagente** (requiere explorar múltiples archivos). | Registro de la decisión de delegación por tarea. | Forzar un caso trivial y uno multi-archivo; verificar la ruta elegida. | PASA si la decisión inline/delegado responde a las reglas (ver Sección 10). |
| **REQ-ORQ-03** | **Sub-Agent Isolation ("Hoja en Blanco")** | El subagente **jamás** arrastra el historial del orquestador; inicia con contexto limpio y recibe sólo las instrucciones/contexto necesarios. | Payload de inicio del subagente: contiene únicamente la tarea y contexto específico, sin historial de chat. | Inspeccionar el contexto inyectado a un subagente recién lanzado. | PASA si no hay fuga de historial del orquestador. **(Hallazgo Crítico si se filtra el historial completo).** |

---

## 6. Arneses de Estado, Inicialización y Memoria (Grupo B)

| ID | Arnés | Comportamiento esperado | Evidencia esperada | Verificación | Criterio de aceptación |
|----|-------|-------------------------|--------------------|--------------|------------------------|
| **REQ-EST-01** | **SDD Init (Paso Cero)** | Antes de escribir código, calibra el proyecto: detecta stack, comandos de prueba y convenciones, generando un **manifiesto YAML**. | Archivo manifiesto YAML con stack, comandos de test y convenciones detectadas. | Inspeccionar el YAML generado por `/sdd-init`. | PASA si el manifiesto existe y refleja correctamente el stack real del proyecto. |
| **REQ-EST-02** | **Artifact Store** | El estado reside en **artefactos físicos recuperables** (archivos locales o memoria en BD); el chat **no** es la fuente de la verdad. Ante compactación/cierre, el trabajo no se pierde y se reanuda. | Artefactos en disco/BD que reflejan el estado del proceso. | Cerrar/compactar una sesión a mitad de tarea y reanudar; confirmar continuidad desde el último estado. | PASA si el trabajo se recupera íntegro tras cierre. **(Hallazgo Crítico si la pérdida de sesión pierde trabajo).** |
| **REQ-EST-03** | **Session Summary / Compaction Recovery** | Guarda un **resumen operativo** justo antes de que la ventana de contexto se desborde y compacte. | Artefacto de resumen de sesión con timestamp previo a la compactación. | Provocar saturación de contexto y verificar la creación del resumen previo. | PASA si el resumen se genera antes de la compactación y la siguiente sesión inicia con contexto intacto. |

---

## 7. Arneses del Grafo de Flujo de Trabajo (Grupo C — SDD)

### 7.1 Secuencia obligatoria del DAG
> **Inicialización → Exploración → Propuesta → (Especificación ∥ Diseño) → Tareas → Aplicación → Verificación → Archivado**
> (Especificación y Diseño se ejecutan **en paralelo**.)

| ID | Arnés | Comportamiento esperado | Evidencia esperada | Verificación | Criterio de aceptación |
|----|-------|-------------------------|--------------------|--------------|------------------------|
| **REQ-DAG-01** | **Face DAG** | Obliga a respetar el grafo acíclico dirigido; **impide saltar directamente a programar**. | Registro de transición de fases en el orden prescrito. | Intentar invocar "Aplicación" sin fases previas; el sistema debe bloquearlo. | PASA si el orden del DAG se respeta y los saltos se rechazan. **(Hallazgo Mayor si se puede saltar a programar).** |
| **REQ-DAG-02** | **Artifact Dependency** | Cada fase exige **entradas obligatorias**: Diseño requiere el artefacto de Propuesta; Tareas requiere Especificación + Diseño. | Validación de dependencias antes de iniciar cada fase. | Eliminar/ocultar el artefacto de Propuesta e intentar iniciar Diseño. | PASA si la fase no arranca sin sus insumos obligatorios. |
| **REQ-DAG-03** | **Result Contract ("Envelopes")** | La comunicación entre fases usa **contratos parseables**. Cada subagente devuelve: **estado, resumen ejecutivo, ubicación del artefacto, riesgos recomendados y resolución de habilidades**. | Objeto "Envelope" estructurado y parseable por cada transición. | Inspeccionar un Envelope real y validar que contiene los 5 campos. | PASA si todos los campos están presentes y son parseables. |
| **REQ-DAG-04** | **Operational Process** | Durante la **Aplicación**, guarda el estado operativo (p. ej. Tarea 1 completada, Tarea 2 en progreso) para evitar que una segunda pasada pise o duplique trabajo. | Registro de progreso por tarea en disco/BD. | Reiniciar a mitad de aplicación; confirmar que no se reescriben tareas ya completadas. | PASA si no hay duplicación/sobre-escritura de trabajo previo. |

---

## 8. Arneses de Implementación y Verificación Estricta (Grupo D)

| ID | Arnés | Comportamiento esperado | Evidencia esperada | Verificación | Criterio de aceptación |
|----|-------|-------------------------|--------------------|--------------|------------------------|
| **REQ-IMP-01** | **Strict TDD** | Si el proyecto lo soporta, obliga el ciclo **Red → Green → Triangulate → Refactor** (prueba que falla → solución → casos extremos → refactor). | Historial de pruebas mostrando primero fallo, luego éxito, luego triangulación y refactor. | Revisar el log de una tarea TDD; confirmar el orden del ciclo. | PASA si el ciclo se respeta cuando el stack soporta TDD; N/A justificado si no lo soporta. |
| **REQ-IMP-02** | **Verify** | Despliega un subagente **imparcial** que **no cree en la palabra del programador** y exige **evidencia criptográfica**: comandos de prueba que pasaron, archivos tocados y confirmación de cumplimiento del alcance del Diseño. | Reporte de verificación con salida real de pruebas, lista de archivos modificados y checklist de alcance. | Inspeccionar el reporte de verificación de una tarea cerrada. | PASA si la tarea sólo se cierra con evidencia verificable; **(Hallazgo Mayor si se cierra "por palabra" sin evidencia).** |

---

## 9. Arneses de Digestión de Habilidades — Skills (Grupo E)

| ID | Arnés | Comportamiento esperado | Evidencia esperada | Verificación | Criterio de aceptación |
|----|-------|-------------------------|--------------------|--------------|------------------------|
| **REQ-SKL-01** | **Skill Registry** | Escanea repositorio del proyecto **y** directorio local del usuario para crear un índice ("Registry") de conocimientos y reglas reutilizables, alojado en un **archivo ignorado por Git**. | Archivo de índice presente y listado en `.gitignore`. | Verificar existencia del índice y su exclusión en `.gitignore`. | PASA si el índice existe y **no** se versiona en Git. |
| **REQ-SKL-02** | **Skill Digestion** | En lugar de inyectar 100 páginas de reglas, **compacta** las convenciones en **4 o 5 "Estándares de Proyecto"** estrictos y accionables para la tarea específica del subagente. | Conjunto reducido (4–5) de estándares accionables inyectados al subagente. | Inspeccionar el contexto inyectado: contar los estándares compactados. | PASA si la inyección está compactada a 4–5 estándares y no al volumen completo. |
| **REQ-SKL-03** | **Skill Resolution Feedback** | Retorna un reporte **auditable** indicando qué habilidades se aplicaron exitosamente y cuáles fallaron o requirieron *fallback*. | Reporte de resolución con estado por habilidad. | Inspeccionar el reporte tras una tarea que use skills. | PASA si el reporte existe y distingue éxito/fallo/fallback. |

---

## 10. Arneses de Estrategia de Entrega — Review Warlock (Grupo F)

| ID | Arnés | Comportamiento esperado | Evidencia esperada | Verificación | Criterio de aceptación |
|----|-------|-------------------------|--------------------|--------------|------------------------|
| **REQ-DLV-01** | **Review Warlock** | Si el cambio supera el **umbral de riesgo** (> **400 líneas** o múltiples **directorios críticos**), el sistema **pausa** y pregunta al humano cómo entregar el código. | Punto de pausa registrado al superar el umbral, con solicitud de decisión al humano. | Generar un cambio > 400 líneas / multi-directorio y confirmar la pausa. | PASA si el sistema pausa y solicita estrategia al superar el umbral. **(Hallazgo Mayor si entrega "AI slop" sin pausa).** |
| **REQ-DLV-02** | **Delivery Strategy** | Permite encadenar entrega mediante geometrías: **Stacked PRs** (PRs encadenadas atómicas), **Feature Branch** (empuje a rama vacía), **Single PR / Exception** (alcance pequeño o justificado). | Las tres geometrías disponibles y seleccionables. | Probar la selección de cada geometría. | PASA si las tres opciones existen y operan según definición. |

---

## 11. Motor de Disparadores de Delegación (Delegation Triggers)

> Principio: el **hilo principal (orquestador) debe permanecer extremadamente ligero**. Cuando una tarea deja de ser trivial, la delegación es **obligatoria**.

| ID | Disparador | Regla esperada | Evidencia esperada | Verificación | Criterio de aceptación |
|----|-----------|----------------|--------------------|--------------|------------------------|
| **REQ-TRG-01** | **Lectura de 4+ archivos** | Se delega a un subagente de Exploración o se inicia una fase de exploración completa. | Traza de delegación al alcanzar el 4.º archivo. | Forzar la lectura de ≥ 4 archivos y observar la delegación. | PASA si delega al cruzar el umbral de lectura. |
| **REQ-TRG-02** | **Edición de 2+ archivos no triviales** | Se delega a un subagente escritor **o** se exige una **Fresh Review** antes de cerrar la tarea. | Traza de delegación o de revisión limpia forzada. | Forzar 2 ediciones no triviales y observar la respuesta. | PASA si delega o exige revisión limpia. |
| **REQ-TRG-03** | **Accidentes en árbol de trabajo (Git/CWD)** | Ante errores de merge, directorio incorrecto o confusión de entorno, el agente **detiene la ejecución** y lanza una **auditoría en limpio**. | Registro de detención + auditoría limpia tras el incidente. | Simular un conflicto de merge / CWD incorrecto. | PASA si detiene y audita en lugar de continuar. **(Hallazgo Crítico si sigue ejecutando sobre un árbol roto).** |
| **REQ-TRG-04** | **Sesiones monolíticas** | Si la sesión acumula demasiada complejidad, se **fuerza a pausar, re-planificar o justificar estrictamente** la continuidad. | Punto de control con pausa/re-planificación o justificación registrada. | Inducir una sesión de alta complejidad acumulada. | PASA si interviene en lugar de continuar indefinidamente. |
| **REQ-TRG-05** | **Ciclo de vida del código** | Comandos destructivos o de integración (**Commit, Push, creación de PR**) fuerzan un **Fresh Audit obligatorio**, salvo modificación trivial en documentación. | Registro de Fresh Audit previo a cada commit/push/PR. | Ejecutar un commit/push/PR y confirmar la auditoría previa. | PASA si todo commit/push/PR no trivial es precedido por Fresh Audit. **(Hallazgo Crítico si integra sin auditoría).** |

---

## 12. Gobernanza a nivel de proyecto y diagnósticos (CLI)

| ID | Comando | Comportamiento esperado | Evidencia esperada | Verificación | Criterio de aceptación |
|----|---------|-------------------------|--------------------|--------------|------------------------|
| **REQ-CLI-01** | `/sdd-init` | Detecta el framework de pruebas del stack y **activa TDD Estricto si es posible**. Se ejecuta la primera vez en un proyecto o si cambia el framework. | Salida del comando + manifiesto generado + flag de TDD. | Ejecutar `/sdd-init` en un proyecto limpio. | PASA si detecta el framework y configura TDD cuando procede. |
| **REQ-CLI-02** | `ecosistema skill-registry refresh` | **Escanea y reconstruye** el índice de habilidades. Los **startup hooks** de OpenCode y Claude mantienen el registro actualizado al abrir el IDE/CLI. | Índice reconstruido + hooks de inicio configurados. | Ejecutar el refresh y verificar actualización; confirmar hooks. | PASA si reconstruye el índice y los hooks de arranque están activos. |
| **REQ-CLI-03** | `ecosistema doctor` | Herramienta de salud **solo lectura (Read-Only)** que verifica: binarios presentes, **integridad de `state.json`**, accesibilidad de la **BD de memoria persistente** y **espacio en disco**. | Reporte de diagnóstico con los 4 chequeos; ninguna escritura realizada. | Ejecutar `doctor` y confirmar que no modifica estado. | PASA si ejecuta los 4 chequeos y **no** escribe nada. **(Hallazgo Mayor si modifica estado).** |

---

## 13. Perfiles SDD y Enrutamiento Multimodelo (Exclusivo OpenCode)

> Objetivo: maximizar capacidad intelectual y minimizar coste de API asignando **un LLM por fase**.

### 13.1 Mapa de enrutamiento esperado
| Fase(s) | Tipo de modelo esperado | Ejemplos declarados en el PRD |
|---------|-------------------------|-------------------------------|
| Inicialización, Exploración, Archivado | Modelos **rápidos y económicos** | Claude 3.5 Sonnet |
| Propuesta, Especificación, Diseño | Modelos **densos / alto razonamiento técnico** | Claude 3 Opus, Gemini 1.5 Pro |
| Implementación, Verificación | Modelos **especializados en código y detección de bugs** | OpenAI Codex 5.3 / 5.4 |

| ID | Requisito | Evidencia esperada | Verificación | Criterio de aceptación |
|----|-----------|--------------------|--------------|------------------------|
| **REQ-RTE-01** | El enrutamiento por fase coincide con el mapa anterior. | Configuración de perfiles fase→modelo. | Inspeccionar la config de enrutamiento de OpenCode. | PASA si cada grupo de fases usa la clase de modelo prescrita. |
| **REQ-RTE-02** | **Prohibido usar Gemini para Implementación** (inestabilidad histórica con tool calls complejas). | Ausencia de Gemini en el perfil de Implementación. | Revisar el perfil de Implementación. | PASA si Gemini **no** aparece como modelo de implementación. **(Hallazgo Mayor si se usa Gemini para implementar).** |
| **REQ-RTE-03** | Existen perfiles tipo `sdd-orchestrator-cheap` y `sdd-orchestrator-premium`. | Definición de ambos perfiles. | Inspeccionar la lista de perfiles. | PASA si ambos perfiles existen. |
| **REQ-RTE-04** | **Activación dinámica con la tecla `Tab`** para alternar perfil en tiempo de ejecución. | Binding de `Tab` para cambio de perfil. | Probar el toggle con `Tab` dentro de OpenCode. | PASA si `Tab` alterna el perfil fluidamente. |

---

## 14. Motor de Respaldo, Recuperación y Seguridad (Backup & Rollback)

> Se activa ante **cualquier** actualización de infraestructura, sincronización de memoria o modificación de configuración de los agentes.

| ID | Requisito | Comportamiento esperado | Evidencia esperada | Verificación | Criterio de aceptación |
|----|-----------|-------------------------|--------------------|--------------|------------------------|
| **REQ-BCK-01** | **Formato de compresión** | Crea snapshots de los archivos de configuración empaquetados y comprimidos en **`tar.gz`**. | Snapshots con extensión `.tar.gz`. | Inspeccionar el directorio de respaldos. | PASA si los snapshots son `tar.gz` válidos y restaurables. |
| **REQ-BCK-02** | **Deduplicación inteligente** | Evalúa **criptográficamente** si hubo cambios reales; si la config es idéntica al estado anterior, **no genera respaldo**. | Hash/checksum por snapshot; ausencia de respaldos duplicados ante config idéntica. | Disparar dos respaldos sin cambios; confirmar que sólo se genera uno. | PASA si no se crean respaldos redundantes. |
| **REQ-BCK-03** | **Auto-Pruning (Podado)** | Rota los respaldos conservando únicamente los **5 más recientes**. | Como máximo 5 respaldos no anclados. | Generar > 5 respaldos y verificar la rotación. | PASA si nunca se superan los 5 respaldos (excluyendo anclados). |
| **REQ-BCK-04** | **Pinning (Fijación)** | Desde la TUI, la tecla **`p`** ancla (Pin) un respaldo crítico, protegiéndolo **permanentemente** contra la purga automática. | Respaldo marcado como anclado, sobreviviente a la rotación. | Anclar un respaldo con `p`, generar > 5 nuevos y confirmar que el anclado persiste. | PASA si el respaldo anclado sobrevive al auto-pruning. **(Hallazgo Crítico si un pin se purga).** |

---

## 15. Estructura de archivos y artefactos esperados

El auditor debe esperar encontrar (los nombres exactos pueden variar según implementación, pero la **función** debe existir):

- **Manifiesto de inicialización** (YAML) — producido por `/sdd-init` → `REQ-EST-01`.
- **`state.json`** — estado central verificable por `ecosistema doctor` → `REQ-CLI-03`.
- **Almacén de artefactos** (archivos locales y/o BD de memoria persistente) → `REQ-EST-02`.
- **Índice de habilidades (Skill Registry)** — archivo **ignorado por Git** → `REQ-SKL-01`.
- **Artefactos de fase del DAG** — Exploración, Propuesta, Especificación, Diseño, Tareas, Verificación, Archivado → Sección 7.
- **Envelopes** de contrato de resultado entre fases → `REQ-DAG-03`.
- **Reportes de verificación** con evidencia de pruebas → `REQ-IMP-02`.
- **Directorio de respaldos** con snapshots `tar.gz`, metadatos de hash y marcas de pin → Sección 14.
- **Perfiles de enrutamiento** (OpenCode) → Sección 13.
- **Configuración de hooks de arranque** (startup hooks) → `REQ-CLI-02`.

---

## 16. Casos de prueba de aceptación de extremo a extremo (E2E)

| ID | Escenario | Pasos | Resultado esperado |
|----|-----------|-------|--------------------|
| **E2E-01** | Flujo SDD completo | Lanzar una tarea no trivial desde cero. | Recorre el DAG completo (Init→…→Archivado), delega correctamente, verifica con evidencia y entrega según geometría elegida. |
| **E2E-02** | Recuperación tras compactación | Cerrar/compactar la sesión a mitad de la fase de Aplicación. | El resumen previo existe; al reanudar, no se pierde ni duplica trabajo (`REQ-EST-02/03`, `REQ-DAG-04`). |
| **E2E-03** | Umbral Review Warlock | Generar un cambio > 400 líneas. | El sistema pausa y solicita estrategia de entrega (`REQ-DLV-01`). |
| **E2E-04** | Aislamiento de subagente | Inspeccionar el contexto de un subagente recién lanzado. | Sin historial del orquestador; sólo tarea + contexto específico (`REQ-ORQ-03`). |
| **E2E-05** | Disparador de integración | Intentar un push sin auditoría. | Se fuerza Fresh Audit obligatorio antes de integrar (`REQ-TRG-05`). |
| **E2E-06** | Rotación + pin de respaldos | Anclar un respaldo, generar > 5 nuevos. | El anclado sobrevive; los no anclados se podan a 5 (`REQ-BCK-03/04`). |
| **E2E-07** | Prohibición de Gemini en implementación | Revisar el perfil de Implementación. | Gemini ausente como modelo de implementación (`REQ-RTE-02`). |

---

## 17. Controles de seguridad e integridad (resumen para dictamen rápido)

| Control de seguridad | Requisito(s) | Severidad si falla |
|----------------------|--------------|--------------------|
| El orquestador no escribe código | `REQ-ORQ-01` | Crítica |
| Aislamiento de contexto del subagente | `REQ-ORQ-03` | Crítica |
| No pérdida de trabajo ante cierre/compactación | `REQ-EST-02`, `REQ-EST-03` | Crítica |
| Cierre de tarea sólo con evidencia verificable | `REQ-IMP-02` | Mayor |
| Detención ante árbol de trabajo roto | `REQ-TRG-03` | Crítica |
| Fresh Audit antes de commit/push/PR | `REQ-TRG-05` | Crítica |
| `doctor` es solo lectura | `REQ-CLI-03` | Mayor |
| Pins de respaldo nunca se purgan | `REQ-BCK-04` | Crítica |
| Registry ignorado por Git | `REQ-SKL-01` | Mayor |

---

## 18. Matriz de trazabilidad PRD → Requisitos

| Sección del PRD | Requisitos asociados |
|-----------------|----------------------|
| 1. Visión general | REQ-GEN-01 … REQ-GEN-04 |
| 2. Compatibilidad de agentes | REQ-AGT-01 … REQ-AGT-04 |
| 3.A Orquestación/Aislamiento | REQ-ORQ-01 … REQ-ORQ-03 |
| 3.B Estado/Init/Memoria | REQ-EST-01 … REQ-EST-03 |
| 3.C Grafo de flujo (SDD) | REQ-DAG-01 … REQ-DAG-04 |
| 3.D Implementación/Verificación | REQ-IMP-01 … REQ-IMP-02 |
| 3.E Digestión de Skills | REQ-SKL-01 … REQ-SKL-03 |
| 3.F Estrategia de entrega | REQ-DLV-01 … REQ-DLV-02 |
| 4. Disparadores de delegación | REQ-TRG-01 … REQ-TRG-05 |
| 5. Gobernanza CLI | REQ-CLI-01 … REQ-CLI-03 |
| 6. Enrutamiento multimodelo | REQ-RTE-01 … REQ-RTE-04 |
| 7. Backups | REQ-BCK-01 … REQ-BCK-04 |

---

## 19. Plantilla de hoja de hallazgos (para el auditor)

| Campo | Contenido |
|-------|-----------|
| **ID de requisito** | (p. ej. REQ-DAG-01) |
| **Estado** | PASA / FALLA / N/A |
| **Evidencia observada** | (archivo, comando, log, captura) |
| **Severidad** | Crítica / Mayor / Menor / Observación |
| **Descripción del hallazgo** | |
| **Recomendación** | |

---

## 20. Glosario

- **Arnés (Harness):** capa de control que envuelve al modelo para imponer proceso, verificación y límites.
- **SDD (Spec Driven Development):** desarrollo guiado por especificación; el código se deriva de artefactos formales.
- **DAG:** grafo acíclico dirigido que define el orden obligatorio de fases.
- **Envelope:** contrato parseable de salida de cada subagente/fase.
- **Fresh Audit / Fresh Review:** auditoría/revisión en contexto limpio, sin sesgo de la sesión previa.
- **AI slop:** producción descontrolada y de baja calidad del modelo que vuelve la revisión inmanejable.
- **TUI:** interfaz visual de terminal.
- **Pin:** anclaje permanente de un respaldo contra el podado automático.
- **Startup hooks:** ganchos ejecutados al abrir el IDE/CLI para mantener el registro actualizado.

---

> **Fin del documento.** Este documento es autocontenido: cada requisito incluye comportamiento esperado, evidencia y criterio de aceptación, de modo que el auditor pueda completar la revisión sin consultas adicionales al equipo de desarrollo.
