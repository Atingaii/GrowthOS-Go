#!/bin/sh
set -eu

repository_root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
mysql_image='mysql:8.4.11'
database_name='growthos_l28_acceptance'
migration_user='growthos_l28_migrator'
api_user='growthos_l28_legacy_api'
graph_user='growthos_l28_graph_repo'
label_key='com.growthos.acceptance.lesson28'
compose_project_label='com.docker.compose.project=growthos'

for required_command in docker openssl mktemp sed tr; do
    if ! command -v "$required_command" >/dev/null 2>&1; then
        printf '%s is required for Lesson 28 MySQL acceptance\n' "$required_command" >&2
        exit 1
    fi
done
if ! docker info >/dev/null 2>&1; then
    printf '%s\n' 'the Docker daemon is unavailable' >&2
    exit 1
fi

random_suffix=$(openssl rand -hex 12)
case "$random_suffix" in
    *[!0-9a-f]*|'')
        printf '%s\n' 'openssl returned an invalid container suffix' >&2
        exit 1
        ;;
esac
if [ "${#random_suffix}" -ne 24 ]; then
    printf '%s\n' 'openssl returned a container suffix with an invalid length' >&2
    exit 1
fi
container_name="growthos-lesson28-mysql-$random_suffix"
label_value="run-$random_suffix"
case "$container_name" in
    growthos-lesson28-mysql-[0-9a-f]*) ;;
    *)
        printf '%s\n' 'refusing an invalid Lesson 28 container name' >&2
        exit 1
        ;;
esac
case "$label_value" in
    run-[0-9a-f]*) ;;
    *)
        printf '%s\n' 'refusing an invalid Lesson 28 container label' >&2
        exit 1
        ;;
esac

if docker container inspect "$container_name" >/dev/null 2>&1; then
    printf 'container name already exists: %s\n' "$container_name" >&2
    exit 1
fi
if [ -n "$(docker container ls -aq --filter "label=$label_key=$label_value")" ]; then
    printf 'container label already exists: %s=%s\n' "$label_key" "$label_value" >&2
    exit 1
fi

snapshot_growthos_containers() {
    growthos_ids=$(docker container ls -aq --filter "label=$compose_project_label")
    if [ -z "$growthos_ids" ]; then
        return 0
    fi
    for growthos_id in $growthos_ids; do
        docker container inspect --format '{{.Id}}|{{.Name}}|{{.Config.Image}}|{{.State.Status}}|{{.RestartCount}}' "$growthos_id"
    done | LC_ALL=C sort
}

snapshot_growthos_volumes() {
    growthos_volumes=$(docker volume ls -q --filter "label=$compose_project_label")
    if [ -z "$growthos_volumes" ]; then
        return 0
    fi
    for growthos_volume in $growthos_volumes; do
        docker volume inspect --format '{{.Name}}|{{.Driver}}|{{.Mountpoint}}' "$growthos_volume"
    done | LC_ALL=C sort
}

snapshot_growthos_networks() {
    growthos_networks=$(docker network ls -q --filter "label=$compose_project_label")
    if [ -z "$growthos_networks" ]; then
        return 0
    fi
    for growthos_network in $growthos_networks; do
        docker network inspect --format '{{.Id}}|{{.Name}}|{{.Driver}}|{{.Scope}}|{{.Internal}}' "$growthos_network"
    done | LC_ALL=C sort
}

growthos_containers_before=$(snapshot_growthos_containers)
growthos_volumes_before=$(snapshot_growthos_volumes)
growthos_networks_before=$(snapshot_growthos_networks)
snapshots_taken=1

temporary_root=${TMPDIR:-/tmp}
temporary_root=${temporary_root%/}
case "$temporary_root" in
    /*) ;;
    *)
        printf 'TMPDIR must be absolute: %s\n' "$temporary_root" >&2
        exit 1
        ;;
esac
secret_directory=$(mktemp -d "$temporary_root/growthos-lesson28-acceptance.XXXXXX")
secret_basename=${secret_directory##*/}
case "$secret_basename" in
    growthos-lesson28-acceptance.[A-Za-z0-9][A-Za-z0-9][A-Za-z0-9][A-Za-z0-9][A-Za-z0-9][A-Za-z0-9]) ;;
    *)
        printf 'mktemp returned an unexpected directory: %s\n' "$secret_directory" >&2
        exit 1
        ;;
