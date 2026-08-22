#!/usr/bin/env bash
set -Eeuo pipefail

readonly doppler_project="nebula-api"
readonly doppler_config="prd"
readonly -a required_secrets=(
  DEPLOY_SSH_HOST
  DEPLOY_SSH_PORT
  DEPLOY_SSH_USER
  DEPLOY_SSH_PASSWORD
  DEPLOY_SSH_KNOWN_HOSTS
)

if [[ -z "${DOPPLER_TOKEN:-}" ]]; then
  printf 'DOPPLER_TOKEN is required to retrieve deployment credentials\n' >&2
  exit 1
fi
if (($# == 0)); then
  printf 'A deployment command is required\n' >&2
  exit 1
fi

for name in "${required_secrets[@]}"; do
  value="$(doppler --attempts 5 --timeout 45s secrets get "$name" \
    --project "$doppler_project" --config "$doppler_config" --plain)"
  if [[ -z "$value" ]]; then
    printf 'Doppler returned an empty deployment credential: %s\n' "$name" >&2
    exit 1
  fi
  printf -v "$name" '%s' "$value"
  export "$name"
done

exec "$@"
