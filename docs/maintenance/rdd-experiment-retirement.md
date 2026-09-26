# RDD Experiment Retirement

RDD and the canonical 4R review model were excluded from v0.0.0. Their instability and frequent blocking created unacceptable workflow friction, so the original experiment was archived rather than shipped.

The historical work remains attributable to:

- Branch: `chore/review-framework-integration`
- Durable archive tag: `archive/rdd-experiment-2026`, identifying the final branch commit
- Draft PR: #445, closed as not planned
- Related RDD issues: #363, #366, #367, #420, #421, #422, #444, and #461, closed as not planned

There is no commitment to reintroduce RDD. Any future adoption must be evaluated again from the then-current `master` branch and against a stable, released Gentle AI contract.

## Later reintroduction and retirement of the four 4R agents (#365, #767)

Issue #365 later reintroduced four Jarvis-issued 4R reviewer agents
(`review-risk`, `review-readability`, `review-reliability`, `review-resilience`)
as generated Claude/OpenCode agents, wired into the embedded orchestrator's
"Mandatory Delegation Triggers" as a fresh-context review/audit requirement.
In practice this caused unwanted reviewer launches on ordinary non-SDD work
and on work outside a git repository, independent of the RDD experiment
described above.

Issue #767 retires the four 4R agents from Jarvis-issued product state:

- Fresh Claude and OpenCode installations contain no `review-*` agent files,
  agent entries, prompts, or task allowlist grants, and the embedded
  orchestrator prompt no longer requires a fresh-context review or audit.
- `jarvis doctor` and `jarvis reconcile` no longer require the four agents.
  On a machine that installed an earlier release, they report leftover
  `review-*` agent files or OpenCode config entries informationally — see
  `invariant.claude.legacy_4r_residue` and
  `invariant.opencode.legacy_4r_residue` in the doctor output. This finding
  is never auto-applied and is never deleted by doctor or reconcile.
- Existing machines are cleaned only through an explicit, consented
  configuration reset offered as the wizard's first step, never through
  silent automatic cleanup. Accepting it snapshots the affected configuration
  before making any change and reports the snapshot ID in the completion
  summary; a failure during the reset itself, or a later install failure for
  that same agent, rolls the reset back automatically. Declining the reset
  leaves the residue in place; this is expected, not a doctor failure. See
  [`../getting-started.md`](../getting-started.md#configuration-reset-upgrading-an-existing-machine)
  for the exact list of surfaces and user-facing wording.

The Council (#647) is a separate, later, explicitly manual-only advisory
review design. It is not installed, activated, or referenced by any
generated Claude/OpenCode configuration as part of #767; it remains an
independent proposal to be built later, if adopted.

Core Hive, Hive API, synchronization, and CLI product work remains
independent of both the retired RDD experiment and the 4R agent retirement.