esac
case "$secret_directory" in
    "$temporary_root"/growthos-lesson28-acceptance.*) ;;
    *)
        printf 'mktemp escaped the expected root: %s\n' "$secret_directory" >&2
        exit 1
        ;;
esac
case "$secret_directory" in
    *,*)
        printf '%s\n' 'the temporary secret path must not contain a comma' >&2
        exit 1
        ;;
esac
chmod 0700 "$secret_directory"

root_secret_file="$secret_directory/root.password"
migration_secret_file="$secret_directory/migration.password"
api_secret_file="$secret_directory/legacy-api.password"
graph_secret_file="$secret_directory/graph-repository.password"
root_client_file="$secret_directory/root-client.cnf"
container_id=
image_added=0

cleanup() {
    requested_status=$1
    trap - EXIT HUP INT TERM
    set +e
    cleanup_failed=0

    if [ -n "${container_id:-}" ] && docker container inspect "$container_id" >/dev/null 2>&1; then
        actual_name=$(docker container inspect --format '{{.Name}}' "$container_id" 2>/dev/null)
        actual_name=${actual_name#/}
        actual_label=$(docker container inspect --format "{{index .Config.Labels \"$label_key\"}}" "$container_id" 2>/dev/null)
        if [ "$actual_name" = "$container_name" ] && [ "$actual_label" = "$label_value" ]; then
            if ! docker container stop --time 15 "$container_id" >/dev/null; then
                if docker container inspect "$container_id" >/dev/null 2>&1; then
                    printf 'failed to stop owned container %s\n' "$container_id" >&2
                    cleanup_failed=1
                fi
            fi
            cleanup_attempt=0
            while docker container inspect "$container_id" >/dev/null 2>&1 && [ "$cleanup_attempt" -lt 30 ]; do
                cleanup_attempt=$((cleanup_attempt + 1))
                sleep 1
            done
        else
            printf 'refusing to stop container %s after its exact identity changed\n' "$container_id" >&2
            cleanup_failed=1
        fi
    fi
    if docker container inspect "$container_name" >/dev/null 2>&1; then
        printf 'Lesson 28 container name remains after cleanup: %s\n' "$container_name" >&2
        cleanup_failed=1
    fi
    if [ -n "$(docker container ls -aq --filter "label=$label_key=$label_value")" ]; then
        printf 'Lesson 28 container label remains after cleanup: %s=%s\n' "$label_key" "$label_value" >&2
        cleanup_failed=1
    fi

    if [ -n "${secret_directory:-}" ]; then
        case "$secret_directory" in
            "$temporary_root"/growthos-lesson28-acceptance.*)
                rm -f -- "$root_secret_file" "$migration_secret_file" "$api_secret_file" "$graph_secret_file" "$root_client_file"
                if ! rmdir "$secret_directory" 2>/dev/null; then
                    printf 'temporary secret directory is not empty: %s\n' "$secret_directory" >&2
                    cleanup_failed=1
                fi
                ;;
            *)
                printf 'refusing to clean unexpected temporary path: %s\n' "$secret_directory" >&2
                cleanup_failed=1
                ;;
        esac
        if [ -e "$secret_directory" ]; then
            printf 'temporary secret directory remains: %s\n' "$secret_directory" >&2
            cleanup_failed=1
        fi
    fi

    if [ "${snapshots_taken:-0}" -eq 1 ]; then
        growthos_containers_after=$(snapshot_growthos_containers)
        growthos_volumes_after=$(snapshot_growthos_volumes)
        growthos_networks_after=$(snapshot_growthos_networks)
        if [ "$growthos_containers_after" != "$growthos_containers_before" ] ||
           [ "$growthos_volumes_after" != "$growthos_volumes_before" ] ||
           [ "$growthos_networks_after" != "$growthos_networks_before" ]; then
            printf '%s\n' 'long-lived growthos Docker resource snapshot changed during Lesson 28 acceptance' >&2
            cleanup_failed=1
        fi
    fi
    if [ "${image_added:-0}" -eq 1 ]; then
        printf '%s\n' "$mysql_image was not present initially and is retained as a reusable dependency"
    fi
    if [ "$cleanup_failed" -ne 0 ]; then
        exit 1
    fi
    exit "$requested_status"
}
trap 'cleanup $?' EXIT
trap 'cleanup 129' HUP
trap 'cleanup 130' INT
trap 'cleanup 143' TERM

