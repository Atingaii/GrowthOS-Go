#!/bin/sh
set -eu

# Bounded-mutation smoke test for the current Docker Compose development stack.
# Its only persistent probe is one invalid Strategy-ID Redis key with a 30s
# TTL; an in-container EXIT trap removes it immediately when possible. The
# provisioner grant probe performs one valid MySQL INSERT inside a transaction
# and rolls it back before the connection exits.
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
identity_public_origin="http://127.0.0.1:$web_port"
identity_csrf_active_key_id=${GROWTHOS_COMPOSE_IDENTITY_CSRF_ACTIVE_KEY_ID:-local-v1}
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
    ''|*[!0-9]*|0|0*)
        fail 'GROWTHOS_COMPOSE_WEB_PORT must be a canonical integer from 1 through 65535'
        ;;
esac
if [ "${#web_port}" -gt 5 ] || [ "$web_port" -gt 65535 ] || [ "$web_port" -eq 80 ]; then
    fail 'GROWTHOS_COMPOSE_WEB_PORT must be a canonical non-default HTTP port from 1 through 65535'
fi
case "$identity_csrf_active_key_id" in
    ''|*[!A-Za-z0-9_-]*)
        fail 'GROWTHOS_COMPOSE_IDENTITY_CSRF_ACTIVE_KEY_ID must use only letters, digits, underscore, or hyphen'
        ;;
esac
if [ "${#identity_csrf_active_key_id}" -gt 16 ]; then
    fail 'GROWTHOS_COMPOSE_IDENTITY_CSRF_ACTIVE_KEY_ID must contain at most 16 characters'
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
if [ "$inspected_value" != 'growthos/migrate:lesson-32' ]; then
    fail "migrate image is $inspected_value instead of growthos/migrate:lesson-32"
fi
ok 'migrate image identifies the lesson-32 Identity schema build'

resolve_container api
inspect_value '{{.Config.Image}}' image
if [ "$inspected_value" != 'growthos/api:lesson-32' ]; then
    fail "api image is $inspected_value instead of growthos/api:lesson-32"
fi
ok 'api image identifies the lesson-32 real session authentication build'

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
if [ "$migration_state" != '14:0' ]; then
    fail "migration state is $migration_state instead of clean version 14"
fi
ok 'schema migrations are clean at version 14'

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
for denied_unassembled_table in \
    lottery_strategy_routing_graph \
    lottery_strategy_routing_node \
    lottery_strategy_routing_edge \
    lottery_strategy_snapshot \
    lottery_strategy_snapshot_award \
    marketing_activity \
    marketing_activity_publication \
    marketing_activity_publication_strategy \
    identity_workforce_account \
    identity_session \
    identity_authentication_throttle; do
    # shellcheck disable=SC2016
    if compose exec -T mysql sh -c '
        export MYSQL_PWD="$(cat /run/secrets/mysql_app_password)"
        mysql --protocol=tcp --host=127.0.0.1 --user=growthos_app --database=growthos --silent \
            --execute="SELECT 1 FROM $1 LIMIT 0"
    ' sh "$denied_unassembled_table" >/dev/null 2>&1; then
        fail "growthos_app unexpectedly has SELECT permission on $denied_unassembled_table"
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
ok 'growthos_app cannot read unassembled graph, snapshot, Activity, or Identity tables and cannot insert graph data'
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

# shellcheck disable=SC2016
if ! compose exec -T mysql sh -c '
    export MYSQL_PWD="$(cat /run/secrets/mysql_identity_password)"
    mysql --protocol=tcp --host=127.0.0.1 --user=growthos_identity --database=growthos --silent \
        --execute="
            SELECT 1 FROM identity_workforce_account LIMIT 0;
            SELECT 1 FROM identity_session LIMIT 0;
            SELECT 1 FROM identity_authentication_throttle LIMIT 0
        "
' >/dev/null; then
    fail 'growthos_identity cannot read its exact three-table allowlist'
