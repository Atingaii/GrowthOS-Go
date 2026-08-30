#!/bin/sh
set -eu

# Bounded-mutation smoke test for the current Docker Compose development stack.
# Its only product-data write is one invalid Strategy-ID Redis key with a 30s
# TTL; an in-container EXIT trap removes it immediately when possible.
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

assert_completed_successfully() {
    inspected_service=$1
    resolve_container "$inspected_service"
    inspect_value '{{.State.Status}}' state
    completed_state=$inspected_value
    inspect_value '{{.State.ExitCode}}' exit-code
    completed_exit_code=$inspected_value
    if [ "$completed_state" != 'exited' ] || [ "$completed_exit_code" != '0' ]; then
        fail "$inspected_service is $completed_state with exit code $completed_exit_code instead of exited with code 0"
    fi
    ok "$inspected_service exited successfully"
}

assert_completed_successfully migrate
assert_completed_successfully mysql-grants

resolve_container migrate
inspect_value '{{.Config.Image}}' image
if [ "$inspected_value" != 'growthos/migrate:lesson-28' ]; then
    fail "migrate image is $inspected_value instead of growthos/migrate:lesson-28"
fi
ok 'migrate image identifies the lesson-28 routing-graph schema build'

resolve_container api
inspect_value '{{.Config.Image}}' image
if [ "$inspected_value" != 'growthos/api:lesson-28' ]; then
    fail "api image is $inspected_value instead of growthos/api:lesson-28"
fi
ok 'api image identifies the lesson-28 build with an unassembled routing-graph adapter'

resolve_container web
inspect_value '{{.Config.Image}}' image
if [ "$inspected_value" != 'growthos/web:lesson-22' ]; then
    fail "web image is $inspected_value instead of growthos/web:lesson-22"
fi
ok 'web image identifies the lesson-22 real React Lottery page build'

resolve_container redis
inspect_value '{{.Config.Image}}' image
if [ "$inspected_value" != 'growthos/redis:7.4.11-lesson-24' ]; then
    fail "redis image is $inspected_value instead of growthos/redis:7.4.11-lesson-24"
fi
ok 'redis image identifies the lesson-24 ACL and memory-policy snapshot'

