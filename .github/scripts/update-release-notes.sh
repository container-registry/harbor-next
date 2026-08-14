#!/usr/bin/env bash
set -euo pipefail

: "${GH_TOKEN:?GH_TOKEN is required}"

preview_pr_number="${RELEASE_NOTES_PREVIEW_PR_NUMBER:-}"
if [[ -n "${preview_pr_number}" && ! "${TAG_NAME:-}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  preview_version=$(node -e "const manifest = require('./.release-please-manifest.json'); const version = manifest['.']; if (!version) { throw new Error('missing root release version'); } console.log(version);")
  TAG_NAME="v${preview_version}"
fi
: "${TAG_NAME:?TAG_NAME is required}"

PATCHES_TOKEN="${PATCHES_TOKEN:-${GH_TOKEN}}"
if [[ -z "${GITHUB_REPOSITORY:-}" ]]; then
  GITHUB_REPOSITORY=$(git remote get-url next 2>/dev/null \
    | sed -E 's#^(git@github\.com:|https://github\.com/)##; s#\.git$##' || true)
  GITHUB_REPOSITORY="${GITHUB_REPOSITORY:-$(gh repo view --json nameWithOwner --jq .nameWithOwner)}"
fi

if [[ ! "${TAG_NAME}" =~ ^v([0-9]+\.[0-9]+\.[0-9]+)$ ]]; then
  echo "TAG_NAME must be a semantic version tag such as v2.15.4" >&2
  exit 1
fi

version="${BASH_REMATCH[1]}"
registry_address="${REGISTRY_ADDRESS:-8gears.container-registry.com}"
registry_project="${REGISTRY_PROJECT:-8gcr}"
registry="${registry_address}/${registry_project}"
dry_run="${RELEASE_NOTES_DRY_RUN:-false}"
release_notes_output="${RELEASE_NOTES_OUTPUT:-}"
images=(core jobservice registryctl exporter portal registry trivy-adapter)

tmp_dir=$(mktemp -d)
trap 'rm -rf "${tmp_dir}"' EXIT

if [[ -n "${preview_pr_number}" ]]; then
  cp CHANGELOG.md "${tmp_dir}/CHANGELOG.md"
else
  git show "${TAG_NAME}:CHANGELOG.md" > "${tmp_dir}/CHANGELOG.md"
fi
node .github/scripts/extract-changelog-release.mjs \
  "${tmp_dir}/CHANGELOG.md" \
  "${version}" \
  "${tmp_dir}/release-source.md"

generated_notes_args=(-f "tag_name=${TAG_NAME}")
if [[ -n "${preview_pr_number}" ]]; then
  generated_notes_args+=(-f "target_commitish=$(git rev-parse HEAD)")
fi
gh api "repos/${GITHUB_REPOSITORY}/releases/generate-notes" \
  "${generated_notes_args[@]}" \
  --jq .body > "${tmp_dir}/generated-notes.md"

node .github/scripts/format-release-notes.mjs \
  "${tmp_dir}/release-source.md" \
  "${tmp_dir}/generated-notes.md" \
  "${tmp_dir}/formatted-notes.md" \
  "${tmp_dir}/contributors.md"

if [[ -n "${preview_pr_number}" ]]; then
  release_branch="${GITHUB_REF_NAME:?GITHUB_REF_NAME is required for a release PR preview}"
else
  release_branch=$(gh release view "${TAG_NAME}" \
    --repo "${GITHUB_REPOSITORY}" \
    --json targetCommitish \
    --jq .targetCommitish)
fi

if [[ -z "${release_branch}" ]]; then
  echo "Release ${TAG_NAME} has no target branch" >&2
  exit 1
fi

# `gh repo clone` shells out to git without reliably propagating GH_TOKEN as
# a credential for the private container-registry/8gcr repo in this
# non-interactive context ("could not read Username for 'https://github.com'").
# Use a plain `git clone` instead, but via a one-shot GIT_ASKPASS helper
# rather than embedding the token in the URL — an embedded token is written
# into the clone's `.git/config` and shows up in the process list (`ps`)
# for the duration of the clone. The askpass script only ever reads the
# token from its own environment, never as a command-line argument or URL.
askpass_script="${tmp_dir}/git-askpass.sh"
cat > "${askpass_script}" <<'EOF'
#!/usr/bin/env bash
echo "${PATCHES_TOKEN}"
EOF
chmod 700 "${askpass_script}"

GIT_ASKPASS="${askpass_script}" PATCHES_TOKEN="${PATCHES_TOKEN}" git clone --depth=1 --branch main \
  "https://x-access-token@github.com/container-registry/8gcr" \
  "${tmp_dir}/patches-repo"

series="${tmp_dir}/patches-repo/8gcr-ee/patches/series"
patch_notes="${tmp_dir}/commercial-patches.md"

# 8gcr-ee/patches/series is now a manifest of commercial jj/git branch names
# (see ADR-0006 in container-registry/8gcr), not a StGit patch-file series.
# None of the branches carry a commit body, only a one-line subject, so a
# direct `git log --format=%s` read replaces the old patch-file parsing.
if [[ -f "${series}" ]]; then
  while IFS= read -r branch; do
    branch="${branch%%#*}"
    branch="${branch#"${branch%%[![:space:]]*}"}"
    branch="${branch%"${branch##*[![:space:]]}"}"
    [[ -z "${branch}" ]] && continue

    if [[ "${branch}" == */* || "${branch}" == *..* ]]; then
      echo "Invalid commercial branch name in series: ${branch}" >&2
      exit 1
    fi

    git -C "${tmp_dir}/patches-repo" fetch --depth=1 origin \
      "${branch}:refs/remotes/origin/${branch}"
    echo "- $(git -C "${tmp_dir}/patches-repo" log -1 --format=%s "refs/remotes/origin/${branch}")" \
      >> "${patch_notes}"
  done < "${series}"
fi

{
  if [[ -s "${patch_notes}" ]]; then
    echo "## Commercial Features"
    echo
    echo "This release includes the following commercial enhancements:"
    echo
    cat "${patch_notes}"
    echo
  fi

  cat "${tmp_dir}/formatted-notes.md"
  echo
  echo "---"
  echo
  echo "## Container Images"
  echo
  echo "Multi-arch images (\`linux/amd64\`, \`linux/arm64\`) signed with [cosign](https://github.com/sigstore/cosign)."
  echo
  echo "| Image | Reference |"
  echo "|-------|-----------|"

  for image in "${images[@]}"; do
    image_name="harbor-${image}"
    [[ "${image}" == "trivy-adapter" ]] && image_name="trivy-adapter"
    echo "| \`${image_name}\` | \`${registry}/${image_name}:${TAG_NAME}\` |"
  done

  echo
  echo "**Verify an image signature:**"
  echo '```sh'
  echo "cosign verify \\"
  echo "  --certificate-identity \"https://github.com/${GITHUB_REPOSITORY}/.github/workflows/publish-images.yml@refs/heads/${release_branch}\" \\"
  echo '  --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \'
  echo "  ${registry}/harbor-core:${TAG_NAME}"
  echo '```'

  if [[ -s "${tmp_dir}/contributors.md" ]]; then
    echo
    echo "---"
    echo
    cat "${tmp_dir}/contributors.md"
  fi
} > "${tmp_dir}/release-notes.md"

if [[ -n "${preview_pr_number}" ]]; then
  gh pr view "${preview_pr_number}" --repo "${GITHUB_REPOSITORY}" --json body --jq .body > "${tmp_dir}/release-pr-body.md"
  node .github/scripts/update-release-notes-preview.mjs \
    "${tmp_dir}/release-pr-body.md" \
    "${tmp_dir}/release-notes.md" \
    "${tmp_dir}/release-pr-body-with-preview.md"
  gh pr edit "${preview_pr_number}" \
    --repo "${GITHUB_REPOSITORY}" \
    --body-file "${tmp_dir}/release-pr-body-with-preview.md"
  exit 0
elif [[ "${dry_run}" == "true" ]]; then
  if [[ -n "${release_notes_output}" ]]; then
    cp "${tmp_dir}/release-notes.md" "${release_notes_output}"
    echo "Wrote release notes to ${release_notes_output}"
  else
    cat "${tmp_dir}/release-notes.md"
  fi
  exit 0
fi

gh release edit "${TAG_NAME}" \
  --repo "${GITHUB_REPOSITORY}" \
  --notes-file "${tmp_dir}/release-notes.md"