fi
# shellcheck disable=SC2016
if ! actual_identity_grants=$(compose exec -T mysql sh -c '
    export MYSQL_PWD="$(cat /run/secrets/mysql_identity_password)"
    mysql --protocol=tcp --host=127.0.0.1 --user=growthos_identity --database=growthos \
        --batch --silent --skip-column-names --execute="SHOW GRANTS FOR CURRENT_USER"
' | LC_ALL=C sort); then
    fail 'could not inspect growthos_identity grants'
fi
expected_identity_grants=$(LC_ALL=C sort <<'EOF'
GRANT SELECT, UPDATE (`updated_at`) ON `growthos`.`identity_workforce_account` TO `growthos_identity`@`%`
GRANT SELECT, INSERT, UPDATE, DELETE ON `growthos`.`identity_session` TO `growthos_identity`@`%`
GRANT SELECT, INSERT, UPDATE, DELETE ON `growthos`.`identity_authentication_throttle` TO `growthos_identity`@`%`
GRANT USAGE ON *.* TO `growthos_identity`@`%`
EOF
)
if [ "$actual_identity_grants" != "$expected_identity_grants" ]; then
    fail 'growthos_identity grants differ from the exact three-table allowlist'
fi
for identity_denied_table in schema_migrations lottery_strategy lottery_strategy_award marketing_activity; do
    # shellcheck disable=SC2016
    if compose exec -T mysql sh -c '
        export MYSQL_PWD="$(cat /run/secrets/mysql_identity_password)"
        mysql --protocol=tcp --host=127.0.0.1 --user=growthos_identity --database=growthos --silent \
            --execute="SELECT 1 FROM $1 LIMIT 0"
    ' sh "$identity_denied_table" >/dev/null 2>&1; then
        fail "growthos_identity unexpectedly has SELECT permission on $identity_denied_table"
    fi
done
# MySQL requires an UPDATE privilege for locking reads. Prove the narrow
# updated_at column grant admits the repository's account lock without granting
# updates to credential-bearing columns.
# shellcheck disable=SC2016
if ! compose exec -T mysql sh -c '
    export MYSQL_PWD="$(cat /run/secrets/mysql_identity_password)"
    mysql --protocol=tcp --host=127.0.0.1 --user=growthos_identity --database=growthos --silent \
        --execute="START TRANSACTION; SELECT account_id FROM identity_workforce_account WHERE FALSE FOR UPDATE; ROLLBACK"
' >/dev/null; then
    fail 'growthos_identity cannot take the required workforce-account locking read'
fi
# shellcheck disable=SC2016
if ! compose exec -T mysql sh -c '
    export MYSQL_PWD="$(cat /run/secrets/mysql_identity_password)"
    mysql --protocol=tcp --host=127.0.0.1 --user=growthos_identity --database=growthos --silent \
        --execute="UPDATE identity_workforce_account SET updated_at = updated_at WHERE FALSE"
' >/dev/null; then
    fail 'growthos_identity cannot exercise its updated_at-only account grant'
fi
# shellcheck disable=SC2016
if compose exec -T mysql sh -c '
    export MYSQL_PWD="$(cat /run/secrets/mysql_identity_password)"
    mysql --protocol=tcp --host=127.0.0.1 --user=growthos_identity --database=growthos --silent \
        --execute="UPDATE identity_workforce_account SET login_name = login_name WHERE FALSE"
' >/dev/null 2>&1; then
    fail 'growthos_identity unexpectedly has workforce-account login_name UPDATE permission'
fi
ok 'growthos_identity has exact session/throttle DML, account SELECT plus updated_at-only UPDATE, and no business/migration/credential-write access'

# The account creator is not a second runtime repository. Its whole durable
# authority is one INSERT on the workforce table, with no readback, mutation,
# deletion, migration, business, session, or throttle access.
# shellcheck disable=SC2016
if ! actual_identity_provisioner_grants=$(compose exec -T mysql sh -c '
    export MYSQL_PWD="$(cat /run/secrets/mysql_identity_provisioner_password)"
    mysql --protocol=tcp --host=127.0.0.1 --user=growthos_identity_provisioner --database=growthos \
        --batch --silent --skip-column-names --execute="SHOW GRANTS FOR CURRENT_USER"
' | LC_ALL=C sort); then
    fail 'could not inspect growthos_identity_provisioner grants'
fi
expected_identity_provisioner_grants=$(LC_ALL=C sort <<'EOF'
GRANT INSERT ON `growthos`.`identity_workforce_account` TO `growthos_identity_provisioner`@`%`
GRANT USAGE ON *.* TO `growthos_identity_provisioner`@`%`
EOF
)
if [ "$actual_identity_provisioner_grants" != "$expected_identity_provisioner_grants" ]; then
    fail 'growthos_identity_provisioner grants differ from the INSERT-only allowlist'
fi
# This is the only positive mutation probe. It inserts a valid random row and
# rolls the transaction back before the connection exits.
# shellcheck disable=SC2016
if ! compose exec -T mysql sh -c '
    export MYSQL_PWD="$(cat /run/secrets/mysql_identity_provisioner_password)"
    mysql --protocol=tcp --host=127.0.0.1 --user=growthos_identity_provisioner --database=growthos --silent \
        --execute="
            SET @probe = REPLACE(UUID(), CHAR(45), CHAR(95));
            START TRANSACTION;
            INSERT INTO identity_workforce_account (
                account_id, login_name, principal_id, password_envelope,
                account_status, credential_version, authentication_epoch,
                created_at, updated_at
            ) VALUES (
                CONCAT('"'"'smoke.account.'"'"', @probe),
                CONCAT('"'"'smoke.login.'"'"', @probe),
                CONCAT('"'"'smoke.principal.'"'"', @probe),
                CONCAT(CHAR(36), '"'"'argon2id'"'"', CHAR(36), '"'"'smoke'"'"'),
                '"'"'enabled'"'"', 1, 1, UTC_TIMESTAMP(6), UTC_TIMESTAMP(6)
            );
            ROLLBACK
        "
' >/dev/null; then
    fail 'growthos_identity_provisioner cannot perform its rolled-back workforce INSERT'
fi
# shellcheck disable=SC2016
if compose exec -T mysql sh -c '
    export MYSQL_PWD="$(cat /run/secrets/mysql_identity_provisioner_password)"
    mysql --protocol=tcp --host=127.0.0.1 --user=growthos_identity_provisioner --database=growthos --silent \
        --execute="SELECT account_id FROM identity_workforce_account LIMIT 0"
' >/dev/null 2>&1; then
    fail 'growthos_identity_provisioner unexpectedly has workforce SELECT permission'
fi
# shellcheck disable=SC2016
if compose exec -T mysql sh -c '
    export MYSQL_PWD="$(cat /run/secrets/mysql_identity_provisioner_password)"
    mysql --protocol=tcp --host=127.0.0.1 --user=growthos_identity_provisioner --database=growthos --silent \
        --execute="UPDATE identity_workforce_account SET updated_at = updated_at WHERE FALSE"
' >/dev/null 2>&1; then
    fail 'growthos_identity_provisioner unexpectedly has workforce UPDATE permission'
fi
# shellcheck disable=SC2016
if compose exec -T mysql sh -c '
    export MYSQL_PWD="$(cat /run/secrets/mysql_identity_provisioner_password)"
    mysql --protocol=tcp --host=127.0.0.1 --user=growthos_identity_provisioner --database=growthos --silent \
        --execute="DELETE FROM identity_workforce_account WHERE FALSE"
' >/dev/null 2>&1; then
    fail 'growthos_identity_provisioner unexpectedly has workforce DELETE permission'
fi
# shellcheck disable=SC2016
if compose exec -T mysql sh -c '
    export MYSQL_PWD="$(cat /run/secrets/mysql_identity_provisioner_password)"
    mysql --protocol=tcp --host=127.0.0.1 --user=growthos_identity_provisioner --database=growthos --silent \
        --execute="INSERT INTO lottery_strategy (strategy_id, name) SELECT 1, '"'"'permission probe'"'"' WHERE FALSE"
' >/dev/null 2>&1; then
    fail 'growthos_identity_provisioner unexpectedly has business-table INSERT permission'
fi
ok 'growthos_identity_provisioner has only workforce INSERT; rollback succeeds while SELECT, UPDATE, DELETE, and other-table writes are denied'

resolve_container api
api_container_id=$resolved_container_id
if ! docker inspect "$api_container_id" | jq -e \
    --arg edge "${compose_project}_edge" \
    --arg data "${compose_project}_data" \
    --arg cache "${compose_project}_cache" \
    --arg identity_public_origin "GROWTHOS_IDENTITY_PUBLIC_ORIGIN=$identity_public_origin" \
    --arg identity_csrf_active_key_id "GROWTHOS_IDENTITY_CSRF_ACTIVE_KEY_ID=$identity_csrf_active_key_id" '
        (.[0].NetworkSettings.Networks | keys | sort) == ([$edge, $data, $cache] | sort) and
        .[0].Config.User == "65532:65532" and
        .[0].HostConfig.ReadonlyRootfs == true and
        ([.[0].Mounts[].Destination | select(startswith("/run/secrets/"))] | sort) ==
            (["/run/secrets/identity_csrf_active_key", "/run/secrets/identity_throttle_hmac_key",
              "/run/secrets/mysql_app_password", "/run/secrets/mysql_identity_password",
              "/run/secrets/redis_password"] | sort) and
        all(.[0].Mounts[];
            if (.Destination | startswith("/run/secrets/")) then .RW == false else true end) and
        (.[0].Config.Env | index($identity_public_origin)) != null and
        (.[0].Config.Env | index($identity_csrf_active_key_id)) != null and
        (.[0].Config.Env | index("GROWTHOS_IDENTITY_THROTTLE_HMAC_KEY_FILE=/run/secrets/identity_throttle_hmac_key")) != null and
        (.[0].Config.Env | index("GROWTHOS_IDENTITY_CSRF_ACTIVE_KEY_FILE=/run/secrets/identity_csrf_active_key")) != null and
        (.[0].Config.Env | index("GROWTHOS_IDENTITY_ARGON2_MAX_CONCURRENT=2")) != null and
        (.[0].Config.Env | index("GROWTHOS_IDENTITY_ARGON2_ACQUIRE_TIMEOUT=250ms")) != null and
        (.[0].Config.Env | index("GROWTHOS_IDENTITY_HTTP_HANDLER_TIMEOUT=3s")) != null
    ' >/dev/null; then
    fail 'api network or Secret mounts differ from the business/Identity runtime plus optional-cache ownership contract'
fi

for mysql_secret_consumer in mysql migrate mysql-grants; do
    resolve_container "$mysql_secret_consumer"
    case "$mysql_secret_consumer" in
        mysql)
            expected_mysql_secret_mounts='["/run/secrets/mysql_app_password","/run/secrets/mysql_identity_password","/run/secrets/mysql_identity_provisioner_password","/run/secrets/mysql_migration_password","/run/secrets/mysql_root_password"]'
            ;;
        migrate)
            expected_mysql_secret_mounts='["/run/secrets/mysql_migration_password"]'
            ;;
        mysql-grants)
            expected_mysql_secret_mounts='["/run/secrets/mysql_identity_password","/run/secrets/mysql_identity_provisioner_password","/run/secrets/mysql_root_password"]'
            ;;
    esac
    if ! docker inspect "$resolved_container_id" | jq -e \
        --argjson expected "$expected_mysql_secret_mounts" '
            ([.[0].Mounts[].Destination | select(startswith("/run/secrets/"))] | sort) ==
                ($expected | sort)
        ' >/dev/null; then
        fail "$mysql_secret_consumer Secret mounts exceed or omit its exact MySQL ownership set"
    fi
