# Propuesta Ejecutiva: Jarvis-Dev MVP 1

**Para**: CEO Conpas  
**De**: Andrés (CTO)  
**Fecha**: 10 de Abril, 2026  
**Asunto**: Solicitud de aprobación para desarrollo de ecosistema AI interno  

---

## TL;DR — Decisión Ejecutiva

**Qué estamos pidiendo**:
- **Tiempo de desarrollo**: 20 horas/semana del CTO durante 5 meses (100 horas totales)
- **Infraestructura**: 1 VPS nuevo (USD $12/mes) + reasignación de recursos existentes
- **Inversión total**: ~USD $350 (setup inicial) + USD $60 (VPS 5 meses)
- **ROI esperado**: 30-40% reducción en tiempo perdido + 50% reducción de bugs en producción

**Retorno de inversión**:
- **Productividad**: Recuperamos ~80 horas/mes del equipo (equivalente a 0.5 FTE)
- **Calidad**: De 10 bugs/mes a <5 bugs/mes (reducción 50% de incidencias en producción)
- **Onboarding**: De 3-6 meses a 1-2 meses para devs nuevos (reducción 66% del tiempo)
- **Payback period**: 2 meses después de launch (recuperamos inversión inicial)

**Timeline**: 5 meses → MVP operativo listo para equipo completo

**Decisión requerida**: ¿Aprobás asignación de tiempo del CTO + infraestructura mínima para desarrollar esto?

---

## 1. El Problema

### Situación Actual

Conpas tiene 8 desarrolladores trabajando en Zoho y PHP. **Invertimos USD $9,600/año en Claude Team (8 seats)**, pero solo 3/8 desarrolladores lo usan efectivamente. **Estamos desperdiciando USD $6,000/año en licencias no utilizadas.**

Más importante aún, tenemos 3 problemas críticos que nos cuestan **productividad y calidad**:

#### Problema 1: Conocimiento en Silos (CRÍTICO)

**Síntoma**:
- Developer A resuelve un bug de autenticación JWT
- Developer B enfrenta el mismo problema 2 semanas después
- Developer B NO encuentra la solución (no está documentada)
- Developer B pierde 3 horas re-descubriendo la misma solución

**Impacto medido**:
- **30-40% del tiempo del equipo** se gasta re-descubriendo soluciones ya encontradas
- Equivalente a **2.4-3.2 FTE perdidos** (casi 3 desarrolladores haciendo trabajo duplicado)
- **Costo anual estimado**: USD $72,000 - $96,000 en productividad perdida

#### Problema 2: QA Débil (CRÍTICO)

**Síntoma**:
- Zoho es SaaS → no podemos escribir tests automatizados
- QA manual es ad-hoc (cada dev hace "lo que se le ocurre")
- Bugs descubiertos en producción por clientes

**Impacto medido**:
- **~10 bugs/mes** llegan a producción
- Cada bug requiere **2-4 horas** de investigación + hotfix + comunicación con cliente
- **Costo anual estimado**: USD $12,000 - $18,000 en tiempo de firefighting

#### Problema 3: Código Inconsistente (IMPORTANTE)

**Síntoma**:
- Cada developer programa "como le da la gana"
- No hay code reviews efectivos
- No hay convenciones documentadas

**Impacto medido**:
- Code reviews toman **~2 horas/PR** (tiempo del reviewer + developer)
- Onboarding de devs nuevos toma **3-6 meses** (tienen que aprender "mirando código existente")
- **Costo anual estimado**: USD $24,000 en tiempo de code review + onboarding

---

### Costo Total del Status Quo

| Problema | Costo Anual Estimado |
|----------|---------------------|
| Conocimiento en silos (30-40% productividad perdida) | USD $72,000 - $96,000 |
| Bugs en producción (10/mes × 3h promedio) | USD $12,000 - $18,000 |
| Code review + onboarding lento | USD $24,000 |
| Licencias Claude sin usar efectivamente | USD $6,000 |
| **TOTAL** | **USD $114,000 - $144,000/año** |

