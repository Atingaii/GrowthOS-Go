#!/bin/sh
set -eu

confirmation=${GROWTHOS_LESSON30_MYSQL_ACCEPTANCE:-}
if [ "$confirmation" != 'run-disposable-mysql-8.4.11' ]; then
    printf '%s\n' 'refusing Lesson 30 acceptance without GROWTHOS_LESSON30_MYSQL_ACCEPTANCE=run-disposable-mysql-8.4.11' >&2
    exit 1
fi

repository_root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
mysql_image='mysql:8.4.11'
label_key='com.growthos.acceptance.lesson30'
compose_project_label='com.docker.compose.project=growthos'

for required_command in docker openssl mktemp sed tr cut cmp go; do
    if ! command -v "$required_command" >/dev/null 2>&1; then
        printf '%s is required for Lesson 30 MySQL acceptance\n' "$required_command" >&2
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
        printf '%s\n' 'openssl returned an invalid Lesson 30 suffix' >&2
        exit 1
        ;;
esac
if [ "${#random_suffix}" -ne 24 ]; then
    printf '%s\n' 'openssl returned a Lesson 30 suffix with an invalid length' >&2
    exit 1
fi
short_suffix=$(printf '%s' "$random_suffix" | cut -c 1-12)
container_name="growthos-lesson30-mysql-$random_suffix"
label_value="run-$random_suffix"
database_name="growthos_l30_$short_suffix"
migration_user="l30_migr_$short_suffix"
snapshot_user="l30_snap_$short_suffix"
marketing_user="l30_mark_$short_suffix"

case "$container_name" in
    growthos-lesson30-mysql-[0-9a-f]*) ;;
    *) printf '%s\n' 'refusing an invalid Lesson 30 container name' >&2; exit 1 ;;
esac
case "$label_value" in
    run-[0-9a-f]*) ;;
    *) printf '%s\n' 'refusing an invalid Lesson 30 label value' >&2; exit 1 ;;
esac
case "$database_name:$migration_user:$snapshot_user:$marketing_user" in
    *[!a-zA-Z0-9_:]*)
        printf '%s\n' 'refusing invalid generated MySQL identifiers' >&2
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
    ids=$(docker container ls -aq --filter "label=$compose_project_label")
    if [ -z "$ids" ]; then
        return 0
    fi
    for id in $ids; do
        docker container inspect --format '{{.Id}}|{{.Name}}|{{.Config.Image}}|{{.State.Status}}|{{.RestartCount}}' "$id"
    done | LC_ALL=C sort
}

snapshot_growthos_volumes() {
    names=$(docker volume ls -q --filter "label=$compose_project_label")
    if [ -z "$names" ]; then
        return 0
    fi
    for name in $names; do
        docker volume inspect --format '{{.Name}}|{{.Driver}}|{{.Mountpoint}}' "$name"
    done | LC_ALL=C sort
}

snapshot_growthos_networks() {
    ids=$(docker network ls -q --filter "label=$compose_project_label")
    if [ -z "$ids" ]; then
        return 0
    fi
    for id in $ids; do
        docker network inspect --format '{{.Id}}|{{.Name}}|{{.Driver}}|{{.Scope}}|{{.Internal}}' "$id"
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
    *) printf 'TMPDIR must be absolute: %s\n' "$temporary_root" >&2; exit 1 ;;
esac
secret_directory=$(mktemp -d "$temporary_root/growthos-lesson30-acceptance.XXXXXX")
secret_basename=${secret_directory##*/}
case "$secret_basename" in
    growthos-lesson30-acceptance.[A-Za-z0-9][A-Za-z0-9][A-Za-z0-9][A-Za-z0-9][A-Za-z0-9][A-Za-z0-9]) ;;
    *) printf 'mktemp returned an unexpected directory: %s\n' "$secret_directory" >&2; exit 1 ;;
