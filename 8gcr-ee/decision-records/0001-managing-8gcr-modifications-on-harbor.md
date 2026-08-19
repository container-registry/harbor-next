# ADR-0001: Managing 8gcr Modifications on Harbor

**Status**: Superseded by [ADR-0006](0006-jj-orphan-branches-replace-stgit.md)
**Date**: 2026-01-15
**Decision Makers**: 8gcr Team
**Technical Area**: Source Code Management, Release Engineering

> **2026-08-13 update**: The StGit patch-queue mechanism selected below (Option 1)
> has been replaced by jj (Jujutsu) orphan branches merged into a local octopus
> merge — see ADR-0006. The problem framing, requirements, and feature-isolation
> rationale in this document remain valid; only the *mechanism* changed. Kept for
> historical context.

## Context

8gcr (8gears Container Registry) is a commercial distribution of Harbor with proprietary features and customizations. We need a sustainable process for:

1. Maintaining private modifications on top of the open-source Harbor codebase
2. Keeping feature-related code grouped together for maintainability and AI-assisted development
3. Rebasing our modifications onto new Harbor releases efficiently
4. Supporting potential future divergence from upstream

### Current State

We currently maintain feature branches under `cr/*` (e.g., `cr/2.14.2`) that contain all 8gcr modifications squashed onto Harbor release tags. On each Harbor release, we cherry-pick/rebase our changes onto the new upstream version.

### Current 8gcr Features (as of cr/2.14.2)

| Feature | Complexity | Type |
|---------|------------|------|
| CR INIT (Build/CI/Dagger) | Large | Structural |
| LDAP admin group filter | Small | Modification |
| Hybrid-auth (Multi Source Auth) | Small | Modification |
| SFTP Replication | Medium | New adapter |
| Randomise Scheduling | Small | Modification |
| Improved Copy Pull Command | Small | UI change |
| Conditional Immutability Rule | Medium | New feature |
| Harbor Satellite adapter | Medium | New adapter |
| Subscription menu | Small | UI change |
| Proxy cache fix | Small | Bug fix |
| Schema migrations | Required | Database |

### Requirements

1. **Feature Isolation**: Each feature's code must be easily identifiable and extractable for:
   - Code review
   - AI-assisted modifications (providing complete context)
   - Selective feature enablement
   - Independent testing

2. **Release Efficiency**: Rebasing onto new Harbor releases should be:
   - Scriptable where possible
   - Traceable (know what changed per release)
   - Recoverable (can rollback individual features)

3. **Privacy**: Keep proprietary features private while tracking upstream

4. **Scalability**: Support growth from current ~15 features to potentially 50+

5. **Evolution Path**: Allow transition to more divergent fork model if needed

## Decision Drivers

- Developer velocity for quick fixes and iterations
- AI tooling effectiveness (requires complete feature context)
- Maintenance burden per Harbor release
- Long-term sustainability as feature count grows
- Clear separation between 8gcr code and upstream

## Options Considered

### Option 1: Patch Queue (Selected)

Store each feature as a discrete patch file checked into version control.

```
patches/
├── series                              # Ordering and dependencies
├── 0001-cr-init-build-setup.patch
├── 0002-ldap-admin-group-filter.patch
├── 0003-hybrid-auth-multi-source.patch
├── 0004-sftp-replication.patch
└── ...
```

**Workflow**:
- Development: Code on `main + patches`, create/update patch files
- Release: Apply patches in order to new Harbor tag
- Quick fixes: Requires regenerating affected patch

**Pros**:
- Each patch is self-contained feature context (ideal for AI)
- Patches are version controlled with history
- Selective application via `series` file
- Industry-proven (Linux kernel, Debian, OpenWrt, Buildroot)
- Clear audit trail of changes per release

**Cons**:
- Quick fixes require patch regeneration workflow
- Cascading conflicts when early patches change
- Learning curve for team

### Option 2: Long-lived Feature Branches

Maintain separate branch per feature, merge all for releases.

```
main                           <- upstream
feature/hybrid-auth            <- single feature
feature/sftp-replication       <- single feature
cr/2.14.2                      <- all merged
```

**Pros**:
- Fast iteration on individual features
- Standard git workflow
- Easy to get feature diff: `git diff main..feature/x`

**Cons**:
- Branch explosion as features grow
- Complex dependencies between branches
- Each branch needs rebasing on release
- Merge conflicts accumulate

### Option 3: Feature Directories (Monorepo Overlay)

Store features as directory structures with patches and full files.

