# Git Workflow — Conventions and Standards

## Conventional Commits (MANDATORY)

Format: `<type>(<scope>): <description>`

| Type | When to use |
|------|-------------|
| `feat` | New feature |
| `fix` | Bug fix |
| `chore` | Maintenance (deps, config) |
| `docs` | Documentation only |
| `refactor` | Code restructure, no behavior change |
| `test` | Adding or fixing tests |
| `perf` | Performance improvement |
| `ci` | CI/CD changes |

Examples:
```
feat(auth): add JWT refresh token support
fix(order): prevent duplicate items in cart
chore(deps): update cobra to v1.9.1
refactor(service): extract email sending to dedicated service
test(order): add integration test for checkout flow
```

## Branch Naming

```
feature/{ticket-id}-short-description
bugfix/{ticket-id}-short-description
hotfix/{ticket-id}-short-description
chore/{description}
```

Examples:
```
feature/PROJ-123-user-auth
bugfix/PROJ-456-null-pointer-checkout
hotfix/payment-gateway-timeout
chore/update-dependencies
```

## Rules

### NEVER force push to main/master
```bash
# This will destroy history and break team members' local repos
git push --force origin main  # ❌ NEVER

# Use --force-with-lease on feature branches only
git push --force-with-lease origin feature/my-branch  # ✅ safer
```

### Commit message rules
- Subject line: imperative mood, 72 chars max
- No period at end of subject line
- Body (optional): explain WHY, not WHAT — the diff shows what
- NEVER add "Co-Authored-By: AI" or similar AI attribution

### Before merging
- Rebase on main, don't merge: `git rebase main`
- Squash fixup commits: `git rebase -i main`
- All CI checks must pass
- At least one code review approval

### Small commits
- One logical change per commit
- If you need "and" in your commit message, split it
