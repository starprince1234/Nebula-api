#!/usr/bin/env bash
set -Eeuo pipefail

readonly api_url="https://api.doppler.com/v3/configs/config/secrets"
readonly project="nebula-api"
readonly config="prd"
readonly secret_names="DEPLOY_SSH_HOST,DEPLOY_SSH_PORT,DEPLOY_SSH_USER,DEPLOY_SSH_PASSWORD,DEPLOY_SSH_KNOWN_HOSTS"
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

response_file="$(mktemp)"
trap 'rm -f -- "$response_file"' EXIT

curl \
  --fail \
  --silent \
  --show-error \
  --retry 5 \
  --retry-delay 5 \
  --connect-timeout 15 \
  --max-time 90 \
  --get "$api_url" \
  --header "Authorization: Bearer $DOPPLER_TOKEN" \
  --data-urlencode "project=$project" \
  --data-urlencode "config=$config" \
  --data-urlencode "secrets=$secret_names" \
  --output "$response_file"

mapfile -t encoded_secret_values < <(python3 - "$response_file" "${required_secrets[@]}" <<'PY'
import base64
import json
import sys

with open(sys.argv[1], encoding="utf-8") as source:
    payload = json.load(source)

secrets = payload.get("secrets")
if not isinstance(secrets, dict):
    raise SystemExit("Doppler response does not contain a secrets object")

for name in sys.argv[2:]:
    item = secrets.get(name)
    value = item.get("raw") if isinstance(item, dict) else None
    if not isinstance(value, str) or not value:
        raise SystemExit(f"Doppler response is missing deployment credential: {name}")
    if "\x00" in value:
        raise SystemExit(f"Doppler deployment credential contains a NUL byte: {name}")
    print(base64.b64encode(value.encode("utf-8")).decode("ascii"))
PY
)

if ((${#encoded_secret_values[@]} != ${#required_secrets[@]})); then
  printf 'Failed to parse all Doppler deployment credentials\n' >&2
  exit 1
fi

for index in "${!required_secrets[@]}"; do
  name="${required_secrets[$index]}"
  printf -v "$name" '%s' "$(printf '%s' "${encoded_secret_values[$index]}" | base64 --decode)"
  export "$name"
done

exec "$@"