```
8gcr-features/
├── hybrid-auth/
│   ├── README.md
│   ├── src/...                # Modified files
│   └── full-diff.patch
└── sftp-replication/
    ├── README.md
    └── src/pkg/replication/sftp/  # New files
```

**Pros**:
- Excellent for AI context (complete files)
- Clear feature boundaries
- Can include documentation per feature

**Cons**:
- Duplicate file storage
- Complex apply mechanism
- Non-standard workflow

### Option 4: Single Branch with Structured Commits

One branch per release with prefixed commits.

```
cr/2.15.0
├── [8gcr/init] CR build setup
├── [8gcr/ldap] LDAP admin group filter
├── [8gcr/hybrid-auth] Multi-source authentication
└── ...
```

**Pros**:
- Simple, no new tools
- Fast iteration
- Works with existing workflow

**Cons**:
- Feature context requires git log/diff commands
- No selective application
- Doesn't scale well beyond 20+ features

### Option 5: Stacked Branches (Graphite/git-town)

Tool-managed dependent branch stacks.

```
main
 └── feature/ldap-admin-filter
      └── feature/hybrid-auth         # depends on ldap
```

**Pros**:
- Tools handle rebase complexity
- Clear dependency model
- Good PR workflow

**Cons**:
- Tool dependency
- Complex setup
- Overkill for current scale

## Decision

**Selected: Option 1 - Patch Queue**

We will implement a patch queue system using standard git tools (`git format-patch`, `git am`) with patches stored in a `patches/` directory.

### Rationale

1. **AI Context**: A single patch file contains the complete diff for a feature - ideal for providing AI assistants with focused context

2. **Industry Proven**: Used successfully by projects managing hundreds of patches (Linux kernel stable, OpenWrt, Debian)

3. **Version Controlled**: Patch files are tracked in git, providing history of how patches evolved across releases

4. **Selective Application**: The `series` file allows enabling/disabling features by commenting lines

5. **Release Automation**: Applying patches can be scripted, with clear failure points when conflicts occur

6. **Evolution Path**: Natural migration to Hybrid Fork when structural changes grow

### Implementation

#### Repository and Branch Structure

The `main` branch is a **fork of Harbor** with the `8gcr-ee/` directory added. This keeps Harbor source code and patches together, simplifying all workflows.

```
main                    # Harbor fork + 8gcr-ee/ directory
├── src/                # Harbor source code
├── api/
├── make/
├── 8gcr-ee/            # 8gcr commercial patches (our addition)
│   └── patches/
│       ├── series
│       ├── README.md
│       └── *.patch
└── docs/

cr/2.14.2               # main + patches applied (release)
cr/2.15.0               # main + patches applied (release)

wip/*                   # Temporary work branches (local only)
```

**Why this structure:**
1. `main` has both Harbor source and patches - no copying between branches
2. Developers can work immediately after clone (IDE, tooling works)
3. Simple `git add 8gcr-ee/patches/` to commit patch changes
4. Upstream sync via `git merge upstream/main`
5. Commercial patches clearly separated in `8gcr-ee/` directory

**Setting up upstream:**
```bash
git remote add upstream https://github.com/goharbor/harbor.git
git fetch upstream --tags
```

**Syncing with upstream Harbor:**
```bash
git fetch upstream
git merge upstream/main
# Resolve conflicts if any (8gcr-ee/ is ours, won't conflict)
```

| Branch | Contains | Pushed to Remote? |
|--------|----------|-------------------|
| `main` | Harbor source + 8gcr-ee/patches/ | Yes (source of truth) |
| `cr/*` | main + patches applied | Yes (releases) |
| `wip/*` | Work in progress | No (local only) |

#### Patches Directory Structure

```
8gcr-ee/
└── patches/
    ├── series     # Order and dependency documentation (stgit format)
    ├── README.md  # Patch workflow documentation
    │
    ├── 0001-cr-init-build-setup.patch
    ├── 0002-ldap-admin-group-filter.patch
    ├── 0003-hybrid-auth-multi-source.patch
    ├── 0004-sftp-replication.patch
    ├── 0005-randomise-scheduling.patch
    ├── 0006-improved-copy-pull-command.patch
    ├── 0007-conditional-immutability.patch
    ├── 0008-satellite-adapter.patch
    ├── 0009-subscription-menu.patch
    ├── 0010-proxy-cache-fix.patch
    └── 0011-schema-migrations.patch        # Always last
```

#### Series File

The `series` file is a plain text file that lists patches in the order they should be applied. This is a convention from `quilt` and other patch management tools.

