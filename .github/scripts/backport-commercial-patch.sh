#!/usr/bin/env bash
# Backport one commercial patch branch (from taskfile/commercial-patches on
# `main`) onto an older Harbor Next release branch's own patch series.
#
# Nothing else backports these: the `/backport vX.Y` PR-comment bot only
# cherry-picks app-repo commits, and never touches the separate 8gcr patch
# branches or a release branch's taskfile/commercial-patches file. This
# script is the missing piece for that gap.
#
# What it does:
#   1. Fetches the named main-line patch branch (e.g. 0007-multi-format-artifacts)
#      and the target release branch's existing patch series from 8gcr.
#   2. Rebases the patch's single commit onto the correct new base: the
#      target release branch's tip if it's the first patch in that series,
#      or the last already-declared release-branch patch's tip otherwise —
#      same chaining rule taskfile/release-ready.yml's _validate-patches
#      enforces (parent must be an ancestor of target, or another declared
#      patch commit).
#   3. Pushes the rebased single commit to 8gcr as
#      release-X.Y/000N-<name>.
#   4. Prints the taskfile/commercial-patches line to add on the release
#      branch — this script does NOT push app-repo changes; open that as a
#      normal PR against the release branch yourself (or pass --pr to have
#      it done for you).
#
# Usage:
#   backport-commercial-patch.sh <patch-name> <release-branch> [options]
#
#   <patch-name>      e.g. 0007-multi-format-artifacts (must exist as a
#                     branch on 8gcr, matching a line in main's
#                     taskfile/commercial-patches)
#   <release-branch>  e.g. release-2.15 (must exist in this repo and have
#                     its own taskfile/commercial-patches)
#
# Options:
#   --patches-repo <ssh-url>   8gcr remote (default: git@github.com:container-registry/8gcr.git)
#   --app-repo <owner/repo>    harbor-next repo for --pr (default: container-registry/harbor-next)
#   --app-remote <name>        local git remote for the app repo itself
#                               (default: origin; harbor-next jj workspaces
#                               typically name it `next` instead — pass
#                               --app-remote next there)
#   --pr                       also open a PR updating taskfile/commercial-patches
#                               on the release branch (requires gh CLI auth)
#   --dry-run                  do everything except push / open the PR
#
# Conflicts are NOT auto-resolved: if the rebase hits one, this script
# stops with the worktree left conflicted so you can resolve it by hand,
# same as the app-repo backport bot's documented fallback.
set -euo pipefail

PATCHES_REPO="git@github.com:container-registry/8gcr.git"
APP_REPO="container-registry/harbor-next"
APP_REMOTE="origin"
OPEN_PR=false
DRY_RUN=false

patch_name=""
release_branch=""

while [ $# -gt 0 ]; do
  case "$1" in
    --patches-repo) PATCHES_REPO="$2"; shift 2 ;;
    --app-repo) APP_REPO="$2"; shift 2 ;;
    --app-remote) APP_REMOTE="$2"; shift 2 ;;
    --pr) OPEN_PR=true; shift ;;
    --dry-run) DRY_RUN=true; shift ;;
    -h|--help) grep '^#' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *)
      if [ -z "${patch_name}" ]; then patch_name="$1";
      elif [ -z "${release_branch}" ]; then release_branch="$1";
      else echo "unexpected argument: $1" >&2; exit 1; fi
      shift ;;
  esac
done

if [ -z "${patch_name}" ] || [ -z "${release_branch}" ]; then
  echo "usage: $0 <patch-name> <release-branch> [--pr] [--dry-run]" >&2
  exit 1
fi

repo_root=$(git rev-parse --show-toplevel)
release_series="${repo_root}/taskfile/commercial-patches"
release_target_branch="${release_branch}/${patch_name}"

echo "==> fetching ${patch_name} and ${release_branch}'s patch series from 8gcr"
git fetch --no-tags "${PATCHES_REPO}" \
  "+refs/heads/${patch_name}:refs/remotes/patches-main/${patch_name}"

echo "==> checking whether ${release_target_branch} already exists on 8gcr"
if git ls-remote --exit-code --heads "${PATCHES_REPO}" "${release_target_branch}" >/dev/null 2>&1; then
  echo "error: ${release_target_branch} already exists on 8gcr — nothing to backport" >&2
  exit 1
fi

echo "==> fetching ${release_branch} (app repo) and its declared patch series"
git fetch --no-tags "${APP_REMOTE}" "${release_branch}"
release_series_content=$(git show "${APP_REMOTE}/${release_branch}:taskfile/commercial-patches" 2>/dev/null) \
  || { echo "error: taskfile/commercial-patches not found on ${release_branch}" >&2; exit 1; }

