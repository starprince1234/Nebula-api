#!/usr/bin/env bash
set -Eeuo pipefail
umask 077

readonly api_url="https://api.doppler.com/v3/configs/config/secrets"
readonly project="nebula-api"
readonly config="prd"
readonly -a requested_secrets=(
  APP_ENV
  HTTP_ADDRESS
  PUBLIC_APP_URL
  DATABASE_URL
  REDIS_URL
  JWT_SIGNING_KEY
  ACCESS_TOKEN_TTL_MINUTES
  REFRESH_TOKEN_TTL_HOURS
  AUTH_STATE_HASH_PEPPER
  API_KEY_HASH_PEPPER
  PROVIDER_CREDENTIAL_ENCRYPTION_KEY
  BOOTSTRAP_TEACHER_NAME
  BOOTSTRAP_TEACHER_EMAIL
  BOOTSTRAP_TEACHER_PASSWORD
  SMTP_HOST
  SMTP_PORT
  SMTP_USER
  SMTP_PASS
  SMTP_FROM
  SMTP_FROM_NAME
  SMTP_TLS_MODE
  SMTP_TIMEOUT_SECONDS
  EMAIL_VERIFICATION_CODE_TTL_MINUTES
  EMAIL_VERIFICATION_SEND_COOLDOWN_SECONDS
  EMAIL_VERIFICATION_MAX_ATTEMPTS
  EMAIL_VERIFICATION_LOCKOUT_MINUTES
  TEACHER_INVITATION_TTL_HOURS
  SSE_STREAM_MAX_LENGTH
  SSE_HEARTBEAT_SECONDS
  UPSTREAM_CONNECT_TIMEOUT_SECONDS
  UPSTREAM_RESPONSE_HEADER_TIMEOUT_SECONDS
  GATEWAY_MAX_REQUEST_BYTES
  VIDEO_TASK_ROUTE_TTL_HOURS
  CF_TUNNEL_TOKEN
)

if [[ -z "${DOPPLER_TOKEN:-}" ]]; then
  printf 'DOPPLER_TOKEN is required to retrieve production configuration\n' >&2
  exit 1
fi
if (($# == 0)); then
  printf 'A deployment command is required\n' >&2
  exit 1
fi

response_file="$(mktemp)"
encoded_file="$(mktemp)"
trap 'rm -f -- "$response_file" "$encoded_file"' EXIT
secret_names="$(IFS=,; printf '%s' "${requested_secrets[*]}")"

curl \
  --fail \
  --silent \
  --show-error \
  --retry 5 \
  --retry-all-errors \
  --retry-delay 5 \
  --connect-timeout 15 \
  --max-time 90 \
  --get "$api_url" \
  --header "Authorization: Bearer $DOPPLER_TOKEN" \
  --data-urlencode "project=$project" \
  --data-urlencode "config=$config" \
  --data-urlencode "secrets=$secret_names" \
  --output "$response_file"

python3 - "$response_file" "${requested_secrets[@]}" >"$encoded_file" <<'PY'
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
    if value is None:
        continue
    if not isinstance(value, str) or "\x00" in value:
        raise SystemExit(f"Doppler response has an invalid production configuration value: {name}")
    encoded = base64.b64encode(value.encode("utf-8")).decode("ascii")
    print(f"{name}\tb64:{encoded}")
PY

while IFS=$'\t' read -r name encoded_value; do
  if [[ ! "$name" =~ ^[A-Z][A-Z0-9_]*$ ]] || [[ "$encoded_value" != b64:* ]]; then
    printf 'Doppler response contains an invalid production configuration entry\n' >&2
    exit 1
  fi
  printf -v "$name" '%s' "$(printf '%s' "${encoded_value#b64:}" | base64 --decode)"
  export "$name"
done <"$encoded_file"

exec "$@"
