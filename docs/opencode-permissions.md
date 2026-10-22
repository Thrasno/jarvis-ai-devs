# OpenCode permission defaults

Jarvis seeds low-prompt OpenCode defaults: ordinary shell commands and access to
any directory are allowed; known destructive commands ask for approval, and
known secret locations are denied to the read and edit tools. There are no
installer questions for this policy. Claude Code receives the same secret-file
guardrails (see [Secret file guardrails](#secret-file-guardrails)); its other
permissions are unchanged.

## What new defaults do

| Surface | Default |
| --- | --- |
| `external_directory` | `allow` for every path, not just the current project |
| Global and generated SDD/Judgment Day agent `bash` | Ordinary commands allowed, including tests, commits and normal pushes |
| Destructive shell patterns | Ask for removal/deletion, forced pushes, hard resets, cleaning, history rewrite, branch deletion, file restoration and dangerous filesystem commands |
| `read` and `edit` | Allow ordinary files; deny known secret locations (see [Secret file guardrails](#secret-file-guardrails)) |
| Generated agents with `edit` | Carry the same secret edit policy, because agent rules are evaluated after global rules |
| Read-only agents | Keep `edit: deny` and `task: deny`; shell defaults do not grant either tool |

The same shell policy is used globally and for newly generated subagents. The
wildcard allow comes first, followed by destructive-command asks because
OpenCode uses last-match-wins ordering.

## Existing installations and replay

The installer, reconfiguration flows and `jarvis sync` apply these source-backed
defaults when generating OpenCode configuration. `jarvis sync` replays recorded
agent configuration; it does not synchronize Hive memory data.

Existing `external_directory`, `bash` and `read` policies, and existing `edit`
object policies, are preserved wholesale, including scalar policies and object
rule order. Missing entries are seeded. One migration applies to `read`: see
[Existing installations](#existing-installations).
An explicit scalar `permission` at global or agent level is also preserved.
Repeated generation does not append shell rules to a user policy. Existing
blanket agent `bash: ask` policies are **not silently migrated**: without ownership
metadata, Jarvis cannot distinguish an old generated policy from a user choice.
Users who want the new defaults must deliberately remove the corresponding
existing entries before replaying configuration. Removing a global scalar policy
also removes that explicit choice for all tools, so review it first.

## Secret file guardrails

Jarvis denies the read and edit tools of Claude Code and OpenCode access to
known secret **locations**. It does not match substrings of file names: the
former `*token*`, `*secret*` and `*credential*` wildcards blocked ordinary
source files such as `Tokenizer.php` or `AuthTokenService.ts` and were removed
(issue #800). The singular directories `secret/`, `token/` and `credential/`
were removed too: Claude Code matches paths case-insensitively, so they also
blocked namespace folders such as `src/Security/Token/JwtToken.php`.

| Protected location | Patterns |
| --- | --- |
| Environment files | `.env`, `.env.*`, `*.env`, `*.env.*` |
| Secret directories | `secrets/`, `tokens/`, `credentials/`, `.credentials/` |
| SSH material | `.ssh/`, `id_rsa*`, `id_ed25519*` |
| Keys and certificates | `*.pem`, `*.key`, `*.p12`, `*.pfx` |
| Credential files | `.npmrc`, `.netrc`, `.pypirc`, `credentials.json`, `.aws/credentials`, `.config/gh/hosts.yml`, `*.token` |
| Keychains | `Library/Keychains/` |

Each pattern applies at the project root and at any depth inside the project.

Home-level secrets live outside the project, so coverage differs by agent:

| Agent | Home directory coverage |
| --- | --- |
| Claude Code | Project patterns do not apply outside the project root. Jarvis adds `~/`-rooted entries for `~/.ssh/`, `~/.aws/credentials`, `~/.npmrc`, `~/.netrc`, `~/.pypirc`, `~/.config/gh/hosts.yml`, `~/.config/tokens/`, `~/.credentials/` and `~/Library/Keychains/`. Other home files, such as `~/secrets/` or a stray `~/key.pem`, are not covered. |
| OpenCode | `*` also matches `/`, so the location patterns above already match home paths, whether reached as absolute paths or through `../`. |

| Agent | Rules |
| --- | --- |
| Claude Code | `Read(...)` and `Edit(...)` entries in `permissions.deny`. `Edit` also covers `Write` and `MultiEdit`. |
| OpenCode | `deny` entries in `permission.read` and `permission.edit`, plus the same edit policy on generated agents that may edit. |

A loose file with a free-form name, such as `my_api_token_backup.txt`, is no
longer denied. Keep secrets in a protected location instead.

### `.env.example` differs between agents

| Agent | `.env.example` and `.env.sample` |
| --- | --- |
| OpenCode | Allowed. Explicit `allow` rules follow the `.env` denies, so last-match-wins lets agents read and edit environment templates. Inside a secret directory (for example `secrets/.env.example`) they stay denied. |
| Claude Code | Denied. A Claude Code `deny` always beats an `allow`, so `.env.*` cannot carry an exception. |

### Existing installations

Deep merge never removes entries, so replay removes the retired rules
explicitly. Retired rules are the substring wildcards (`*secret*`, `*token*`,
`*credential*`) and the singular directories (`secret/`, `token/`,
`credential/`), in all their forms.

- **Claude Code:** the exact retired strings, such as `Read(**/*token*)` or
  `Read(**/token/**)`, are removed from `permissions.deny`. Every other entry
  keeps its value and position. The new `Read(...)` and `Edit(...)` entries,
  including the `~/` ones, are added.
- **OpenCode:** a global `read` policy that allows by default (`"*": "allow"`)
  and still contains a retired rule set to `deny` was written by Jarvis.
  Jarvis removes those retired entries and inserts the missing location rules
  at their template positions. User-added rules keep their values. Any other
  `read` policy, for example a deny-by-default one, is user-owned and left
  untouched. A generated agent `edit: allow` is upgraded to the secret edit
  policy. `ask` and `deny` scalars and existing `edit` objects are preserved.

Replay is idempotent. The installer's
[configuration reset](getting-started.md#configuration-reset-upgrading-an-existing-machine)
still removes the retired Claude Code entries from older installs.

### Not a security boundary

These rules are a convenience guardrail that keeps agents' file tools away from
secrets during normal work. They are not a security boundary:

- **Shell access is not covered.** `cat .env`, `grep`, scripts and any other
  shell command can still read or write these files.
- **Deletion is not covered.** Neither agent exposes a delete permission, and
  `rm` patterns are trivially bypassed.
- **Other tools are not covered.** Only file contents reached through the
  read/edit tools are protected. Location patterns do not catch a secret stored
  under an ordinary name, such as `config/prod.yaml`.

Keep secrets out of the working tree (for example in `~/.config/tokens` or a
secret manager) when they must not reach an agent.

## Limits: not a sandbox

These are command-string patterns, not arbitrary script analysis. Wrappers,
aliases, compound commands, unusual flags and scripts can evade the destructive
patterns. Shell programs can also read secrets: read-tool denies do not constrain
`bash`, other tools or operating-system access. Directory-wide allow is not a
filesystem confinement boundary. Use OS-level isolation and explicit stricter
user policies when stronger protection is required.
