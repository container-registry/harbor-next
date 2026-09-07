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

# The Helm chart releases on its own release-please line, so its version is
# unrelated to TAG_NAME. Read whatever chart version is committed at the app
# release ref: that is the newest chart published at the time of this release.
if [[ -n "${preview_pr_number}" ]]; then
  chart_yaml_source="$(cat deploy/chart/Chart.yaml 2>/dev/null || true)"
else
  chart_yaml_source="$(git show "${TAG_NAME}:deploy/chart/Chart.yaml" 2>/dev/null || true)"
fi
chart_version="$(printf '%s\n' "${chart_yaml_source}" | awk -F'[:[:space:]]+' '$1 == "version" { gsub(/"/, "", $2); print $2; exit }')"

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

node .github/scripts/extract-pr-summary.mjs \
  "${tmp_dir}/formatted-notes.md" \
  "${GITHUB_REPOSITORY}" \
  "${tmp_dir}/summary.md"

if [[ -n "${preview_pr_number}" ]]; then
  release_branch="${GITHUB_REF_NAME:?GITHUB_REF_NAME is required for a release PR preview}"
elif [[ "${GITHUB_REF_TYPE:-}" == "branch" && -n "${GITHUB_REF_NAME:-}" ]]; then
  # push-triggered path only. Tag-triggered runs (manual recreate on a
  # tag ref) have GITHUB_REF_NAME=<tag>, not a branch, so fall through.
  release_branch="${GITHUB_REF_NAME}"
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
# Each patch commit's message carries an append-only changelog ledger (see
# 8gcr's COMMERCIAL-PATCH-FLOW.md): entries after the last release marker
# are this release's delta; when re-rendering an old tag, the window before
# that tag's marker is used instead.
commercial_count=0
unchanged_features=()
if [[ -f "${series}" ]]; then
  while IFS= read -r branch; do
    branch="${branch%%#*}"
    branch="${branch#"${branch%%[![:space:]]*}"}"
    branch="${branch%"${branch##*[![:space:]]}"}"
    [[ -z "${branch}" ]] && continue

    if ! git check-ref-format --branch "${branch}" >/dev/null 2>&1; then
      echo "Invalid commercial branch name in series: ${branch}" >&2
      exit 1
    fi

    git -C "${tmp_dir}/patches-repo" fetch --depth=1 "${patches_remote}" \
      "${branch}:refs/remotes/origin/${branch}"
    commercial_count=$((commercial_count + 1))
    feature_title=$(git -C "${tmp_dir}/patches-repo" log -1 --format=%s "refs/remotes/origin/${branch}")
    # No early exit inside the awk: the whole ledger is always consumed, so
    # a long message can never SIGPIPE `git log` under pipefail.
    feature_entries=$(git -C "${tmp_dir}/patches-repo" log -1 --format=%B "refs/remotes/origin/${branch}" \
      | awk -v tag="${TAG_NAME}" '
          function newer(a, b,   x, y, i) {
            sub(/^v/, "", a); sub(/^v/, "", b)
            split(a, x, "."); split(b, y, ".")
            for (i = 1; i <= 3; i++) {
              if (x[i] + 0 > y[i] + 0) return 1
              if (x[i] + 0 < y[i] + 0) return 0
            }
            return 0
          }
          /^Changelog:$/ { inledger = 1; buf = ""; next }
          !inledger { next }
          /^--- release / {
            if ($3 == tag) { found = 1; out = buf }
            if (newer($3, tag)) sawnewer = 1
            buf = ""; next
          }
          /^- / || /^  / { buf = buf $0 "\n" }
          END {
            if (found) { printf "%s", out }
            # No marker for the tag: the trailing entries are the unreleased
            # delta — correct for the current release (notes render before
            # the marker is stamped), but a marker NEWER than the tag means
            # this is a historical re-render of a pre-ledger tag: no entries.
            else if (!sawnewer) { printf "%s", buf }
          }
        ')
    if [[ -n "${feature_entries}" ]]; then
      {
        echo "### ${feature_title}"
        echo
        printf '%s' "${feature_entries}"
        echo
      } >> "${patch_notes}"
    else
      unchanged_features+=("${feature_title}")
    fi
  done < "${series}"
fi

{
  if [[ -s "${tmp_dir}/summary.md" ]]; then
    cat "${tmp_dir}/summary.md"
    echo
  fi

  if [[ "${commercial_count}" -gt 0 ]]; then
    echo "## Commercial Features"
    echo
    if [[ -s "${patch_notes}" ]]; then
      echo "Changes to commercial features in this release:"
      echo
      cat "${patch_notes}"
    fi
    if [[ "${#unchanged_features[@]}" -gt 0 ]]; then
      printf -v unchanged_list '%s, ' "${unchanged_features[@]}"
      echo "_No changes this release: ${unchanged_list%, }._"
      echo
    fi
  fi

  cat "${tmp_dir}/formatted-notes.md"
  echo
  echo "---"
  echo
  if [[ -n "${chart_version}" ]]; then
    echo "## Helm Chart"
    echo
    echo '```sh'
    echo "helm install harbor oci://${registry}/charts/harbor-next \\"
    echo "  --version ${chart_version} -n harbor --create-namespace -f my-values.yaml"
    echo '```'
    echo
    echo "---"
    echo
  fi

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

if [[ -n "${preview_pr_number}" && "${dry_run}" != "true" ]]; then
  node .github/scripts/update-release-notes-preview.mjs \
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
