# Proposal: Guarded Memory Delete and Restore

## Intent

Complete the installed `jarvis hive` human workflow for safely deleting one local memory and restoring it from Recently Deleted. Today the daemon has guarded soft-mutation foundations, but the canonical UI cannot create the required backup, reach tombstones, or distinguish local completion from shared propagation.

## Intended Outcomes

- Users verify project, local ID, `sync_id`, session, and sync state before mutation.
- Delete and restore automatically obtain a fresh backup, require exact confirmation and a non-empty reason, and revalidate identity before execution.
- Local success means the tombstone/restore and mutation journal committed atomically; shared propagation is reported separately as pending, completed, failed/retryable, or legacy-unsupported.
- Active and Recently Deleted views remain distinct.

## Scope

### In Scope
- End-to-end delete/restore UX in installed `jarvis hive`, including backup renewal and actionable failure states.
- Per-target local and shared status, v2 propagation, and safe handling of unsynced create-then-delete so the memory never appears remotely.
- Capability-based compatibility with older daemons; an operation is enabled only when its complete safety contract is supported.
- Regression protection for human-only destructive governance.

### Out of Scope
- Hard delete, bulk cleanup, retention, project deletion, direct Hive API administration, or a general move operation.
- Canonizing the standalone `hive` CLI or adding agent/MCP delete, restore, or guard execution.

## Capabilities

### New Capabilities
- `memory-delete-restore`: Human-governed local soft delete, Recently Deleted restore, audit, identity safety, and propagation visibility.

### Modified Capabilities
None.

## Approach and Boundaries

Preserve ownership: `jarvis hive` → `hiveclient` → loopback daemon governance; the daemon owns backups, identity validation, storage transactions, journaling, and sync. Compose existing backup and guard operations unless design proves that composition cannot preserve safety. Backup expiry triggers automatic renewal, but execution continues only after exact project/local ID/`sync_id` revalidation. MCP remains non-destructive.

## Affected Areas

| Area | Impact |
|---|---|
| `jarvis-cli/internal/hiveclient`, `hiveui`, `cmd/jarvis` | Canonical UX, client capability/status models |
| `hive-daemon/internal/governance`, `httpapi`, `db`, `sync` | Safety evidence, causal propagation, compatibility |
| `hive-daemon/internal/mcp` | Preserve exclusion tests |

## Risks

- Identity drift could mutate the wrong copy; abort on any identity mismatch.
- UI could overstate shared completion; require target-level acknowledgement states.
- Legacy or unordered sync could resurrect data; never weaken enabled operations or expose an unsynced create remotely.

## Deferred to Specs/Design

Define causal event/compaction semantics, retry/idempotency after transport loss, capability discovery, audit presentation, and whether response/read-model enrichment is sufficient—without prematurely prescribing endpoints or schemas.

## Rollback Plan

Disable the new UI capabilities and status presentation while retaining existing tombstones, journals, backups, and daemon guards; no destructive data migration is allowed.

## Dependencies

- Existing governance backup/guard boundaries, transactional mutation journal, and mutation-sync-v2/Hive API support.

## Success Criteria

- [ ] Installed `jarvis hive` safely deletes and restores exactly one verified target with symmetric mandatory reasons.
- [ ] Local and shared outcomes remain accurate across v2, retries, backup renewal, and legacy capability limits.
- [ ] Corrected copies and unrelated data remain untouched; agents/MCP remain unable to invoke destructive operations.