# The dollar-prefixed expressions belong to the container shell.
# shellcheck disable=SC2016
if ! migration_state=$(compose exec -T mysql sh -c '
    export MYSQL_PWD="$(cat /run/secrets/mysql_migration_password)"
    mysql --protocol=tcp --host=127.0.0.1 --user=growthos_migrator --database=growthos \
        --batch --silent --skip-column-names \
        --execute="SELECT CONCAT(version, CHAR(58), dirty) FROM schema_migrations"
'); then
    fail 'could not inspect the migration version through the migration identity'
fi
if [ "$migration_state" != '5:0' ]; then
    fail "migration state is $migration_state instead of clean version 5"
fi
ok 'schema migrations are clean at version 5'

# shellcheck disable=SC2016
if ! name_constraint_state=$(compose exec -T mysql sh -c '
    export MYSQL_PWD="$(cat /run/secrets/mysql_migration_password)"
    mysql --protocol=tcp --host=127.0.0.1 --user=growthos_migrator --database=growthos \
        --batch --silent --skip-column-names --execute="
            SELECT CONCAT(
                SUM(constraint_name IN (
                    0x63686b5f6c6f74746572795f73747261746567795f6e616d655f6261736963,
                    0x63686b5f6c6f74746572795f73747261746567795f61776172645f6e616d655f6261736963
                )),
                CHAR(58),
                SUM(constraint_name IN (
                    0x63686b5f6c6f74746572795f73747261746567795f6e616d655f63616e6f6e6963616c,
                    0x63686b5f6c6f74746572795f73747261746567795f61776172645f6e616d655f63616e6f6e6963616c
                ))
            )
            FROM information_schema.table_constraints
            WHERE constraint_schema = DATABASE()
        "
'); then
    fail 'could not inspect the lesson-18 name constraints'
fi
if [ "$name_constraint_state" != '2:0' ]; then
    fail "name constraint state is $name_constraint_state instead of two basic constraints and no stale canonical labels"
fi
ok 'live name constraints match the final lesson-18 migration contract'

# shellcheck disable=SC2016
if ! compose exec -T mysql sh -c '
    export MYSQL_PWD="$(cat /run/secrets/mysql_app_password)"
    mysql --protocol=tcp --host=127.0.0.1 --user=growthos_app --database=growthos --silent \
        --execute="SELECT 1 FROM lottery_strategy LIMIT 0; SELECT 1 FROM lottery_strategy_award LIMIT 0"
' >/dev/null; then
    fail 'growthos_app cannot read the current business table allowlist'
fi
# shellcheck disable=SC2016
if ! actual_app_grants=$(compose exec -T mysql sh -c '
    export MYSQL_PWD="$(cat /run/secrets/mysql_app_password)"
    mysql --protocol=tcp --host=127.0.0.1 --user=growthos_app --database=growthos \
        --batch --silent --skip-column-names --execute="SHOW GRANTS FOR CURRENT_USER"
' | LC_ALL=C sort); then
    fail 'could not inspect growthos_app grants'
fi
expected_app_grants=$(LC_ALL=C sort <<'EOF'
GRANT SELECT ON `growthos`.`lottery_strategy` TO `growthos_app`@`%`
GRANT SELECT ON `growthos`.`lottery_strategy_award` TO `growthos_app`@`%`
GRANT USAGE ON *.* TO `growthos_app`@`%`
EOF
)
if [ "$actual_app_grants" != "$expected_app_grants" ]; then
    fail 'growthos_app grants differ from the current SELECT-only allowlist'
fi
# Mandatory roles are effective privileges even though they are not assigned
# directly to growthos_app, so an exact per-account SHOW GRANTS check is not
# sufficient on its own.
# shellcheck disable=SC2016
if ! mandatory_roles=$(compose exec -T mysql sh -c '
    export MYSQL_PWD="$(cat /run/secrets/mysql_app_password)"
    mysql --protocol=tcp --host=127.0.0.1 --user=growthos_app --database=growthos \
        --batch --silent --skip-column-names --execute="SELECT @@GLOBAL.mandatory_roles"
'); then
    fail 'could not inspect MySQL mandatory roles through growthos_app'
fi
if [ -n "$mandatory_roles" ]; then
    fail "MySQL mandatory roles expand growthos_app privileges: $mandatory_roles"
fi
# shellcheck disable=SC2016
if compose exec -T mysql sh -c '
    export MYSQL_PWD="$(cat /run/secrets/mysql_app_password)"
    mysql --protocol=tcp --host=127.0.0.1 --user=growthos_app --database=growthos --silent \
        --execute="SELECT version FROM schema_migrations"
' >/dev/null 2>&1; then
    fail 'growthos_app unexpectedly has access to schema_migrations'
fi
for denied_graph_table in \
    lottery_strategy_routing_graph \
    lottery_strategy_routing_node \
    lottery_strategy_routing_edge; do
    # shellcheck disable=SC2016
    if compose exec -T mysql sh -c '
        export MYSQL_PWD="$(cat /run/secrets/mysql_app_password)"
        mysql --protocol=tcp --host=127.0.0.1 --user=growthos_app --database=growthos --silent \
            --execute="SELECT 1 FROM $1 LIMIT 0"
    ' sh "$denied_graph_table" >/dev/null 2>&1; then
        fail "growthos_app unexpectedly has SELECT permission on $denied_graph_table"
    fi
done
# A zero-row write probe cannot mutate data even if an INSERT grant regresses.
# shellcheck disable=SC2016
if compose exec -T mysql sh -c '
    export MYSQL_PWD="$(cat /run/secrets/mysql_app_password)"
    mysql --protocol=tcp --host=127.0.0.1 --user=growthos_app --database=growthos --silent \
        --execute="
            INSERT INTO lottery_strategy_routing_graph
                (graph_id, revision, schema_version, root_node_id)
            SELECT 1, '\''permission-probe-v1'\'', 1, 1 WHERE FALSE
        "
' >/dev/null 2>&1; then
    fail 'growthos_app unexpectedly has routing-graph INSERT permission'
fi
ok 'growthos_app cannot read or insert the unassembled routing-graph tables'
# shellcheck disable=SC2016
if compose exec -T mysql sh -c '
    export MYSQL_PWD="$(cat /run/secrets/mysql_app_password)"
    mysql --protocol=tcp --host=127.0.0.1 --user=growthos_app --database=growthos --silent \
        --execute="UPDATE lottery_strategy SET name = name WHERE 1 = 0"
' >/dev/null 2>&1; then
    fail 'growthos_app unexpectedly has UPDATE permission'
fi
# shellcheck disable=SC2016
if compose exec -T mysql sh -c '
    export MYSQL_PWD="$(cat /run/secrets/mysql_app_password)"
    mysql --protocol=tcp --host=127.0.0.1 --user=growthos_app --database=growthos --silent \
        --execute="DELETE FROM lottery_strategy WHERE 1 = 0"
' >/dev/null 2>&1; then
    fail 'growthos_app unexpectedly has DELETE permission'
fi
# A zero-row INSERT still requires table INSERT privilege but cannot mutate the
# database even if an over-broad grant regresses.
# shellcheck disable=SC2016
if compose exec -T mysql sh -c '
    export MYSQL_PWD="$(cat /run/secrets/mysql_app_password)"
    mysql --protocol=tcp --host=127.0.0.1 --user=growthos_app --database=growthos --silent \
        --execute="INSERT INTO lottery_strategy (strategy_id, name) SELECT 1, '\''permission probe'\'' WHERE FALSE"
' >/dev/null 2>&1; then
    fail 'growthos_app unexpectedly has lottery_strategy INSERT permission'
fi
# shellcheck disable=SC2016
if compose exec -T mysql sh -c '
    export MYSQL_PWD="$(cat /run/secrets/mysql_app_password)"
    mysql --protocol=tcp --host=127.0.0.1 --user=growthos_app --database=growthos --silent \
        --execute="INSERT INTO lottery_strategy_award (strategy_id, award_id, name, weight, outcome) SELECT 1, 1, '\''permission probe'\'', 1, '\''reward'\'' WHERE FALSE"
' >/dev/null 2>&1; then
    fail 'growthos_app unexpectedly has lottery_strategy_award INSERT permission'
fi
ok 'growthos_app has exactly two-table SELECT and no INSERT, UPDATE, DELETE, or migration-table access'

resolve_container api
api_container_id=$resolved_container_id
if ! docker inspect "$api_container_id" | jq -e \
    --arg edge "${compose_project}_edge" \
    --arg data "${compose_project}_data" \
    --arg cache "${compose_project}_cache" '
        (.[0].NetworkSettings.Networks | keys | sort) == ([$edge, $data, $cache] | sort) and
        ([.[0].Mounts[].Destination | select(startswith("/run/secrets/"))] | sort) ==
            (["/run/secrets/mysql_app_password", "/run/secrets/redis_password"] | sort)
    ' >/dev/null; then
    fail 'api network or Secret mounts differ from the MySQL plus optional-cache ownership contract'
fi

resolve_container redis
redis_container_id=$resolved_container_id
if ! docker inspect "$redis_container_id" | jq -e --arg cache "${compose_project}_cache" '
    (.[0].NetworkSettings.Networks | keys) == [$cache] and
    ([.[0].Mounts[].Destination | select(startswith("/run/secrets/"))]) ==
        ["/run/secrets/redis_password"]
' >/dev/null; then
    fail 'redis must have only the internal cache network and its own Secret'
fi
if ! docker network inspect "${compose_project}_cache" | jq -e '.[0].Internal == true' >/dev/null; then
    fail 'cache network is not Docker-internal'
fi
for cache_non_consumer in web migrate mysql mysql-grants; do
    resolve_container "$cache_non_consumer"
    if docker inspect "$resolved_container_id" | jq -e \
        --arg cache "${compose_project}_cache" '
            (.[0].NetworkSettings.Networks | has($cache)) or
            any(.[0].Mounts[]?; .Destination == "/run/secrets/redis_password")
        ' >/dev/null; then
        fail "$cache_non_consumer unexpectedly consumes the cache network or Redis Secret"
    fi
done
ok 'only api and redis consume the Docker-internal cache network and Redis Secret'

# shellcheck disable=SC2016
if ! compose exec -T redis sh -eu -c '
    export REDISCLI_AUTH="$(cat /run/secrets/redis_password)"
    allowed_key=growthos:development:lottery:strategy:projection:v1:0
    outside_key=growthos:development:lottery:result:v1:0
    cleanup_probe() {
        redis-cli --raw --no-auth-warning --user growthos_api del "$allowed_key" >/dev/null 2>&1 || true
    }
    trap cleanup_probe 0
    assert_denied() {
        denied_label=$1
        shift
        denied_output=$(redis-cli --raw --no-auth-warning --user growthos_api "$@" 2>&1 || true)
        case "$denied_output" in
            *NOPERM*) ;;
            *) printf "expected NOPERM for %s\n" "$denied_label" >&2; exit 1 ;;
        esac
    }
    [ "$(redis-cli --raw --no-auth-warning --user growthos_api ping)" = PONG ]
    [ "$(redis-cli --raw --no-auth-warning --user growthos_api set "$allowed_key" smoke EX 30)" = OK ]
    [ "$(redis-cli --raw --no-auth-warning --user growthos_api getrange "$allowed_key" 0 4)" = smoke ]
    assert_denied "GET inside the cache prefix" get "$allowed_key"
    assert_denied "SET outside the cache prefix" set "$outside_key" forbidden EX 30
    assert_denied SCAN scan 0
    assert_denied CONFIG config get maxmemory
    assert_denied ACL acl users
    assert_denied EVAL eval "return 1" 0
    assert_denied SUBSCRIBE subscribe smoke-channel
    assert_denied PUBLISH publish smoke-channel value
    default_output=$(redis-cli --raw --no-auth-warning ping 2>&1 || true)
    case "$default_output" in
        *WRONGPASS*|*NOAUTH*) ;;
        *) printf "%s\n" "default user unexpectedly authenticated" >&2; exit 1 ;;
    esac
    exec 3</tmp/growthos-redis/users.acl
    IFS= read -r first_acl_line <&3
    IFS= read -r second_acl_line <&3
    expected_acl_line="user growthos_api on >$REDISCLI_AUTH resetkeys ~growthos:development:lottery:strategy:projection:v1:* resetchannels -@all +ping +getrange +set +del"
    if [ "$first_acl_line" != "user default off" ] ||
       [ "$second_acl_line" != "$expected_acl_line" ] ||
       IFS= read -r unexpected_acl_line <&3; then
        printf "%s\n" "generated Redis ACL differs from the exact allowlist" >&2
        exit 1
    fi
    exec 3<&-
    for expected_config_line in \
        "save \"\"" \
        "appendonly no" \
        "maxmemory 48mb" \
        "maxmemory-policy allkeys-lru"; do
        if ! grep -Fqx "$expected_config_line" /tmp/growthos-redis/redis.conf; then
            printf "missing Redis config boundary: %s\n" "$expected_config_line" >&2
            exit 1
        fi
    done
    redis-cli --raw --no-auth-warning --user growthos_api del "$allowed_key" >/dev/null
    trap - 0
    unset REDISCLI_AUTH