umask 077
generate_secret() {
    secret_target=$1
    openssl rand -hex 32 > "$secret_target"
    chmod 0600 "$secret_target"
    generated_secret=$(tr -d '\r\n' < "$secret_target")
    case "$generated_secret" in
        *[!0-9a-f]*|'')
            printf 'generated secret has an invalid format: %s\n' "$secret_target" >&2
            exit 1
            ;;
    esac
    if [ "${#generated_secret}" -ne 64 ]; then
        printf 'generated secret has an invalid length: %s\n' "$secret_target" >&2
        exit 1
    fi
}

generate_secret "$root_secret_file"
root_password=$generated_secret
generate_secret "$migration_secret_file"
migration_password=$generated_secret
generate_secret "$api_secret_file"
api_password=$generated_secret
generate_secret "$graph_secret_file"
graph_password=$generated_secret
unset generated_secret
{
    printf '%s\n' \
        '[client]' \
        'user=root' \
        "password=$root_password" \
        'protocol=tcp' \
        'host=127.0.0.1' \
        'port=3306'
} > "$root_client_file"
chmod 0600 "$root_client_file"

if ! docker image inspect "$mysql_image" >/dev/null 2>&1; then
    docker image pull "$mysql_image" >/dev/null
    image_added=1
fi

container_id=$(docker container run --detach --rm \
    --name "$container_name" \
    --label "$label_key=$label_value" \
    --publish '127.0.0.1::3306' \
    --tmpfs '/var/lib/mysql:rw,noexec,nosuid,size=512m' \
    --mount "type=bind,src=$secret_directory,dst=/run/lesson28-secrets,readonly" \
    --env 'MYSQL_ROOT_PASSWORD_FILE=/run/lesson28-secrets/root.password' \
    "$mysql_image" \
    --mandatory-roles=)
case "$container_id" in
    *[!0-9a-f]*|'')
        printf '%s\n' 'docker returned an invalid container ID' >&2
        exit 1
        ;;
esac
if [ "${#container_id}" -ne 64 ]; then
    printf '%s\n' 'docker returned a container ID with an invalid length' >&2
    exit 1
fi

ready=0
attempt=0
while [ "$attempt" -lt 120 ]; do
    if docker container exec "$container_id" mysqladmin \
        --defaults-extra-file=/run/lesson28-secrets/root-client.cnf \
        ping --silent >/dev/null 2>&1; then
        ready=1
        break
    fi
    if ! docker container inspect "$container_id" >/dev/null 2>&1; then
        break
    fi
    attempt=$((attempt + 1))
    sleep 1
done
if [ "$ready" -ne 1 ]; then
    docker container logs --tail 80 "$container_id" >&2 || true
    printf '%s\n' 'disposable MySQL did not become ready' >&2
    exit 1
fi

mysql_address=$(docker container port "$container_id" 3306/tcp | sed -n 's/^127\.0\.0\.1:\([0-9][0-9]*\)$/127.0.0.1:\1/p')
case "$mysql_address" in
    127.0.0.1:[0-9]*) ;;
    *)
        printf 'could not resolve the loopback-only MySQL port: %s\n' "$mysql_address" >&2
        exit 1
        ;;
esac

docker container exec -i "$container_id" mysql \
    --defaults-extra-file=/run/lesson28-secrets/root-client.cnf <<SQL
