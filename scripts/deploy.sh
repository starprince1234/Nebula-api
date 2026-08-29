#!/usr/bin/env bash
set -Eeuo pipefail
umask 077

readonly compose_project="nebula-api"
readonly repository_root="/opt/nebula-api"
readonly deployment_pause_seconds=10

export COMPOSE_PARALLEL_LIMIT=1
export DOCKER_BUILDKIT=1

pause_between_stages() {
  printf 'Waiting %s seconds before the next deployment stage\n' "$deployment_pause_seconds"
  sleep "$deployment_pause_seconds"
}

wait_for_healthy_service() {
  local service="$1"
  local container_id
  local status

  for _ in $(seq 1 60); do
    container_id="$(docker compose --project-name "$compose_project" --file compose.production.yaml ps -q "$service")"
    if [[ -n "$container_id" ]]; then
      status="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$container_id")"
      if [[ "$status" == "healthy" || "$status" == "running" ]]; then
        return 0
      fi
      if [[ "$status" == "unhealthy" || "$status" == "exited" || "$status" == "dead" ]]; then
        printf 'Service %s entered terminal state: %s\n' "$service" "$status" >&2
        return 1
      fi
    fi
    sleep 2
  done

  printf 'Service %s did not become healthy within 120 seconds\n' "$service" >&2
  return 1
}

if [[ -d "$repository_root" ]]; then
  chmod 755 "$repository_root"
fi
if [[ "$(pwd -P)" != "$repository_root" ]]; then
  cd "$repository_root"
fi

version="$(tr -d '\r\n' <VERSION)"
if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  printf 'VERSION must contain a complete semantic version\n' >&2
  exit 1
fi

required_secrets=(
  DATABASE_URL
  REDIS_URL
  JWT_SIGNING_KEY
  AUTH_STATE_HASH_PEPPER
  API_KEY_HASH_PEPPER
  PROVIDER_CREDENTIAL_ENCRYPTION_KEY
  BOOTSTRAP_TEACHER_NAME
  BOOTSTRAP_TEACHER_EMAIL
  BOOTSTRAP_TEACHER_PASSWORD
  TEST_STUDENT_1_NAME
  TEST_STUDENT_1_EMAIL
  TEST_STUDENT_1_PASSWORD
  TEST_STUDENT_2_NAME
  TEST_STUDENT_2_EMAIL
  TEST_STUDENT_2_PASSWORD
  TEST_STUDENT_3_NAME
  TEST_STUDENT_3_EMAIL
  TEST_STUDENT_3_PASSWORD
  TEST_MENTOR_1_NAME
  TEST_MENTOR_1_EMAIL
  TEST_MENTOR_1_PASSWORD
  TEST_MENTOR_2_NAME
  TEST_MENTOR_2_EMAIL
  TEST_MENTOR_2_PASSWORD
  TEST_MENTOR_3_NAME
  TEST_MENTOR_3_EMAIL
  TEST_MENTOR_3_PASSWORD
  SMTP_HOST
  SMTP_FROM
)
for name in "${required_secrets[@]}"; do
  value="${!name:-}"
  if [[ -z "$value" || "$value" == replace_with* ]]; then
    printf 'Production Doppler config has a missing or placeholder value: %s\n' "$name" >&2
    exit 1
  fi
done

if [[ "${APP_ENV:-}" != "production" ]]; then
  printf 'APP_ENV must be production for server deployment\n' >&2
  exit 1
fi
if [[ "${HTTP_ADDRESS:-}" != ":8080" ]]; then
  printf 'HTTP_ADDRESS must be :8080 for the Compose port and health-check contract\n' >&2
  exit 1
fi