esac
case "$secret_directory" in
    "$temporary_root"/growthos-lesson30-acceptance.*) ;;
    *) printf 'mktemp escaped the expected root: %s\n' "$secret_directory" >&2; exit 1 ;;
esac
case "$secret_directory" in
    *,*) printf '%s\n' 'temporary secret path must not contain a comma' >&2; exit 1 ;;
esac
chmod 0700 "$secret_directory"

root_secret_file="$secret_directory/root.password"
migration_secret_file="$secret_directory/migration.password"
snapshot_secret_file="$secret_directory/snapshot.password"
marketing_secret_file="$secret_directory/marketing.password"
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
                    printf 'failed to stop owned Lesson 30 container %s\n' "$container_id" >&2
                    cleanup_failed=1
                fi
            fi
            attempt=0
            while docker container inspect "$container_id" >/dev/null 2>&1 && [ "$attempt" -lt 30 ]; do
                attempt=$((attempt + 1))
                sleep 1
            done
        else
            printf 'refusing to stop container %s after exact identity changed\n' "$container_id" >&2
            cleanup_failed=1
        fi
    fi
    if docker container inspect "$container_name" >/dev/null 2>&1; then
        printf 'Lesson 30 container remains: %s\n' "$container_name" >&2
        cleanup_failed=1
    fi
    if [ -n "$(docker container ls -aq --filter "label=$label_key=$label_value")" ]; then
        printf 'Lesson 30 labeled container remains: %s=%s\n' "$label_key" "$label_value" >&2
        cleanup_failed=1
    fi

    if [ -n "${secret_directory:-}" ]; then
        case "$secret_directory" in
            "$temporary_root"/growthos-lesson30-acceptance.*)
                rm -f -- \
                    "$root_secret_file" \
                    "$migration_secret_file" \
                    "$snapshot_secret_file" \
                    "$marketing_secret_file" \
                    "$root_client_file"
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
            printf '%s\n' 'long-lived growthos Docker resources changed during Lesson 30 acceptance' >&2
            cleanup_failed=1
        fi
    fi
    if [ "${image_added:-0}" -eq 1 ]; then
        printf '%s\n' "$mysql_image was newly pulled and is retained as a reusable dependency"
    fi
    if [ "$cleanup_failed" -ne 0 ]; then
        exit 1
    fi
    printf 'Lesson 30 cleanup verified: container=0 label=0 secrets=0 long-lived-growthos=unchanged owned-name=%s owned-label=%s\n' "$container_name" "$label_value"
    exit "$requested_status"
}
trap 'cleanup $?' EXIT
trap 'cleanup 129' HUP
trap 'cleanup 130' INT
trap 'cleanup 143' TERM

umask 077
generate_secret() {
    target=$1
    openssl rand -hex 32 > "$target"
    chmod 0600 "$target"
    generated=$(tr -d '\r\n' < "$target")
    case "$generated" in
        *[!0-9a-f]*|'') printf 'invalid generated secret: %s\n' "$target" >&2; exit 1 ;;
    esac
    if [ "${#generated}" -ne 64 ]; then
        printf 'generated secret has invalid length: %s\n' "$target" >&2
        exit 1
    fi
}

generate_secret "$root_secret_file"
root_password=$generated
generate_secret "$migration_secret_file"
migration_password=$generated
generate_secret "$snapshot_secret_file"
snapshot_password=$generated
generate_secret "$marketing_secret_file"
marketing_password=$generated
unset generated
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
    --mount "type=bind,src=$secret_directory,dst=/run/lesson30-secrets,readonly" \
    --env 'MYSQL_ROOT_PASSWORD_FILE=/run/lesson30-secrets/root.password' \
    "$mysql_image" \
    --mandatory-roles=)
case "$container_id" in
    *[!0-9a-f]*|'') printf '%s\n' 'docker returned an invalid container ID' >&2; exit 1 ;;
esac
if [ "${#container_id}" -ne 64 ]; then
    printf '%s\n' 'docker returned a container ID with invalid length' >&2
    exit 1
