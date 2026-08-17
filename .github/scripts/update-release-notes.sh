#!/usr/bin/env bash
set -euo pipefail

: "${GH_TOKEN:?GH_TOKEN is required}"

github_retry() {
  local attempt
  for attempt in 1 2 3 4 5; do
    if "$@"; then
      return 0
    fi
    if [[ "${attempt}" -eq 5 ]]; then
      return 1
    fi
    echo "GitHub request failed (attempt ${attempt}/5); retrying..." >&2
    sleep $((attempt * 2))
  done
}

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
github_retry gh api "repos/${GITHUB_REPOSITORY}/releases/generate-notes" \
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
  release_branch=$(github_retry gh release view "${TAG_NAME}" \
    --repo "${GITHUB_REPOSITORY}" \
    --json targetCommitish \
    --jq .targetCommitish)
fi

if [[ -z "${release_branch}" ]]; then
  echo "Release ${TAG_NAME} has no target branch" >&2
  exit 1
fi

# Fetch only the branches declared by this Harbor branch. The token remains
# in the environment and never appears in a URL, process argument, or Git
# config file.
askpass_script="${tmp_dir}/git-askpass.sh"
cat > "${askpass_script}" <<'EOF'
#!/usr/bin/env bash
case "$1" in
  *Username*) printf '%s\n' 'x-access-token' ;;
  *Password*) printf '%s\n' "${PATCHES_TOKEN}" ;;
  *) exit 1 ;;
esac
EOF
chmod 700 "${askpass_script}"
export GIT_ASKPASS="${askpass_script}" GIT_TERMINAL_PROMPT=0 PATCHES_TOKEN

git init --bare "${tmp_dir}/patches-repo"
patches_remote="https://x-access-token@github.com/container-registry/8gcr"
series="taskfile/commercial-patches"
patch_notes="${tmp_dir}/commercial-patches.md"

# The Harbor branch owns the ordered manifest. 8gcr only stores the branch
# commits, so release notes and image builds always use the same exact list.
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

    git -C "${tmp_dir}/patches-repo" fetch --depth=1 "${patches_remote}" \
      "${branch}:refs/remotes/origin/${branch}"
    echo "- $(git -C "${tmp_dir}/patches-repo" log -1 --format=%s "refs/remotes/origin/${branch}")" \
      >> "${patch_notes}"

    if git -C "${tmp_dir}/patches-repo" cat-file -e \
      "refs/remotes/origin/${branch}:dockerfile/grype-scanner.dockerfile" 2>/dev/null; then
      images+=(grype-scanner snyk-scanner)
    fi
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
    [[ "${image}" == "grype-scanner" ]] && image_name="harbor-grype-adapter"
    [[ "${image}" == "snyk-scanner" ]] && image_name="harbor-snyk-adapter"
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

if [[ -n "${preview_pr_number}" && "${dry_run}" != "true" ]]; then
  github_retry gh pr view "${preview_pr_number}" --repo "${GITHUB_REPOSITORY}" --json body --jq .body > "${tmp_dir}/release-pr-body.md"
  node .github/scripts/update-release-notes-preview.mjs \
    "${tmp_dir}/release-pr-body.md" \
    "${tmp_dir}/release-notes.md" \
    "${tmp_dir}/release-pr-body-with-preview.md"
  github_retry gh pr edit "${preview_pr_number}" \
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

github_retry gh release edit "${TAG_NAME}" \
  --repo "${GITHUB_REPOSITORY}" \
  --notes-file "${tmp_dir}/release-notes.md"