**Purpose:**
- **Defines order** - Patches are applied top to bottom
- **Documents dependencies** - Comments explain relationships between patches
- **Enables/disables features** - Comment out a line with `#` to skip that patch
- **Human readable** - Anyone can quickly see what features are included

**Why not just use `*.patch` glob?**
Using `git am patches/*.patch` works but relies on filename sorting. The `series` file gives explicit control over order and allows disabling patches without deleting them.

#### Branches + Patches Workflow

**Patches are the artifact. Branches are the workspace.**

You can and should use temporary branches for development. The patch file is the **source of truth** (checked into git), but branches are your **workspace** (local, disposable).

```
8gcr-ee/patches/                  # Source of truth (checked in)
├── series
├── 0001-init.patch
├── 0002-ldap.patch
└── 0003-hybrid-auth.patch

branches (temporary, local):      # Workspace (not pushed)
├── wip/hybrid-auth               # Working on hybrid-auth
└── wip/new-feature               # Developing new feature
```

**Why use temp branches:**
- Normal git workflow for development
- Can make multiple WIP commits before squashing
- Easy to test changes before generating patch
- Multiple developers can work in parallel

**Branch naming convention:**
```
wip/                    # Work in progress (temporary, local)
├── wip/applied         # All patches applied (base for work)
├── wip/hybrid-auth     # Working on specific feature
└── wip/release-2.15    # Preparing release

cr/                     # Release branches (permanent, pushed)
├── cr/2.14.2           # Released version
└── cr/2.15.0           # Released version
```

#### Workflows

**New Feature Development**:
```bash
# 1. Start from main (has Harbor source + 8gcr-ee/patches/)
git checkout main -b wip/new-thing
stg init

# 2. Apply existing patches
stg import -s 8gcr-ee/patches/series

# 3. Create new patch and develop feature
stg new 00XX-new-thing -m "[8gcr/new-thing] Description"
# ... code ...
git add -A
stg refresh

# 4. Export patches and update series
stg export -d 8gcr-ee/patches/ -n
echo "00XX-new-thing.patch" >> 8gcr-ee/patches/series

# 5. Commit to main
git checkout main
git add 8gcr-ee/patches/
git commit -m "Add new-thing patch"
```

**New Release**:
```bash
# 1. Sync main with new Harbor version
git fetch upstream
git checkout main
git merge upstream/vX.Y.Z
# Resolve any conflicts (8gcr-ee/ is ours, won't conflict)

# 2. Create release branch from main
git checkout -b cr/X.Y.Z
stg init

# 3. Apply patches (fix conflicts as they arise)
stg import -s 8gcr-ee/patches/series
# If conflicts occur:
#   1. Resolve conflicts
#   2. git add resolved files
#   3. stg refresh
#   4. stg import continues automatically

# 4. Export updated patches
stg export -d 8gcr-ee/patches/ -n

# 5. Commit updated patches to main
git checkout main
git add 8gcr-ee/patches/
git commit -m "Update patches for Harbor vX.Y.Z"
```

**Updating a Single Patch** (e.g., fixing a bug in `0003-hybrid-auth.patch`):

With StGit, this workflow becomes simple - no manual squashing or rebasing required:

```bash
# 1. Start from main (has Harbor source + 8gcr-ee/patches/)
git checkout main -b wip/fix-hybrid-auth
stg init

# 2. Import patches
stg import -s 8gcr-ee/patches/series

# 3. Go to the patch you want to modify
stg goto 0003-hybrid-auth

# 4. Make your fix
vim src/core/auth/authenticator.go
git add -A
stg refresh   # Automatically updates the patch - no squashing needed!

# 5. Re-apply remaining patches
stg push -a

# 6. Export updated patches
stg export -d 8gcr-ee/patches/ -n

# 7. Commit updated patch to main
git checkout main
git add 8gcr-ee/patches/
git commit -m "Update hybrid-auth patch: fix authentication edge case"
```

**Why StGit is better:** The `stg refresh` command automatically incorporates your changes into the current patch - no manual squashing with `git rebase -i` required.

**Onboarding a New Developer:**

```bash
# 1. Install StGit
brew install stgit

# 2. Clone the repo (contains Harbor source + 8gcr-ee/patches/)
git clone git@github.com:container-registry/harbor-next.git
cd harbor-next

# 3. Add upstream Harbor (for future syncs)
git remote add upstream https://github.com/goharbor/harbor.git

# 4. Create a working branch and apply patches
git checkout main -b wip/applied
stg init
stg import -s 8gcr-ee/patches/series

# 5. Done - full 8gcr codebase ready for development
# Run, test, develop from wip/applied branch
```

### Tooling

