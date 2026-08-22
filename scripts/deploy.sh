#!/usr/bin/env bash
set -Eeuo pipefail
umask 077

readonly compose_project="nebula-api"
readonly repository_root="/opt/nebula-api"

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

if [[ ! -c /dev/net/tun ]]; then
  printf '/dev/net/tun is required for the production Mihomo TUN\n' >&2
  exit 1
fi
if ! python3 -c 'import yaml' >/dev/null 2>&1; then
  printf 'Python PyYAML is required to configure the production Mihomo TUN\n' >&2
  exit 1
fi
python3 scripts/configure-production-mihomo.py
docker run --rm \
  --volume /opt/mihomo:/root/.config/mihomo:ro \
  dockerproxy.net/metacubex/mihomo:v1.19.30 \
  -t -d /root/.config/mihomo

compose=(docker compose --project-name "$compose_project" --file compose.production.yaml)
"${compose[@]}" config --quiet
"${compose[@]}" build backend

if docker container inspect mihomo >/dev/null 2>&1; then
  docker container rm --force mihomo >/dev/null
fi

"${compose[@]}" up -d postgres redis
"${compose[@]}" up --no-build --force-recreate migrate
"${compose[@]}" up -d --no-build --force-recreate --remove-orphans backend mihomo cloudflared

for _ in $(seq 1 60); do
  if curl --fail --silent --show-error --noproxy '*' http://127.0.0.1:8080/health/ready >/dev/null && \
    curl --fail --silent --show-error --proxy http://127.0.0.1:7890 \
      https://api.lyn91r.cn/health/ready >/dev/null; then
    "${compose[@]}" ps
    printf 'Nebula %s is ready\n' "$version"
    exit 0
  fi
  sleep 2
done

"${compose[@]}" ps >&2
"${compose[@]}" logs --tail 50 mihomo cloudflared >&2
printf 'Internal or public deployment health checks did not become ready within 120 seconds\n' >&2
exit 1
