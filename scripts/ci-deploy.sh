#!/usr/bin/env bash
set -Eeuo pipefail

readonly remote_root="/opt/nebula-api"

for name in \
  DOPPLER_TOKEN \
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
  -o IdentitiesOnly=yes
  -o LogLevel=ERROR
  -o PreferredAuthentications=password
  -o PubkeyAuthentication=no
  -o StrictHostKeyChecking=yes
  -o "UserKnownHostsFile=$known_hosts_file"
)
remote="$DEPLOY_SSH_USER@$DEPLOY_SSH_HOST"

sshpass -e ssh "${ssh_options[@]}" "$remote" "mkdir -p '$remote_root'"
tar \
  --exclude='./.git' \
  --exclude='./frontend/node_modules' \
  --exclude='./frontend/dist' \
  --exclude='./.playwright-mcp' \
  -czf - . | \
  sshpass -e ssh "${ssh_options[@]}" "$remote" \
    "mkdir -p '$remote_root' && tar -xzf - -C '$remote_root'"

token_b64="$(printf '%s' "$DOPPLER_TOKEN" | base64 -w 0)"
printf '%s\n' "$token_b64" | sshpass -e ssh "${ssh_options[@]}" "$remote" \
  "IFS= read -r DOPPLER_TOKEN_B64 && export DOPPLER_TOKEN=\$(printf '%s' \"\$DOPPLER_TOKEN_B64\" | base64 -d) && exec doppler run --project nebula-api --config prd --no-fallback -- bash '$remote_root/scripts/deploy.sh'"