#### StGit (Stacked Git) - Required

We use **StGit** for all patch management. StGit is git-native and dramatically simplifies the patch workflow by treating patches as editable commits.

**Install StGit:**
```bash
brew install stgit
```

**Why StGit:**
- `stg refresh` replaces the manual squash + `git format-patch` dance
- Patches are git commits (familiar workflow)
- `stg goto` to jump between patches instantly
- `stg export` generates patch files for storage
- Industry-proven (used by Linux kernel maintainers)

**StGit workflow:**

```bash
# Initialize on a working branch
git checkout main -b wip/applied
stg init

# Import existing patches
stg import -s 8gcr-ee/patches/series

# Work on a specific patch
stg goto 0003-hybrid-auth          # Jump to that patch
vim src/core/auth/authenticator.go
git add -A
stg refresh                        # Updates patch automatically (no manual squash!)

# Re-apply remaining patches
stg push -a

# Export back to patch files
stg export -d 8gcr-ee/patches/ -n

# Commit updated patches to main
git checkout main
git add 8gcr-ee/patches/
git commit -m "Update patches"

# New release workflow
git fetch upstream
git checkout main
git merge upstream/vX.Y.Z          # Sync with new Harbor version
git checkout -b wip/release
stg init
stg import -s 8gcr-ee/patches/series  # Fix conflicts as they arise
stg export -d 8gcr-ee/patches/ -n     # Save updated patches
git checkout main
git add 8gcr-ee/patches/
git commit -m "Update patches for Harbor vX.Y.Z"
```

#### Quick Reference

| Task | Command |
|------|---------|
| Initialize | `stg init` |
| Import patches | `stg import -s 8gcr-ee/patches/series` |
| Apply to specific | `stg goto <patch>` |
| List stack | `stg series` |
| Create new patch | `stg new <name> -m "message"` |
| Update patch | `stg refresh` |
| Export to files | `stg export -d 8gcr-ee/patches/ -n` |

See `8gcr-ee/patches/README.md` for detailed workflow documentation.

## Evolution Path

### Stage 1: Current (Patch Queue)
- All changes as patches
- Suitable for: < 20 features, close to upstream

### Stage 2: Hybrid Fork (When Needed)
Transition when:
- Build/CI changes become too large for patches
- New directories/adapters stabilize
- Maintenance burden of large patches exceeds benefit

Structure:
```
harbor-next/                    # Open source fork (container-registry/harbor-next)
├── .8gcr/                      # 8gcr-owned: build, CI, config
├── src/pkg/8gcr/               # 8gcr-owned: new code
├── src/pkg/replication/sftp/   # 8gcr-owned: new adapters
├── 8gcr-ee/patches/            # Remaining modifications
└── UPSTREAM.md                 # Tracks upstream version
```

Migration:
1. Create permanent fork with structural changes
2. Move stable new directories out of patches
3. Keep modifications to upstream files as patches
4. Sync upstream via periodic merges

### Stage 3: Full Fork (If Required)
Transition when:
- Strategic direction diverges from Harbor
- Upstream compatibility no longer valuable
- Majority of codebase is modified

This would involve:
- Renaming/rebranding
- Removing patch system
- Independent versioning

## Consequences

### Positive
- Clear feature isolation for AI-assisted development
- Version-controlled patch history
- Selective feature application
- Scriptable release process
- Proven methodology

### Negative
- Learning curve for StGit and patch workflow
- Conflicts may cascade through patch series

### Mitigations
- Document workflows thoroughly in `8gcr-ee/patches/README.md`
- Use StGit which dramatically simplifies patch management
- Start with current features, refine process
- Review and adjust after 2-3 releases

## References

- [StGit (Stacked Git)](https://stacked-git.github.io/) - Our recommended patch management tool
- [StGit Tutorial](https://stacked-git.github.io/guides/tutorial/)
- [Quilt Patch Management](https://savannah.nongnu.org/projects/quilt)
- [OpenWrt Patch Workflow](https://openwrt.org/docs/guide-developer/toolchain/use-patches-with-buildsystem)
- [Debian Patch System](https://wiki.debian.org/debian/patches)
- [Git Format-Patch Documentation](https://git-scm.com/docs/git-format-patch)
- [Linux Kernel Stable Maintenance](https://www.kernel.org/doc/html/latest/process/stable-kernel-rules.html)

## Changelog

| Date | Change | Author |
|------|--------|--------|
| 2026-01-15 | Initial decision | 8gcr Team |
| 2026-01-30 | Simplify to StGit-only (remove custom scripts) | 8gcr Team |