declared_branches=$(printf '%s\n' "${release_series_content}" | sed 's/#.*//; s/^[[:space:]]*//; s/[[:space:]]*$//' | grep -v '^$' || true)

new_base=""
if [ -z "${declared_branches}" ]; then
  new_base="${APP_REMOTE}/${release_branch}"
  echo "==> ${release_branch} has no declared patches yet; basing on ${new_base}"
else
  last_branch=$(printf '%s\n' "${declared_branches}" | tail -1)
  echo "==> fetching last declared patch on ${release_branch}: ${last_branch}"
  git fetch --no-tags "${PATCHES_REPO}" \
    "+refs/heads/${last_branch}:refs/remotes/patches-release/${last_branch}"
  new_base="refs/remotes/patches-release/${last_branch}"
  echo "==> basing on ${new_base} (chained after the last declared patch)"
fi

main_patch_ref="refs/remotes/patches-main/${patch_name}"
parents=$(git show -s --format='%P' "${main_patch_ref}")
if [ "$(wc -w <<<"${parents}")" -ne 1 ]; then
  echo "error: ${patch_name} is not a single non-merge commit on main — squash it first" >&2
  exit 1
fi

work_branch="backport-${patch_name}-to-${release_branch}"
if git rev-parse --verify --quiet "${work_branch}" >/dev/null; then
  echo "==> ${work_branch} already exists locally — resuming with it as-is"
  echo "    (delete it first with 'git branch -D ${work_branch}' to start over)"
  git checkout "${work_branch}"
else
  echo "==> rebasing ${patch_name} (${main_patch_ref}) onto ${new_base} as ${work_branch}"
  git checkout -B "${work_branch}" "${main_patch_ref}"
  if ! git rebase --onto "${new_base}" "${parents}" "${work_branch}"; then
    cat >&2 <<EOF

Rebase conflicted. This is expected sometimes — the target release branch's
app code may not match what this patch expects yet (a prerequisite fix may
need its own backport first, same as the app-repo bot's documented fallback).

Resolve by hand, then:
  git add <files>
  git rebase --continue

Once clean, re-run this script — it will find ${work_branch} already exists
and push it as-is instead of rebasing again. Or push it yourself:
  git push --force ${PATCHES_REPO} ${work_branch}:${release_target_branch}
EOF
    exit 1
  fi
fi

if git rev-parse --verify --quiet REBASE_HEAD >/dev/null 2>&1 || [ -d "$(git rev-parse --git-path rebase-merge 2>/dev/null)" ] || [ -d "$(git rev-parse --git-path rebase-apply 2>/dev/null)" ]; then
  echo "error: ${work_branch} still has an unfinished rebase — resolve it (git rebase --continue) before re-running" >&2
  exit 1
fi

new_commit=$(git rev-parse "${work_branch}")
echo "==> rebased commit: ${new_commit}"

if [ "${DRY_RUN}" = "true" ]; then
  echo "dry-run: would push ${work_branch} to ${PATCHES_REPO} as ${release_target_branch}"
else
  git push --force "${PATCHES_REPO}" "${work_branch}:${release_target_branch}"
  echo "==> pushed ${release_target_branch} to 8gcr"
fi

new_line="${release_target_branch}"
echo
echo "==> add this line to taskfile/commercial-patches on ${release_branch}"
echo "    (keep the existing 0001..000N ordering):"
echo
echo "    ${new_line}"
echo

if [ "${OPEN_PR}" = "true" ]; then
  if [ "${DRY_RUN}" = "true" ]; then
    echo "dry-run: would open a PR against ${release_branch} adding '${new_line}' to taskfile/commercial-patches"
  else
    pr_branch="${work_branch}-series-update"
    git checkout -B "${pr_branch}" "${APP_REMOTE}/${release_branch}"
    printf '%s\n%s\n' "${release_series_content}" "${new_line}" > "${release_series}"
    git add taskfile/commercial-patches
    git commit -s -m "chore: Backport ${patch_name} to ${release_branch}

Backports the ${patch_name} commercial patch (already on main) to
${release_branch}'s own patch series. See ${release_target_branch} on 8gcr."
    git push --force "${APP_REMOTE}" "${pr_branch}"
    gh pr create --repo "${APP_REPO}" --base "${release_branch}" --head "${pr_branch}" \
      --title "chore: Backport ${patch_name} to ${release_branch}" \
      --body "Adds \`${new_line}\` to \`taskfile/commercial-patches\` on \`${release_branch}\`.

Run \`task apply-patches\` and the full build/test suite before merging —
this script only rebases the patch commit cleanly; it does not verify the
patch still behaves correctly against ${release_branch}'s app code."
    echo "==> opened backport PR against ${release_branch}"
  fi
fi