fi

ready=0
attempt=0
while [ "$attempt" -lt 120 ]; do
    if docker container exec "$container_id" mysqladmin \
        --defaults-extra-file=/run/lesson30-secrets/root-client.cnf \
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
    docker container logs --tail 100 "$container_id" >&2 || true
    printf '%s\n' 'disposable MySQL did not become ready' >&2
    exit 1
fi

mysql_version=$(docker container exec "$container_id" mysql \
    --defaults-extra-file=/run/lesson30-secrets/root-client.cnf \
    --batch --skip-column-names \
    --execute='SELECT VERSION()')
case "$mysql_version" in
    8.4.11|8.4.11-*) ;;
    *) printf 'MySQL version = %s, want exact 8.4.11 image line\n' "$mysql_version" >&2; exit 1 ;;
esac

mysql_address=$(docker container port "$container_id" 3306/tcp | sed -n 's/^127\.0\.0\.1:\([0-9][0-9]*\)$/127.0.0.1:\1/p')
case "$mysql_address" in
    127.0.0.1:[0-9]*) ;;
    *) printf 'could not resolve loopback-only MySQL port: %s\n' "$mysql_address" >&2; exit 1 ;;
esac

docker container exec -i "$container_id" mysql \
    --defaults-extra-file=/run/lesson30-secrets/root-client.cnf <<SQL
SET GLOBAL mandatory_roles = '';
CREATE DATABASE $database_name CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci;
CREATE USER '$migration_user'@'%' IDENTIFIED BY '$migration_password';
CREATE USER '$snapshot_user'@'%' IDENTIFIED BY '$snapshot_password';
CREATE USER '$marketing_user'@'%' IDENTIFIED BY '$marketing_password';
GRANT ALL PRIVILEGES ON $database_name.* TO '$migration_user'@'%';
SQL

unset GROWTHOS_TEST_MYSQL_MIGRATION_ADDRESS \
    GROWTHOS_TEST_MYSQL_MIGRATION_DATABASE \
    GROWTHOS_TEST_MYSQL_MIGRATION_USER \
    GROWTHOS_TEST_MYSQL_MIGRATION_PASSWORD \
    GROWTHOS_TEST_MYSQL_SNAPSHOT_ADDRESS \
    GROWTHOS_TEST_MYSQL_SNAPSHOT_DATABASE \
    GROWTHOS_TEST_MYSQL_SNAPSHOT_USER \
    GROWTHOS_TEST_MYSQL_SNAPSHOT_PASSWORD \
    GROWTHOS_TEST_MYSQL_MARKETING_ADDRESS \
    GROWTHOS_TEST_MYSQL_MARKETING_DATABASE \
    GROWTHOS_TEST_MYSQL_MARKETING_USER \
    GROWTHOS_TEST_MYSQL_MARKETING_PASSWORD
export GROWTHOS_TEST_MYSQL_ALLOW_SCHEMA_CHANGES='lesson-30-isolated-schema'
export GROWTHOS_TEST_MYSQL_MIGRATION_ADDRESS="$mysql_address"
export GROWTHOS_TEST_MYSQL_MIGRATION_DATABASE="$database_name"
export GROWTHOS_TEST_MYSQL_MIGRATION_USER="$migration_user"
export GROWTHOS_TEST_MYSQL_MIGRATION_PASSWORD="$migration_password"
export GROWTHOS_TEST_MYSQL_MIGRATION_TLS_MODE='disabled'

cd "$repository_root"
printf 'Lesson 30 MySQL acceptance: image=%s version=%s container=%s\n' "$mysql_image" "$mysql_version" "$container_name"
go test -v -count=1 -run '^TestActivityPublicationSchemaMySQLIntegration$' ./migrations
go test -count=1 -run '^(TestInitialLotteryMigrationsRemainImmutable|TestRoutingGraphMigrationsRemainImmutable|TestStrategySnapshotMigrationsRemainImmutable|TestActivityPublicationMigrationsRemainImmutable|TestEmbeddedLotteryMigrationInventoryEndsAtVersionEleven)$' ./migrations

