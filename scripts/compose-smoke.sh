#!/bin/sh
set -eu

# Read-only smoke test for the lesson-16 Docker Compose development stack.
# Supported overrides:
#   GROWTHOS_COMPOSE_PROJECT       Compose project name (default: growthos)
#   GROWTHOS_COMPOSE_FILE          Compose file path
#   GROWTHOS_COMPOSE_WEB_PORT      expected loopback host port (default: 8088)
#   GROWTHOS_COMPOSE_BASE_URL      HTTP origin to probe
#   GROWTHOS_COMPOSE_CONNECT_TIMEOUT  curl connect timeout in seconds
#   GROWTHOS_COMPOSE_MAX_TIME         curl request timeout in seconds

repository_root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
compose_project=${GROWTHOS_COMPOSE_PROJECT:-growthos}
compose_file=${GROWTHOS_COMPOSE_FILE:-"$repository_root/deploy/compose/compose.yaml"}
web_port=${GROWTHOS_COMPOSE_WEB_PORT:-8088}
base_url=${GROWTHOS_COMPOSE_BASE_URL:-"http://127.0.0.1:$web_port"}
connect_timeout=${GROWTHOS_COMPOSE_CONNECT_TIMEOUT:-3}
max_time=${GROWTHOS_COMPOSE_MAX_TIME:-10}

ok() {
    printf 'ok - %s\n' "$1"
}

fail() {
    printf 'not ok - %s\n' "$1" >&2
    exit 1
}

require_command() {
    if ! command -v "$1" >/dev/null 2>&1; then
        fail "$1 is required"
    fi
}

require_command docker
require_command curl
require_command jq

if [ ! -r "$compose_file" ]; then
    fail 'the configured Compose file is not readable'
fi

case "$compose_project" in
    ''|*[!A-Za-z0-9_.-]*)
        fail 'GROWTHOS_COMPOSE_PROJECT contains unsupported characters'
        ;;
esac

case "$web_port" in
    ''|*[!0-9]*)
        fail 'GROWTHOS_COMPOSE_WEB_PORT must be an integer from 1 through 65535'
        ;;
esac
if [ "$web_port" -lt 1 ] || [ "$web_port" -gt 65535 ]; then
    fail 'GROWTHOS_COMPOSE_WEB_PORT must be an integer from 1 through 65535'
fi

case "$base_url" in
    http://*|https://*)
        ;;
    *)
        fail 'GROWTHOS_COMPOSE_BASE_URL must use http or https'
        ;;
esac
base_authority=${base_url#*://}
base_authority=${base_authority%%/*}
case "$base_authority" in
    ''|*@*)
        fail 'GROWTHOS_COMPOSE_BASE_URL must not contain credentials'
        ;;
esac
case "$base_url" in
    *\?*|*\#*)
        fail 'GROWTHOS_COMPOSE_BASE_URL must not contain a query or fragment'
        ;;
esac
while [ "${base_url%/}" != "$base_url" ]; do
    base_url=${base_url%/}
done
base_remainder=${base_url#*://}
case "$base_remainder" in
    */*)
        fail 'GROWTHOS_COMPOSE_BASE_URL must be an origin without a path'
        ;;
esac

if ! docker compose version >/dev/null 2>&1; then
    fail 'the Docker Compose plugin is unavailable'
fi

compose() {
    docker compose --project-name "$compose_project" --file "$compose_file" "$@"
}

resolve_container() {
    inspected_service=$1
    if ! resolved_container_id=$(compose ps --all --quiet "$inspected_service" 2>/dev/null); then
        fail "could not inspect the $inspected_service service"
    fi

    case "$resolved_container_id" in
        '')
            fail "$inspected_service has no container"
            ;;
        *'
'*)
            fail "$inspected_service must have exactly one container"
            ;;
    esac
}

inspect_value() {
    inspect_template=$1
    inspect_description=$2
    if ! inspected_value=$(docker inspect --format "$inspect_template" "$resolved_container_id" 2>/dev/null); then
        fail "could not inspect $inspect_description for $inspected_service"
    fi
}

assert_running_healthy() {
    inspected_service=$1
    resolve_container "$inspected_service"

    inspect_value '{{.State.Status}}' state
    container_state=$inspected_value
    inspect_value '{{if .State.Health}}{{.State.Health.Status}}{{else}}missing{{end}}' health
    container_health=$inspected_value

    if [ "$container_state" != 'running' ]; then
        fail "$inspected_service is $container_state instead of running"
    fi
    if [ "$container_health" != 'healthy' ]; then
        fail "$inspected_service health is $container_health instead of healthy"
    fi

    ok "$inspected_service is running and healthy"
}

