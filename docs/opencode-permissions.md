# OpenCode permission defaults

Jarvis seeds low-prompt OpenCode defaults: ordinary shell commands and access to
any directory are allowed; known destructive commands ask for approval, and
secret reads through the read tool are denied. There are no installer questions
for this policy. Claude permissions are unchanged.

## What new defaults do

| Surface | Default |
| --- | --- |
| `external_directory` | `allow` for every path, not just the current project |
| Global and generated SDD/Judgment Day agent `bash` | Ordinary commands allowed, including tests, commits and normal pushes |
| Destructive shell patterns | Ask for removal/deletion, forced pushes, hard resets, cleaning, history rewrite, branch deletion, file restoration and dangerous filesystem commands |
| `read` | Allow ordinary files; retain the existing deny patterns for environment files, secret/token/credential paths, SSH keys, PEM and key files |
| Read-only agents | Keep `edit: deny` and `task: deny`; shell defaults do not grant either tool |

The same shell policy is used globally and for newly generated subagents. The
wildcard allow comes first, followed by destructive-command asks because
OpenCode uses last-match-wins ordering.

## Existing installations and replay

The installer, reconfiguration flows and `jarvis sync` apply these source-backed
defaults when generating OpenCode configuration. `jarvis sync` replays recorded
agent configuration; it does not synchronize Hive memory data.

Existing `external_directory`, `bash` and `read` policies are preserved wholesale,
including scalar policies and object rule order. Missing entries are seeded.
An explicit scalar `permission` at global or agent level is also preserved.
Repeated generation does not append shell rules to a user policy. Existing
blanket agent `bash: ask` policies are **not silently migrated**: without ownership
metadata, Jarvis cannot distinguish an old generated policy from a user choice.
Users who want the new defaults must deliberately remove the corresponding
existing entries before replaying configuration. Removing a global scalar policy
also removes that explicit choice for all tools, so review it first.

## Limits: not a sandbox

These are command-string patterns, not arbitrary script analysis. Wrappers,
aliases, compound commands, unusual flags and scripts can evade the destructive
patterns. Shell programs can also read secrets: read-tool denies do not constrain
`bash`, other tools or operating-system access. Directory-wide allow is not a
filesystem confinement boundary. Use OS-level isolation and explicit stricter
user policies when stronger protection is required.