SET GLOBAL mandatory_roles = '';
CREATE DATABASE \`$database_name\` CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci;
CREATE USER '$migration_user'@'%' IDENTIFIED BY '$migration_password';
CREATE USER '$api_user'@'%' IDENTIFIED BY '$api_password';
CREATE USER '$graph_user'@'%' IDENTIFIED BY '$graph_password';
GRANT ALL PRIVILEGES ON \`$database_name\`.* TO '$migration_user'@'%';
SQL

unset GROWTHOS_MYSQL_MIGRATION_PASSWORD_FILE GROWTHOS_MYSQL_TLS_CA_FILE
export GROWTHOS_ENVIRONMENT='test'
export GROWTHOS_MYSQL_ADDRESS="$mysql_address"
export GROWTHOS_MYSQL_DATABASE="$database_name"
export GROWTHOS_MYSQL_TLS_MODE='disabled'
export GROWTHOS_MYSQL_MIGRATION_USER="$migration_user"
export GROWTHOS_MYSQL_MIGRATION_PASSWORD="$migration_password"
export GROWTHOS_MYSQL_MIGRATION_READ_TIMEOUT='35s'
export GROWTHOS_MYSQL_MIGRATION_STATEMENT_TIMEOUT='30s'
export GROWTHOS_MYSQL_MIGRATION_LOCK_TIMEOUT='40s'

(cd "$repository_root" && go run ./cmd/growth-migrate up)

docker container exec -i "$container_id" mysql \
    --defaults-extra-file=/run/lesson28-secrets/root-client.cnf <<SQL
GRANT SELECT, INSERT ON \`$database_name\`.\`lottery_strategy\` TO '$api_user'@'%';
GRANT SELECT, INSERT ON \`$database_name\`.\`lottery_strategy_award\` TO '$api_user'@'%';
GRANT SELECT, INSERT ON \`$database_name\`.\`lottery_strategy_routing_graph\` TO '$graph_user'@'%';
GRANT SELECT, INSERT ON \`$database_name\`.\`lottery_strategy_routing_node\` TO '$graph_user'@'%';
GRANT SELECT, INSERT ON \`$database_name\`.\`lottery_strategy_routing_edge\` TO '$graph_user'@'%';
SQL

export GROWTHOS_TEST_MYSQL_ALLOW_SCHEMA_CHANGES='lesson-19-isolated-schema'
export GROWTHOS_TEST_MYSQL_ALLOW_REPOSITORY_WRITES='lesson-19-isolated-repository'
export GROWTHOS_TEST_MYSQL_ALLOW_RULE_GRAPH_WRITES='lesson-28-isolated-rule-graph'
export GROWTHOS_TEST_MYSQL_API_ADDRESS="$mysql_address"
export GROWTHOS_TEST_MYSQL_API_DATABASE="$database_name"
export GROWTHOS_TEST_MYSQL_API_USER="$api_user"
export GROWTHOS_TEST_MYSQL_API_PASSWORD="$api_password"
export GROWTHOS_TEST_MYSQL_API_TLS_MODE='disabled'
export GROWTHOS_TEST_MYSQL_MIGRATION_ADDRESS="$mysql_address"
export GROWTHOS_TEST_MYSQL_MIGRATION_DATABASE="$database_name"
export GROWTHOS_TEST_MYSQL_MIGRATION_USER="$migration_user"
export GROWTHOS_TEST_MYSQL_MIGRATION_PASSWORD="$migration_password"
export GROWTHOS_TEST_MYSQL_MIGRATION_TLS_MODE='disabled'
export GROWTHOS_TEST_MYSQL_RULE_GRAPH_ADDRESS="$mysql_address"
export GROWTHOS_TEST_MYSQL_RULE_GRAPH_DATABASE="$database_name"
export GROWTHOS_TEST_MYSQL_RULE_GRAPH_USER="$graph_user"
export GROWTHOS_TEST_MYSQL_RULE_GRAPH_PASSWORD="$graph_password"
export GROWTHOS_TEST_MYSQL_RULE_GRAPH_TLS_MODE='disabled'

cd "$repository_root"
go test -v -count=1 -p=1 -run 'Integration$' \
    ./internal/infrastructure/mysql \
    ./internal/infrastructure/migration \
    ./migrations \
    ./internal/lottery/adapter/mysqlrepo

printf '%s\n' 'Lesson 28 disposable MySQL 8.4.11 acceptance passed'
cleanup 0
