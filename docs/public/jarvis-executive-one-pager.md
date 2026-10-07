# Jarvis Dev: continuity and control for AI-assisted development

Jarvis Dev is an operating layer around AI coding tools, not another chatbot. It combines repeatable setup, local-first project memory, optional team sharing, and structured delivery. Nexus is the ecosystem’s brand identity; Nexus Hive is its optional shared-memory dashboard, not a separate development platform.

## Two months of beta on real projects

For two months, two of us have used the beta on real client projects, with very good results. We tested it throughout development as the ecosystem evolved. This firsthand experience provides a practical basis for the ecosystem’s value.

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
| Nexus Hive | When deployed and authorized, the dashboard supports shared-memory search, filtering and detail inspection, project oversight, and administration of users and access. This supports visibility and control of team knowledge. |

Configuration replay through `jarvis sync`, drift repair through `jarvis reconcile`, and memory transfer through `mem_sync` or configured daemon background sync are different operations. Reinstallation does not synchronize memory.

## Administration with Nexus Hive

Nexus Hive brings shared knowledge into a single view: browse and search memories, filter by project, category and dates, and inspect individual records. Project summaries, activity and synchronization status help operators oversee the shared service.

With administrator permissions and project governance enabled, administrators can quarantine projects, track application of the operation, and release them reversibly. They can also manage users, roles and access. Quarantine protects projects without deleting them; project deletion and merging are not dashboard operations.

## Operating considerations

Selective reuse of recorded context may reduce repeated explanation. Recording, retrieval, SDD and verification also add effort and consumption; quantitative token savings are not established.

Teams retain control over what is recorded and shared, access and retention policies, and shared-service operation. Keep secrets outside repositories, protect the service with HTTPS, and diagnose setup problems through supported CLI flows rather than hand-editing generated configuration.

## Sources and continuation

[Visual executive one-pager (Spanish)](../presentaciones/jarvis-executive-one-pager.html) · [Ecosystem presentation (Spanish)](../presentaciones/jarvis-ecosystem.html).

[Overview](jarvis-overview.md) · [Adoption stages](jarvis-adoption-guide.md) · [Security and privacy](jarvis-security-and-privacy.md) · [Local memory](../hive/local-memory-guide.md) · [Memory sync](../hive/sync-guide.md) · [Dashboard](../hive/dashboard-guide.md) · [SDD](../sdd-user-guide.md) · [Setup recovery](../setup-recovery.md).
