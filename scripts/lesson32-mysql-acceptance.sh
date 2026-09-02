#!/bin/sh
set -eu

confirmation=${GROWTHOS_LESSON32_MYSQL_ACCEPTANCE:-}
if [ "$confirmation" != 'run-disposable-mysql-8.4.11' ]; then
    printf '%s\n' 'refusing Lesson 32 acceptance without GROWTHOS_LESSON32_MYSQL_ACCEPTANCE=run-disposable-mysql-8.4.11' >&2
    exit 1
fi

repository_root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
mysql_image='mysql:8.4.11'
label_key='com.growthos.acceptance.lesson32'
compose_project_label='com.docker.compose.project=growthos'

for required_command in docker openssl mktemp sed tr cut cmp wc dd unlink sort go; do
    if ! command -v "$required_command" >/dev/null 2>&1; then
        printf '%s is required for Lesson 32 MySQL acceptance\n' "$required_command" >&2
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
        printf '%s\n' 'openssl returned an invalid Lesson 32 suffix' >&2
        exit 1
        ;;
esac
if [ "${#random_suffix}" -ne 24 ]; then
    printf '%s\n' 'openssl returned a Lesson 32 suffix with an invalid length' >&2
    exit 1
fi
short_suffix=$(printf '%s' "$random_suffix" | cut -c 1-12)
container_name="growthos-lesson32-mysql-$random_suffix"
label_value="run-$random_suffix"
database_name="growthos_l32_$short_suffix"
migration_user="l32_migr_$short_suffix"
runtime_user="l32_runtime_$short_suffix"

case "$container_name" in
    growthos-lesson32-mysql-[0-9a-f]*) ;;
    *) printf '%s\n' 'refusing an invalid Lesson 32 container name' >&2; exit 1 ;;
esac
case "$label_value" in
    run-[0-9a-f]*) ;;
    *) printf '%s\n' 'refusing an invalid Lesson 32 label value' >&2; exit 1 ;;
esac
case "$database_name:$migration_user:$runtime_user" in
    *[!a-zA-Z0-9_:]*)
        printf '%s\n' 'refusing invalid generated MySQL identifiers' >&2
        exit 1
        ;;
esac
if [ "$migration_user" = "$runtime_user" ]; then
    printf '%s\n' 'generated migration and runtime identities must differ' >&2
    exit 1
fi
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
secret_directory=$(mktemp -d "$temporary_root/growthos-lesson32-acceptance.XXXXXX")
secret_basename=${secret_directory##*/}
case "$secret_basename" in
    growthos-lesson32-acceptance.[A-Za-z0-9][A-Za-z0-9][A-Za-z0-9][A-Za-z0-9][A-Za-z0-9][A-Za-z0-9]) ;;
    *) printf 'mktemp returned an unexpected directory: %s\n' "$secret_directory" >&2; exit 1 ;;
esac
case "$secret_directory" in
    "$temporary_root"/growthos-lesson32-acceptance.*) ;;
    *) printf 'mktemp escaped the expected root: %s\n' "$secret_directory" >&2; exit 1 ;;
esac
case "$secret_directory" in
    *,*) printf '%s\n' 'temporary secret path must not contain a comma' >&2; exit 1 ;;
esac
chmod 0700 "$secret_directory"

root_secret_file="$secret_directory/root.password"
migration_secret_file="$secret_directory/migration.password"
runtime_secret_file="$secret_directory/runtime.password"
root_client_file="$secret_directory/root-client.cnf"
container_id=
image_added=0