'; then
    fail 'redis business ACL allowlist or negative command/key checks failed'
fi
ok 'redis named-user command allowlist, key boundary, and default-user denial are enforced'

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
perform_request() {
    request_method=$1
    request_route=$2
    expected_status=$3
    request_accept=$4
    shift 4
    response_number=$((response_number + 1))
    response_headers="$temporary_directory/headers-$response_number"
    response_body="$temporary_directory/body-$response_number"

    if ! response_status=$(curl \
        --silent \
        --show-error \
        --globoff \
        --connect-timeout "$connect_timeout" \
        --max-time "$max_time" \
        --request "$request_method" \
        --header "Accept: $request_accept" \
        "$@" \
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

request() {
    perform_request GET "$1" "$2" "$3"
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

header_count() {
    requested_header=$1
    header_file=$2
    awk -v requested_header="$requested_header" '
        {
            header_line = $0
            sub(/\r$/, "", header_line)
            header_name = header_line
            sub(/:.*/, "", header_name)
            if (tolower(header_name) == tolower(requested_header)) {
                header_total++
            }
        }
        END { print header_total + 0 }
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
if ! jq -e '.status == "ok" and .version == "lesson-28"' "$response_body" >/dev/null 2>&1; then
    fail '/health did not identify the lesson-28 API build'
fi
ok '/health returned HTTP 200, JSON, and the lesson-28 build through the web proxy'

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

# Derive a missing canonical uint64 without mutating or assuming that a
# particular ID remains unused in a developer's retained database. Candidate
# 1 plus each stored successor guarantees a gap unless every uint64 is stored,
# which is not a representable MySQL deployment state.
# shellcheck disable=SC2016
missing_strategy_id=$(compose exec -T mysql sh -c '
    export MYSQL_PWD="$(cat /run/secrets/mysql_app_password)"
    mysql --protocol=tcp --host=127.0.0.1 --user=growthos_app --database=growthos \
        --batch --silent --skip-column-names --execute="
            SELECT CAST(candidate AS CHAR)
            FROM (
                SELECT CAST(1 AS UNSIGNED) AS candidate
                UNION ALL
                SELECT strategy_id + 1
                FROM lottery_strategy
                WHERE strategy_id < 18446744073709551615
            ) AS candidates
            LEFT JOIN lottery_strategy AS existing
                ON existing.strategy_id = candidates.candidate
            WHERE existing.strategy_id IS NULL
            ORDER BY candidate
            LIMIT 1
        "
') || fail 'could not derive a missing Lottery strategy ID through growthos_app'
case "$missing_strategy_id" in
    ''|0|*[!0-9]*)
        fail 'the derived missing Lottery strategy ID is not a canonical positive decimal integer'
        ;;
esac
lottery_route="/api/v1/lottery/strategies/$missing_strategy_id/ephemeral-selections"
perform_request \
    POST \
    "$lottery_route" \
    404 \
    application/json \
    --header 'X-GrowthOS-Demo-Mode: ephemeral-selection'
assert_json_response "$lottery_route"
if ! jq -e '
    .error |
    type == "object" and
    .code == "lottery_strategy_not_found" and
    .message == "lottery strategy not found" and
    (.request_id | type == "string" and length > 0)
' "$response_body" >/dev/null 2>&1; then
    fail 'the ephemeral selection route did not return the strategy-not-found contract against the unseeded database'
fi
body_request_id=$(jq -r '.error.request_id' "$response_body")
header_request_id=$(header_value 'X-Request-ID' "$response_headers")
if [ -z "$header_request_id" ] || [ "$header_request_id" != "$body_request_id" ]; then
    fail 'the ephemeral selection route returned inconsistent request IDs'
fi
cache_control=$(header_value 'Cache-Control' "$response_headers" | tr '[:upper:]' '[:lower:]')
cache_control_count=$(header_count 'Cache-Control' "$response_headers")
if [ "$cache_control_count" -ne 1 ] || [ "$cache_control" != 'no-store' ]; then
    fail 'the ephemeral selection route must return exactly one Cache-Control: no-store header'
fi
ok 'the ephemeral selection route reached MySQL and returned the correlated no-store 404 contract'

published_ports() {
    # The dollar-prefixed names belong to Docker's Go template, not this shell.
    # shellcheck disable=SC2016
    inspect_value '{{range $port, $bindings := .NetworkSettings.Ports}}{{range $bindings}}{{printf "%s %s %s\n" $port .HostIp .HostPort}}{{end}}{{end}}' published-ports
    service_published_ports=$inspected_value
}

for service_name in api mysql redis migrate mysql-grants; do
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
