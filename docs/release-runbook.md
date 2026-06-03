# Release Runbook

Use this runbook when generating Jarvis beta or production releases.

## Quick path

1. For beta, update the fixed `v0.0.1-beta` channel to current sources.
2. For production, create a new semantic version tag.
3. Let the Release workflow and GoReleaser publish assets.
4. Verify release metadata and uploaded artifacts before reporting completion.

## Beta release: fixed moving channel

The beta channel is always:

```text
v0.0.1-beta
```

Use this path when the user asks for a beta release, beta update, or equivalent wording.

### Procedure

1. Confirm the working tree and current branch state:

   ```bash
   git status --short --branch
   git rev-parse HEAD
   git rev-list -n 1 v0.0.1-beta
   ```

2. Move the beta tag to the current source commit and push it:

   ```bash
   git tag -f v0.0.1-beta HEAD
   git push origin refs/tags/v0.0.1-beta --force
   ```

3. Check the Release workflow run created by the tag push:

   ```bash
   gh run list --workflow Release --limit 5 --json databaseId,status,conclusion,headBranch,headSha,createdAt,url,event
   gh run watch <run-id> --exit-status
   ```

4. If GoReleaser fails because assets already exist, delete the existing GitHub release but keep the updated tag, then rerun the workflow:

   ```bash
   gh release delete v0.0.1-beta --yes
   gh run rerun <run-id>
   gh run watch <run-id> --exit-status
   ```

5. Verify the recreated release:

   ```bash
   gh release view v0.0.1-beta --json tagName,targetCommitish,name,isPrerelease,isDraft,publishedAt,url,assets
   git rev-list -n 1 v0.0.1-beta
   git rev-parse HEAD
   git status --short --branch
   ```

### Expected result

- `v0.0.1-beta` points to the current source commit.
- The GitHub release exists as a prerelease.
- Fresh assets exist for both `jarvis` and `hive-daemon`.
- The local working tree remains clean.

## Production release: semantic versioned release

Use this path when the user asks for a production release, PROD release, stable release, or equivalent wording.

Production releases must not reuse `v0.0.1-beta`.

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

## Guardrails

- Do not create new beta version numbers unless the maintainer explicitly asks for them.
- Do not reuse `v0.0.1-beta` for production.
- Do not overwrite production releases unless the maintainer explicitly approves replacement.
- Do not run local builds unless explicitly requested; use the Release workflow as the release builder.
- Never expose tokens or secrets while using `gh` or release tooling.