done
ok 'MySQL, migrator, grant reconciler, and API each receive only their declared MySQL Secrets'

for provisioner_secret_non_consumer in migrate api redis web; do
    resolve_container "$provisioner_secret_non_consumer"
    if docker inspect "$resolved_container_id" | jq -e '
        any(.[0].Mounts[]?;
            .Destination == "/run/secrets/mysql_identity_provisioner_password")
    ' >/dev/null; then
        fail "$provisioner_secret_non_consumer unexpectedly receives the one-shot provisioner database credential"
    fi
done
ok 'no long-lived application, cache, web, or migration process receives the one-shot provisioner credential'

if ! docker compose \
    --project-name "$compose_project" \
    --file "$compose_file" \
    --profile operations \
    config --format json | jq -e '
        .services["identity-provision"] as $service |
        $service.profiles == ["operations"] and
        $service.image == "growthos/identity-provision:lesson-32" and
        $service.build.target == "identity-provision" and
        $service.user == "65532:65532" and
        $service.read_only == true and
        $service.restart == "no" and
        $service.cap_drop == ["ALL"] and
        $service.security_opt == ["no-new-privileges:true"] and
        ($service.networks | keys) == ["data"] and
        ($service.ports // []) == [] and
        ($service.volumes // []) == [] and
        ($service.secrets | map(.source)) == ["mysql_identity_provisioner_password"] and
        $service.environment.GROWTHOS_IDENTITY_PROVISIONER_MYSQL_USER == "growthos_identity_provisioner" and
        $service.environment.GROWTHOS_IDENTITY_PROVISIONER_MYSQL_PASSWORD_FILE == "/run/secrets/mysql_identity_provisioner_password" and
        $service.depends_on["mysql-grants"].condition == "service_completed_successfully"
    ' >/dev/null; then
    fail 'identity-provision differs from the operations-only, non-root, read-only, one-secret Compose contract'
fi
ok 'identity-provision is an operations-only non-root/read-only service with one database secret and no static enrollment-password mount'

for identity_key_non_consumer in mysql migrate mysql-grants redis web; do
    resolve_container "$identity_key_non_consumer"
    if docker inspect "$resolved_container_id" | jq -e '
        any(.[0].Mounts[]?;
            .Destination == "/run/secrets/identity_throttle_hmac_key" or
            .Destination == "/run/secrets/identity_csrf_active_key")
    ' >/dev/null; then
        fail "$identity_key_non_consumer unexpectedly receives an API-only Identity signing key"
    fi
done
ok 'only the API receives the independent Identity throttle and active CSRF keys'

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
if ! jq -e '.status == "ok" and .version == "lesson-32"' "$response_body" >/dev/null 2>&1; then
    fail '/health did not identify the lesson-32 API build'
fi
ok '/health returned HTTP 200, JSON, and the lesson-32 build through the web proxy'

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
