# VM Dogfooding Phase 1 — Jarvis Ecosystem Test Guide

Use this guide to test Jarvis in real development work inside Windows and Linux VMs. This is a **dogfooding phase**, not a lifecycle certification pass.

## Quick path

1. Create a fresh VM.
2. Install `jarvis` and `hive-daemon`.
3. Run root `jarvis` to configure the ecosystem.
4. Open a real project and run `jarvis init`.
5. Use SDD on a small real change.
6. Verify Hive local memory, Hive API sync, skills, personas, and agent behavior.
7. Record every result in the test log template below.

## Scope

| Area | Phase 1 status |
|------|----------------|
| Real project development in VMs | In scope |
| Hive local memory behavior | In scope |
| Hive API sync behavior | In scope |
| SDD workflow execution | In scope |
| Skill registry and path-injected skill loading | In scope |
| Personas/agents/prompt injection | In scope |
| Backup/restore/uninstall certification | Out of scope for Phase 1 |
| Full lifecycle recovery guarantees | Out of scope for Phase 1 |
| Minimal distro dependency certification | Out of scope for Phase 1 |

> Phase 1 answers: “Can we use Jarvis productively in real VM development?”  
> It does **not** answer: “Is every install/recovery/lifecycle command release-certified?”

## VM matrix

Start with this small matrix. Add more distros only after the happy path works.

| OS | VM | Priority | Notes |
|----|----|----------|-------|
| Windows 11 | Fresh desktop VM | P0 | Main Windows dogfooding target |
| Ubuntu LTS | Fresh desktop/server VM | P0 | Main Linux baseline |
| Debian stable | Fresh VM | P1 | Checks conservative package baseline |
| Fedora latest | Fresh VM | P1 | Checks newer distro behavior |
| Arch/Manjaro | Fresh VM | P2 | Useful later, not first-pass blocker |

## Before each run

Record:

| Field | Value |
|-------|-------|
| Tester |  |
| Date |  |
| OS + version |  |
| VM provider |  |
| Shell |  |
| Jarvis version/commit |  |
| Hive API environment | local / staging / production-like |
| Test project |  |

## Test cases

### 1. Install the ecosystem

**Goal:** Confirm the release channel installs the ecosystem pack.

Steps:

1. Install Jarvis using the platform installer.
2. Open a new terminal.
3. Run:

```bash
jarvis --help
hive-daemon --help
```

Expected result:

- `jarvis` is available from PATH.
- `hive-daemon` is available from PATH.
- No manual binary copying is required after installer completion.

Record:

| Result | Notes |
|--------|-------|
| PASS / FAIL / BLOCKED |  |

### 2. Run setup/reconfiguration wizard

**Goal:** Confirm the canonical setup entrypoint works.

Steps:

1. Run:

```bash
jarvis
```

2. Complete the wizard for the target provider(s).
3. If testing reconfiguration, run `jarvis` again after setup.

Expected result:

- Root `jarvis` launches setup/reconfiguration.
- No nonexistent `reconfigure` command is needed.
- Managed assets are written without confusing prompts.

Record provider behavior:

| Provider | PASS / FAIL / BLOCKED | Notes |
|----------|------------------------|-------|
| Claude |  |  |
| OpenCode |  |  |

### 3. Initialize a real project

**Goal:** Confirm project-local Jarvis assets are generated correctly.

Steps:

1. Open a real repo in the VM.
2. Run:

```bash
jarvis init
```

3. Inspect generated files:

```bash
ls .jarvis
ls .jarvis/skills
```

Expected result:

- `.jarvis/skill-registry.md` exists.
- `.jarvis/skills/<skill>/SKILL.md` files exist.
- Skill registry paths point to loadable `.jarvis/skills/.../SKILL.md` files.
- `.atl/skill-registry.md` is not treated as canonical.

Record:

| Check | PASS / FAIL | Notes |
|-------|-------------|-------|
| `.jarvis/skill-registry.md` exists |  |  |
| `.jarvis/skills` copied |  |  |
| Registry paths load |  |  |

### 4. Verify Hive local memory

**Goal:** Confirm local memory initializes and persists.

Steps:

1. Start/use the configured agent normally.
2. Ask it to remember a small project fact.
3. Restart terminal/agent session.
4. Ask it to recall that fact.
5. Check local Hive storage exists.

Expected result:

- Hive local DB exists under the configured Jarvis/Hive location.
- Memory survives a session restart.
- The agent can recall the saved project fact.

Record:

| Check | PASS / FAIL | Notes |
|-------|-------------|-------|
| Local DB created |  |  |
| Memory saved |  |  |
| Memory recalled after restart |  |  |

### 5. Verify Hive API sync

**Goal:** Confirm local Hive can synchronize with Hive API when configured.

Steps:

1. Configure Hive API endpoint and credentials for the VM.
2. Save a test memory in the VM.
3. Trigger or wait for sync using the supported flow.
4. Confirm the memory is visible through Hive API or another synced client.
5. Repeat in the opposite direction if supported.

Expected result:

- Sync configuration is accepted.
- Local memory reaches Hive API.
- No secret is printed in logs or terminal output.
- Sync failures are diagnosable.

Record:

| Direction | PASS / FAIL / BLOCKED | Notes |
|-----------|------------------------|-------|
| VM → Hive API |  |  |
| Hive API → VM |  |  |

### 6. Run a complete SDD flow

**Goal:** Confirm the development workflow works in a real project.

Steps:

1. Pick a tiny real change.
2. Run the SDD flow:
   - explore
   - proposal
   - spec
   - design
   - tasks
   - apply
   - verify
   - archive
3. Use the project’s normal test command during verify.

Expected result:

- SDD artifacts are persisted in the configured store.
- Apply follows task boundaries.
- Verify runs real tests.
- Archive captures final state.
- The agent does not lose context between phases.

Record:

| Phase | PASS / FAIL / BLOCKED | Notes |
|-------|------------------------|-------|
| Explore |  |  |
| Proposal |  |  |
| Spec |  |  |
| Design |  |  |
| Tasks |  |  |
| Apply |  |  |
| Verify |  |  |
| Archive |  |  |

### 7. Verify skills and personas

**Goal:** Confirm prompt injection, personas, agents, and path-injected skills behave correctly.

Steps:

1. Ask the orchestrator to delegate a task requiring a skill, such as Go testing or Judgment Day.
2. Confirm launch prompts include:

```markdown
## Skills to load before work
```

3. Confirm sub-agent result includes:

```text
skill_resolution: paths-injected
```

4. Check that persona/tone and agent roles match expected behavior.

Expected result:

- Skills are loaded from `.jarvis/skills/.../SKILL.md`.
- No old `Project Standards`/compact-rule-primary behavior appears as the main contract.
- Personas and role boundaries are respected.

Record:

| Check | PASS / FAIL | Notes |
|-------|-------------|-------|
| Skill paths injected |  |  |
| `paths-injected` reported |  |  |
| Persona applied |  |  |
| Agent role respected |  |  |

### 8. Run Judgment Day on a small change

**Goal:** Confirm adversarial review works in the VM.

Steps:

1. Make a small local change.
2. Ask for Judgment Day.
3. Confirm two blind judges run.
4. If findings appear, classify them as CRITICAL, WARNING, or SUGGESTION.

Expected result:

- Two judges run independently.
- Findings include concrete evidence.
- Warnings are real normal-use issues, not theoretical noise.
- If fixes are applied, re-judgment runs before approval.

Record:

| Check | PASS / FAIL | Notes |
|-------|-------------|-------|
| Two judges launched |  |  |
| Evidence included |  |  |
| Re-judgment after fix |  |  |

## Known exclusions for Phase 1

Do not block Phase 1 dogfooding on these unless they prevent basic usage:

- `jarvis backup`
- `jarvis restore`
- `jarvis uninstall`
- full lifecycle recovery guarantees
- certifying `jarvis verify` as strictly read-only
- minimal Linux distro dependency hardening for hooks

Still record issues if you see them. Just label them as **Lifecycle Hardening**, not Phase 1 dogfooding failures.

## Finding severity

| Severity | Meaning | Example |
|----------|---------|---------|
| BLOCKER | Cannot continue basic VM dogfooding | `jarvis` cannot run after install |
| CRITICAL | Core ecosystem behavior broken | SDD cannot persist artifacts; Hive cannot start |
| WARNING | Normal use works but important behavior is degraded | Sync works only after undocumented manual config |
| SUGGESTION | Improvement or clarity issue | Output wording is confusing but behavior works |

## Test log template

Copy this section once per VM.

```markdown
## VM Test Log

| Field | Value |
|-------|-------|
| Tester |  |
| Date |  |
| OS + version |  |
| VM provider |  |
| Jarvis version/commit |  |
| Hive API environment |  |
| Test project |  |

### Summary

- Overall result: PASS / FAIL / BLOCKED
- Main blocker, if any:
- Follow-up tasks:

### Results

| Test case | Result | Notes |
|-----------|--------|-------|
| Install ecosystem |  |  |
| Setup/reconfiguration wizard |  |  |
| Project init |  |  |
| Hive local memory |  |  |
| Hive API sync |  |  |
| SDD flow |  |  |
| Skills/personas |  |  |
| Judgment Day |  |  |

### Findings

| Severity | Area | Description | Evidence | Follow-up |
|----------|------|-------------|----------|-----------|
|  |  |  |  |  |
```

## Exit criteria for Phase 1

Phase 1 is successful when:

- Windows 11 and Ubuntu LTS can run the complete dogfooding path.
- At least one additional Linux distro completes install, init, Hive local memory, and SDD verify.
- Hive API sync is proven in at least one Windows VM and one Linux VM.
- Skill path injection reports `paths-injected` in real delegated work.
- All BLOCKER and CRITICAL findings are either fixed or explicitly deferred with owner and rationale.

## Next phase

After Phase 1, run a separate lifecycle hardening pass for:

- backup/restore/uninstall,
- `verify` read-only semantics,
- minimal distro dependency checks,
- scripted reconfiguration expectations,
- release certification checklist.
