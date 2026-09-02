#!/bin/sh
set -eu

# Runs exactly one bounded maintenance operation through the operations-only
# service. Time, retention, row budgets, looping, and retry policy remain owned
# by the binary and are deliberately absent from this host boundary.

repository_root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
compose_project=${GROWTHOS_COMPOSE_PROJECT:-growthos}
compose_file=${GROWTHOS_COMPOSE_FILE:-"$repository_root/deploy/compose/compose.yaml"}
secret_generator="$repository_root/scripts/generate-compose-secrets.sh"
umask 077

fail() {
    printf 'identity maintenance refused: %s\n' "$1" >&2
    exit 1
}

if [ "$#" -ne 0 ]; then
    fail 'this operation accepts no arguments'
fi
if ! command -v docker >/dev/null 2>&1; then
    fail 'docker is required'
fi
if ! docker compose version >/dev/null 2>&1; then
    fail 'the Docker Compose plugin is unavailable'
fi
if [ ! -f "$compose_file" ] || [ -L "$compose_file" ] || [ ! -r "$compose_file" ]; then
    fail 'the configured Compose file must be a readable non-symbolic regular file'
fi
if [ ! -f "$secret_generator" ] || [ -L "$secret_generator" ] || [ ! -x "$secret_generator" ]; then
    fail 'the Compose secret generator is unavailable'
fi
case "$compose_project" in
    ''|*[!A-Za-z0-9_.-]*)
        fail 'GROWTHOS_COMPOSE_PROJECT contains unsupported characters'
        ;;
esac

GROWTHOS_COMPOSE_PROJECT="$compose_project" \
GROWTHOS_COMPOSE_WEB_PORT="${GROWTHOS_COMPOSE_WEB_PORT:-8088}" \
GROWTHOS_COMPOSE_IDENTITY_CSRF_ACTIVE_KEY_ID="${GROWTHOS_COMPOSE_IDENTITY_CSRF_ACTIVE_KEY_ID:-local-v1}" \
    "$secret_generator"

compose() {
    docker compose \
        --project-name "$compose_project" \
        --file "$compose_file" \
        --profile operations \
        "$@"
}

compose config --quiet
compose build identity-maintenance
compose up --detach --build mysql-grants
compose wait mysql-grants
compose run --rm --no-deps --no-tty identity-maintenance