**Esto es equivalente a tener 3-4 desarrolladores trabajando a la mitad de su capacidad.**

---

## 2. La Solución: Jarvis-Dev

### ¿Qué es Jarvis-Dev?

**Jarvis-Dev es el "segundo cerebro" del equipo**: un ecosistema de desarrollo AI que:

1. **Recuerda TODO** lo que el equipo aprende (arquitectura, bugs, decisiones)
2. **Guía a los devs** a través de un proceso estructurado (pensar ANTES de codear)
3. **Aplica QA sistemático** (checklist manual para Zoho, donde tests automatizados son imposibles)
4. **Enseña fundamentos** (no solo "hace el trabajo", sino que explica el por qué)

### Analogía Simple

**Imaginate que cada developer tiene a Tony Stark como mentor**:
- Stark (la persona del CTO) define la filosofía y estrategia
- Jarvis (la IA) ejecuta, recuerda, y guía 24/7
- El developer aprende constantemente, nunca está solo

---

## 3. Componentes Principales

### 3.1 Hive: Memoria Compartida del Equipo

**Problema que resuelve**: "Developer A ya resolvió esto, pero Developer B no lo sabe"

**Cómo funciona**:
- Cuando un dev resuelve un bug, Jarvis lo guarda automáticamente en Hive
- Cuando otro dev enfrenta un problema similar, Jarvis encuentra la solución en <2 segundos
- La memoria se sincroniza automáticamente entre todos los devs del equipo

**Ejemplo real**:
```
Dev Junior: "Jarvis, ¿cómo implementamos JWT en este proyecto?"

Jarvis (busca en Hive):
  "El 15 de Marzo, Carlos implementó JWT con refresh tokens.
   Decisión arquitectónica: 
   - Access token: 15 min (httpOnly cookie)
   - Refresh token: 7 días (DB con rotación)
   - Razón: Balance entre seguridad y UX
   
   Código en: api/middleware/auth.php
   Tests en: tests/AuthTest.php"
```

**ROI**:
- **Antes**: 3 horas buscando/preguntando/re-descubriendo
- **Después**: 2 minutos (find + read + implement siguiendo patrón existente)
- **Ahorro**: 2h 58min × 20 búsquedas/mes/dev × 8 devs = **~476 horas/mes** recuperadas

---

### 3.2 SDD: Spec-Driven Development (Pensar Antes de Codear)

**Problema que resuelve**: "Devs escriben código sin planear, resulta en refactors constantes"

**Cómo funciona** (9 fases automáticas):
1. **Explore**: Investigar codebase y requerimientos
2. **Propose**: Crear propuesta de cambio
3. **Spec**: Escribir especificación técnica
4. **Design**: Diseñar arquitectura y approach
5. **Tasks**: Dividir en tareas implementables
6. **Apply**: Implementar código
7. **QA**: Ejecutar checklist de QA manual (BLOQUEANTE)
8. **Verify**: Validar que implementación cumple spec
9. **Archive**: Cerrar change y documentar

**Ejemplo real**:
```
Sin SDD:
  Dev: "Voy a agregar login con Google"
  → 2 días codeando
  → Se da cuenta que no pensó en edge cases
  → 1 día de refactor
  → Bug en producción: "¿Qué pasa si el email de Google no está verificado?"
  → 4 horas de hotfix
  Total: 3.5 días

Con SDD:
  Jarvis: "Antes de codear, pensemos esto juntos"
  → 1 hora de planning (explore + propose + spec + design)
  → Identificamos 5 edge cases ANTES de codear
  → 1.5 días de implementación limpia
  → QA checklist identifica 1 edge case más
  → Arreglamos ANTES de production
  Total: 2 días (43% más rápido, 0 bugs en prod)
```

**ROI**:
- **Reducción de refactors**: 30-40% menos tiempo corrigiendo código mal pensado
- **Reducción de bugs**: De 10/mes a <5/mes (50% menos incidencias)

---

### 3.3 QA Manual Sistemático (Para Zoho)

**Problema que resuelve**: "Zoho es SaaS, no podemos escribir tests automatizados"

