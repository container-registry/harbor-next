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
component="${RELEASE_NOTES_COMPONENT:-}"
if [[ -n "${preview_pr_number}" && "${component}" == "chart" ]]; then
  # A chart release PR bumps the chart manifest on its branch to the new version.
  preview_version=$(node -e "const manifest = require('./.release-please-manifest-chart.json'); const version = manifest['deploy/chart']; if (!version) { throw new Error('missing chart release version'); } console.log(version);")
  TAG_NAME="chart-v${preview_version}"
elif [[ -n "${preview_pr_number}" && ! "${TAG_NAME:-}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
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

chart_mode=false
if [[ "${TAG_NAME}" =~ ^chart-v([0-9]+\.[0-9]+\.[0-9]+)$ ]]; then
  chart_mode=true
  version="${BASH_REMATCH[1]}"
elif [[ "${TAG_NAME}" =~ ^v([0-9]+\.[0-9]+\.[0-9]+)$ ]]; then
  version="${BASH_REMATCH[1]}"
else
  echo "TAG_NAME must be vX.Y.Z (app release) or chart-vX.Y.Z (chart release)" >&2
  exit 1
fi
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
chart_name="$(printf '%s\n' "${chart_yaml_source}" | awk -F'[:[:space:]]+' '$1 == "name" { gsub(/"/, "", $2); print $2; exit }')"
chart_name="${chart_name:-harbor-next}"
if [[ "${chart_mode}" == true ]]; then
  chart_version="${version}"
fi

tmp_dir=$(mktemp -d)
trap 'rm -rf "${tmp_dir}"' EXIT

changelog_path="CHANGELOG.md"
if [[ "${chart_mode}" == true ]]; then
  changelog_path="deploy/chart/CHANGELOG.md"
fi
if [[ -n "${preview_pr_number}" ]]; then
  cp "${changelog_path}" "${tmp_dir}/CHANGELOG.md"
else
  git show "${TAG_NAME}:${changelog_path}" > "${tmp_dir}/CHANGELOG.md"
fi
node .github/scripts/extract-changelog-release.mjs \
  "${tmp_dir}/CHANGELOG.md" \
  "${version}" \
  "${tmp_dir}/release-source.md"

generated_notes_args=(-f "tag_name=${TAG_NAME}")
skip_generated_notes=""
if [[ "${chart_mode}" == true ]]; then
  # Without an explicit previous tag GitHub diffs against the latest app
  # release and attributes unrelated PRs. The predecessor is the greatest
  # chart tag LOWER than the target (descending sort, line after the
  # target), so recreating an older release never diffs against a newer
  # tag; previews (target tag not created yet) fall back to the newest
  # existing chart tag.
  previous_chart_tag=$(git tag --list 'chart-v*' --sort=-v:refname \
    | awk -v t="${TAG_NAME}" 'found { print; exit } $0 == t { found = 1 }')
  if [[ -z "${previous_chart_tag}" ]]; then
    previous_chart_tag=$(git tag --list 'chart-v*' --sort=-v:refname | grep -vx "${TAG_NAME}" | head -n 1 || true)
  fi
  if [[ -n "${previous_chart_tag}" ]]; then
    generated_notes_args+=(-f "previous_tag_name=${previous_chart_tag}")
  else
    # Bootstrap chart release: no chart tag exists at all, so any range
    # GitHub picks would attribute unrelated app PRs. Skip What's Changed.
    skip_generated_notes="yes"
  fi
fi
if [[ -n "${preview_pr_number}" ]]; then
  generated_notes_args+=(-f "target_commitish=$(git rev-parse HEAD)")
fi
if [[ -n "${skip_generated_notes}" ]]; then
  : > "${tmp_dir}/generated-notes.md"
else
  github_retry gh api "repos/${GITHUB_REPOSITORY}/releases/generate-notes" \
    "${generated_notes_args[@]}" \
    --jq .body > "${tmp_dir}/generated-notes.md"
fi

node .github/scripts/format-release-notes.mjs \
  "${tmp_dir}/release-source.md" \
  "${tmp_dir}/generated-notes.md" \
  "${tmp_dir}/formatted-notes.md" \
  "${tmp_dir}/contributors.md"

node .github/scripts/extract-pr-summary.mjs \
  "${tmp_dir}/formatted-notes.md" \
  "${GITHUB_REPOSITORY}" \
  "${tmp_dir}/summary.md"

if [[ "${chart_mode}" == true ]]; then
  # Chart releases run on main only, and their target_commitish is a pinned
  # SHA rather than a branch, so the app-path lookup does not apply.
  release_branch="main"
elif [[ -n "${preview_pr_number}" ]]; then
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
stacks_file="taskfile/commercial-patch-stacks"
patch_notes="${tmp_dir}/commercial-patches.md"
prepo() { git -C "${tmp_dir}/patches-repo" "$@"; }