mapfile -t connection_values < <(python3 <<'PY'
import base64
import os
import sys
from urllib.parse import quote, unquote, urlsplit, urlunsplit


def fail(message: str) -> None:
    print(message, file=sys.stderr)
    raise SystemExit(1)


def clean(value: str, name: str) -> str:
    if any(character in value for character in "\r\n\0"):
        fail(f"{name} contains a control character")
    return value


for name in ("JWT_SIGNING_KEY", "AUTH_STATE_HASH_PEPPER", "API_KEY_HASH_PEPPER"):
    if len(os.environ[name].encode("utf-8")) < 32:
        fail(f"{name} must contain at least 32 UTF-8 bytes")

try:
    provider_key = base64.b64decode(
        os.environ["PROVIDER_CREDENTIAL_ENCRYPTION_KEY"], validate=True
    )
except ValueError:
    fail("PROVIDER_CREDENTIAL_ENCRYPTION_KEY must be valid Base64")
if len(provider_key) != 32:
    fail("PROVIDER_CREDENTIAL_ENCRYPTION_KEY must decode to exactly 32 bytes")

public_url = urlsplit(os.environ.get("PUBLIC_APP_URL", ""))
if not public_url.scheme or not public_url.hostname:
    fail("PUBLIC_APP_URL must be an absolute URL")
if public_url.scheme != "https" and not (
    public_url.scheme == "http" and public_url.hostname in {"localhost", "127.0.0.1", "::1"}
):
    fail("PUBLIC_APP_URL must use HTTPS unless it targets a loopback SSH tunnel")

database = urlsplit(os.environ["DATABASE_URL"])
if database.scheme not in {"postgres", "postgresql"}:
    fail("DATABASE_URL must use the postgres or postgresql scheme")
if database.hostname not in {"localhost", "127.0.0.1", "::1", "postgres"}:
    fail("DATABASE_URL must target loopback or the postgres Compose service")
if database.username is None or database.password is None:
    fail("DATABASE_URL must contain a username and password")
database_name = clean(unquote(database.path.lstrip("/")), "DATABASE_URL database name")
if not database_name or "/" in database_name:
    fail("DATABASE_URL must contain one database name")
database_user = clean(unquote(database.username), "DATABASE_URL username")
database_password = clean(unquote(database.password), "DATABASE_URL password")
database_netloc = (
    f"{quote(database_user, safe='')}:{quote(database_password, safe='')}@postgres:5432"
)
container_database_url = urlunsplit(
    (database.scheme, database_netloc, f"/{quote(database_name, safe='')}", database.query, "")
)

redis = urlsplit(os.environ["REDIS_URL"])
if redis.scheme not in {"redis", "rediss"}:
    fail("REDIS_URL must use the redis or rediss scheme")
if redis.hostname not in {"localhost", "127.0.0.1", "::1", "redis"}:
    fail("REDIS_URL must target loopback or the redis Compose service")
if redis.username is not None or redis.password is not None:
    fail("REDIS_URL credentials are unsupported by the current Compose Redis service")
container_redis_url = urlunsplit((redis.scheme, "redis:6379", redis.path, redis.query, ""))

for value in (
    container_database_url,
    database_user,
    database_password,
    database_name,
    container_redis_url,
):
    print(value)
PY
)

if ((${#connection_values[@]} != 5)); then
  printf 'Failed to derive container connection settings\n' >&2
  exit 1
fi

export DATABASE_URL="${connection_values[0]}"
export POSTGRES_USER="${connection_values[1]}"
export POSTGRES_PASSWORD="${connection_values[2]}"
export POSTGRES_DB="${connection_values[3]}"
export REDIS_URL="${connection_values[4]}"
export NEBULA_VERSION="$version"

if [[ -z "${CF_TUNNEL_TOKEN:-}" || "$CF_TUNNEL_TOKEN" == replace_with* ]]; then
  printf 'CF_TUNNEL_TOKEN is required for production deployment\n' >&2
  exit 1
fi
export TUNNEL_TOKEN="$CF_TUNNEL_TOKEN"

compose=(docker compose --project-name "$compose_project" --file compose.production.yaml)
"${compose[@]}" config --quiet
for service in postgres redis cloudflared; do
  printf 'Pulling production image for %s\n' "$service"
  "${compose[@]}" pull --quiet "$service"
  pause_between_stages
done

printf 'Building backend image with single-core Go compilation and persistent BuildKit caches\n'
"${compose[@]}" build backend
pause_between_stages

printf 'Starting postgres\n'
"${compose[@]}" up -d --no-deps --force-recreate postgres
wait_for_healthy_service postgres
pause_between_stages

printf 'Starting redis\n'
"${compose[@]}" up -d --no-deps redis
wait_for_healthy_service redis
pause_between_stages

printf 'Running database migration\n'
"${compose[@]}" up --no-build --force-recreate migrate
pause_between_stages

printf 'Starting backend\n'
"${compose[@]}" up -d --no-build --no-deps --force-recreate backend
for _ in $(seq 1 60); do
  if curl --fail --silent --show-error --noproxy '*' \
    --connect-timeout 2 --max-time 5 \
    http://127.0.0.1:8080/health/ready >/dev/null; then
    break
  fi
  sleep 2
done
curl --fail --silent --show-error --noproxy '*' \
  --connect-timeout 2 --max-time 5 \
  http://127.0.0.1:8080/health/ready >/dev/null
pause_between_stages

printf 'Starting maintenance worker\n'
"${compose[@]}" up -d --no-build --no-deps --force-recreate maintenance
wait_for_healthy_service maintenance
pause_between_stages

printf 'Starting Cloudflare Tunnel\n'
"${compose[@]}" up -d --no-build --no-deps --force-recreate cloudflared

for _ in $(seq 1 36); do
  if curl --fail --silent --show-error --noproxy '*' \
    --connect-timeout 2 --max-time 5 \
    http://127.0.0.1:8080/health/ready >/dev/null && \
    "${compose[@]}" logs --since 30s cloudflared | \
      grep -q 'Registered tunnel connection'; then
    "${compose[@]}" ps
    printf 'Nebula %s is ready\n' "$version"
    exit 0
  fi
  sleep 5
done

"${compose[@]}" ps >&2
"${compose[@]}" logs --tail 50 cloudflared >&2
printf 'Internal health or Cloudflare edge registration did not become ready within the staged deployment timeout\n' >&2
exit 1
