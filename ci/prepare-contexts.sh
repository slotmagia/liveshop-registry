#!/usr/bin/env bash
set -Eeuo pipefail

: "${CI_PROJECT_DIR:?CI_PROJECT_DIR is required}"

workspace_root="$(cd "$(dirname "$CI_PROJECT_DIR")" && pwd -P)"

clone_repo() {
  local repository="$1"
  local target="$2"
  if [ -n "${GITHUB_ACTIONS:-}" ]; then
    : "${GITHUB_REPOSITORY_OWNER:?GITHUB_REPOSITORY_OWNER is required}"
    local token="${LIVESHOP_CLONE_TOKEN:-${GITHUB_TOKEN:?GITHUB_TOKEN or LIVESHOP_CLONE_TOKEN is required}}"
    local url
    if [ "$repository" = "kernel-go" ]; then
      url="https://github.com/lvtuopen-ai/kernel-go.git"
    else
      url="https://x-access-token:${token}@github.com/${GITHUB_REPOSITORY_OWNER}/${repository}.git"
    fi
    git clone --quiet --depth 1 "$url" "$target"
    if [ "$repository" = "kernel-go" ]; then
      git -C "$target" remote set-url origin "https://github.com/lvtuopen-ai/kernel-go.git"
    else
      git -C "$target" remote set-url origin "https://github.com/${GITHUB_REPOSITORY_OWNER}/${repository}.git"
    fi
    return 0
  fi

  : "${CI_SERVER_URL:?CI_SERVER_URL is required}"
  : "${CI_PROJECT_NAMESPACE:?CI_PROJECT_NAMESPACE is required}"
  : "${CI_JOB_TOKEN:?CI_JOB_TOKEN is required}"
  local server_without_scheme="${CI_SERVER_URL#*://}"
  local authenticated_base="${CI_SERVER_PROTOCOL}://gitlab-ci-token:${CI_JOB_TOKEN}@${server_without_scheme}"
  git clone --quiet --depth 1 "$authenticated_base/$CI_PROJECT_NAMESPACE/$repository.git" "$target"
  git -C "$target" remote set-url origin "$CI_SERVER_URL/$CI_PROJECT_NAMESPACE/$repository.git"
}

for repository in ${DEPENDENCY_REPOSITORIES:-}; do
  case "$repository" in
    kernel-go|liveshop-platform|liveshop-identity|liveshop-gateway|liveshop-catalog|liveshop-trade|liveshop-live|liveshop-protocol|liveshop-registry) ;;
    *) printf 'Unsupported dependency repository: %s\n' "$repository" >&2; exit 1 ;;
  esac

  target="$workspace_root/$repository"
  resolved_parent="$(cd "$(dirname "$target")" && pwd -P)"
  if [ "$resolved_parent" != "$workspace_root" ] || [ "$target" = "$CI_PROJECT_DIR" ]; then
    printf 'Unsafe dependency target: %s\n' "$target" >&2
    exit 1
  fi

  rm -rf -- "$target"
  clone_repo "$repository" "$target"
  printf 'Prepared dependency %s\n' "$repository"
done
