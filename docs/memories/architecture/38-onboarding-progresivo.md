---
id: 38
type: architecture
project: jarvis-dev
scope: project
session: manual-save-jarvis-dev
created_at: 2026-04-10 07:50:05
---

# Sistema de Onboarding Progresivo - Diseño completo

**What**: Sistema de onboarding progresivo con 3 niveles automáticos

**Design**:
- **Beginner (0-10)**: Solo complexity:low, NO fast-forward, bloqueo pedagógico
- **Intermediate (11-30)**: Hasta complexity:medium, fast-forward permitido
- **Advanced (30+)**: Sin restricciones

**Metrics** (config-driven): tasks_completed, tasks_by_complexity, qa_success_rate, avg_task_duration, projects

**Admin System** (Opción B con safeguards):
- Admin inicial en setup, límite 3 admins
- Comando `jarvis-admin grant-admin` (confirmación doble)
- Audit log completo, notificaciones automáticas

**Degradación**: Si QA success rate <40% en últimas 10 → baja 1 nivel

**Learned**: Bloqueo pedagógico > bloqueo duro (enseña vs frustra)
