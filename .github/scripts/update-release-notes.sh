#!/usr/bin/env bash
set -euo pipefail

: "${GH_TOKEN:?GH_TOKEN is required}"
: "${PATCHES_TOKEN:?PATCHES_TOKEN is required}"
: "${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is required}"
: "${TAG_NAME:?TAG_NAME is required}"

if [[ ! "${TAG_NAME}" =~ ^v([0-9]+\.[0-9]+\.[0-9]+)$ ]]; then
  echo "TAG_NAME must be a semantic version tag such as v2.15.4" >&2
  exit 1
fi

version="${BASH_REMATCH[1]}"
registry_address="${REGISTRY_ADDRESS:-8gears.container-registry.com}"
registry_project="${REGISTRY_PROJECT:-8gcr}"
registry="${registry_address}/${registry_project}"
dry_run="${RELEASE_NOTES_DRY_RUN:-false}"
images=(core jobservice registryctl exporter portal registry trivy-adapter)

tmp_dir=$(mktemp -d)
trap 'rm -rf "${tmp_dir}"' EXIT

git show "${TAG_NAME}:CHANGELOG.md" > "${tmp_dir}/CHANGELOG.md"
node .github/scripts/extract-changelog-release.mjs \
  "${tmp_dir}/CHANGELOG.md" \
  "${version}" \
  "${tmp_dir}/release-source.md"

gh api "repos/${GITHUB_REPOSITORY}/releases/generate-notes" \
  -f "tag_name=${TAG_NAME}" \
  --jq .body > "${tmp_dir}/generated-notes.md"

node .github/scripts/format-release-notes.mjs \
  "${tmp_dir}/release-source.md" \
  "${tmp_dir}/generated-notes.md" \
  "${tmp_dir}/formatted-notes.md" \
  "${tmp_dir}/contributors.md"

release_branch=$(gh release view "${TAG_NAME}" \
  --repo "${GITHUB_REPOSITORY}" \
  --json targetCommitish \
  --jq .targetCommitish)
release_created_at=$(gh release view "${TAG_NAME}" \
  --repo "${GITHUB_REPOSITORY}" \
  --json createdAt \
  --jq .createdAt)

if [[ -z "${release_branch}" ]]; then
  echo "Release ${TAG_NAME} has no target branch" >&2
  exit 1
fi

previous_tag=$(git tag --merged "${TAG_NAME}" --sort=-version:refname \
  | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' \
  | grep -v "^${TAG_NAME}$" \
  | head -n 1 || true)
highlights=""

if [[ -n "${previous_tag}" ]]; then
  previous_release_created_at=$(gh release view "${previous_tag}" \
    --repo "${GITHUB_REPOSITORY}" \
    --json createdAt \
    --jq .createdAt)
  previous_release_date="${previous_release_created_at%%T*}"
  release_date="${release_created_at%%T*}"
  highlights=$(gh pr list \
    --repo "${GITHUB_REPOSITORY}" \
    --state merged \
    --base "${release_branch}" \
    --search "merged:${previous_release_date}..${release_date}" \
    --json number,title,url,body \
    --limit 100 \
    --jq '
      [.[] | select(.body | test("(?m)^## Release Notes"))] |
      if length == 0 then ""
      else
        "## Highlights\n\n" +
        (map(
          "### [#\(.number)](\(.url)) \(.title)\n" +
          (.body |
            gsub("(?s).*\n## Release Notes\n"; "") |
            gsub("(?s)\n## .*"; "") |
            gsub("<!--[\\s\\S]*?-->"; "") |
            ltrimstr("\n") | rtrimstr("\n")
          ) + "\n"
        ) | join("\n"))
      end
    ')
fi

GH_TOKEN="${PATCHES_TOKEN}" gh repo clone container-registry/8gcr \
  "${tmp_dir}/patches-repo" \
  -- --depth=1 --branch main

series="${tmp_dir}/patches-repo/8gcr-ee/patches/series"
patch_notes="${tmp_dir}/commercial-patches.md"

if [[ -f "${series}" ]]; then
  while IFS= read -r patch; do
    patch="${patch%%#*}"
    patch="${patch#"${patch%%[![:space:]]*}"}"
    patch="${patch%"${patch##*[![:space:]]}"}"
    [[ -z "${patch}" ]] && continue

    if [[ "${patch}" == */* || "${patch}" == *..* ]]; then
      echo "Invalid commercial patch name in series: ${patch}" >&2
      exit 1
    fi

    node .github/scripts/format-commercial-patch.mjs \
      "${tmp_dir}/patches-repo/8gcr-ee/patches/${patch}" >> "${patch_notes}"
  done < "${series}"
fi

{
  if [[ -n "${highlights}" ]]; then
    printf '%s\n\n' "${highlights}"
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
  echo '\`\`\`sh'
  echo "cosign verify \\"
  echo "  --certificate-identity \"https://github.com/${GITHUB_REPOSITORY}/.github/workflows/release-please.yml@refs/heads/${release_branch}\" \\"
  echo '  --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \\'
  echo "  ${registry}/harbor-core:${TAG_NAME}"
  echo '\`\`\`'

  if [[ -s "${tmp_dir}/contributors.md" ]]; then
    echo
    echo "---"
    echo
    cat "${tmp_dir}/contributors.md"
  fi
} > "${tmp_dir}/release-notes.md"

if [[ "${dry_run}" == "true" ]]; then
  cat "${tmp_dir}/release-notes.md"
  exit 0
fi

gh release edit "${TAG_NAME}" \
  --repo "${GITHUB_REPOSITORY}" \
  --notes-file "${tmp_dir}/release-notes.md"