for service_name in mysql api redis web; do
    assert_running_healthy "$service_name"
done

resolve_container migrate
inspect_value '{{.State.Status}}' state
migrate_state=$inspected_value
inspect_value '{{.State.ExitCode}}' exit-code
migrate_exit_code=$inspected_value
if [ "$migrate_state" != 'exited' ] || [ "$migrate_exit_code" != '0' ]; then
    fail "migrate is $migrate_state with exit code $migrate_exit_code instead of exited with code 0"
fi
ok 'migrate exited successfully'

temporary_directory=$(mktemp -d "${TMPDIR:-/tmp}/growthos-compose-smoke.XXXXXX")
cleanup() {
    if [ -n "${temporary_directory:-}" ] && [ -d "$temporary_directory" ]; then
        rm -rf "$temporary_directory"
    fi
}
trap cleanup 0
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

response_number=0
request() {
    request_route=$1
    expected_status=$2
    request_accept=$3
    response_number=$((response_number + 1))
    response_headers="$temporary_directory/headers-$response_number"
    response_body="$temporary_directory/body-$response_number"

    if ! response_status=$(curl \
        --silent \
        --show-error \
        --globoff \
        --connect-timeout "$connect_timeout" \
        --max-time "$max_time" \
        --header "Accept: $request_accept" \
        --dump-header "$response_headers" \
        --output "$response_body" \
        --write-out '%{http_code}' \
        "$base_url$request_route"); then
        fail "request to $request_route failed"
    fi

    if [ "$response_status" != "$expected_status" ]; then
        fail "$request_route returned HTTP $response_status instead of $expected_status"
    fi
}

header_value() {
    requested_header=$1
    header_file=$2
    awk -v requested_header="$requested_header" '
        {
            header_line = $0
            sub(/\r$/, "", header_line)
            header_name = header_line
            sub(/:.*/, "", header_name)
            if (tolower(header_name) == tolower(requested_header)) {
                sub(/^[^:]*:[[:space:]]*/, "", header_line)
                header_result = header_line
            }
        }
        END { print header_result }
    ' "$header_file"
}

assert_json_response() {
    json_route=$1
    content_type=$(header_value 'Content-Type' "$response_headers" | tr '[:upper:]' '[:lower:]')
    media_type=${content_type%%;*}
    if [ "$media_type" != 'application/json' ]; then
        fail "$json_route did not return application/json"
    fi
    if ! jq -e . "$response_body" >/dev/null 2>&1; then
        fail "$json_route did not return valid JSON"
    fi
}

request /health 200 application/json
assert_json_response /health
ok '/health returned HTTP 200 and JSON through the web proxy'

request /ready 200 application/json
assert_json_response /ready
ok '/ready returned HTTP 200 and JSON through the web proxy'

request / 200 text/html
ok 'the SPA entry point returned HTTP 200'

request /api/__growthos_compose_smoke_missing 404 application/json
assert_json_response /api/__growthos_compose_smoke_missing
if ! jq -e '
    .error |
    type == "object" and
    .code == "route_not_found" and
    .message == "resource not found" and
    (.request_id | type == "string" and length > 0)
' "$response_body" >/dev/null 2>&1; then
    fail 'the unknown API route did not return the route_not_found error contract'
fi
body_request_id=$(jq -r '.error.request_id' "$response_body")
header_request_id=$(header_value 'X-Request-ID' "$response_headers")
if [ -z "$header_request_id" ] || [ "$header_request_id" != "$body_request_id" ]; then
    fail 'the unknown API route returned inconsistent request IDs'
fi
ok 'an unknown API route returned the correlated 404 JSON contract'

published_ports() {
    # The dollar-prefixed names belong to Docker's Go template, not this shell.
    # shellcheck disable=SC2016
    inspect_value '{{range $port, $bindings := .NetworkSettings.Ports}}{{range $bindings}}{{printf "%s %s %s\n" $port .HostIp .HostPort}}{{end}}{{end}}' published-ports
    service_published_ports=$inspected_value
}

for service_name in api mysql redis migrate; do
    resolve_container "$service_name"
    published_ports
    if [ -n "$service_published_ports" ]; then
        fail "$service_name unexpectedly publishes a host port"
    fi
done

resolve_container web
published_ports
expected_web_binding="8080/tcp 127.0.0.1 $web_port"
if [ "$service_published_ports" != "$expected_web_binding" ]; then
    fail 'web must publish only container port 8080 on the configured loopback port'
fi
ok "only web publishes 127.0.0.1:$web_port"

ok 'Compose smoke checks passed'
