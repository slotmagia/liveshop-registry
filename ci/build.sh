#!/usr/bin/env bash
set -Eeuo pipefail
bash ci/prepare-contexts.sh
workspace_root="$(cd "$(dirname "$CI_PROJECT_DIR")" && pwd -P)"
tag="$CI_COMMIT_SHA-$CI_PIPELINE_ID"
trap 'docker logout "$CI_REGISTRY" >/dev/null 2>&1 || true' EXIT
printf '%s' "$CI_REGISTRY_PASSWORD" | docker login "$CI_REGISTRY" --username "$CI_REGISTRY_USER" --password-stdin
docker buildx build --pull --push   --build-context "kernel=$workspace_root/kernel-go"   --build-context "protocol=$workspace_root/liveshop-protocol/platform"   --file "$CI_PROJECT_DIR/business/backend/deploy/Dockerfile"   --tag "$CI_REGISTRY_IMAGE/backend:$tag"   "$CI_PROJECT_DIR/business/backend"