# The Harbor branch owns the ordered manifest. 8gcr only stores the branch
# commits, so release notes and image builds always use the same exact list.
#
# Patch Stream: this release's delta per branch = chain PRs not in the previous
# patch-record tag's "prs=" set. No patch-base ref yet -> flat tip-subject list.
if [[ "${chart_mode}" == false && -f "${series}" ]]; then
  branches=()
  while IFS= read -r branch; do
    branch="${branch%%#*}"
    branch="${branch#"${branch%%[![:space:]]*}"}"
    branch="${branch%"${branch##*[![:space:]]}"}"
    [[ -z "${branch}" ]] && continue
    if ! git check-ref-format --branch "${branch}" >/dev/null 2>&1; then
      echo "Invalid commercial branch name in series: ${branch}" >&2
      exit 1
    fi
    branches+=("${branch}")
  done < "${series}"

  base_ref="patch-base/${release_branch}"
  if prepo fetch --depth=1 "${patches_remote}" \
      "+refs/heads/${base_ref}:refs/heads/${base_ref}" 2>/dev/null; then
    # previous record: highest patch-record semver strictly below this tag
    record_ver=$( {
        prepo ls-remote "${patches_remote}" 'refs/tags/patch-record/*' \
          | awk '{ sub(/\^\{\}$/, "", $2); sub(/^refs\/tags\/patch-record\/v/, "", $2); print $2 }'
        echo "${version}"
      } | sort -u -V | awk -v c="${version}" '$0 == c { exit } { last = $0 } END { if (last != "") print last }')
    record_tag=""
    [[ -n "${record_ver}" ]] && record_tag="patch-record/v${record_ver}"
    seen_file="${tmp_dir}/record-msg.txt"
    : > "${seen_file}"
    if [[ -n "${record_tag}" ]]; then
      prepo fetch --depth=1 "${patches_remote}" \
        "+refs/tags/${record_tag}:refs/tags/${record_tag}"
      prepo tag -l --format='%(contents)' "${record_tag}" > "${seen_file}"
    fi

    unchanged=()
    for branch in "${branches[@]}"; do
      # chains only: everything newer than the pinned baseline
      prepo fetch --shallow-exclude="${base_ref}" "${patches_remote}" \
        "+refs/heads/${branch}:refs/remotes/origin/${branch}"
      excl=()
      if [[ -f "${stacks_file}" ]]; then
        stack_parent=$(awk -v b="${branch}" '{ sub(/#.*/, "") } NF == 2 && $1 == b { print $2 }' "${stacks_file}")
        [[ -n "${stack_parent}" ]] && excl=("^refs/remotes/origin/${stack_parent}")
      fi
      mapfile -t chain < <(prepo log --reverse --format=%H \
        "refs/remotes/origin/${branch}" "${excl[@]}" 2>/dev/null || true)
      if [[ "${#chain[@]}" -eq 0 ]]; then
        echo "commercial branch has no chain above ${base_ref}: ${branch}" >&2
        exit 1
      fi
      title=$(prepo log -1 --format=%s "${chain[0]}")
      seen=$(awk -v b="${branch}" '$1 == b { sub(/^prs=/, "", $3); gsub(/,/, "\n", $3); print $3 }' "${seen_file}")
      section=""
      for c in "${chain[@]}"; do
        subj=$(prepo log -1 --format=%s "${c}")
        pr=$(sed -n 's/.* (#\([0-9][0-9]*\))$/\1/p' <<< "${subj}")
        [[ -z "${pr}" ]] && continue
        grep -qx "${pr}" <<< "${seen}" && continue
        section+="- ${subj}"$'\n'
        section+=$(prepo log -1 --format=%b "${c}" | awk '
          /^## Release Notes[[:space:]]*$/ { grab=1; next }
          /^## / { grab=0 }
          grab && NF { print "  " $0 }
        ')
        section="${section%$'\n'}"$'\n'
      done
      if [[ -n "${section}" ]]; then
        {
          echo "### ${title}"
          printf '%s' "${section}"
          echo
        } >> "${patch_notes}"
      else
        unchanged+=("${title}")
      fi
    done
    if [[ "${#unchanged[@]}" -gt 0 && -s "${patch_notes}" ]]; then
      titles=$(printf '%s, ' "${unchanged[@]}")
      echo "_No changes this release:_ ${titles%, }" >> "${patch_notes}"
    fi
  else
    # legacy (pre-Patch-Stream): flat list of branch tip subjects
    for branch in "${branches[@]}"; do
      prepo fetch --depth=1 "${patches_remote}" \
        "+refs/heads/${branch}:refs/remotes/origin/${branch}"
      echo "- $(prepo log -1 --format=%s "refs/remotes/origin/${branch}")" \
        >> "${patch_notes}"
    done
  fi
fi

{
  if [[ -s "${tmp_dir}/summary.md" ]]; then
    cat "${tmp_dir}/summary.md"
    echo
  fi

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
  if [[ -n "${chart_version}" ]]; then
    echo "## Helm Chart"
    echo
    echo '```sh'
    echo "helm install harbor oci://${registry}/charts/${chart_name} \\"
    echo "  --version ${chart_version} -n harbor --create-namespace -f my-values.yaml"
    echo '```'
    echo
    if [[ "${chart_mode}" == true ]]; then
      echo "Signed with [cosign](https://github.com/sigstore/cosign). **Verify the chart signature:**"
      echo '```sh'
      echo "cosign verify \\"
      echo "  --certificate-identity \"https://github.com/${GITHUB_REPOSITORY}/.github/workflows/publish-chart.yml@refs/heads/main\" \\"
      echo '  --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \'
      echo "  ${registry}/charts/${chart_name}:${chart_version}"
      echo '```'
      echo
    fi
    echo "---"
    echo
  fi

  if [[ "${chart_mode}" == false ]]; then
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
  else
    # The chart's default image tags: the appVersion committed at the tag.
    app_version="$(printf '%s\n' "${chart_yaml_source}" | awk -F'[:[:space:]]+' '$1 == "appVersion" { gsub(/"/, "", $2); print $2; exit }')"
    if [[ -n "${app_version}" ]]; then
      echo "Default Harbor image tags follow the chart appVersion: \`${app_version}\`."
    fi
  fi

  # New Contributors spans every PR between the tags, which for the chart
  # line would credit unrelated app work — app releases only.
  if [[ "${chart_mode}" == false && -s "${tmp_dir}/contributors.md" ]]; then
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