**Cómo funciona**:
- Jarvis genera un **checklist de QA manual** basado en el spec
- El dev NO puede marcar feature como "terminada" hasta pasar QA
- El checklist se guarda en Hive para referencia futura

**Ejemplo real**:
```
Feature: "Agregar campo 'Urgencia' a módulo de Tickets"

QA Checklist generado por Jarvis:
□ Campo se muestra en formulario de creación
□ Campo es obligatorio (validación frontend)
□ Valores permitidos: Baja/Media/Alta/Crítica
□ Default: Media
□ Campo se guarda correctamente en DB
□ Campo aparece en lista de tickets
□ Filtro por urgencia funciona
□ Edge case: ¿Qué pasa si cambio urgencia de ticket existente?
□ Edge case: ¿Qué pasa si elimino el valor default?
```

**ROI**:
- **Antes**: Developer prueba "lo básico", 2-3 edge cases se escapan a producción
- **Después**: Checklist exhaustivo, bugs detectados ANTES de deploy
- **Impacto**: 50% reducción de bugs en producción (de 10/mes a <5/mes)

---

### 3.4 Sistema de Personas (Customización AI)

**Problema que resuelve**: "No todos los devs quieren el mismo tono de la IA"

**Cómo funciona**:
- 7 presets de personalidad: Argentino (directo), Neutra, Tony Stark, Yoda, etc.
- Cada dev elige su preferencia
- La IA adapta tono/lenguaje pero mantiene MISMA filosofía pedagógica

**Ejemplo**:
```
Mismo error, diferentes personas:

Argentino:
"Boludo, estás mezclando lógica de negocio en el controller. 
 Esto va en un Service. Bancá que te muestro."

Tony Stark:
"JARVIS would never mix business logic with presentation layer.
 Let me show you the proper architecture."

Neutra:
"He detectado lógica de negocio en el controlador.
 Sugiero moverla a un Service siguiendo Clean Architecture."
```

**ROI**:
- **Adopción**: Devs usan herramienta que les GUSTA (no sienten que "les imponen un robot")
- **Aprendizaje**: Mismo contenido, diferentes estilos → mayor engagement

---

## 4. Recursos Necesarios

### 4.1 Tiempo del CTO

| Fase | Duración | Horas/Semana CTO | Total Horas |
|------|----------|------------------|-------------|
| Mes 1: Hive Foundation | 4 semanas | 20h | 80h |
| Mes 2: SDD Core | 4 semanas | 20h | 80h |
| Mes 3: Persona + Skills | 4 semanas | 20h | 80h |
| Mes 4: CLI + Integration | 4 semanas | 20h | 80h |
| Mes 5: Documentation + Alpha Testing | 4 semanas | 20h | 80h |
| **TOTAL** | **20 semanas** | **20h/sem** | **400h** |

**Nota**: CTO continuará con responsabilidades actuales (no es full-time en Jarvis-Dev)

---

### 4.2 Infraestructura

#### Nuevo VPS (Hive Cloud)

