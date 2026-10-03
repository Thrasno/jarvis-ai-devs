# Jarvis Dev: continuity and control for AI-assisted development

Jarvis Dev is an operating layer around AI coding tools, not another chatbot. It combines repeatable setup, local-first project memory, optional team sharing, and structured delivery. Nexus is the ecosystem’s brand identity; Nexus Hive is its optional shared-memory dashboard, not a separate development platform.

## Current usage and practical value

Two colleagues are already testing the ecosystem (declared usage; not independently measured). Dates, versions, platforms, outcomes, and timings have not been documented here. The next step is to capture that experience and assess impact before broader adoption.

Two illustrative situations explain the intended value:

- **A forgotten change:** a developer finds an unusual retry limit in their own code. A recorded decision can explain why it exists, helping them focus their investigation and compare the rationale with current code and tests.
- **An unavailable colleague:** during an incident, a teammate is on holiday. If the relevant decision or change context was recorded and shared, colleagues can retrieve it to narrow investigation without relying solely on that person. Logs, testing, and engineering judgment still determine the diagnosis.

Memory does not automatically capture every code change, reconstruct missing history, or guarantee incident resolution.

## How the ecosystem works

| Capability | Operating model and intended value |
|---|---|
| Setup and recovery | The CLI generates managed tool configuration. Verification and doctor checks identify drift; reconciliation repairs eligible managed changes with backup and subsequent verification. This supports consistency and diagnosability. |
| Hive local memory | With `hive-daemon` running, recorded decisions, bug causes, and conventions remain available locally without the shared server. This supports continuity across sessions. |
| Optional team sharing | Hive API sharing requires configuration, credentials, and agreed data boundaries. Background synchronization must be explicitly enabled. This supports knowledge reuse across machines. |
| SDD delivery | Spec-Driven Development clarifies requirements and design, slices work into reviewable tasks, verifies implementation against expectations, and archives knowledge. This supports scope control and review traceability. |
| Nexus Hive | When deployed and authorized, the dashboard lets operators inspect shared memories and available project, user, activity, audit, and quarantine surfaces. It does not manage local daemons; deferred graph and analytics features are not promised. |

Configuration replay through `jarvis sync`, drift repair through `jarvis reconcile`, and memory transfer through `mem_sync` or configured daemon background sync are different operations. Reinstallation does not synchronize memory.

## Evidence, cost, and governance

Documented capabilities support this operating model; the two-colleague testing remains declared experience, not independently measured. A rehearsed demonstration and real evidence dossier are pending. No installation certification, incident timing, token-saving percentage, or ROI is established here.

Selective reuse of recorded context may reduce repeated explanation, but SDD, persistence, retrieval, and verification also add effort and consumption. Evaluate comparable tasks using the same model and outcome criteria; account for total input, output, cache, and tool consumption alongside review effort and result quality.

Teams should define what may be recorded or shared, keep secrets outside repositories, assign responsibility for shared-service operation, and review access, retention, and HTTPS deployment. Diagnose setup problems through supported CLI flows rather than hand-editing generated configuration. Support remains bounded by deployed capabilities and team ownership; no SLA is asserted.

## Sources and continuation

[Visual executive one-pager (Spanish)](../presentaciones/jarvis-executive-one-pager.html) · [Ecosystem presentation (Spanish)](../presentaciones/jarvis-ecosystem.html).

[Overview](jarvis-overview.md) · [Adoption stages](jarvis-adoption-guide.md) · [Security and privacy](jarvis-security-and-privacy.md) · [Local memory](../hive/local-memory-guide.md) · [Memory sync](../hive/sync-guide.md) · [Dashboard](../hive/dashboard-guide.md) · [SDD](../sdd-user-guide.md) · [Setup recovery](../setup-recovery.md).
