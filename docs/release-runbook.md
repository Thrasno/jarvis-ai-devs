# Release Runbook

Use this runbook when publishing Jarvis beta or production releases.

## Quick path

1. For beta, publish from updated `master`; it uses `v0.0.1-beta` internally and publishes the public `beta` release.
2. For production, create and push a new immutable `vX.Y.Z` tag.
3. Verify release metadata and assets for both `jarvis` and `hive-daemon`.
4. Hand off install commands and the macOS best-effort caveat.

## Beta release: mutable beta channel

Use this path when the user asks for a beta release, beta update, or equivalent wording. Beta is always published from updated `master`, not from an arbitrary local branch or ref. The public beta channel is the mutable `beta` tag and prerelease. The workflow uses the SemVer-compatible `v0.0.1-beta` tag internally because GoReleaser release mode requires SemVer tags.

### Procedure

1. Confirm intended changes are merged to `master`, then update local `master`:

   ```bash
   git switch master
   git pull --ff-only origin master
   git status --short --branch
   git rev-parse origin/master
   ```

2. Run the workflow. It always checks out and verifies remote `master` before publishing:

   ```bash
   gh workflow run "Beta Release"
   gh run list --workflow "Beta Release" --limit 5 --json databaseId,status,conclusion,headSha,url,event
   gh run watch <run-id> --exit-status
   ```

3. Verify the release:

   ```bash
   git ls-remote origin refs/heads/master refs/tags/beta refs/tags/beta^{} refs/tags/v0.0.1-beta refs/tags/v0.0.1-beta^{}
   gh run view <run-id> --json headSha,url
   gh release view beta --json tagName,targetCommitish,name,isPrerelease,isDraft,publishedAt,url,assets
   ```

   Compare the remote `origin/master` commit, workflow `headSha`, public `beta` release target/tag, and remote tag refs. Do not rely on a local `git rev-list beta` unless tags were fetched first.

4. Share beta install commands:

   ```bash
   export JARVIS_INSTALL_VERSION=beta
   curl -sSL https://raw.githubusercontent.com/Thrasno/jarvis-ai-devs/master/scripts/install.sh | bash
   ```

   ```powershell
   $env:JARVIS_INSTALL_VERSION = "beta"
   irm https://raw.githubusercontent.com/Thrasno/jarvis-ai-devs/master/scripts/install.ps1 | iex
   ```

### Expected result

- Remote `origin/master`, workflow `headSha`, `v0.0.1-beta`, and `beta` point to the same source commit.
- The GitHub release exists as a prerelease and is not marked latest.
- Fresh public beta assets exist for both `jarvis` and `hive-daemon` with `beta` in the asset name.
- Installers can use `JARVIS_INSTALL_VERSION=beta`.

### v0.0.1-beta compatibility fallback

If the public `beta` release publishing step fails after GoReleaser succeeds, the internal release remains available at:

```text
v0.0.1-beta
```

Delete only the old public GitHub release if assets already exist, keep the moved tags, rerun the workflow, and document that beta downloads must temporarily use `JARVIS_INSTALL_VERSION=v0.0.1-beta`.

```bash
gh workflow run "Beta Release"
```

## Production release: immutable semantic version

Use this path when the user asks for a production release, PROD release, stable release, or equivalent wording. Production releases must not reuse `beta` or `v0.0.1-beta`.

### Choose the version

Inspect the changes since the latest production tag:

```bash
git tag --list 'v*' --sort=-version:refname
git log --oneline <latest-production-tag>..HEAD
```

Ignore prerelease tags such as `v0.0.1-beta` when selecting the latest production tag.

Choose the next version by impact:

| Change impact | Version bump |
|---------------|--------------|
| Bug fixes or small safe improvements | Patch: `vX.Y.Z` to `vX.Y.(Z+1)` |
| Meaningful features or product additions | Minor: `vX.Y.Z` to `vX.(Y+1).0` |
| Breaking changes, migration-heavy changes, or major architecture shifts | Major: `vX.Y.Z` to `v(X+1).0.0` |

### Procedure

1. Confirm the selected production version with the release impact analysis.

2. Create and push the production tag:

   ```bash
   git tag vX.Y.Z HEAD
   git push origin refs/tags/vX.Y.Z
   ```

3. Watch the Release workflow:

   ```bash
   gh run list --workflow Release --limit 5 --json databaseId,status,conclusion,headBranch,headSha,createdAt,url,event
   gh run watch <run-id> --exit-status
   ```

4. Verify the release:

   ```bash
   gh release view vX.Y.Z --json tagName,targetCommitish,name,isPrerelease,isDraft,publishedAt,url,assets
   git rev-list -n 1 vX.Y.Z
   git rev-parse HEAD
   git status --short --branch
   ```

### Expected result

- A new immutable production tag exists.
- The GitHub release is not a draft.
- Release assets exist for both `jarvis` and `hive-daemon`.
- The version bump matches the implemented change impact.

## Teammate handoff notes

- Stable installs use the latest production release endpoint by default.
- Beta installs use `JARVIS_INSTALL_VERSION=beta`; use `v0.0.1-beta` only if the runbook announces the internal compatibility release as a temporary fallback.
- Release assets include Linux and Windows archives. macOS artifacts are best effort from GoReleaser and are not separately validated by CI.
- After installing, run `jarvis` to apply or reapply managed configuration.

## Guardrails

- Do not create new beta version numbers unless the maintainer explicitly asks for them.
- Do not reuse `beta` or `v0.0.1-beta` for production.
- Do not overwrite production releases unless the maintainer explicitly approves replacement.
- Do not run local builds unless explicitly requested; use the Release workflow as the release builder.
- Never expose tokens or secrets while using `gh` or release tooling.
