#!/usr/bin/env bash
set -Eeuo pipefail

readonly remote_root="/opt/nebula-api"

for name in \
  DOPPLER_TOKEN \
  NEBULA_IMAGE \
  NEBULA_VERSION \
  DEPLOY_SSH_HOST \
  DEPLOY_SSH_PORT \
  DEPLOY_SSH_USER \
  DEPLOY_SSH_PASSWORD \
  DEPLOY_SSH_KNOWN_HOSTS; do
  if [[ -z "${!name:-}" ]]; then
    printf 'Required deployment secret is missing: %s\n' "$name" >&2
    exit 1
  fi
done

if [[ ! "$DEPLOY_SSH_HOST" =~ ^[A-Za-z0-9._:-]+$ ]]; then
  printf 'DEPLOY_SSH_HOST contains unsupported characters\n' >&2
  exit 1
fi
if [[ ! "$DEPLOY_SSH_PORT" =~ ^[0-9]+$ ]] || ((DEPLOY_SSH_PORT < 1 || DEPLOY_SSH_PORT > 65535)); then
  printf 'DEPLOY_SSH_PORT must be an integer from 1 to 65535\n' >&2
  exit 1
fi
if [[ ! "$DEPLOY_SSH_USER" =~ ^[a-z_][a-z0-9_-]*$ ]]; then
  printf 'DEPLOY_SSH_USER is not a valid Linux username\n' >&2
  exit 1
fi
if [[ ! "$NEBULA_VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  printf 'NEBULA_VERSION must contain a complete semantic version\n' >&2
  exit 1
fi
if [[ ! "$NEBULA_IMAGE" =~ ^ghcr\.io/starprince1234/nebula-api@sha256:[a-f0-9]{64}$ ]]; then
  printf 'NEBULA_IMAGE must be the immutable Nebula production image digest\n' >&2
  exit 1
fi

temporary_directory="$(mktemp -d)"
trap 'rm -rf -- "$temporary_directory"' EXIT
known_hosts_file="$temporary_directory/known_hosts"
printf '%s\n' "$DEPLOY_SSH_KNOWN_HOSTS" >"$known_hosts_file"
chmod 600 "$known_hosts_file"

export SSHPASS="$DEPLOY_SSH_PASSWORD"
ssh_options=(
  -p "$DEPLOY_SSH_PORT"
  -o BatchMode=no
  -o ConnectTimeout=15
  -o ServerAliveInterval=30
  -o ServerAliveCountMax=20
  -o IdentitiesOnly=yes
  -o LogLevel=ERROR
  -o PreferredAuthentications=password
  -o PubkeyAuthentication=no
  -o StrictHostKeyChecking=yes
  -o "UserKnownHostsFile=$known_hosts_file"
)
remote="$DEPLOY_SSH_USER@$DEPLOY_SSH_HOST"

sshpass -e ssh "${ssh_options[@]}" "$remote" "mkdir -p '$remote_root'"
tar -czf - compose.production.yaml scripts/deploy.sh | \
  sshpass -e ssh "${ssh_options[@]}" "$remote" \
    "mkdir -p '$remote_root' && tar -xzf - -C '$remote_root'"

token_b64="$(printf '%s' "$DOPPLER_TOKEN" | base64 -w 0)"
image_b64="$(printf '%s' "$NEBULA_IMAGE" | base64 -w 0)"
version_b64="$(printf '%s' "$NEBULA_VERSION" | base64 -w 0)"
printf '%s\n%s\n%s\n' \
  "$token_b64" \
  "$image_b64" \
  "$version_b64" | \
  sshpass -e ssh "${ssh_options[@]}" "$remote" '
    IFS= read -r DOPPLER_TOKEN_B64
    IFS= read -r NEBULA_IMAGE_B64
    IFS= read -r NEBULA_VERSION_B64
    export DOPPLER_TOKEN="$(printf "%s" "$DOPPLER_TOKEN_B64" | base64 -d)"
    export NEBULA_IMAGE="$(printf "%s" "$NEBULA_IMAGE_B64" | base64 -d)"
    export NEBULA_VERSION="$(printf "%s" "$NEBULA_VERSION_B64" | base64 -d)"
    exec doppler run --project nebula-api --config prd --no-fallback -- bash "'"$remote_root"'/scripts/deploy.sh"
  '
