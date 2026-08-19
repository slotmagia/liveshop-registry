#!/usr/bin/env bash
set -Eeuo pipefail

: "${CI_PROJECT_DIR:?CI_PROJECT_DIR is required}"
: "${CI_COMMIT_SHA:?CI_COMMIT_SHA is required}"
: "${CI_COMMIT_SHORT_SHA:?CI_COMMIT_SHORT_SHA is required}"
: "${CI_PIPELINE_ID:?CI_PIPELINE_ID is required}"
: "${CI_REGISTRY:?CI_REGISTRY is required}"
: "${CI_REGISTRY_USER:?CI_REGISTRY_USER is required}"
: "${CI_REGISTRY_PASSWORD:?CI_REGISTRY_PASSWORD is required}"
: "${CI_REGISTRY_IMAGE:?CI_REGISTRY_IMAGE is required}"
: "${MODULE_NAME:?MODULE_NAME is required}"
: "${MODULE_ID:?MODULE_ID is required}"
: "${BACKEND_HOST:=127.0.0.1}"
: "${BACKEND_PORT:?BACKEND_PORT is required}"
: "${READINESS_URL:?READINESS_URL is required}"
: "${READINESS_MODE:?READINESS_MODE is required}"
: "${REGISTER_MODULE:?REGISTER_MODULE is required}"

deploy_root="/opt/liveshop/deploy/$MODULE_NAME"
releases_root="$deploy_root/releases"
release_key="$CI_COMMIT_SHA-$CI_PIPELINE_ID"
release_dir="$releases_root/$release_key"
current_link="$deploy_root/current"
workspace_root="$(cd "$(dirname "$CI_PROJECT_DIR")" && pwd -P)"
export KERNEL_ROOT="$workspace_root/kernel-go"

printf '%s' "$CI_REGISTRY_PASSWORD" | docker login "$CI_REGISTRY" --username "$CI_REGISTRY_USER" --password-stdin
trap 'docker logout "$CI_REGISTRY" >/dev/null 2>&1 || true' EXIT

install -d -m 0750 "$releases_root"
exec 9>"/opt/liveshop/deploy/.lock"
flock -x 9

previous_release=""
if [ -L "$current_link" ]; then
  previous_release="$(readlink -f "$current_link")"
fi

mkdir -p "$release_dir"
git -C "$CI_PROJECT_DIR" archive "$CI_COMMIT_SHA" | tar -x -C "$release_dir"

raw_version=""
if [ -f "$release_dir/business/module.json" ]; then
  raw_version="$(jq -r '.metadata.version' "$release_dir/business/module.json")"
fi
if [[ "$raw_version" =~ ^([0-9]+)\.([0-9]+)\.[0-9]+$ ]]; then
  release_version="${BASH_REMATCH[1]}.${BASH_REMATCH[2]}.$CI_PIPELINE_ID"
elif [ "$REGISTER_MODULE" = "false" ] && [ -z "$raw_version" ]; then
  release_version="0.0.$CI_PIPELINE_ID"
else
  printf 'Module manifest version must be strict X.Y.Z: %s\n' "$raw_version" >&2
  exit 1
fi

cat > "$release_dir/.env" <<EOF
LIVESHOP_IMAGE_PREFIX=$CI_REGISTRY_IMAGE
LIVESHOP_IMAGE_TAG=$release_key
LIVESHOP_RELEASE_VERSION=$release_version
EOF
chmod 0600 "$release_dir/.env"

wait_tcp() {
  local port="$1"
  local attempt
  for attempt in $(seq 1 120); do
    if timeout 1 bash -c "exec 3<>/dev/tcp/$BACKEND_HOST/$port" 2>/dev/null; then
      return 0
    fi
    sleep 1
  done
  return 1
}

wait_http() {
  local url="$1"
  local mode="$2"
  local attempt status
  for attempt in $(seq 1 180); do
    status="$(curl -sS -o /dev/null -w '%{http_code}' --max-time 3 "$url" || true)"
    if [ "$mode" = "success" ] && [ "$status" -ge 200 ] 2>/dev/null && [ "$status" -lt 400 ]; then
      return 0
    fi
    if [ "$mode" = "non5xx" ] && [ "$status" -ge 200 ] 2>/dev/null && [ "$status" -lt 500 ]; then
      return 0
    fi
    sleep 1
  done
  printf 'Health check failed: %s (last HTTP %s)\n' "$url" "$status" >&2
  return 1
}

activate_release() {
  local target_release="$1"
  set -a
  source "$target_release/.env"
  set +a

  docker compose     --env-file "$target_release/.env"     -f "$target_release/business/backend/deploy/compose.local.yml"     -f "$target_release/business/backend/deploy/compose.test.yml"     pull
  docker compose     --env-file "$target_release/.env"     -f "$target_release/business/backend/deploy/compose.local.yml"     -f "$target_release/business/backend/deploy/compose.test.yml"     up -d --no-build --remove-orphans

  wait_tcp "$BACKEND_PORT"

  while IFS= read -r entry; do
    [ -n "$entry" ] && wait_http "$entry" non5xx
  done < <(jq -r '.[]' <<<"$ARTIFACT_URLS")

  if [ "$REGISTER_MODULE" = "true" ]; then
    bash "$target_release/ci/register.sh" "$target_release"
  fi

  wait_http "$READINESS_URL" "$READINESS_MODE"
}

rollback() {
  local exit_code="$?"
  trap - ERR
  set +e
  printf 'Deployment failed for %s; starting application rollback.\n' "$MODULE_NAME" >&2
  if [ -n "$previous_release" ] && [ -d "$previous_release" ]; then
    if activate_release "$previous_release"; then
      printf 'Rolled back %s to %s. Database migrations remain forward-only.\n' "$MODULE_NAME" "$(basename "$previous_release")" >&2
    else
      printf 'Rollback also failed for %s; manual recovery is required.\n' "$MODULE_NAME" >&2
    fi
  else
    docker compose       --env-file "$release_dir/.env"       -f "$release_dir/business/backend/deploy/compose.local.yml"       -f "$release_dir/business/backend/deploy/compose.test.yml"       down --remove-orphans
  fi
  exit "$exit_code"
}
trap rollback ERR

if ! docker network inspect liveshop-local >/dev/null 2>&1; then
  docker network create liveshop-local >/dev/null
fi

activate_release "$release_dir"
temporary_link="$deploy_root/.current.$CI_PIPELINE_ID"
ln -s "$release_dir" "$temporary_link"
mv -Tf "$temporary_link" "$current_link"
trap - ERR

printf 'Deployed %s release %s\n' "$MODULE_NAME" "$release_key"