overwrite_and_unlink_secret() {
    secret_target=$1
    if [ ! -e "$secret_target" ] && [ ! -L "$secret_target" ]; then
        return 0
    fi
    case "$secret_target" in
        "$secret_directory"/*) ;;
        *)
            printf 'refusing to clean secret outside the owned directory: %s\n' "$secret_target" >&2
            cleanup_failed=1
            return 0
            ;;
    esac
    if [ -L "$secret_target" ] || [ ! -f "$secret_target" ]; then
        printf 'refusing to overwrite a non-regular secret path: %s\n' "$secret_target" >&2
        cleanup_failed=1
        return 0
    fi
    secret_size=$(wc -c < "$secret_target" | tr -d '[:space:]')
    case "$secret_size" in
        ''|*[!0-9]*)
            printf 'could not determine secret size: %s\n' "$secret_target" >&2
            cleanup_failed=1
            return 0
            ;;
    esac
    if [ "$secret_size" -gt 0 ] && ! dd if=/dev/zero of="$secret_target" bs=1 count="$secret_size" conv=notrunc 2>/dev/null; then
        printf 'failed to overwrite secret file: %s\n' "$secret_target" >&2
        cleanup_failed=1
        return 0
    fi
    if ! unlink "$secret_target"; then
        printf 'failed to unlink secret file: %s\n' "$secret_target" >&2
        cleanup_failed=1
    fi
}

cleanup() {
    requested_status=$1
    trap - EXIT HUP INT TERM
    set +e
    cleanup_failed=0

    cleanup_container_id=${container_id:-}
    if [ -z "$cleanup_container_id" ] && docker container inspect "$container_name" >/dev/null 2>&1; then
        cleanup_container_id=$(docker container inspect --format '{{.Id}}' "$container_name" 2>/dev/null)
    fi
    if [ -n "$cleanup_container_id" ] && docker container inspect "$cleanup_container_id" >/dev/null 2>&1; then
        actual_name=$(docker container inspect --format '{{.Name}}' "$cleanup_container_id" 2>/dev/null)
        actual_name=${actual_name#/}
        actual_label=$(docker container inspect --format "{{index .Config.Labels \"$label_key\"}}" "$cleanup_container_id" 2>/dev/null)
        if [ "$actual_name" = "$container_name" ] && [ "$actual_label" = "$label_value" ]; then
            if ! docker container stop --time 15 "$cleanup_container_id" >/dev/null; then
                if docker container inspect "$cleanup_container_id" >/dev/null 2>&1; then
                    printf 'failed to stop owned Lesson 32 container %s\n' "$cleanup_container_id" >&2
                    cleanup_failed=1
                fi
            fi
            attempt=0
            while docker container inspect "$cleanup_container_id" >/dev/null 2>&1 && [ "$attempt" -lt 30 ]; do
                attempt=$((attempt + 1))
                sleep 1
            done
        else
            printf 'refusing to stop container %s after exact identity changed\n' "$cleanup_container_id" >&2
            cleanup_failed=1
        fi
    fi
    if docker container inspect "$container_name" >/dev/null 2>&1; then
        printf 'Lesson 32 container remains: %s\n' "$container_name" >&2
        cleanup_failed=1
    fi
    if [ -n "$(docker container ls -aq --filter "label=$label_key=$label_value")" ]; then
        printf 'Lesson 32 labeled container remains: %s=%s\n' "$label_key" "$label_value" >&2
        cleanup_failed=1
    fi

    if [ -n "${secret_directory:-}" ]; then
        case "$secret_directory" in
            "$temporary_root"/growthos-lesson32-acceptance.*)
                overwrite_and_unlink_secret "$root_secret_file"
                overwrite_and_unlink_secret "$migration_secret_file"
                overwrite_and_unlink_secret "$runtime_secret_file"
                overwrite_and_unlink_secret "$root_client_file"
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
            printf '%s\n' 'long-lived growthos Docker resources changed during Lesson 32 acceptance' >&2
            cleanup_failed=1
        fi
    fi
    if [ "${image_added:-0}" -eq 1 ]; then
        printf '%s\n' "$mysql_image was newly pulled and is retained as a reusable dependency"
    fi
    printf '%s\n' 'Lesson 32 secret cleanup overwrote then unlinked owned files; SSD, CoW filesystem, snapshots, and controller remapping can prevent physical-erasure guarantees.'
    if [ "$cleanup_failed" -ne 0 ]; then
        exit 1
    fi
    printf 'Lesson 32 cleanup verified: container=0 label=0 secrets=0 long-lived-growthos=unchanged owned-name=%s owned-label=%s\n' "$container_name" "$label_value"
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
    generated=$(tr -d '\r\n' < "$secret_target")
    case "$generated" in
        *[!0-9a-f]*|'') printf 'invalid generated secret: %s\n' "$secret_target" >&2; exit 1 ;;
    esac
    if [ "${#generated}" -ne 64 ]; then
        printf 'generated secret has invalid length: %s\n' "$secret_target" >&2
        exit 1
    fi
}

generate_secret "$root_secret_file"
root_password=$generated
generate_secret "$migration_secret_file"
migration_password=$generated
generate_secret "$runtime_secret_file"
runtime_password=$generated
unset generated
if cmp -s "$root_secret_file" "$migration_secret_file" ||
   cmp -s "$root_secret_file" "$runtime_secret_file" ||
   cmp -s "$migration_secret_file" "$runtime_secret_file"; then
    printf '%s\n' 'generated MySQL credentials unexpectedly collided' >&2
    exit 1
fi
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
    --mount "type=bind,src=$secret_directory,dst=/run/lesson32-secrets,readonly" \
    --env 'MYSQL_ROOT_PASSWORD_FILE=/run/lesson32-secrets/root.password' \
    "$mysql_image" \
    --mandatory-roles=)
case "$container_id" in
    *[!0-9a-f]*|'') printf '%s\n' 'docker returned an invalid container ID' >&2; exit 1 ;;
esac
if [ "${#container_id}" -ne 64 ]; then
    printf '%s\n' 'docker returned a container ID with an invalid length' >&2
    exit 1
fi
actual_name=$(docker container inspect --format '{{.Name}}' "$container_id")
actual_name=${actual_name#/}
actual_label=$(docker container inspect --format "{{index .Config.Labels \"$label_key\"}}" "$container_id")
if [ "$actual_name" != "$container_name" ] || [ "$actual_label" != "$label_value" ]; then
    printf '%s\n' 'disposable MySQL container identity does not match its reserved name and label' >&2
    exit 1
fi

ready=0
attempt=0
while [ "$attempt" -lt 120 ]; do
    if docker container exec "$container_id" mysqladmin \
        --defaults-extra-file=/run/lesson32-secrets/root-client.cnf \
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
    --defaults-extra-file=/run/lesson32-secrets/root-client.cnf \
    --batch --skip-column-names \
    --execute='SELECT VERSION()')
case "$mysql_version" in
    8.4.11|8.4.11-*) ;;
    *) printf 'MySQL version = %s, want exact 8.4.11 image line\n' "$mysql_version" >&2; exit 1 ;;
esac

mysql_address=$(docker container port "$container_id" 3306/tcp | sed -n 's/^127\.0\.0\.1:\([0-9][0-9]*\)$/127.0.0.1:\1/p')
mysql_port=${mysql_address#127.0.0.1:}
case "$mysql_port" in
    ''|*[!0-9]*) printf 'could not resolve one loopback-only MySQL port: %s\n' "$mysql_address" >&2; exit 1 ;;
esac

docker container exec -i "$container_id" mysql \
    --defaults-extra-file=/run/lesson32-secrets/root-client.cnf <<SQL
SET GLOBAL mandatory_roles = '';
CREATE DATABASE \`$database_name\` CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci;
CREATE USER '$migration_user'@'%' IDENTIFIED BY '$migration_password';
CREATE USER '$runtime_user'@'%' IDENTIFIED BY '$runtime_password';
GRANT ALL PRIVILEGES ON \`$database_name\`.* TO '$migration_user'@'%';
SQL

unset GROWTHOS_TEST_MYSQL_ALLOW_IDENTITY_SCHEMA_CHANGES \
    GROWTHOS_TEST_MYSQL_IDENTITY_MIGRATION_ADDRESS \
    GROWTHOS_TEST_MYSQL_IDENTITY_MIGRATION_DATABASE \
    GROWTHOS_TEST_MYSQL_IDENTITY_MIGRATION_USER \
    GROWTHOS_TEST_MYSQL_IDENTITY_MIGRATION_PASSWORD \
    GROWTHOS_TEST_MYSQL_IDENTITY_MIGRATION_TLS_MODE \
    GROWTHOS_TEST_MYSQL_IDENTITY_MIGRATION_TLS_CA_FILE \
    GROWTHOS_TEST_IDENTITY_MYSQL_ACCEPTANCE \
    GROWTHOS_TEST_IDENTITY_MYSQL_ADMIN_DSN \
    GROWTHOS_TEST_IDENTITY_MYSQL_RUNTIME_DSN
export GROWTHOS_TEST_MYSQL_ALLOW_IDENTITY_SCHEMA_CHANGES='lesson-32-isolated-schema'
export GROWTHOS_TEST_MYSQL_IDENTITY_MIGRATION_ADDRESS="$mysql_address"
export GROWTHOS_TEST_MYSQL_IDENTITY_MIGRATION_DATABASE="$database_name"
export GROWTHOS_TEST_MYSQL_IDENTITY_MIGRATION_USER="$migration_user"
export GROWTHOS_TEST_MYSQL_IDENTITY_MIGRATION_PASSWORD="$migration_password"
export GROWTHOS_TEST_MYSQL_IDENTITY_MIGRATION_TLS_MODE='disabled'

cd "$repository_root"
printf 'Lesson 32 MySQL acceptance: image=%s version=%s container=%s\n' "$mysql_image" "$mysql_version" "$container_name"
go test -v -count=1 -run '^TestIdentitySchemaMySQLIntegration$' ./migrations

schema_tables_after_test=$(docker container exec "$container_id" mysql \
    --defaults-extra-file=/run/lesson32-secrets/root-client.cnf \
    --batch --skip-column-names "$database_name" \
    --execute='SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE()')
if [ "$schema_tables_after_test" != '0' ]; then
    printf 'Identity schema integration left %s tables, want an empty database\n' "$schema_tables_after_test" >&2
    exit 1
fi

go test -count=1 -run '^(TestInitialLotteryMigrationsRemainImmutable|TestRoutingGraphMigrationsRemainImmutable|TestStrategySnapshotMigrationsRemainImmutable|TestActivityPublicationMigrationsRemainImmutable|TestIdentityMigrationsRemainImmutable|TestEmbeddedMigrationInventoryEndsAtVersionFourteen)$' ./migrations

unset GROWTHOS_ENVIRONMENT \
    GROWTHOS_LOG_LEVEL \
    GROWTHOS_LOG_FORMAT \
    GROWTHOS_MYSQL_ADDRESS \
    GROWTHOS_MYSQL_DATABASE \
    GROWTHOS_MYSQL_TLS_MODE \
    GROWTHOS_MYSQL_TLS_CA_FILE \
    GROWTHOS_MYSQL_CONNECT_TIMEOUT \
    GROWTHOS_MYSQL_WRITE_TIMEOUT \
    GROWTHOS_MYSQL_MIGRATION_USER \
    GROWTHOS_MYSQL_MIGRATION_PASSWORD \
    GROWTHOS_MYSQL_MIGRATION_PASSWORD_FILE \
    GROWTHOS_MYSQL_MIGRATION_READ_TIMEOUT \
    GROWTHOS_MYSQL_MIGRATION_STATEMENT_TIMEOUT \
    GROWTHOS_MYSQL_MIGRATION_LOCK_TIMEOUT
export GROWTHOS_ENVIRONMENT='test'
export GROWTHOS_LOG_LEVEL='info'
export GROWTHOS_LOG_FORMAT='json'
export GROWTHOS_MYSQL_ADDRESS="$mysql_address"
export GROWTHOS_MYSQL_DATABASE="$database_name"
export GROWTHOS_MYSQL_TLS_MODE='disabled'
export GROWTHOS_MYSQL_MIGRATION_USER="$migration_user"
export GROWTHOS_MYSQL_MIGRATION_PASSWORD_FILE="$migration_secret_file"
go run ./cmd/growth-migrate up
go run ./cmd/growth-migrate status

docker container exec -i "$container_id" mysql \
    --defaults-extra-file=/run/lesson32-secrets/root-client.cnf <<SQL
GRANT SELECT ON \`$database_name\`.\`identity_workforce_account\` TO '$runtime_user'@'%';
GRANT UPDATE (\`updated_at\`) ON \`$database_name\`.\`identity_workforce_account\` TO '$runtime_user'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON \`$database_name\`.\`identity_session\` TO '$runtime_user'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON \`$database_name\`.\`identity_authentication_throttle\` TO '$runtime_user'@'%';
SQL

admin_dsn="$migration_user:$migration_password@tcp($mysql_address)/$database_name?charset=utf8mb4&collation=utf8mb4_0900_ai_ci&parseTime=true&loc=UTC&timeout=5s&readTimeout=30s&writeTimeout=20s&tls=false"
runtime_dsn="$runtime_user:$runtime_password@tcp($mysql_address)/$database_name?charset=utf8mb4&collation=utf8mb4_0900_ai_ci&parseTime=true&loc=UTC&timeout=5s&readTimeout=30s&writeTimeout=20s&tls=false"
export GROWTHOS_TEST_IDENTITY_MYSQL_ACCEPTANCE='lesson-32-disposable-mysql-8.4'
export GROWTHOS_TEST_IDENTITY_MYSQL_ADMIN_DSN="$admin_dsn"
export GROWTHOS_TEST_IDENTITY_MYSQL_RUNTIME_DSN="$runtime_dsn"
go test -v -count=1 -run '^TestRepositoryMySQL84Acceptance$' ./internal/identity/adapter/mysqlrepo

unset GROWTHOS_TEST_MYSQL_IDENTITY_MIGRATION_PASSWORD \
    GROWTHOS_TEST_IDENTITY_MYSQL_ADMIN_DSN \
    GROWTHOS_TEST_IDENTITY_MYSQL_RUNTIME_DSN \
    GROWTHOS_MYSQL_MIGRATION_PASSWORD_FILE
unset root_password migration_password runtime_password admin_dsn runtime_dsn

final_status=$(docker container exec "$container_id" mysql \
    --defaults-extra-file=/run/lesson32-secrets/root-client.cnf \
    --batch --skip-column-names "$database_name" \
    --execute='SELECT CONCAT(version, CHAR(58), dirty) FROM schema_migrations')
if [ "$final_status" != '14:0' ]; then
    printf 'final migration status = %s, want 14:0\n' "$final_status" >&2
    exit 1
fi
identity_rows=$(docker container exec "$container_id" mysql \
    --defaults-extra-file=/run/lesson32-secrets/root-client.cnf \
    --batch --skip-column-names "$database_name" \
    --execute='SELECT CONCAT((SELECT COUNT(*) FROM identity_workforce_account), CHAR(58), (SELECT COUNT(*) FROM identity_session), CHAR(58), (SELECT COUNT(*) FROM identity_authentication_throttle))')
if [ "$identity_rows" != '0:0:0' ]; then
    printf 'final Identity row counts = %s, want workforce:session:throttle = 0:0:0\n' "$identity_rows" >&2
    exit 1
fi
reserved_probe_tables=$(docker container exec "$container_id" mysql \
    --defaults-extra-file=/run/lesson32-secrets/root-client.cnf \
    --batch --skip-column-names "$database_name" \
    --execute="SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = 'identity_l32_forbidden'")
if [ "$reserved_probe_tables" != '0' ]; then
    printf 'reserved Identity DDL probe remains: %s\n' "$reserved_probe_tables" >&2
    exit 1
fi

printf 'Lesson 32 database postconditions: schema_migrations=%s identity-rows=%s reserved-ddl-probes=%s\n' "$final_status" "$identity_rows" "$reserved_probe_tables"
printf '%s\n' 'Lesson 32 MySQL acceptance passed: empty -> v14 -> empty, real growth-migrate restore, exact runtime grants, repository concurrency/session/maintenance/denial contracts'
