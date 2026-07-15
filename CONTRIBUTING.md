# Contributing to Harbor Next

Thank you for contributing! This guide explains how to open PRs and merge them correctly so the automated release pipeline works as expected.

## Table of Contents

- [Workflow Overview](#workflow-overview)
- [Creating a Pull Request](#creating-a-pull-request)
- [Merging a Pull Request](#merging-a-pull-request)
- [How Releases Work](#how-releases-work)
- [Adding Release Notes to Your PR](#adding-release-notes-to-your-pr)
- [Local Development Setup](#local-development-setup)

---

<<<<<<< HEAD
## Workflow Overview
=======
* [Bi-weekly public community meetings][community-meetings]
  * Catch up with [past meetings on YouTube][past-meetings]
* Chat with us on the CNCF Slack ([get an invitation here][cncf-slack])
  * [#harbor][users-slack] for end-user discussions
  * [#harbor-dev][dev-slack] for development of Harbor
* Want long-form communication instead of Slack? We have two distribution lists:
  * [harbor-users][users-dl] for end-user discussions
  * [harbor-dev][dev-dl] for development of Harbor

Follow us on Twitter at [@project_harbor][twitter]

## Getting Started

### Fork Repository

Fork the Harbor repository on GitHub to your personal account.
```sh
#Set golang environment
export GOPATH=$HOME/go
mkdir -p $GOPATH/src/github.com/goharbor

#Get code
cd $GOPATH/src/github.com/goharbor/harbor
git clone git@github.com:goharbor/harbor.git

#Track repository under your personal account
git config push.default nothing # Anything to avoid pushing to goharbor/harbor by default
git remote rename origin goharbor
git remote add $USER git@github.com:$USER/harbor.git
git fetch $USER
>>>>>>> 36d6e8c24 (docs: fix markdown formatting issues in CONTRIBUTING.md and README.md (#23537))

```
fork/branch -> commit (conventional) -> PR -> CI passes -> squash merge -> release-please -> release
```

All changes go through PRs. Never push directly to `main`.

---

## Creating a Pull Request

### 1. Branch Naming

Use a short, descriptive branch name prefixed by the change type:

```
feat/oidc-federated-login
fix/x509-negative-serial
ci/parallel-image-builds
docs/contributing-guide
```

### 2. Commit Messages (Conventional Commits)

Every commit must follow [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <short description>

[optional body]

Signed-off-by: Your Name <your@email.com>
```

Common types:

| Type | When to use | Release effect |
|------|------------|----------------|
| `feat` | New user-facing feature | Minor version bump |
| `fix` | Bug fix | Patch version bump |
| `upstream` | Cherry-picked upstream Harbor change | Patch version bump |
| `feat!` / `fix!` | Breaking change | Major version bump |
| `refactor` | Code change, no behaviour change | No release |
| `docs` | Documentation only | No release |
| `ci` | CI/CD pipeline changes | No release |
| `chore` | Maintenance, dependencies | No release |
| `test` | Tests only | No release |
| `build` | Build system changes | No release |

DCO sign-off is required on every commit. Use `git commit -s` to add it automatically.

### 3. PR Title

The PR title becomes the squash commit message on main, so it must also follow Conventional Commits. The `pr-title` CI check enforces this and will block merging if the format is wrong.

The type prefix must be lowercase, and the subject must start with a capital letter:

Good:
```
feat(portal): Add Repository-Level Pull Command to Artifact List Tab
fix: Allow Negative Serial Numbers in X509 Certificates
ci: Split Image Builds into Parallel Matrix Jobs
```

Bad:
```
Updated the portal
Fix bug
feat: add new feature
Merge pull request #5
```

### 4. Scopes (Optional but Recommended)

Use a scope in parentheses to indicate the component:

```
feat(portal): ...
fix(core): ...
upstream(proxy): ...
ci(release): ...
```

### 5. PR Description

Use the following template for your PR description:

```markdown
## Summary
<!-- Brief description of what this PR does -->

## Related Issues
<!-- Fixes #123 -->

## Type of Change
- [ ] Bug fix (`fix:`)
- [ ] New feature (`feat:`)
- [ ] Breaking change (`feat!:` / `fix!:`)
- [ ] Documentation (`docs:`)
- [ ] Refactoring (`refactor:`)
- [ ] CI/CD or build changes (`ci:` / `build:`)
- [ ] Upstream Harbor cherry-pick (`upstream:`)
- [ ] Dependencies update (`chore:`)
- [ ] Tests (`test:`)

## Release Notes
<!--
Required for new features (feat:); recommended for user-facing fixes (fix:).
Also fill in for breaking changes and deprecations.
Leave blank for ci:/chore:/refactor:/docs:/test: PRs.
-->

## Testing
- [ ] Unit tests added/updated
- [ ] Manual testing performed

## Checklist
- [ ] PR title follows [Conventional Commits](https://www.conventionalcommits.org/) format
- [ ] Commits are signed off (`git commit -s`)
- [ ] No new warnings introduced
```

### 6. Breaking Changes

For breaking changes, use `!` after the type and add a `BREAKING CHANGE:` footer in the **squash commit body** (the GitHub merge dialog body field, not the PR description body):

```
feat!: remove legacy v1 API endpoints

BREAKING CHANGE: The /api/v1 endpoints have been removed. Migrate to /api/v2.
```

---

## Merging a Pull Request

### Always Use Squash and Merge

When merging any PR, **always choose "Squash and merge"** on GitHub. Never use "Create a merge commit" or "Rebase and merge".

Why this matters: non-squash merges create `Merge pull request #N` commits on main. These commits do not follow Conventional Commits format and break release-please, which reads commit messages to decide when and how to bump the version.

**How to squash merge:**

1. Click the dropdown arrow next to the merge button
2. Select "Squash and merge"
3. Edit the commit title to match the PR title (GitHub usually pre-fills this)
4. Add any relevant body text or `BREAKING CHANGE:` footer
5. Ensure the `Signed-off-by:` line is present in the body
6. Click "Confirm squash and merge"

### What Lands on Release Branches

After squash merging, exactly one commit lands on `main` or a `release-X.Y` maintenance branch with the message from the PR title. This is the commit release-please reads.

---

## How Releases Work

Releases are fully automated via [release-please](https://github.com/googleapis/release-please).
Maintainers should use [RELEASE.md](RELEASE.md) as the release and backport runbook.

### The Flow

1. A `feat:`, `fix:`, or `upstream:` PR is squash-merged to `main`
2. Release-please scans commits since the last release
3. It opens a `chore: release X.Y.0` PR that updates `.release-please-manifest.json` and `CHANGELOG.md`
4. The release workflow advances `VERSION` on that PR branch to the following minor development target
5. Maintainer reviews and merges the release PR (squash merge)
6. GitHub Release is created automatically from the manifest version
7. Docker images are built, signed, and pushed with the release-please output version
8. A `release-X.Y` maintenance branch is created automatically and its `VERSION` is reset to `X.Y.0`

On `main`, `VERSION` is the next development target, not the last published release. For example, after publishing `v2.16.0`, `main` immediately moves to `VERSION` `2.17.0`. `.release-please-manifest.json` remains the authoritative release-please record of published versions.

### Maintenance Branches

Patch releases are cut from `release-X.Y` branches, for example `release-2.15` produces `v2.15.1`, `v2.15.2`, and later patch releases.

Maintenance branches use release-please with patch-only versioning. Even if a backported commit is titled `feat:`, the maintenance branch still produces a patch release.

For eligible merged PRs on `main`, a maintainer can comment `/backport vX.Y` on the merged PR to cherry-pick the merge commit and open a backport PR against `release-X.Y`.

### Version Bump Rules

| Commit type | `main` bump | `release-X.Y` bump | Example |
|-------------|-------------|--------------------|---------|
| `fix:` | Minor | Patch | `main`: `2.16.0` -> `2.17.0`; `release-2.16`: `2.16.0` -> `2.16.1` |
| `upstream:` | Minor | Patch | `main`: `2.16.0` -> `2.17.0`; `release-2.16`: `2.16.0` -> `2.16.1` |
| `perf:` | Minor | Patch | `main`: `2.16.0` -> `2.17.0`; `release-2.16`: `2.16.0` -> `2.16.1` |
| `feat:` | Minor | Patch | `main`: `2.16.0` -> `2.17.0`; `release-2.16`: `2.16.0` -> `2.16.1` |
| `feat!:` / `BREAKING CHANGE:` | Minor | Patch | `main`: `2.16.0` -> `2.17.0`; `release-2.16`: `2.16.0` -> `2.16.1` |
| `ci:` / `chore:` / `docs:` / `test:` / `build:` | No release | No release | - |

### What Triggers a Release PR

Release-please only counts commits that touch files outside of these excluded paths:

- `.github/`
- `docs/`
- `tests/`

A `feat:` PR that only changes `.github/` files (e.g. a CI workflow improvement) will NOT trigger a version bump. Use `ci:` for such changes.

### CHANGELOG.md

The changelog is generated automatically from squash commit messages. `ci:`, `chore:`, `test:`, and `build:` commits are hidden from the changelog. Only `feat:`, `fix:`, `upstream:`, `perf:`, `revert:`, `refactor:`, and `docs:` appear.

### Upstream Cherry-Picks

Use `upstream:` for cherry-picked changes from `goharbor/harbor` so release-please puts them in the `Upstream` release notes section.

Add the upstream PR and author to the commit body so the release notes can show the original attribution instead of the sync bot:

```text
upstream(proxy): Preserve URL path prefix during registry auth discovery

Upstream-PR: goharbor/harbor#12345
Upstream-Author: @original-author
Signed-off-by: Your Name <your@email.com>
```

The GitHub release note will render that entry as `by @original-author in goharbor/harbor#12345`.

### Commercial Patch Commits

Commercial patches can use a simple subject instead of a conventional commit. The patch `Subject:` becomes the release-note title, and the patch body before `---` becomes the description:

```text
Subject: [PATCH] Branding customization

Allows operators to configure product branding for the portal without rebuilding
the Harbor Next image.

Supports custom names, logos, and landing page copy from deployment
configuration.
---
```

This renders in the release notes as:

```markdown
- **Branding customization**

  Allows operators to configure product branding for the portal without rebuilding
  the Harbor Next image.

  Supports custom names, logos, and landing page copy from deployment
  configuration.
```

---

## Adding Release Notes to Your PR

**New features (`feat:`) must add a `## Release Notes` section to the PR description.** It is also expected for other user-facing changes (breaking changes, deprecations). The prose appears on the GitHub Release page under a `## Highlights` section.

Fill in the `## Release Notes` section in the PR description:

```markdown
## Release Notes

Adds federated OIDC support. Configure via the new `federated_oidc` key in `harbor.yml`.
See the [OIDC documentation](https://docs.example.com/oidc) for configuration details.
```

**Rules:**

- Required for `feat:` PRs; recommended for any user-facing `fix:` PRs
- Leave it blank for `ci:`, `chore:`, `refactor:`, `docs:` PRs
- Write for your users, not for developers (explain what changed and why it matters)
- Links are fine and encouraged
- HTML comments in the section are stripped automatically

The `## Release Notes` section is extracted by the release pipeline and injected into the GitHub Release body. It does not affect `CHANGELOG.md`.

---

## Local Development Setup

Install [lefthook](https://github.com/evilmartians/lefthook) to enforce these rules locally before pushing:

```bash
lefthook install
```

Hooks enforce:
- Conventional commit message format on every commit
- DCO sign-off presence
- Spell check on staged `.md` and `.yml` files

### Common Task Commands

```bash
task dev:up           # Start dev environment with hot reload
task build            # Build all Go binaries
task test:quick       # API lint + unit tests (fast)
task test:unit        # Go unit tests with race detection
task test:lint        # golangci-lint
task images           # Build and push Docker images
task info             # Print version and build info
```

<<<<<<< HEAD
See [README.md](README.md) for full prerequisites and setup instructions.
=======
The commit message should follow the convention on [How to Write a Git Commit Message](http://chris.beams.io/posts/git-commit/). Be sure to include any related GitHub issue references in the commit message. See [GFM syntax](https://guides.github.com/features/mastering-markdown/#GitHub-flavored-markdown) for referencing issues and commits.

To help write conformant commit messages, it is recommended to set up the [git-good-commit](https://github.com/tommarshall/git-good-commit) commit hook. Run this command in the Harbor repo's root directory:

```sh
curl https://cdn.jsdelivr.net/gh/tommarshall/git-good-commit@v0.6.1/hook.sh > .git/hooks/commit-msg && chmod +x .git/hooks/commit-msg
```

### Automated Testing
Once your pull request has been opened, Harbor will run two CI pipelines against it.
1. In the travis CI, your source code will be checked via `golint`, `go vet` and `go race` that makes sure the code is readable, safe and correct. Also, all of unit tests will be triggered via `go test` against the pull request. What you need to pay attention to is the travis result and the coverage report.
* If any failure in travis, you need to figure out whether it is introduced by your commits.
* If the coverage dramatically declines, then you need to commit a unit test to cover your code.
2. In the drone CI, the E2E test will be triggered against the pull request. Also, the source code will be checked via `gosec`, and the result is stored in google storage for later analysis. The pipeline is about to build and install harbor from source code, then to run four very basic E2E tests to validate the basic functionalities of Harbor, like:
* Registry Basic Verification, to validate that the image can be pulled and pushed successfully.
* Trivy Basic Verification, to validate that the image can be scanned successfully.
* Notary Basic Verification, to validate that the image can be signed successfully.
* Ldap Basic Verification, to validate that Harbor can work in LDAP environment.

### Push and Create PR
When ready for review, push your branch to your fork repository on `github.com`:
```sh
git push --force-with-lease $user my_feature

```

Then visit your fork at https://github.com/$user/harbor and click the `Compare & Pull Request` button next to your `my_feature` branch to create a new pull request (PR). Description of a pull request should refer to all the issues that it addresses. Remember to put a reference to issues (such as `Closes #XXX` and `Fixes #XXX`) in commits so that the issues can be closed when the PR is merged.

Once your pull request has been opened it will be assigned to one or more reviewers. Those reviewers will do a thorough code review, looking for correctness, bugs, opportunities for improvement, documentation and comments, and style.

Commit changes made in response to review comments to the same branch on your fork.

## Reporting issues

It is a great way to contribute to Harbor by reporting an issue. Well-written and complete bug reports are always welcome! Please open an issue on GitHub and follow the template to fill in required information.

Before opening any issue, please look up the existing [issues](https://github.com/goharbor/harbor/issues) to avoid submitting a duplicate.
If you find a match, you can "subscribe" to it to get notified on updates. If you have additional helpful information about the issue, please leave a comment.

When reporting issues, always include:

* Version of docker engine and docker-compose
* Configuration files of Harbor
* Log files in /var/log/harbor/

Because the issues are open to the public, when submitting the log and configuration files, be sure to remove any sensitive information, e.g. user name, password, IP address, and company name. You can
replace those parts with "REDACTED" or other strings like "****".

Be sure to include the steps to reproduce the problem if applicable. It can help us understand and fix your issue faster.

## Documenting

Update the documentation if you are creating or changing features. Good documentation is as important as the code itself.

The main location for the documentation is the [website repository](https://github.com/goharbor/website). The images referred to in documents can be placed in `docs/img` in that repo.

Documents are written with Markdown. See [Writing on GitHub](https://help.github.com/categories/writing-on-github/) for more details.

## Develop and propose new features.
### The following simple process can be used to submit new features or changes to the existing code.

- See if your feature is already being worked on. Check both the [Issues](https://github.com/goharbor/harbor/issues) and the [PRs](https://github.com/goharbor/harbor/pulls) in the main Harbor repository as well as the [Community repository](https://github.com/goharbor/community).
- Submit(open PR) the new proposal at [community/proposals/new](https://github.com/goharbor/community/tree/main/proposals/new) using the already existing [template](https://github.com/goharbor/community/blob/main/proposals/TEMPLATE.md)
- The proposal must be labeled as "kind/proposal" - check examples [here](https://github.com/goharbor/community/pulls?q=is%3Apr+is%3Aopen+sort%3Aupdated-desc+label%3Akind%2Fproposal)
- The proposal can be modified and adapted to meet the requirements from the community, other maintainers and contributors. The overall architecture needs to be consistent to avoid duplicate work in the [Roadmap](https://github.com/goharbor/harbor/wiki#roadmap).
- Proposal should be discussed at Community meeting [Community Meeting agenda](https://github.com/goharbor/community/wiki/Harbor-Community-Meetings) to be presented to maintainers and contributors.
- When reviewed and approved it can be implemented either by the original submitter or anyone else from the community which we highly encourage, as the project is community driven. Open PRs in the respective repositories with all the necessary code and test changes as described in the current document.
- Once implemented or during the implementation, the PRs are reviewed by maintainers and contributors, following the best practices and methods.
- After merging the new PRs, the proposal must be moved to [community/proposals](https://github.com/goharbor/community/tree/main/proposals) and marked as done!
- You have made Harbor even better, congratulations. Thank you!

[community-meetings]: https://github.com/goharbor/community/blob/main/MEETING_SCHEDULE.md
[past-meetings]: https://www.youtube.com/playlist?list=PLgInP-D86bCwTC0DYAa1pgupsQIAWPomv
[users-slack]: https://cloud-native.slack.com/archives/CC1E09J6S
[dev-slack]: https://cloud-native.slack.com/archives/CC1E0J0MC
[cncf-slack]: https://slack.cncf.io
[users-dl]: https://lists.cncf.io/g/harbor-users
[dev-dl]: https://lists.cncf.io/g/harbor-dev
[twitter]: http://twitter.com/project_harbor
>>>>>>> 36d6e8c24 (docs: fix markdown formatting issues in CONTRIBUTING.md and README.md (#23537))