docker container exec -i "$container_id" mysql \
    --defaults-extra-file=/run/lesson30-secrets/root-client.cnf <<SQL
GRANT SELECT, INSERT ON $database_name.lottery_strategy_snapshot TO '$snapshot_user'@'%';
GRANT SELECT, INSERT ON $database_name.lottery_strategy_snapshot_award TO '$snapshot_user'@'%';
GRANT SELECT, INSERT, UPDATE ON $database_name.marketing_activity TO '$marketing_user'@'%';
GRANT SELECT, INSERT ON $database_name.marketing_activity_publication TO '$marketing_user'@'%';
GRANT SELECT, INSERT ON $database_name.marketing_activity_publication_strategy TO '$marketing_user'@'%';
SQL

export GROWTHOS_TEST_MYSQL_ALLOW_SNAPSHOT_WRITES='lesson-30-isolated-strategy-snapshot'
export GROWTHOS_TEST_MYSQL_SNAPSHOT_ADDRESS="$mysql_address"
export GROWTHOS_TEST_MYSQL_SNAPSHOT_DATABASE="$database_name"
export GROWTHOS_TEST_MYSQL_SNAPSHOT_USER="$snapshot_user"
export GROWTHOS_TEST_MYSQL_SNAPSHOT_PASSWORD="$snapshot_password"
export GROWTHOS_TEST_MYSQL_SNAPSHOT_TLS_MODE='disabled'
export GROWTHOS_TEST_MYSQL_ALLOW_ACTIVITY_WRITES='lesson-30-isolated-activity'
export GROWTHOS_TEST_MYSQL_MARKETING_ADDRESS="$mysql_address"
export GROWTHOS_TEST_MYSQL_MARKETING_DATABASE="$database_name"
export GROWTHOS_TEST_MYSQL_MARKETING_USER="$marketing_user"
export GROWTHOS_TEST_MYSQL_MARKETING_PASSWORD="$marketing_password"
export GROWTHOS_TEST_MYSQL_MARKETING_TLS_MODE='disabled'

go test -v -count=1 -run '^TestStrategySnapshotRepositoryMySQLIntegration$' ./internal/lottery/adapter/mysqlrepo
go test -v -count=1 -run '^TestActivityRepositoryMySQLIntegration$' ./internal/marketing/adapter/mysqlrepo
go test -count=1 ./internal/marketing/adapter/lotteryconfig ./internal/marketing/application

final_status=$(docker container exec "$container_id" mysql \
    --defaults-extra-file=/run/lesson30-secrets/root-client.cnf \
    --batch --skip-column-names "$database_name" \
    --execute='SELECT CONCAT(version, CHAR(58), dirty) FROM schema_migrations')
if [ "$final_status" != '11:0' ]; then
    printf 'final migration status = %s, want 11:0\n' "$final_status" >&2
    exit 1
fi
remaining_probes=$(docker container exec "$container_id" mysql \
    --defaults-extra-file=/run/lesson30-secrets/root-client.cnf \
    --batch --skip-column-names "$database_name" \
    --execute="SELECT COUNT(*) FROM information_schema.table_constraints WHERE constraint_schema = DATABASE() AND constraint_name LIKE 'chk_l30\\_%'")
if [ "$remaining_probes" != '0' ]; then
    printf 'temporary Lesson 30 CHECK probes remain: %s\n' "$remaining_probes" >&2
    exit 1
fi

printf 'Lesson 30 database postconditions: schema_migrations=%s temporary-check-probes=%s\n' "$final_status" "$remaining_probes"
printf '%s\n' 'Lesson 30 MySQL acceptance passed: v5 seed+fingerprint -> v11, no_change, dirty fail-closed, schema, grants, snapshots, Activity lifecycle, CAS/RR/rollback/cleanup'