| Recurso | Especificación | Costo Mensual | Proveedor Sugerido |
|---------|----------------|---------------|-------------------|
| **VPS** | 2GB RAM, 2 vCPU, 50GB SSD | USD $12/mes | DigitalOcean / Hetzner |
| **Dominio** | hive.conpas.dev | USD $0 (subdominio existente) | — |
| **SSL** | Certificado HTTPS | USD $0 (Let's Encrypt) | — |
| **Backups** | Daily PostgreSQL dumps | USD $0 (incluido en VPS) | — |
| **Monitoring** | Uptime + alertas | USD $0 (UptimeRobot free tier) | — |

**Total mensual**: USD $12/mes  
**Total 5 meses desarrollo**: USD $60  
**Total año 1 (después de launch)**: USD $144/año

#### Infraestructura Existente (Ya Tenemos)

| Recurso | Status | Costo Adicional |
|---------|--------|-----------------|
| **Claude Team** (8 seats) | ✅ Ya pagado (USD $9,600/año) | USD $0 |
| **GitLab Self-Hosted** | ✅ Funcionando | USD $0 |
| **VPS PHP Apps** (3 existentes) | ✅ Tienen recursos libres | USD $0 (reutilizar) |

---

### 4.3 Inversión Total

| Concepto | Costo |
|----------|-------|
| **Setup inicial** (dominio config, deployment scripts) | USD $0 (CTO lo hace) |
| **VPS 5 meses desarrollo** | USD $60 |
| **VPS año 1 después de launch** | USD $144 |
| **Licencias Claude** | USD $0 (ya pagadas) |
| **Tiempo CTO** | USD $0 (parte de su salario actual) |
| **TOTAL INVERSIÓN CASH** | **USD $204 (primer año)** |

**Esto es menos del 3% del costo anual estimado del status quo (USD $114k-$144k).**

---

## 5. Retorno de Inversión (ROI)

### 5.1 Ahorro en Productividad

| Métrica | Antes | Después | Ahorro Mensual | Ahorro Anual |
|---------|-------|---------|----------------|--------------|
| **Tiempo re-descubriendo soluciones** | 30-40% (2.4-3.2 FTE) | 5-10% (0.4-0.8 FTE) | 1.6-2.4 FTE | **USD $48k-$72k** |
| **Tiempo en code review** | 2h/PR | <30min/PR | ~60h/mes | **USD $21,600** |
| **Tiempo en firefighting bugs** | 30-40h/mes | 10-15h/mes | 20-25h/mes | **USD $7,200-$9,000** |

**Total ahorro anual en productividad**: **USD $76,800 - $102,600**

---

### 5.2 Mejora en Calidad

| Métrica | Antes | Después | Impacto |
|---------|-------|---------|---------|
| **Bugs en producción** | 10/mes | <5/mes | 50% reducción |
| **Tiempo onboarding dev nuevo** | 3-6 meses | 1-2 meses | 66% reducción |
| **Decisiones arquitectónicas documentadas** | 0% | 90%+ | Knowledge retention |

**Valor cualitativo**:
- Menos fricción con clientes (menos bugs)
- Equipo más confiado y autónomo
- Nuevos devs productivos en 1/3 del tiempo

---

### 5.3 Payback Period

| Año | Inversión | Ahorro | Balance |
|-----|-----------|--------|---------|
| **Mes 0-5** (desarrollo) | USD $60 VPS | USD $0 | -USD $60 |
| **Mes 6** (alpha testing) | USD $12 VPS | ~USD $3,000 (30% adopción) | +USD $2,988 |
| **Mes 7** (team rollout) | USD $12 VPS | ~USD $6,000 (60% adopción) | +USD $11,976 |
| **Mes 8+** (full adoption) | USD $12/mes | ~USD $7,500/mes | +USD $19,464 (acumulado) |

**Recuperamos inversión inicial en 2 meses después de launch.**

---

## 6. Comparación con Alternativas

### Alternativa 1: Contratar 1 Developer Adicional

**Costo**:
- Salario: USD $2,500-$3,500/mes
- Total año 1: USD $30,000 - $42,000

**Pros**:
- Más manos para codear
- Conocimiento del dominio PHP/Zoho

**Contras**:
- NO resuelve problema de conocimiento en silos
- NO mejora QA (sigue siendo manual ad-hoc)
- NO enseña fundamentos al equipo actual
- Costo recurrente permanente

**Veredicto**: ❌ NO resuelve problemas raíz

---

### Alternativa 2: Comprar Herramienta SaaS de Knowledge Management

**Opciones**: Notion AI, Confluence, Document360

**Costo**:
- USD $10-$20/usuario/mes × 8 = USD $80-$160/mes
- Total año 1: USD $960 - $1,920

**Pros**:
- No requiere desarrollo
- Setup rápido

**Contras**:
- NO está integrada con el workflow de coding
- Requiere que devs RECUERDEN documentar (no es automático)
- NO guía en desarrollo estructurado (SDD)
- NO aplica QA sistemático

**Veredicto**: ❌ Solución superficial, no cambia cultura

---

### Alternativa 3: Status Quo (No Hacer Nada)

**Costo**:
- USD $0 en inversión nueva
- USD $114,000 - $144,000/año en productividad perdida (continúa)

**Pros**:
- Ninguno

**Contras**:
- Problemas empeoran a medida que equipo crece
- Licencias Claude desperdiciadas (USD $6,000/año)
- Competidores que adopten AI nos superan en velocidad/calidad

**Veredicto**: ❌ NO es opción viable a largo plazo

---

### Alternativa 4: Jarvis-Dev (Propuesta)

**Costo**:
- USD $204 año 1 (VPS)
- 400 horas CTO (5 meses, parte de su rol)

**Pros**:
- Resuelve 3 problemas raíz (conocimiento, QA, estructura)
- ROI positivo en 2 meses post-launch
- Ahorro USD $76k-$102k/año en productividad
- Escalable (nuevos devs aprenden más rápido)
- Construido 100% para nuestro contexto (Zoho, PHP, equipo chico)

**Contras**:
- Requiere 5 meses de desarrollo
- Requiere adopción del equipo (riesgo mitigable)

**Veredicto**: ✅ **MEJOR OPCIÓN** (ROI 376x en año 1)

---

## 7. Riesgos y Mitigaciones

### Riesgo 1: El Equipo No Lo Adopta (ALTO)

**Probabilidad**: Media (5/8 devs nunca usaron AI)

**Impacto**: Alto (proyecto falla si nadie lo usa)

**Mitigación**:
- **Alpha testing**: 3 devs primero (Andrés + 2 eager devs)
- **Quick wins**: Mostrar resultados en 1ra semana (Hive resolviendo búsquedas)
- **Mandato**: SDD obligatorio para features nuevas (enforced en code review)
- **Training**: 30 min/semana durante 1 mes (hands-on, no teoría)
- **Gamification**: Leaderboard de completions SDD (friendly competition)

**Probabilidad después de mitigación**: Baja

---

### Riesgo 2: CTO No Termina en 5 Meses (MEDIO)

**Probabilidad**: Media (estimaciones pueden fallar)

**Impacto**: Medio (retrasa ROI)

**Mitigación**:
- **Timeline conservador**: 5 meses es estimación HIGH (optimista: 3.5 meses)
- **Pair programming**: 2 senior devs aprenden Go en paralelo (backup si CTO tiene imprevisto)
- **Milestones claros**: Weekly check-ins con CEO (transparencia total)
- **MVP incremental**: Lanzar Hive solo en Mes 2 si SDD no está listo (valor parcial temprano)

**Probabilidad después de mitigación**: Baja

---

### Riesgo 3: VPS Downtime (BAJO)

**Probabilidad**: Baja (VPS modernos tienen 99.9% uptime)

**Impacto**: Bajo (Hive local sigue funcionando sin cloud)

**Mitigación**:
- **Offline-first**: SQLite local funciona SIN conexión a cloud
- **Auto-retry**: Sync se reintenta cuando network vuelve
- **Backups**: Daily dumps a S3/Wasabi (recovery <1 hora)
- **Monitoring**: UptimeRobot alerta en downtime (CTO recibe SMS)

**Probabilidad después de mitigación**: Muy baja

---

## 8. Métricas de Éxito (Cómo Medimos)

### Fase 1: Alpha Testing (Mes 6, 3 devs)

| Métrica | Target |
|---------|--------|
| % decisiones documentadas en Hive | >80% |
| % features nuevos con SDD | >60% |
| Bugs en producción (de alpha testers) | <3/mes |
| Satisfacción alpha testers (1-10) | >7/10 |

**Go/No-Go**: Si <60% de targets → investigar antes de rollout completo

---

### Fase 2: Team Rollout (Mes 7-8, 8 devs)

| Métrica | Mes 7 | Mes 9 | Mes 12 |
|---------|-------|-------|--------|
| % decisiones documentadas | 60% | 80% | 90% |
| % features con SDD | 50% | 70% | 80% |
| % features con QA checklist | 80% | 95% | 100% |
| Bugs en producción | <7/mes | <6/mes | <5/mes |
| Devs usando Jarvis diariamente | 4/8 | 6/8 | 6/8 |
| Tiempo code review promedio | <1h | <45min | <30min |

---

### ROI Tracking (Mensual)

Vamos a medir:
1. **Horas ahorradas** (devs reportan: "Jarvis me ahorró X minutos en búsqueda")
2. **Bugs evitados** (QA checklist detectó X bugs ANTES de producción)
3. **Búsquedas en Hive** (cuántas veces se usa memoria compartida)
4. **SDD completions** (cuántos features se hicieron con proceso completo)

**Review trimestral con CEO**: Mostramos números reales vs. targets.

---

## 9. Timeline Visual

```
┌─────────────────────────────────────────────────────────────────┐
│ DESARROLLO (Mes 1-5) — CTO 20h/semana                           │
├─────────────────────────────────────────────────────────────────┤
│ Mes 1: Hive Foundation (SQLite local + PostgreSQL cloud + sync) │
│ Mes 2: SDD Core (9 fases: explore → archive)                    │
│ Mes 3: Persona System + Skill System (auto-load standards)      │
│ Mes 4: CLI Commands + Integration Tests                         │
│ Mes 5: Documentation + Alpha Testing (3 devs)                   │
└─────────────────────────────────────────────────────────────────┘
                             ↓
┌─────────────────────────────────────────────────────────────────┐
│ ALPHA TESTING (Mes 6) — 3 devs early adopters                   │
├─────────────────────────────────────────────────────────────────┤
│ - Andrés (CTO) + 2 devs más técnicos                            │
│ - Usar Jarvis en features reales                                │
│ - Collect feedback diario                                       │
│ - Fix critical bugs                                             │
│ - Refinar UX basado en uso real                                 │
└─────────────────────────────────────────────────────────────────┘
                             ↓
┌─────────────────────────────────────────────────────────────────┐
│ TEAM ROLLOUT (Mes 7-8) — 8 devs completos                       │
├─────────────────────────────────────────────────────────────────┤
│ - Training: 30 min/semana × 4 semanas                            │
│ - SDD obligatorio para features nuevos                          │
│ - Weekly check-in: ¿Qué funciona? ¿Qué no?                      │
│ - Ajustes basados en feedback                                   │
└─────────────────────────────────────────────────────────────────┘
                             ↓
┌─────────────────────────────────────────────────────────────────┐
│ STEADY STATE (Mes 9+) — Operación normal                        │
├─────────────────────────────────────────────────────────────────┤
│ - CTO mantiene sistema (2-3 h/semana)                           │
│ - Equipo usa Jarvis diariamente                                 │
│ - Quarterly reviews: métricas de ROI                            │
│ - Plan MVP 2 si ROI es positivo (onboarding progresivo, etc.)   │
└─────────────────────────────────────────────────────────────────┘
```

**Primer valor entregado**: Mes 2 (Hive funcional, aunque SDD no esté completo)  
**ROI positivo**: Mes 7-8 (2 meses post-rollout)

---

## 10. Decisión Requerida

### Opción A: Aprobar Desarrollo ✅

**Aprobás**:
- 20 horas/semana CTO durante 5 meses (Abril-Agosto 2026)
- USD $60 VPS durante desarrollo (5 meses)
- USD $144/año VPS después de launch (operación normal)
- Total cash investment: USD $204 año 1

**Siguiente paso**:
- CTO arranca Mes 1 (Hive Foundation) el 14 de Abril
- Weekly updates por Slack (viernes EOD)
- Demo mensual al equipo (progreso visible)
- Alpha testing con 3 devs en Agosto

**Expected outcome**:
- MVP operativo en Septiembre 2026
- ROI positivo en Noviembre 2026
- Ahorro USD $76k-$102k/año en productividad

---

### Opción B: Rechazar ❌

**No aprobás**:
- Mantenemos status quo
- Licencias Claude (USD $9,600/año) siguen subutilizadas
- Productividad perdida (USD $114k-$144k/año) continúa
- Equipo sigue sin estructura ni memoria compartida

**Riesgo**:
- Competidores adoptan AI más rápido
- Equipo se frustra (no mejora nunca)
- Nuevos devs tardan 3-6 meses en ser productivos

---

### Opción C: Aprobar con Scope Reducido (Alternativa)

**Aprobás solo Hive** (sin SDD):
- 2 meses desarrollo (CTO 20h/sem)
- USD $24 VPS desarrollo
- Total: USD $168 año 1

**Pros**:
- Menos riesgo (scope más chico)
- Valor más rápido (2 meses vs 5 meses)

**Contras**:
- NO resuelve problema de QA débil
- NO enseña proceso estructurado
- ROI menor (solo ahorro en búsquedas, no en calidad)

**Recomendación del CTO**: NO (Hive sin SDD es solo la mitad del valor)

---

## 11. Recomendación Final del CTO

**Recomiendo APROBAR Opción A (desarrollo completo).**

**Razones**:

1. **ROI es indiscutible**: USD $204 de inversión → USD $76k-$102k ahorro anual = 376x ROI
2. **Riesgo es bajo**: Timeline conservador, mitigaciones claras, alpha testing valida antes de rollout
3. **Payback rápido**: Recuperamos inversión en 2 meses post-launch
4. **Problemas son reales**: No son hipotéticos, los vivimos todos los días
5. **Ventana de oportunidad**: Ya tenemos Claude Team (USD $9,600 invertido), no aprovecharlos es desperdiciar

**Si no hacemos esto ahora**:
- En 1 año seguiremos perdiendo USD $114k-$144k/año en productividad
- Competidores nos superan en velocidad/calidad
- Equipo se estanca (no aprende, no mejora)

**Si lo hacemos**:
- En 1 año habremos ahorrado USD $76k-$102k
- Equipo es 30-40% más productivo
- Nuevos devs productive en 1/3 del tiempo
- Bugs en producción reducidos 50%

---

## Anexo: Preguntas Frecuentes

### ¿Por qué Go y no Laravel? El equipo conoce PHP.

**Respuesta**: Go es más adecuado para servicios de larga duración (daemon) y alta concurrencia. Laravel es excelente para web apps, pero overhead innecesario para un daemon local. Además, CTO puede aprender Go con ayuda de IA 24/7 (pair programming with Claude). La documentación será EXTREMA para que el equipo pueda mantenerlo después.

### ¿Qué pasa si el CTO se va?

**Respuesta**: 
- Todo el código está ultra-documentado (comentarios explican WHY, no solo WHAT)
- 2 senior devs aprenden Go en paralelo (training durante desarrollo)
- Documentación arquitectónica completa (ADRs, diagramas, troubleshooting)
- Fallback: Migrar a Laravel API en MVP 2 si es necesario (el costo ya se justificó)

### ¿No es más fácil usar Notion para memoria compartida?

**Respuesta**: Notion NO está integrado con el workflow de coding. Requiere que devs RECUERDEN documentar (disciplina manual). Jarvis guarda memorias AUTOMÁTICAMENTE mientras codean (sin fricción). Además, Notion no tiene SDD ni QA checklists.

### ¿Qué pasa si solo 3 devs lo adoptan?

**Respuesta**: Con 3 devs adoptando, ya tenemos ROI positivo (ahorro ~USD $24k/año vs USD $204 inversión). Pero la estrategia es mandato suave: SDD obligatorio para features nuevos (enforced en code review). No es opcional.

### ¿El VPS es un riesgo si se cae?

**Respuesta**: NO. Hive es offline-first (SQLite local funciona sin cloud). El VPS solo sincroniza entre devs. Si cae, cada dev sigue trabajando localmente. Cuando vuelve, se sincroniza automáticamente.

### ¿5 meses es realista?

**Respuesta**: Es estimación conservadora. Optimista: 3.5 meses. Pesimista: 6 meses. Tenemos milestones mensuales para detectar desvíos temprano. Además, CTO tiene experiencia en proyectos similares (referencia: Engram open source).

---

**¿Preguntas? Hablemos.**

Andrés (CTO)  
andres@conpas.dev
