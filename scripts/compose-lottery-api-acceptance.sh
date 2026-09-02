#!/bin/sh
set -eu
umask 077

# Destructive-to-self acceptance for the current Lesson 32 Identity schema and
# the still-ephemeral Lesson 24 Lottery API/cache behavior. Every run gets
# a new Compose project, Docker-assigned loopback port, secret set, and volumes.
# The long-lived `growthos` project is only snapshotted to prove that its
# containers, volumes, and networks were not replaced or removed.

repository_root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
base_compose_file="$repository_root/deploy/compose/compose.yaml"
acceptance_compose_file="$repository_root/deploy/compose/compose.lesson21-acceptance.yaml"
secret_generator="$repository_root/scripts/generate-compose-secrets.sh"
default_project=growthos
concurrent_requests=64
concurrent_workers=16
connect_timeout=3
request_timeout=10
baseline_rate=50
baseline_duration=10s
baseline_workers=16

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

for required_command in awk cat chmod cmp cp curl dd docker go jq mktemp openssl sed sort stat tr unlink wc xargs; do
    require_command "$required_command"
done
if ! docker compose version >/dev/null 2>&1; then
    fail 'the Docker Compose plugin is unavailable'
fi
for required_file in "$base_compose_file" "$acceptance_compose_file" "$secret_generator"; do
    if [ ! -r "$required_file" ]; then
        fail "$required_file is not readable"
    fi
done

random_suffix=$(openssl rand -hex 12)
case "$random_suffix" in
    ''|*[!0-9a-f]*)
        fail 'openssl did not produce the expected lowercase hexadecimal project suffix'
        ;;
esac
if [ "${#random_suffix}" -ne 24 ]; then
    fail 'openssl did not produce the expected 24-character project suffix'
fi
compose_project="growthosl24$random_suffix"
case "$compose_project" in
    growthosl24[0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f])
        ;;
    *)
        fail 'the generated Compose project name failed its exact format check'
        ;;
esac
if [ "$compose_project" = "$default_project" ]; then
    fail 'the disposable project must never equal the long-lived growthos project'
fi

export GROWTHOS_LESSON24_ACCEPTANCE_PROJECT="$compose_project"
acceptance_api_image="growthos/acceptance-api:$compose_project"
acceptance_migrate_image="growthos/acceptance-migrate:$compose_project"
acceptance_identity_provision_image="growthos/acceptance-identity-provision:$compose_project"
acceptance_identity_maintenance_image="growthos/acceptance-identity-maintenance:$compose_project"
acceptance_redis_image="growthos/acceptance-redis:$compose_project"
acceptance_web_image="growthos/acceptance-web:$compose_project"
acceptance_images="$acceptance_api_image $acceptance_migrate_image $acceptance_identity_provision_image $acceptance_identity_maintenance_image $acceptance_redis_image $acceptance_web_image"
buildx_builder="${compose_project}builder"
buildkit_image=moby/buildkit:buildx-stable-1
expected_builder_container="buildx_buildkit_${buildx_builder}0"
expected_builder_volume="${expected_builder_container}_state"
export GROWTHOS_LESSON24_ACCEPTANCE_API_IMAGE="$acceptance_api_image"
export GROWTHOS_LESSON24_ACCEPTANCE_MIGRATE_IMAGE="$acceptance_migrate_image"
export GROWTHOS_LESSON24_ACCEPTANCE_IDENTITY_PROVISION_IMAGE="$acceptance_identity_provision_image"
export GROWTHOS_LESSON24_ACCEPTANCE_IDENTITY_MAINTENANCE_IMAGE="$acceptance_identity_maintenance_image"
export GROWTHOS_LESSON24_ACCEPTANCE_REDIS_IMAGE="$acceptance_redis_image"
export GROWTHOS_LESSON24_ACCEPTANCE_WEB_IMAGE="$acceptance_web_image"
export GROWTHOS_LESSON24_ACCEPTANCE_CACHE_ENABLED=true
# Compose must be renderable before Web exists. Port 1 is deliberately never
# contacted; it is replaced with Docker's exact allocation before API exists.
export GROWTHOS_LESSON24_ACCEPTANCE_PUBLIC_ORIGIN=http://127.0.0.1:1
export BUILDX_BUILDER="$buildx_builder"

compose() {
    docker compose \
        --project-name "$compose_project" \
        --file "$base_compose_file" \
        --file "$acceptance_compose_file" \
        "$@"
}

redis_business() {
    # shellcheck disable=SC2016
    compose exec -T redis sh -eu -c '
        export REDISCLI_AUTH="$(cat /run/secrets/redis_password)"
        exec redis-cli --raw --no-auth-warning --user growthos_api "$@"
    ' sh "$@"
}

assert_redis_denied() {
    denied_label=$1
    shift
    denied_output=$(redis_business "$@" 2>&1 || true)
    case "$denied_output" in
        *NOPERM*) ;;
        *) fail "Redis ACL did not reject: $denied_label" ;;
    esac
}

snapshot_default_containers() {
    default_container_ids=$(docker ps --all \
        --filter "label=com.docker.compose.project=$default_project" \
        --format '{{.ID}}') || return 1
    if [ -n "$default_container_ids" ]; then
        printf '%s\n' "$default_container_ids" | LC_ALL=C sort
    fi
}

snapshot_default_volumes() {
    default_volume_names=$(docker volume ls \
        --filter "label=com.docker.compose.project=$default_project" \
        --format '{{.Name}}') || return 1
    default_volume_details=
    for default_volume_name in $default_volume_names; do
        default_volume_detail=$(docker volume inspect --format '{{.Name}}|{{.Driver}}|{{.Mountpoint}}|{{.CreatedAt}}' "$default_volume_name") || return 1
        if [ -z "$default_volume_details" ]; then
            default_volume_details=$default_volume_detail
        else
            default_volume_details="$default_volume_details
$default_volume_detail"
        fi
    done
    if [ -n "$default_volume_details" ]; then
        printf '%s\n' "$default_volume_details" | LC_ALL=C sort
    fi
}

snapshot_default_networks() {
    default_network_ids=$(docker network ls \
        --filter "label=com.docker.compose.project=$default_project" \
        --format '{{.ID}}') || return 1
    if [ -n "$default_network_ids" ]; then
        printf '%s\n' "$default_network_ids" | LC_ALL=C sort
    fi
}

default_containers_before=$(snapshot_default_containers)
default_volumes_before=$(snapshot_default_volumes)
default_networks_before=$(snapshot_default_networks)

if [ -n "$(docker ps --all --filter "label=com.docker.compose.project=$compose_project" --quiet)" ] ||
   [ -n "$(docker volume ls --filter "label=com.docker.compose.project=$compose_project" --quiet)" ] ||
   [ -n "$(docker network ls --filter "label=com.docker.compose.project=$compose_project" --quiet)" ]; then
    fail 'the generated project name unexpectedly collides with existing Docker resources'
fi
for expected_volume in "${compose_project}_mysql_data" "${compose_project}_mysql_socket"; do
    if docker volume inspect "$expected_volume" >/dev/null 2>&1; then
        fail "the generated volume name already exists: $expected_volume"
    fi
done
for acceptance_image in $acceptance_images; do
    if docker image inspect "$acceptance_image" >/dev/null 2>&1; then
        fail "the generated acceptance image tag already exists: $acceptance_image"
    fi
done
if docker buildx inspect "$buildx_builder" >/dev/null 2>&1; then
    fail "the generated buildx builder already exists: $buildx_builder"
fi
if docker container inspect "$expected_builder_container" >/dev/null 2>&1; then
    fail "the generated buildx node container already exists: $expected_builder_container"
fi
if docker volume inspect "$expected_builder_volume" >/dev/null 2>&1; then
    fail "the generated buildx state volume already exists: $expected_builder_volume"
fi
buildkit_image_preexisting=0
buildkit_image_id=
if docker image inspect "$buildkit_image" >/dev/null 2>&1; then
    buildkit_image_preexisting=1
fi

secret_directory=
response_directory=
identity_directory=
secret_directory_identity=
response_directory_identity=
identity_directory_identity=
identity_password_snapshot=
identity_password_snapshot_bytes=0
response_number=0
cleanup_project=0
cleanup_images=0
cleanup_builder=0
builder_container_id=
builder_volume_name=
image_ownership_recorded=0
preserve_disposable_resources=0

verify_cleanup_project_labels() {
    cleanup_container_ids=$(docker ps --all \
        --filter "label=com.docker.compose.project=$compose_project" \
        --quiet)
    cleanup_seen_services=' '
    for cleanup_container_id in $cleanup_container_ids; do
        cleanup_label=$(docker inspect --format '{{ index .Config.Labels "com.docker.compose.project" }}' "$cleanup_container_id") || return 1
        cleanup_service=$(docker inspect --format '{{ index .Config.Labels "com.docker.compose.service" }}' "$cleanup_container_id") || return 1
        if [ "$cleanup_label" != "$compose_project" ]; then
            printf 'refusing cleanup: container %s has project label %s\n' "$cleanup_container_id" "$cleanup_label" >&2
            return 1
        fi
        case "$cleanup_service" in
            api|identity-maintenance|identity-provision|migrate|mysql|mysql-grants|redis|web)
                ;;
            *)
                printf 'refusing cleanup: container %s has unexpected service label %s\n' "$cleanup_container_id" "$cleanup_service" >&2
                return 1
                ;;
        esac
        case "$cleanup_seen_services" in
            *" $cleanup_service "*)
                printf 'refusing cleanup: service %s has more than one project container\n' "$cleanup_service" >&2
                return 1
                ;;
        esac
        cleanup_seen_services="$cleanup_seen_services$cleanup_service "
    done

    for cleanup_volume_name in "${compose_project}_mysql_data" "${compose_project}_mysql_socket"; do
        if docker volume inspect "$cleanup_volume_name" >/dev/null 2>&1; then
            cleanup_label=$(docker volume inspect --format '{{ index .Labels "com.docker.compose.project" }}' "$cleanup_volume_name") || return 1
            cleanup_acceptance_label=$(docker volume inspect --format '{{ index .Labels "com.growthos.acceptance.project" }}' "$cleanup_volume_name") || return 1
            if [ "$cleanup_label" != "$compose_project" ] || [ "$cleanup_acceptance_label" != "$compose_project" ]; then
                printf 'refusing cleanup: volume %s lacks the exact disposable project labels\n' "$cleanup_volume_name" >&2
                return 1
            fi
        fi
    done

    cleanup_volume_names=$(docker volume ls \
        --filter "label=com.docker.compose.project=$compose_project" \
        --format '{{.Name}}')
    for cleanup_volume_name in $cleanup_volume_names; do
        cleanup_label=$(docker volume inspect --format '{{ index .Labels "com.docker.compose.project" }}' "$cleanup_volume_name") || return 1
        if [ "$cleanup_label" != "$compose_project" ]; then
            printf 'refusing cleanup: volume %s has project label %s\n' "$cleanup_volume_name" "$cleanup_label" >&2
            return 1
        fi
        case "$cleanup_volume_name" in
            "${compose_project}_mysql_data"|"${compose_project}_mysql_socket")
                ;;
            *)
                printf 'refusing cleanup: unexpected project volume %s\n' "$cleanup_volume_name" >&2
                return 1
                ;;
        esac
    done

    for cleanup_network_name in "${compose_project}_edge" "${compose_project}_data" "${compose_project}_cache"; do
        if docker network inspect "$cleanup_network_name" >/dev/null 2>&1; then
            cleanup_label=$(docker network inspect --format '{{ index .Labels "com.docker.compose.project" }}' "$cleanup_network_name") || return 1
            if [ "$cleanup_label" != "$compose_project" ]; then
                printf 'refusing cleanup: network %s lacks the exact disposable project label\n' "$cleanup_network_name" >&2
                return 1
            fi
        fi
    done

    cleanup_network_ids=$(docker network ls \
        --filter "label=com.docker.compose.project=$compose_project" \
        --quiet)
    for cleanup_network_id in $cleanup_network_ids; do
        cleanup_label=$(docker network inspect --format '{{ index .Labels "com.docker.compose.project" }}' "$cleanup_network_id") || return 1
        cleanup_network_name=$(docker network inspect --format '{{.Name}}' "$cleanup_network_id") || return 1
        if [ "$cleanup_label" != "$compose_project" ]; then
            printf 'refusing cleanup: network %s has project label %s\n' "$cleanup_network_name" "$cleanup_label" >&2
            return 1
        fi
        case "$cleanup_network_name" in
            "${compose_project}_edge"|"${compose_project}_data"|"${compose_project}_cache")
                ;;
            *)
                printf 'refusing cleanup: unexpected project network %s\n' "$cleanup_network_name" >&2
                return 1
                ;;
        esac
    done
}

remove_regular_file() {
    cleanup_target=$1
    if [ -L "$cleanup_target" ]; then
        printf 'refusing cleanup: temporary target became a symlink: %s\n' "$cleanup_target" >&2
        return 1
    fi
    if [ -e "$cleanup_target" ]; then
        if [ ! -f "$cleanup_target" ]; then
            printf 'refusing cleanup: temporary target is not a regular file: %s\n' "$cleanup_target" >&2
            return 1
        fi
        # Secret files are deliberately read-only. BSD rm prompts for those
        # when stdin is a TTY unless -f is supplied, even without an alias or
        # interactive flag. The target has already been proven to be the exact
        # task-created regular file, so make cleanup deterministic.
        rm -f -- "$cleanup_target"
    fi
}

remove_private_file() {
    private_target=$1
    if [ -L "$private_target" ]; then
        printf 'refusing cleanup: private target became a symlink: %s\n' "$private_target" >&2
        return 1
    fi
    if [ ! -e "$private_target" ]; then
        return 0
    fi
    if [ ! -f "$private_target" ]; then
        printf 'refusing cleanup: private target is not a regular file: %s\n' "$private_target" >&2
        return 1
    fi
    private_bytes=$(LC_ALL=C wc -c < "$private_target" | awk '{ print $1 }') || return 1
    case "$private_bytes" in
        ''|*[!0-9]*)
            printf 'refusing cleanup: private target size is invalid: %s\n' "$private_target" >&2
            return 1
            ;;
    esac
    chmod 0600 "$private_target" 2>/dev/null || return 1
    if [ "$private_bytes" -gt 0 ]; then
        dd if=/dev/zero of="$private_target" bs=1 count="$private_bytes" conv=notrunc \
            >/dev/null 2>&1 || return 1
    fi
    unlink "$private_target"
}

remove_identity_password_snapshot() {
    if [ -z "$identity_password_snapshot" ] ||
       { [ ! -e "$identity_password_snapshot" ] && [ ! -L "$identity_password_snapshot" ]; }; then
        return 0
    fi
    if [ "$identity_password_snapshot_bytes" -le 0 ]; then
        printf '%s\n' 'refusing cleanup: enrollment snapshot size was not recorded' >&2
        return 1
    fi
    remove_private_file "$identity_password_snapshot"
}

directory_identity() {
    identity_target=$1
    if identity_value=$(stat -f '%d:%i' -- "$identity_target" 2>/dev/null); then
        printf '%s\n' "$identity_value"
        return 0
    fi
    stat -c '%d:%i' -- "$identity_target" 2>/dev/null
}

verify_temporary_directory() {
    cleanup_directory=$1
    expected_identity=$2
    cleanup_description=$3

    if [ -L "$cleanup_directory" ]; then
        printf 'refusing cleanup: temporary %s directory became a symlink: %s\n' \
            "$cleanup_description" "$cleanup_directory" >&2
        return 1
    fi
    if [ ! -d "$cleanup_directory" ]; then
        printf 'refusing cleanup: temporary %s target is not a directory: %s\n' \
            "$cleanup_description" "$cleanup_directory" >&2
        return 1
    fi
    if ! cleanup_identity=$(directory_identity "$cleanup_directory"); then
        printf 'refusing cleanup: could not inspect temporary %s directory identity: %s\n' \
            "$cleanup_description" "$cleanup_directory" >&2
        return 1
    fi
    if [ -z "$expected_identity" ] || [ "$cleanup_identity" != "$expected_identity" ]; then
        printf 'refusing cleanup: temporary %s directory identity changed: %s\n' \
            "$cleanup_description" "$cleanup_directory" >&2
        return 1
    fi
}

cleanup_temporary_directories() {
    temporary_cleanup_status=0
    remove_identity_password_snapshot || temporary_cleanup_status=1
    if [ -n "$response_directory" ] && { [ -e "$response_directory" ] || [ -L "$response_directory" ]; }; then
        if ! verify_temporary_directory "$response_directory" "$response_directory_identity" response; then
            temporary_cleanup_status=1
        else
            cleanup_index=1
            while [ "$cleanup_index" -le "$response_number" ]; do
                remove_private_file "$response_directory/headers-$cleanup_index" || temporary_cleanup_status=1
                remove_private_file "$response_directory/body-$cleanup_index" || temporary_cleanup_status=1
                cleanup_index=$((cleanup_index + 1))
            done
            cleanup_index=1
            while [ "$cleanup_index" -le "$concurrent_requests" ]; do
                remove_regular_file "$response_directory/concurrent-headers-$cleanup_index" || temporary_cleanup_status=1
                remove_regular_file "$response_directory/concurrent-body-$cleanup_index" || temporary_cleanup_status=1
                remove_regular_file "$response_directory/concurrent-status-$cleanup_index" || temporary_cleanup_status=1
                cleanup_index=$((cleanup_index + 1))
            done
            remove_regular_file "$response_directory/gateway-oversize-request" || temporary_cleanup_status=1
            remove_regular_file "$response_directory/image-ownership" || temporary_cleanup_status=1
            for baseline_scenario in warm-cache direct-mysql redis-down; do
                remove_regular_file "$response_directory/m1-$baseline_scenario.json" || temporary_cleanup_status=1
            done
            if ! rmdir "$response_directory"; then
                printf 'temporary response directory was not empty: %s\n' "$response_directory" >&2
                temporary_cleanup_status=1
            fi
        fi
    fi
    if [ -n "$identity_directory" ] && { [ -e "$identity_directory" ] || [ -L "$identity_directory" ]; }; then
        if ! verify_temporary_directory "$identity_directory" "$identity_directory_identity" identity; then
            temporary_cleanup_status=1
        else
            for identity_private_name in \
                enrollment-password \
                wrong-password \
                login-body \
                malformed-body \
                curl.conf \
                sensitive-patterns \
                token-a \
                token-b \
                token-scratch \
                csrf-a \
                csrf-b \
                csrf-current \
                cookie-a \
                cookie-b \
                cookie-state \
                provision-output \
                api-logs \
                web-logs; do
                remove_private_file "$identity_directory/$identity_private_name" || temporary_cleanup_status=1
            done
            cleanup_index=1
            while [ "$cleanup_index" -le 6 ]; do
                remove_private_file "$identity_directory/cap-cookie-$cleanup_index" || temporary_cleanup_status=1
                cleanup_index=$((cleanup_index + 1))
            done
            if ! rmdir "$identity_directory"; then
                printf 'temporary identity directory was not empty: %s\n' "$identity_directory" >&2
                temporary_cleanup_status=1
            fi
        fi
    fi
    if [ -n "$secret_directory" ] && { [ -e "$secret_directory" ] || [ -L "$secret_directory" ]; }; then
        if ! verify_temporary_directory "$secret_directory" "$secret_directory_identity" secret; then
            temporary_cleanup_status=1
        else
            for secret_name in mysql_root_password mysql_app_password mysql_migration_password mysql_identity_password mysql_identity_provisioner_password redis_password identity_throttle_hmac_key identity_csrf_active_key; do
                remove_regular_file "$secret_directory/$secret_name" || temporary_cleanup_status=1
            done
            if ! rmdir "$secret_directory"; then
                printf 'temporary secret directory was not empty: %s\n' "$secret_directory" >&2
                temporary_cleanup_status=1
            fi
        fi
    fi
    return "$temporary_cleanup_status"
}

cleanup() {
    cleanup_status=$?
    trap - 0 HUP INT TERM

    if [ "$cleanup_project" -eq 1 ]; then
        if verify_cleanup_project_labels; then
            if ! compose down --volumes --remove-orphans; then
                printf 'the disposable Compose project could not be removed cleanly\n' >&2
                preserve_disposable_resources=1
                cleanup_status=1
            fi
            if [ -n "$(docker ps --all --filter "label=com.docker.compose.project=$compose_project" --quiet)" ] ||
               [ -n "$(docker volume ls --filter "label=com.docker.compose.project=$compose_project" --quiet)" ] ||
               [ -n "$(docker network ls --filter "label=com.docker.compose.project=$compose_project" --quiet)" ]; then
                printf 'disposable project resources remain after cleanup\n' >&2
                preserve_disposable_resources=1
                cleanup_status=1
            fi
            for cleanup_volume_name in "${compose_project}_mysql_data" "${compose_project}_mysql_socket"; do
                if docker volume inspect "$cleanup_volume_name" >/dev/null 2>&1; then
                    printf 'disposable volume remains after cleanup: %s\n' "$cleanup_volume_name" >&2
                    preserve_disposable_resources=1
                    cleanup_status=1
                fi
            done
            for cleanup_network_name in "${compose_project}_edge" "${compose_project}_data" "${compose_project}_cache"; do
                if docker network inspect "$cleanup_network_name" >/dev/null 2>&1; then
                    printf 'disposable network remains after cleanup: %s\n' "$cleanup_network_name" >&2
                    preserve_disposable_resources=1
                    cleanup_status=1
                fi
            done
        else
            printf 'project label verification failed; preserving disposable Docker resources for manual inspection\n' >&2
            preserve_disposable_resources=1
            cleanup_status=1
        fi
    fi

    if [ "$preserve_disposable_resources" -eq 0 ] && [ "$cleanup_images" -eq 1 ]; then
        for cleanup_image in $acceptance_images; do
            if docker image inspect "$cleanup_image" >/dev/null 2>&1; then
                if [ "$image_ownership_recorded" -ne 1 ] || [ ! -f "$response_directory/image-ownership" ]; then
                    printf 'refusing cleanup: no ownership record for disposable image %s\n' "$cleanup_image" >&2
                    cleanup_status=1
                    continue
                fi
                cleanup_image_record=$(awk -F '|' -v image="$cleanup_image" '$1 == image { print $2 }' "$response_directory/image-ownership")
                cleanup_image_id=$(docker image inspect --format '{{.Id}}' "$cleanup_image") || cleanup_image_id=
                if [ -z "$cleanup_image_record" ] || [ "$cleanup_image_record" != "$cleanup_image_id" ]; then
                    printf 'refusing cleanup: disposable image tag ownership drifted: %s\n' "$cleanup_image" >&2
                    cleanup_status=1
                elif ! docker image rm "$cleanup_image"; then
                    printf 'could not remove exact disposable image tag: %s\n' "$cleanup_image" >&2
                    cleanup_status=1
                elif docker image inspect "$cleanup_image" >/dev/null 2>&1; then
                    printf 'disposable image tag remains after cleanup: %s\n' "$cleanup_image" >&2
                    cleanup_status=1
                fi
            fi
        done
    fi

    if [ "$preserve_disposable_resources" -eq 0 ] && [ "$cleanup_builder" -eq 1 ] && docker buildx inspect "$buildx_builder" >/dev/null 2>&1; then
        cleanup_builder_name=$(docker buildx inspect "$buildx_builder" | awk '$1 == "Name:" { print $2; exit }')
        cleanup_builder_driver=$(docker buildx inspect "$buildx_builder" | awk '$1 == "Driver:" { print $2; exit }')
        if [ "$cleanup_builder_name" != "$buildx_builder" ] || [ "$cleanup_builder_driver" != docker-container ]; then
            printf 'refusing cleanup: buildx builder identity does not match the disposable target\n' >&2
            cleanup_status=1
        elif [ -n "$builder_container_id" ] &&
             { [ "$(docker inspect --format '{{.Name}}' "$builder_container_id" 2>/dev/null)" != "/$expected_builder_container" ] ||
               [ "$builder_volume_name" != "$expected_builder_volume" ]; }; then
            printf 'refusing cleanup: buildx node container or state-volume ownership drifted\n' >&2
            cleanup_status=1
        elif ! docker buildx rm "$buildx_builder"; then
            printf 'could not remove the exact disposable buildx builder and cache\n' >&2
            cleanup_status=1
        fi
    fi
    if [ "$preserve_disposable_resources" -eq 0 ] && [ "$cleanup_builder" -eq 1 ]; then
        if docker buildx inspect "$buildx_builder" >/dev/null 2>&1; then
            printf 'disposable buildx builder metadata remains after cleanup\n' >&2
            cleanup_status=1
        fi
        if docker container inspect "$expected_builder_container" >/dev/null 2>&1; then
            printf 'disposable buildx node container remains after cleanup\n' >&2
            cleanup_status=1
        fi
        if docker volume inspect "$expected_builder_volume" >/dev/null 2>&1; then
            printf 'disposable buildx state volume remains after cleanup\n' >&2
            cleanup_status=1
        fi
    fi

    if [ "$preserve_disposable_resources" -eq 0 ] && [ "$buildkit_image_preexisting" -eq 0 ] && docker image inspect "$buildkit_image" >/dev/null 2>&1; then
        cleanup_buildkit_image_id=$(docker image inspect --format '{{.Id}}' "$buildkit_image") || cleanup_buildkit_image_id=
        if [ -z "$buildkit_image_id" ] || [ "$cleanup_buildkit_image_id" != "$buildkit_image_id" ]; then
            printf 'refusing cleanup: downloaded buildkit image ownership drifted\n' >&2
            cleanup_status=1
        elif ! docker image rm "$buildkit_image"; then
            printf 'could not remove the buildkit tool image downloaded solely for acceptance\n' >&2
            cleanup_status=1
        fi
    fi

    if [ "$preserve_disposable_resources" -eq 0 ]; then
        if ! cleanup_temporary_directories; then
            cleanup_status=1
        fi
    else
        printf 'preserved disposable secrets/responses at %s, %s, and %s for manual cleanup\n' \
            "$secret_directory" "$response_directory" "$identity_directory" >&2
    fi

    default_containers_after=$(snapshot_default_containers) || cleanup_status=1
    default_volumes_after=$(snapshot_default_volumes) || cleanup_status=1
    default_networks_after=$(snapshot_default_networks) || cleanup_status=1
    if [ "$default_containers_after" != "$default_containers_before" ] ||
       [ "$default_volumes_after" != "$default_volumes_before" ] ||
       [ "$default_networks_after" != "$default_networks_before" ]; then
        printf 'the long-lived growthos project resource identity changed during acceptance\n' >&2
        cleanup_status=1
    fi

    if [ "$cleanup_status" -eq 0 ]; then
        ok 'removed only label/ID-verified Docker resources and identity/type-verified temporary files'
        ok 'the long-lived growthos project resource identity remained unchanged'
    fi
    exit "$cleanup_status"
}

trap cleanup 0
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

secret_directory=$(mktemp -d "${TMPDIR:-/tmp}/growthos-lesson24-secrets.XXXXXX")
secret_directory_identity=$(directory_identity "$secret_directory") || fail 'could not record the temporary secret directory identity'
response_directory=$(mktemp -d "${TMPDIR:-/tmp}/growthos-lesson24-responses.XXXXXX")
response_directory_identity=$(directory_identity "$response_directory") || fail 'could not record the temporary response directory identity'
identity_directory=$(mktemp -d "${TMPDIR:-/tmp}/growthos-lesson32-identity.XXXXXX")
identity_directory_identity=$(directory_identity "$identity_directory") || fail 'could not record the temporary identity directory identity'
export GROWTHOS_LESSON24_ACCEPTANCE_SECRET_DIRECTORY="$secret_directory"

# Arm project cleanup before the first command that may create a Docker
# resource. The generator also receives the unique project, so its old-volume
# guard can never inspect the default growthos volume.
cleanup_project=1
GROWTHOS_COMPOSE_PROJECT="$compose_project" \
    "$secret_generator" "$secret_directory"

compose config --quiet
cleanup_builder=1
if ! docker buildx create \
    --name "$buildx_builder" \
    --driver docker-container \
    --driver-opt "image=$buildkit_image" \
    --driver-opt default-load=true >/dev/null; then
    fail 'could not create the disposable buildx builder'
fi
if ! docker buildx inspect "$buildx_builder" --bootstrap >/dev/null; then
    fail 'could not bootstrap the disposable buildx builder'
fi
if [ "$buildkit_image_preexisting" -eq 0 ]; then
    buildkit_image_id=$(docker image inspect --format '{{.Id}}' "$buildkit_image") ||
        fail 'could not record the buildkit image downloaded for acceptance'
fi
builder_container_id=$(docker container inspect --format '{{.Id}}' "$expected_builder_container") ||
    fail 'could not resolve the disposable buildx node container'
builder_container_name=$(docker container inspect --format '{{.Name}}' "$builder_container_id") ||
    fail 'could not inspect the disposable buildx node container'
builder_volume_name=$(docker container inspect --format '{{range .Mounts}}{{if eq .Destination "/var/lib/buildkit"}}{{.Name}}{{end}}{{end}}' "$builder_container_id") ||
    fail 'could not inspect the disposable buildx state volume'
if [ "$builder_container_name" != "/$expected_builder_container" ] ||
   [ "$builder_volume_name" != "$expected_builder_volume" ]; then
    fail 'the disposable buildx node does not have the expected container and state-volume identity'
fi
cleanup_images=1
build_status=0
# Compose may build the api and migrate targets concurrently even though both
# targets share the same Go builder stage. Docker Desktop then runs two copies
# of the compiler against the same dependency graph, which can exceed a
# deliberately small local memory budget. Build each service in dependency-
# neutral order: the api build populates the shared builder cache, then migrate
# and Identity maintenance reuse it while Redis and the web bundle never
# compete with the Go compiler.
for acceptance_build_service in api migrate identity-provision identity-maintenance redis web; do
    if ! compose build "$acceptance_build_service"; then
        build_status=1
        break
    fi
done
image_ownership_file="$response_directory/image-ownership"
: > "$image_ownership_file"
for acceptance_image in $acceptance_images; do
    if docker image inspect "$acceptance_image" >/dev/null 2>&1; then
        acceptance_image_id=$(docker image inspect --format '{{.Id}}' "$acceptance_image") ||
            fail "could not inspect built acceptance image: $acceptance_image"
        printf '%s|%s\n' "$acceptance_image" "$acceptance_image_id" >> "$image_ownership_file"
    fi
done
image_ownership_recorded=1
if [ "$build_status" -ne 0 ]; then
    fail 'the disposable acceptance images did not build successfully'
fi
for acceptance_image in $acceptance_images; do
    if ! awk -F '|' -v image="$acceptance_image" '$1 == image && $2 != "" { found++ } END { exit found == 1 ? 0 : 1 }' "$image_ownership_file"; then
        fail "the build did not produce exactly one ownership record for $acceptance_image"
    fi
done

# Start only the edge first. Docker owns the atomic random-port allocation; API
# does not exist yet and therefore cannot observe the bootstrap origin.
if ! compose up --detach --no-deps --wait --wait-timeout 60 web; then
    fail 'could not preallocate the disposable browser origin through Web'
fi
preallocated_web_container_id=$(compose ps --all --quiet web) ||
    fail 'could not resolve the preallocated Web container'
case "$preallocated_web_container_id" in
    ''|*'
'*)
        fail 'the preallocated Web service must have exactly one container'
        ;;
esac
if [ -n "$(compose ps --all --quiet api)" ]; then
    fail 'API existed before the exact disposable browser origin was known'
fi
if [ "$(docker inspect --format '{{ index .Config.Labels "com.docker.compose.project" }}' "$preallocated_web_container_id")" != "$compose_project" ] ||
   [ "$(docker inspect --format '{{ index .Config.Labels "com.docker.compose.service" }}' "$preallocated_web_container_id")" != web ]; then
    fail 'the preallocated Web container lacks the exact disposable identity'
fi
preallocated_web_binding=$(compose port web 8080) ||
    fail 'could not resolve the preallocated Web host port'
case "$preallocated_web_binding" in
    *'
'*)
        fail 'preallocated Web must have exactly one published host binding'
        ;;
esac
preallocated_web_host=${preallocated_web_binding%:*}
web_port=${preallocated_web_binding##*:}
if [ "$preallocated_web_host" != '127.0.0.1' ]; then
    fail "preallocated Web is published on $preallocated_web_host instead of 127.0.0.1"
fi
case "$web_port" in
    ''|*[!0-9]*)
        fail 'Docker returned a non-numeric preallocated Web port'
        ;;
esac
if [ "$web_port" -lt 1 ] || [ "$web_port" -gt 65535 ]; then
    fail 'Docker returned a preallocated Web port outside 1 through 65535'
fi
base_url="http://127.0.0.1:$web_port"
export GROWTHOS_LESSON24_ACCEPTANCE_PUBLIC_ORIGIN="$base_url"
compose config --quiet
compose up --detach --wait --wait-timeout 180

resolve_container() {
    resolved_service=$1
    resolved_container_id=$(compose ps --all --quiet "$resolved_service") || fail "could not resolve $resolved_service"
    case "$resolved_container_id" in
        '')
            fail "$resolved_service has no container"
            ;;
        *'
'*)
            fail "$resolved_service must have exactly one container"
            ;;
    esac
    resolved_project_label=$(docker inspect --format '{{ index .Config.Labels "com.docker.compose.project" }}' "$resolved_container_id") || fail "could not inspect $resolved_service project label"
    resolved_service_label=$(docker inspect --format '{{ index .Config.Labels "com.docker.compose.service" }}' "$resolved_container_id") || fail "could not inspect $resolved_service service label"
    if [ "$resolved_project_label" != "$compose_project" ] || [ "$resolved_service_label" != "$resolved_service" ]; then
        fail "$resolved_service does not carry the exact disposable project/service labels"
    fi
}

assert_running_healthy() {
    resolve_container "$1"
    service_state=$(docker inspect --format '{{.State.Status}}' "$resolved_container_id")
    service_health=$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}missing{{end}}' "$resolved_container_id")
    if [ "$service_state" != running ] || [ "$service_health" != healthy ]; then
        fail "$1 is $service_state/$service_health instead of running/healthy"
    fi
}

wait_gateway_reachable() {
    baseline_gateway_attempt=0
    while [ "$baseline_gateway_attempt" -lt 30 ]; do
        if curl \
            --silent \
            --show-error \
            --fail \
            --connect-timeout 1 \
            --max-time 2 \
            --header 'Accept: application/json' \
            --output /dev/null \
            "$base_url/health" 2>/dev/null; then
            return 0
        fi
        baseline_gateway_attempt=$((baseline_gateway_attempt + 1))
        sleep 1
    done
    fail 'the disposable web gateway did not become reachable within 30 seconds'
}

recreate_api_for_cache_mode() {
    baseline_cache_mode=$1
    export GROWTHOS_LESSON24_ACCEPTANCE_CACHE_ENABLED="$baseline_cache_mode"
    if ! compose up --detach --no-deps --force-recreate --wait --wait-timeout 90 api; then
        fail "could not recreate the API with Strategy cache enabled=$baseline_cache_mode"
    fi
    resolve_container api
    baseline_api_container_id=$resolved_container_id
    # Keep the edge container and its Docker-assigned host port stable. Nginx
    # resolves the service name through Docker DNS with a short validity, so a
    # successful gateway probe proves that service discovery followed the new
    # API container instead of hiding the transition behind an edge restart.
    assert_running_healthy web
    wait_gateway_reachable
    request GET /ready 200 - - -
}

mysql_app_select_count() {
    # go-sql-driver/mysql sends the repository's parameterized SELECTs through
    # COM_STMT_EXECUTE, so statement/sql/select does not count API source
    # reads. The application identity has an exact SELECT-only grant and the
    # fixture fingerprint proves no writes, making this execute delta the
    # database-independent evidence for the two repository SELECTs per load.
    # The root identity reads only Performance Schema counters inside this
    # disposable project. Hex literals keep shell quoting deterministic.
    # shellcheck disable=SC2016
    compose exec -T mysql sh -eu -c '
        export MYSQL_PWD="$(cat /run/secrets/mysql_root_password)"
        mysql --protocol=socket --user=root --batch --silent --skip-column-names \
            --execute="
                SELECT COALESCE(SUM(COUNT_STAR), 0)
                FROM performance_schema.events_statements_summary_by_account_by_event_name
                WHERE USER = 0x67726f7774686f735f617070
                  AND EVENT_NAME = 0x73746174656d656e742f636f6d2f45786563757465
            "
    '
}

mysql_root_execute() {
    maintenance_sql=$1
    # SQL is generated entirely by this disposable acceptance script; passing
    # it positionally avoids a second shell interpolation boundary. The root
    # credential remains file-backed inside the isolated MySQL container.
    # shellcheck disable=SC2016
    compose exec -T mysql sh -eu -c '
        export MYSQL_PWD="$(cat /run/secrets/mysql_root_password)"
        mysql --protocol=socket --user=root --database=growthos \
            --batch --silent --skip-column-names --execute="$1"
    ' sh "$maintenance_sql"
}

cache_outcome_count() {
    baseline_log_container=$1
    baseline_log_kind=$2
    docker logs "$baseline_log_container" 2>&1 |
        jq -Rr --arg kind "$baseline_log_kind" '
            fromjson? |
            select(has("cache_outcome")) |
            select($kind == "*" or .cache_outcome == $kind) |
            1
        ' |
        wc -l |
        tr -d '[:space:]'
}

assert_counter() {
    baseline_counter_name=$1
    baseline_counter_value=$2
    case "$baseline_counter_value" in
        ''|*[!0-9]*)
            fail "$baseline_counter_name is not a non-negative integer"
            ;;
    esac
}

run_m1_load() {
    baseline_scenario=$1
    baseline_report="$response_directory/m1-$baseline_scenario.json"
    if ! (
        cd "$repository_root"
        go run ./cmd/healthload \
            -url "$base_url/api/v1/lottery/strategies/21003/ephemeral-selections" \
            -method POST \
            -ephemeral-selection=true \
            -rate "$baseline_rate" \
            -duration "$baseline_duration" \
            -workers "$baseline_workers" \
            -timeout 3s \
            -expected-status 200 > "$baseline_report"
    ); then
        fail "the $baseline_scenario M1 load returned transport, status, or completion failures"
    fi
    if ! jq -e '
        .method == "POST" and
        .ephemeral_selection == true and
        .scheduled > 0 and
        .completed == .scheduled and
        .success == .scheduled and
        .errors == 0 and
        .unexpected_status == 0 and
        .dropped == 0 and
        .status_counts["200"] == .scheduled
    ' "$baseline_report" >/dev/null; then
        fail "the $baseline_scenario M1 report failed its exact count contract"
    fi
    baseline_completed=$(jq -r '.completed' "$baseline_report")
    baseline_summary=$(jq -c '{scheduled, actual_rps, p50_ms, p95_ms, p99_ms, max_ms}' "$baseline_report")
}

containers_publishing_loopback_port() {
    inspected_host_port=$1
    inspected_owner_ids=
    for inspected_container_id in $(docker ps --no-trunc --quiet); do
        if docker inspect "$inspected_container_id" | jq -e --arg port "$inspected_host_port" '
            [
                .[0].NetworkSettings.Ports // {} |
                to_entries[]?.value[]? |
                select(.HostIp == "127.0.0.1" and .HostPort == $port)
            ] |
            length > 0
        ' >/dev/null; then
            if [ -z "$inspected_owner_ids" ]; then
                inspected_owner_ids=$inspected_container_id
            else
                inspected_owner_ids="$inspected_owner_ids
$inspected_container_id"
            fi
        fi
    done
    printf '%s' "$inspected_owner_ids"
}

for running_service in mysql api redis web; do
    assert_running_healthy "$running_service"
done
for completed_service in migrate mysql-grants; do
    resolve_container "$completed_service"
    completed_state=$(docker inspect --format '{{.State.Status}}' "$resolved_container_id")
    completed_exit=$(docker inspect --format '{{.State.ExitCode}}' "$resolved_container_id")
    if [ "$completed_state" != exited ] || [ "$completed_exit" != 0 ]; then
        fail "$completed_service did not complete successfully"
    fi
done
ok 'all Compose services reached their expected states on the lesson-32 schema and lesson-24 cache snapshot'

if ! docker network inspect \
    "${compose_project}_edge" \
    "${compose_project}_data" \
    "${compose_project}_cache" | jq -e \
    --arg edge "${compose_project}_edge" \
    --arg data "${compose_project}_data" \
    --arg cache "${compose_project}_cache" '
        (map({key: .Name, value: .Internal}) | from_entries) as $networks |
        length == 3 and
        $networks[$edge] == false and
        $networks[$data] == true and
        $networks[$cache] == true
    ' >/dev/null; then
    fail 'edge/data/cache internal-network flags differ from the exact topology'
fi

resolve_container api
if ! docker inspect "$resolved_container_id" | jq -e \
    --arg edge "${compose_project}_edge" \
    --arg data "${compose_project}_data" \
    --arg cache "${compose_project}_cache" \
    --arg public_origin "$base_url" '
        (.[0].NetworkSettings.Networks | keys | sort) == ([$edge, $data, $cache] | sort) and
        .[0].Config.User == "65532:65532" and
        .[0].HostConfig.ReadonlyRootfs == true and
        ([.[0].Mounts[].Destination | select(startswith("/run/secrets/"))] | sort) ==
            (["/run/secrets/identity_csrf_active_key", "/run/secrets/identity_throttle_hmac_key",
              "/run/secrets/mysql_app_password", "/run/secrets/mysql_identity_password",
              "/run/secrets/redis_password"] | sort) and
        all(.[0].Mounts[];
            if (.Destination | startswith("/run/secrets/")) then .RW == false else true end) and
        (.[0].Config.Env | index("GROWTHOS_IDENTITY_THROTTLE_HMAC_KEY_FILE=/run/secrets/identity_throttle_hmac_key")) != null and
        (.[0].Config.Env | index("GROWTHOS_IDENTITY_CSRF_ACTIVE_KEY_FILE=/run/secrets/identity_csrf_active_key")) != null and
        (.[0].Config.Env | index("GROWTHOS_IDENTITY_ARGON2_MAX_CONCURRENT=2")) != null and
        (.[0].Config.Env | index("GROWTHOS_IDENTITY_ARGON2_ACQUIRE_TIMEOUT=250ms")) != null and
        (.[0].Config.Env | index("GROWTHOS_IDENTITY_HTTP_HANDLER_TIMEOUT=3s")) != null and
        (.[0].Config.Env | index("GROWTHOS_IDENTITY_PUBLIC_ORIGIN=" + $public_origin)) != null and
        any(.[0].Config.Env[]; startswith("GROWTHOS_IDENTITY_CSRF_ACTIVE_KEY_ID="))
    ' >/dev/null; then
    fail 'api network or Secret mounts differ from the business/Identity runtime plus cache contract'
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
ok 'no disposable long-lived application process receives the one-shot provisioner credential'
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

if ! compose --profile operations config --format json | jq -e \
    --arg maintenance_image "$acceptance_identity_maintenance_image" '
        .services["identity-maintenance"] as $service |
        $service.profiles == ["operations"] and
        $service.image == $maintenance_image and
        $service.build.target == "identity-maintenance" and
        $service.user == "65532:65532" and
        $service.command == ["run"] and
        $service.read_only == true and
        $service.restart == "no" and
        $service.cap_drop == ["ALL"] and
        $service.security_opt == ["no-new-privileges:true"] and
        $service.init == true and
        ($service.networks | keys) == ["data"] and
        ($service.ports // []) == [] and
        ($service.volumes // []) == [] and
        ($service.secrets | map(.source)) == ["mysql_identity_password"] and
        ($service.environment | keys) == ([
            "GROWTHOS_ENVIRONMENT",
            "GROWTHOS_LOG_LEVEL",
            "GROWTHOS_LOG_FORMAT",
            "GROWTHOS_MYSQL_ADDRESS",
            "GROWTHOS_MYSQL_DATABASE",
            "GROWTHOS_MYSQL_TLS_MODE",
            "GROWTHOS_MYSQL_CONNECT_TIMEOUT",
            "GROWTHOS_MYSQL_WRITE_TIMEOUT",
            "GROWTHOS_IDENTITY_MYSQL_USER",
            "GROWTHOS_IDENTITY_MYSQL_PASSWORD_FILE",
            "GROWTHOS_IDENTITY_MAINTENANCE_MYSQL_READ_TIMEOUT",
            "GROWTHOS_IDENTITY_MAINTENANCE_MYSQL_PING_TIMEOUT",
            "GROWTHOS_IDENTITY_MAINTENANCE_OPERATION_TIMEOUT"
        ] | sort) and
        $service.environment.GROWTHOS_ENVIRONMENT == "development" and
        $service.environment.GROWTHOS_LOG_LEVEL == "info" and
        $service.environment.GROWTHOS_LOG_FORMAT == "json" and
        $service.environment.GROWTHOS_MYSQL_ADDRESS == "mysql:3306" and
        $service.environment.GROWTHOS_MYSQL_DATABASE == "growthos" and
        $service.environment.GROWTHOS_MYSQL_TLS_MODE == "disabled" and
        $service.environment.GROWTHOS_MYSQL_CONNECT_TIMEOUT == "3s" and
        $service.environment.GROWTHOS_MYSQL_WRITE_TIMEOUT == "5s" and
        $service.environment.GROWTHOS_IDENTITY_MYSQL_USER == "growthos_identity" and
        $service.environment.GROWTHOS_IDENTITY_MYSQL_PASSWORD_FILE == "/run/secrets/mysql_identity_password" and
        $service.environment.GROWTHOS_IDENTITY_MAINTENANCE_MYSQL_READ_TIMEOUT == "5s" and
        $service.environment.GROWTHOS_IDENTITY_MAINTENANCE_MYSQL_PING_TIMEOUT == "3s" and
        $service.environment.GROWTHOS_IDENTITY_MAINTENANCE_OPERATION_TIMEOUT == "3s" and
        ($service.depends_on | keys | sort) == ["mysql", "mysql-grants"] and
        $service.depends_on["mysql-grants"].condition == "service_completed_successfully" and
        $service.depends_on.mysql.condition == "service_healthy"
    ' >/dev/null; then
    fail 'identity-maintenance static Compose contract exceeds or omits its one-shot LoadIdentityMaintenance boundary'
fi

# Materialize but do not start the operations container so Docker's actual
# user, mount, network, filesystem, capability, and fixed-command contract can
# be attested before the real one-shot run. Remove this exact container before
# exercising the disposable cleanup fixtures below.
if ! compose --profile operations create identity-maintenance >/dev/null; then
    fail 'could not create the identity-maintenance contract probe container'
fi
resolve_container identity-maintenance
maintenance_contract_container_id=$resolved_container_id
if ! docker inspect "$maintenance_contract_container_id" | jq -e \
    --arg data "${compose_project}_data" \
    --arg maintenance_image "$acceptance_identity_maintenance_image" '
        .[0].Config.Image == $maintenance_image and
        .[0].Config.User == "65532:65532" and
        .[0].Config.Entrypoint == ["/usr/local/bin/growth-identity-maintenance"] and
        .[0].Config.Cmd == ["run"] and
        .[0].HostConfig.ReadonlyRootfs == true and
        .[0].HostConfig.Privileged == false and
        .[0].HostConfig.NetworkMode == $data and
        .[0].HostConfig.RestartPolicy.Name == "no" and
        .[0].HostConfig.CapDrop == ["ALL"] and
        .[0].HostConfig.SecurityOpt == ["no-new-privileges:true"] and
        ((.[0].HostConfig.PortBindings // {}) | length) == 0 and
        ((.[0].Config.ExposedPorts // {}) | length) == 0 and
        ([.[0].Mounts[]? | select(.Destination | startswith("/run/secrets/"))] | length) == 1 and
        any(.[0].Mounts[]?;
            .Destination == "/run/secrets/mysql_identity_password" and
            .Type == "bind" and .RW == false)
    ' >/dev/null; then
    fail 'identity-maintenance materialized container differs from the non-root, read-only, data-only, one-secret contract'
fi
if ! compose --profile operations rm --force --stop identity-maintenance >/dev/null; then
    fail 'could not remove the identity-maintenance contract probe container'
fi
if docker container inspect "$maintenance_contract_container_id" >/dev/null 2>&1 ||
   [ -n "$(compose --profile operations ps --all --quiet identity-maintenance)" ]; then
    fail 'identity-maintenance contract probe container remains after exact removal'
fi
ok 'identity-maintenance has a fixed run command and its materialized container is non-root/read-only, data-only, and runtime-credential-only'

resolve_container redis
if ! docker inspect "$resolved_container_id" | jq -e --arg cache "${compose_project}_cache" '
    (.[0].NetworkSettings.Networks | keys) == [$cache] and
    ([.[0].Mounts[].Destination | select(startswith("/run/secrets/"))]) ==
        ["/run/secrets/redis_password"]
' >/dev/null; then
    fail 'redis network or Secret mounts differ from the cache-only contract'
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
ok 'cache topology is internal and limited to the API and Redis consumers'

acl_probe_key=growthos:development:lottery:strategy:projection:v1:0
acl_outside_key=growthos:development:lottery:result:v1:0
if [ "$(redis_business ping)" != PONG ] ||
   [ "$(redis_business set "$acl_probe_key" acceptance EX 30)" != OK ] ||
   [ "$(redis_business getrange "$acl_probe_key" 0 9)" != acceptance ]; then
    fail 'growthos_api cannot execute its required Redis command set'
fi
assert_redis_denied 'GET inside the cache prefix' get "$acl_probe_key"
assert_redis_denied 'SET outside the cache prefix' set "$acl_outside_key" forbidden EX 30
assert_redis_denied SCAN scan 0
assert_redis_denied CONFIG config get maxmemory
assert_redis_denied ACL acl users
assert_redis_denied EVAL eval 'return 1' 0
assert_redis_denied SUBSCRIBE subscribe acceptance-channel
assert_redis_denied PUBLISH publish acceptance-channel value
# shellcheck disable=SC2016
default_output=$(compose exec -T redis sh -c '
    export REDISCLI_AUTH="$(cat /run/secrets/redis_password)"
    redis-cli --raw --no-auth-warning ping 2>&1 || true
') || fail 'could not test the disabled Redis default user'
case "$default_output" in
    *WRONGPASS*|*NOAUTH*) ;;
    *) fail 'the Redis default user unexpectedly authenticated' ;;
esac
# shellcheck disable=SC2016
if ! compose exec -T redis sh -eu -c '
    export REDISCLI_AUTH="$(cat /run/secrets/redis_password)"
    exec 3</tmp/growthos-redis/users.acl
    IFS= read -r first_acl_line <&3
    IFS= read -r second_acl_line <&3
    expected_acl_line="user growthos_api on >$REDISCLI_AUTH resetkeys ~growthos:development:lottery:strategy:projection:v1:* resetchannels -@all +ping +getrange +set +del"
    [ "$first_acl_line" = "user default off" ]
    [ "$second_acl_line" = "$expected_acl_line" ]
    ! IFS= read -r unexpected_acl_line <&3
    exec 3<&-
    for expected_config_line in \
        "save \"\"" \
        "appendonly no" \
        "maxmemory 48mb" \
        "maxmemory-policy allkeys-lru"; do
        grep -Fqx "$expected_config_line" /tmp/growthos-redis/redis.conf
    done
'; then
    fail 'the generated Redis ACL or memory policy differs from the exact boundary'
fi
redis_business del "$acl_probe_key" >/dev/null
ok 'Redis named-user commands, exact keyspace, denied commands, and disabled default user passed'

resolve_container api
api_image=$(docker inspect --format '{{.Config.Image}}' "$resolved_container_id")
if [ "$api_image" != "$acceptance_api_image" ]; then
    fail "API image is $api_image instead of the unique acceptance tag"
fi

resolve_container web
web_container_id=$(docker inspect --format '{{.Id}}' "$resolved_container_id") ||
    fail 'could not normalize the disposable web container ID'
if [ "$web_container_id" != "$preallocated_web_container_id" ]; then
    fail 'Web was replaced after its browser origin was atomically allocated'
fi
web_binding=$(compose port web 8080) || fail 'could not resolve the web host port'
if [ "$web_binding" != "$preallocated_web_binding" ]; then
    fail 'Web host binding changed after API received its exact public origin'
fi
case "$web_binding" in
    *'
'*)
        fail 'web must have exactly one published host binding'
        ;;
esac
web_host=${web_binding%:*}
web_port=${web_binding##*:}
if [ "$web_host" != '127.0.0.1' ]; then
    fail "web is published on $web_host instead of 127.0.0.1"
fi
case "$web_port" in
    ''|*[!0-9]*)
        fail 'Docker returned a non-numeric web host port'
        ;;
esac
if [ "$web_port" -lt 1 ] || [ "$web_port" -gt 65535 ]; then
    fail 'Docker returned a web host port outside 1 through 65535'
fi
if ! docker inspect "$web_container_id" | jq -e --arg port "$web_port" '
    .[0].NetworkSettings.Ports == {
        "8080/tcp": [{"HostIp": "127.0.0.1", "HostPort": $port}]
    }
' >/dev/null; then
    fail 'web container port metadata does not match the exact loopback allocation'
fi
published_container_ids=$(containers_publishing_loopback_port "$web_port")
if [ "$published_container_ids" != "$web_container_id" ]; then
    fail 'the allocated host port is not unique to the disposable web container'
fi
for unpublished_service in mysql migrate mysql-grants api redis; do
    resolve_container "$unpublished_service"
    if ! docker inspect "$resolved_container_id" | jq -e '
        [.[0].NetworkSettings.Ports // {} | to_entries[]? | .value[]?] | length == 0
    ' >/dev/null; then
        fail "$unpublished_service unexpectedly publishes a host port"
    fi
done
if [ "$base_url" != "http://127.0.0.1:$web_port" ] ||
   [ "$GROWTHOS_LESSON24_ACCEPTANCE_PUBLIC_ORIGIN" != "$base_url" ]; then
    fail 'the browser URL and API public origin diverged'
fi
ok "Docker assigned unique loopback-only port $web_port to the disposable web proxy"

# shellcheck disable=SC2016
migration_state=$(compose exec -T mysql sh -c '
    export MYSQL_PWD="$(cat /run/secrets/mysql_migration_password)"
    mysql --protocol=tcp --host=127.0.0.1 --user=growthos_migrator --database=growthos \
        --batch --silent --skip-column-names \
        --execute="SELECT CONCAT(version, CHAR(58), dirty) FROM schema_migrations"
') || fail 'could not inspect migration state'
if [ "$migration_state" != '14:0' ]; then
    fail "migration state is $migration_state instead of clean version 14"
fi
ok 'schema migration state is clean version 14'

# shellcheck disable=SC2016
actual_app_grants=$(compose exec -T mysql sh -c '
    export MYSQL_PWD="$(cat /run/secrets/mysql_app_password)"
    mysql --protocol=tcp --host=127.0.0.1 --user=growthos_app --database=growthos \
        --batch --silent --skip-column-names --execute="SHOW GRANTS FOR CURRENT_USER"
' | LC_ALL=C sort) || fail 'could not inspect growthos_app grants'
expected_app_grants=$(LC_ALL=C sort <<'EOF'
GRANT SELECT ON `growthos`.`lottery_strategy` TO `growthos_app`@`%`
GRANT SELECT ON `growthos`.`lottery_strategy_award` TO `growthos_app`@`%`
GRANT USAGE ON *.* TO `growthos_app`@`%`
EOF
)
if [ "$actual_app_grants" != "$expected_app_grants" ]; then
    fail 'growthos_app grants differ from the exact two-table SELECT-only allowlist'
fi
# shellcheck disable=SC2016
mandatory_roles=$(compose exec -T mysql sh -c '
    export MYSQL_PWD="$(cat /run/secrets/mysql_app_password)"
    mysql --protocol=tcp --host=127.0.0.1 --user=growthos_app --database=growthos \
        --batch --silent --skip-column-names --execute="SELECT @@GLOBAL.mandatory_roles"
') || fail 'could not inspect mandatory roles through growthos_app'
if [ -n "$mandatory_roles" ]; then
    fail "mandatory roles expand growthos_app privileges: $mandatory_roles"
fi
ok 'growthos_app has the exact direct grants and no mandatory-role expansion'

# shellcheck disable=SC2016
actual_identity_grants=$(compose exec -T mysql sh -c '
    export MYSQL_PWD="$(cat /run/secrets/mysql_identity_password)"
    mysql --protocol=tcp --host=127.0.0.1 --user=growthos_identity --database=growthos \
        --batch --silent --skip-column-names --execute="SHOW GRANTS FOR CURRENT_USER"
' | LC_ALL=C sort) || fail 'could not inspect growthos_identity grants'
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
    fail 'growthos_identity cannot read its exact Identity table allowlist'
fi
for identity_denied_table in schema_migrations lottery_strategy lottery_strategy_award marketing_activity; do
    # shellcheck disable=SC2016
    if compose exec -T mysql sh -c '
        export MYSQL_PWD="$(cat /run/secrets/mysql_identity_password)"
        mysql --protocol=tcp --host=127.0.0.1 --user=growthos_identity --database=growthos --silent \
            --execute="SELECT 1 FROM $1 LIMIT 0"
    ' sh "$identity_denied_table" >/dev/null 2>&1; then
        fail "growthos_identity unexpectedly read $identity_denied_table"
    fi
done
# MySQL requires an UPDATE privilege for locking reads. The account grant is
# deliberately limited to updated_at, never the credential-bearing columns.
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

# shellcheck disable=SC2016
actual_identity_provisioner_grants=$(compose exec -T mysql sh -c '
    export MYSQL_PWD="$(cat /run/secrets/mysql_identity_provisioner_password)"
    mysql --protocol=tcp --host=127.0.0.1 --user=growthos_identity_provisioner --database=growthos \
        --batch --silent --skip-column-names --execute="SHOW GRANTS FOR CURRENT_USER"
' | LC_ALL=C sort) || fail 'could not inspect growthos_identity_provisioner grants'
expected_identity_provisioner_grants=$(LC_ALL=C sort <<'EOF'
GRANT INSERT ON `growthos`.`identity_workforce_account` TO `growthos_identity_provisioner`@`%`
GRANT USAGE ON *.* TO `growthos_identity_provisioner`@`%`
EOF
)
if [ "$actual_identity_provisioner_grants" != "$expected_identity_provisioner_grants" ]; then
    fail 'growthos_identity_provisioner grants differ from the INSERT-only allowlist'
fi
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
                CONCAT('"'"'accept.account.'"'"', @probe),
                CONCAT('"'"'accept.login.'"'"', @probe),
                CONCAT('"'"'accept.principal.'"'"', @probe),
                CONCAT(CHAR(36), '"'"'argon2id'"'"', CHAR(36), '"'"'acceptance'"'"'),
                '"'"'enabled'"'"', 1, 1, UTC_TIMESTAMP(6), UTC_TIMESTAMP(6)
            );
            ROLLBACK
        "
' >/dev/null; then
    fail 'growthos_identity_provisioner cannot perform its rolled-back workforce INSERT'
fi
for provisioner_denied_operation in select update delete other_insert; do
    case "$provisioner_denied_operation" in
        select)
            denied_sql='SELECT account_id FROM identity_workforce_account LIMIT 0'
            ;;
        update)
            denied_sql='UPDATE identity_workforce_account SET updated_at = updated_at WHERE FALSE'
            ;;
        delete)
            denied_sql='DELETE FROM identity_workforce_account WHERE FALSE'
            ;;
        other_insert)
            denied_sql="INSERT INTO lottery_strategy (strategy_id, name) SELECT 1, 'permission probe' WHERE FALSE"
            ;;
    esac
    # The SQL strings are fixed in the case statement and contain no external
    # input; positional parameters avoid interpolating them into this shell.
    # shellcheck disable=SC2016
    if compose exec -T mysql sh -c '
        export MYSQL_PWD="$(cat /run/secrets/mysql_identity_provisioner_password)"
        mysql --protocol=tcp --host=127.0.0.1 --user=growthos_identity_provisioner --database=growthos --silent \
            --execute="$1"
    ' sh "$denied_sql" >/dev/null 2>&1; then
        fail "growthos_identity_provisioner unexpectedly passed the $provisioner_denied_operation denial"
    fi
done
ok 'growthos_identity_provisioner has exact workforce INSERT with rollback and all read/update/delete/other-table probes denied'

# Exercise the real maintenance image against exact, isolated fixtures. Two
# independently eligible session histories and one expired inactive throttle
# must be removed, while an active session and an unexpired throttle remain
# byte-for-byte stable. The CLI receives only its fixed `run` command; cutoff,
# retention, and the 250/250 budgets come from the trusted process clock and
# application constants.
maintenance_fixture_marker="accept.maintenance.$random_suffix"
maintenance_account_id="$maintenance_fixture_marker.account"
maintenance_active_session_ref="$maintenance_fixture_marker.active"
maintenance_fixture_sql="
    SET @observed = UTC_TIMESTAMP(6);
    INSERT INTO identity_workforce_account (
        account_id, login_name, principal_id, password_envelope,
        account_status, credential_version, authentication_epoch,
        created_at, updated_at
    ) VALUES (
        '$maintenance_account_id', 'maint_$random_suffix',
        '$maintenance_fixture_marker.principal',
        CONCAT(CHAR(36), 'argon2id', CHAR(36), 'acceptance'),
        'enabled', 1, 1, DATE_SUB(@observed, INTERVAL 12 DAY), @observed
    );
    INSERT INTO identity_session (
        session_ref, issue_operation_ref, account_id, token_digest,
        authentication_epoch, issued_at, last_seen_at, idle_expires_at,
        absolute_expires_at, revoked_at, revoke_reason,
        revoke_operation_ref, updated_at
    ) VALUES
    (
        '$maintenance_fixture_marker.expired',
        '$maintenance_fixture_marker.issue.expired', '$maintenance_account_id',
        UNHEX(SHA2('$maintenance_fixture_marker.token.expired', 256)), 1,
        DATE_SUB(@observed, INTERVAL 12 DAY),
        DATE_SUB(@observed, INTERVAL 11 DAY),
        DATE_SUB(@observed, INTERVAL 10 DAY),
        DATE_SUB(@observed, INTERVAL 9 DAY),
        NULL, NULL, NULL, @observed
    ),
    (
        '$maintenance_fixture_marker.revoked',
        '$maintenance_fixture_marker.issue.revoked', '$maintenance_account_id',
        UNHEX(SHA2('$maintenance_fixture_marker.token.revoked', 256)), 1,
        DATE_SUB(@observed, INTERVAL 12 DAY),
        DATE_SUB(@observed, INTERVAL 10 DAY),
        DATE_SUB(@observed, INTERVAL 5 DAY),
        DATE_ADD(@observed, INTERVAL 1 DAY),
        DATE_SUB(@observed, INTERVAL 8 DAY), 'logout',
        '$maintenance_fixture_marker.revoke.revoked', @observed
    ),
    (
        '$maintenance_active_session_ref',
        '$maintenance_fixture_marker.issue.active', '$maintenance_account_id',
        UNHEX(SHA2('$maintenance_fixture_marker.token.active', 256)), 1,
        DATE_SUB(@observed, INTERVAL 1 HOUR),
        DATE_SUB(@observed, INTERVAL 30 MINUTE),
        DATE_ADD(@observed, INTERVAL 30 MINUTE),
        DATE_ADD(@observed, INTERVAL 1 DAY),
        NULL, NULL, NULL, @observed
    );
    INSERT INTO identity_authentication_throttle (
        dimension, subject_digest, window_started_at, window_expires_at,
        failure_count, inflight_count, admission_epoch, inflight_expires_at,
        blocked_until, updated_at, row_expires_at
    ) VALUES
    (
        'login', UNHEX(SHA2('$maintenance_fixture_marker.throttle.expired', 256)),
        DATE_SUB(@observed, INTERVAL 3 DAY),
        DATE_SUB(@observed, INTERVAL 2 DAY),
        0, 0, 1, NULL, NULL,
        DATE_ADD(DATE_SUB(@observed, INTERVAL 3 DAY), INTERVAL 1 HOUR),
        DATE_SUB(@observed, INTERVAL 1 DAY)
    ),
    (
        'source', UNHEX(SHA2('$maintenance_fixture_marker.throttle.active', 256)),
        DATE_SUB(@observed, INTERVAL 1 HOUR),
        DATE_ADD(@observed, INTERVAL 1 HOUR),
        0, 0, 1, NULL, NULL, @observed,
        DATE_ADD(@observed, INTERVAL 1 DAY)
    );
"
if ! mysql_root_execute "$maintenance_fixture_sql" >/dev/null; then
    fail 'could not create the exact disposable Identity maintenance fixtures'
fi

maintenance_active_fingerprint_sql="
    SELECT SHA2(CONCAT_WS(
        CHAR(31), session_ref, issue_operation_ref, account_id,
        HEX(token_digest), CAST(authentication_epoch AS CHAR),
        DATE_FORMAT(issued_at, '%Y-%m-%d %H:%i:%s.%f'),
        DATE_FORMAT(last_seen_at, '%Y-%m-%d %H:%i:%s.%f'),
        DATE_FORMAT(idle_expires_at, '%Y-%m-%d %H:%i:%s.%f'),
        DATE_FORMAT(absolute_expires_at, '%Y-%m-%d %H:%i:%s.%f'),
        COALESCE(DATE_FORMAT(revoked_at, '%Y-%m-%d %H:%i:%s.%f'), 'NULL'),
        COALESCE(revoke_reason, 'NULL'),
        COALESCE(revoke_operation_ref, 'NULL'),
        DATE_FORMAT(updated_at, '%Y-%m-%d %H:%i:%s.%f')
    ), 256)
    FROM identity_session
    WHERE session_ref = '$maintenance_active_session_ref'
"
maintenance_active_fingerprint_before=$(mysql_root_execute "$maintenance_active_fingerprint_sql") ||
    fail 'could not fingerprint the active maintenance session before cleanup'
case "$maintenance_active_fingerprint_before" in
    ''|*[!0-9a-fA-F]*)
        fail 'the active maintenance session fingerprint is invalid'
        ;;
esac
if [ "${#maintenance_active_fingerprint_before}" -ne 64 ]; then
    fail 'the active maintenance session fingerprint has an invalid length'
fi

if ! maintenance_output=$(compose --profile operations run --rm --no-deps --no-tty identity-maintenance); then
    fail 'the real one-shot Identity maintenance container failed'
fi
if ! printf '%s\n' "$maintenance_output" | jq -e -s '
    length == 1 and
    .[0].msg == "identity maintenance completed" and
    .[0].service == "growth-identity-maintenance" and
    .[0].component == "identity_maintenance" and
    .[0].operation == "run" and
    .[0].sessions_deleted == 2 and
    .[0].throttles_deleted == 1 and
    .[0].total_deleted == 3
' >/dev/null; then
    fail 'identity-maintenance did not report the exact bounded fixture cleanup result'
fi
if [ -n "$(compose --profile operations ps --all --quiet identity-maintenance)" ]; then
    fail 'the one-shot identity-maintenance run left a project container behind'
fi

maintenance_state=$(mysql_root_execute "
    SELECT CONCAT(
        (SELECT COUNT(*) FROM identity_session
         WHERE session_ref = '$maintenance_fixture_marker.expired'), ':',
        (SELECT COUNT(*) FROM identity_session
         WHERE session_ref = '$maintenance_fixture_marker.revoked'), ':',
        (SELECT COUNT(*) FROM identity_session
         WHERE session_ref = '$maintenance_active_session_ref'), ':',
        (SELECT COUNT(*) FROM identity_authentication_throttle
         WHERE dimension = 'login'
           AND subject_digest = UNHEX(SHA2('$maintenance_fixture_marker.throttle.expired', 256))), ':',
        (SELECT COUNT(*) FROM identity_authentication_throttle
         WHERE dimension = 'source'
           AND subject_digest = UNHEX(SHA2('$maintenance_fixture_marker.throttle.active', 256)))
    )
") || fail 'could not inspect maintenance fixture eligibility after cleanup'
if [ "$maintenance_state" != '0:0:1:0:1' ]; then
    fail "identity-maintenance changed the wrong fixture set: $maintenance_state"
fi
maintenance_active_fingerprint_after=$(mysql_root_execute "$maintenance_active_fingerprint_sql") ||
    fail 'could not fingerprint the active maintenance session after cleanup'
if [ "$maintenance_active_fingerprint_after" != "$maintenance_active_fingerprint_before" ]; then
    fail 'identity-maintenance modified the active session while removing eligible history'
fi

if ! maintenance_convergence_output=$(compose --profile operations run --rm --no-deps --no-tty identity-maintenance); then
    fail 'the convergent second Identity maintenance container failed'
fi
if ! printf '%s\n' "$maintenance_convergence_output" | jq -e -s '
    length == 1 and
    .[0].msg == "identity maintenance completed" and
    .[0].service == "growth-identity-maintenance" and
    .[0].component == "identity_maintenance" and
    .[0].operation == "run" and
    .[0].sessions_deleted == 0 and
    .[0].throttles_deleted == 0 and
    .[0].total_deleted == 0
' >/dev/null; then
    fail 'the convergent maintenance run did not report exact zero deletion counts'
fi
if [ -n "$(compose --profile operations ps --all --quiet identity-maintenance)" ]; then
    fail 'the convergent identity-maintenance run left a project container behind'
fi
maintenance_active_fingerprint_converged=$(mysql_root_execute "$maintenance_active_fingerprint_sql") ||
    fail 'could not fingerprint the active maintenance session after convergence'
if [ "$maintenance_active_fingerprint_converged" != "$maintenance_active_fingerprint_before" ]; then
    fail 'the convergent maintenance run changed the active session'
fi

maintenance_fixture_cleanup=$(mysql_root_execute "
    DELETE FROM identity_session
    WHERE session_ref LIKE '$maintenance_fixture_marker.%';
    SET @remaining_sessions = ROW_COUNT();
    DELETE FROM identity_authentication_throttle
    WHERE subject_digest IN (
        UNHEX(SHA2('$maintenance_fixture_marker.throttle.expired', 256)),
        UNHEX(SHA2('$maintenance_fixture_marker.throttle.active', 256))
    );
    SET @remaining_throttles = ROW_COUNT();
    DELETE FROM identity_workforce_account
    WHERE account_id = '$maintenance_account_id';
    SET @remaining_accounts = ROW_COUNT();
    SELECT CONCAT(@remaining_sessions, ':', @remaining_throttles, ':', @remaining_accounts);
") || fail 'could not remove the exact surviving maintenance fixtures'
if [ "$maintenance_fixture_cleanup" != '1:1:1' ]; then
    fail "surviving maintenance fixture cleanup was not exact: $maintenance_fixture_cleanup"
fi
maintenance_fixture_residue=$(mysql_root_execute "
    SELECT CONCAT(
        (SELECT COUNT(*) FROM identity_session
         WHERE session_ref LIKE '$maintenance_fixture_marker.%'), ':',
        (SELECT COUNT(*) FROM identity_authentication_throttle
         WHERE subject_digest IN (
             UNHEX(SHA2('$maintenance_fixture_marker.throttle.expired', 256)),
             UNHEX(SHA2('$maintenance_fixture_marker.throttle.active', 256))
         )), ':',
        (SELECT COUNT(*) FROM identity_workforce_account
         WHERE account_id = '$maintenance_account_id')
    )
") || fail 'could not verify exact maintenance fixture removal'
if [ "$maintenance_fixture_residue" != '0:0:0' ]; then
    fail "maintenance fixture residue remains: $maintenance_fixture_residue"
fi
ok 'real maintenance removed only 2 eligible sessions and 1 expired throttle, converged to exact zero, preserved the active session, and left no fixture residue'

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
        fail "growthos_app unexpectedly read $denied_unassembled_table"
    fi
done
# The SELECT source is empty, so this proves INSERT denial without creating a
# graph header even if an over-broad grant regresses.
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

# Fixture writes use the migration/fixture identity, never the product API
# identity. All inserts are in one transaction, so an intermediate SQL failure
# cannot leave a partial set.
# shellcheck disable=SC2016
if ! compose exec -T mysql sh -c '
    export MYSQL_PWD="$(cat /run/secrets/mysql_migration_password)"
    mysql --protocol=tcp --host=127.0.0.1 --user=growthos_migrator --database=growthos \
        --batch --silent --skip-column-names --execute="
            START TRANSACTION;
            INSERT INTO lottery_strategy (strategy_id, name) VALUES
                (18446744073709551615, 0x4d6178207374726174656779),
                (21002, 0x4e6f20726577617264207374726174656779),
                (21003, 0x5765696768746564207374726174656779);
            INSERT INTO lottery_strategy_award
                (strategy_id, award_id, name, weight, outcome) VALUES
                (18446744073709551615, 18446744073709551615, 0x4d617820726577617264, 18446744073709551615, 0x726577617264),
                (21002, 1, 0x54727920616761696e, 7, 0x6e6f5f726577617264),
                (21003, 1, 0x526577617264, 1, 0x726577617264),
                (21003, 2, 0x4e6f20726577617264, 3, 0x6e6f5f726577617264);
            COMMIT;
        "
'; then
    fail 'growthos_migrator could not atomically create the acceptance fixtures'
fi
# shellcheck disable=SC2016
fixture_shape=$(compose exec -T mysql sh -c '
    export MYSQL_PWD="$(cat /run/secrets/mysql_app_password)"
    mysql --protocol=tcp --host=127.0.0.1 --user=growthos_app --database=growthos \
        --batch --silent --skip-column-names --execute="
            SELECT CONCAT(
                (SELECT COUNT(*) FROM lottery_strategy), CHAR(58),
                (SELECT COUNT(*) FROM lottery_strategy_award), CHAR(58),
                (SELECT COALESCE(SUM(weight), 0) FROM lottery_strategy_award WHERE strategy_id = 21003)
            )
        "
') || fail 'could not verify fixture shape through growthos_app'
if [ "$fixture_shape" != '3:4:4' ]; then
    fail "fixture shape is $fixture_shape instead of 3 strategies, 4 awards, and multi-award total 4"
fi
ok 'the fixture identity atomically inserted max, no_reward, and 1:3 multi-award fixtures'

database_fingerprint() {
    # Every persisted business column participates; the random timestamps are
    # stable within this run and therefore suitable for before/after comparison.
    # shellcheck disable=SC2016
    compose exec -T mysql sh -c '
        export MYSQL_PWD="$(cat /run/secrets/mysql_app_password)"
        mysql --protocol=tcp --host=127.0.0.1 --user=growthos_app --database=growthos \
            --batch --silent --skip-column-names --execute="
                SELECT SHA2(GROUP_CONCAT(payload ORDER BY row_key SEPARATOR 0x3b), 256)
                FROM (
                    SELECT
                        CONCAT(0x53, LPAD(strategy_id, 20, 0x30)) AS row_key,
                        CONCAT_WS(0x7c, 0x53, strategy_id, HEX(name),
                            DATE_FORMAT(created_at, 0x25592d256d2d25642025483a25693a25732e2566),
                            DATE_FORMAT(updated_at, 0x25592d256d2d25642025483a25693a25732e2566)) AS payload
                    FROM lottery_strategy
                    UNION ALL
                    SELECT
                        CONCAT(0x41, LPAD(strategy_id, 20, 0x30), LPAD(award_id, 20, 0x30)) AS row_key,
                        CONCAT_WS(0x7c, 0x41, strategy_id, award_id, HEX(name), weight, HEX(outcome),
                            DATE_FORMAT(created_at, 0x25592d256d2d25642025483a25693a25732e2566),
                            DATE_FORMAT(updated_at, 0x25592d256d2d25642025483a25693a25732e2566)) AS payload
                    FROM lottery_strategy_award
                ) AS persisted_rows
            "
    '
}

fingerprint_before=$(database_fingerprint) || fail 'could not fingerprint the fixtures before HTTP requests'
case "$fingerprint_before" in
    ''|*[!0-9A-Fa-f]*)
        fail 'the pre-request database fingerprint has an unexpected format'
        ;;
    *)
        ;;
esac
if [ "${#fingerprint_before}" -ne 64 ]; then
    fail 'the pre-request database fingerprint is not SHA-256 length'
fi

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
                header_matches++
            }
        }
        END { print header_matches + 0 }
    ' "$header_file"
}

request() {
    request_method=$1
    request_route=$2
    expected_status=$3
    request_extra_header=$4
    request_body=$5
    request_demo_mode=$6
    response_number=$((response_number + 1))
    response_headers="$response_directory/headers-$response_number"
    response_body="$response_directory/body-$response_number"

    set -- curl \
        --silent \
        --show-error \
        --globoff \
        --connect-timeout "$connect_timeout" \
        --max-time "$request_timeout" \
        --request "$request_method" \
        --header 'Accept: application/json' \
        --dump-header "$response_headers" \
        --output "$response_body" \
        --write-out '%{http_code}'
    if [ "$request_extra_header" != '-' ]; then
        set -- "$@" --header "$request_extra_header"
    fi
    if [ "$request_demo_mode" != '-' ]; then
        set -- "$@" --header "X-GrowthOS-Demo-Mode: $request_demo_mode"
    fi
    if [ "$request_body" != '-' ]; then
        set -- "$@" --header 'Content-Type: application/json' --data-binary "$request_body"
    fi
    set -- "$@" "$base_url$request_route"
    response_status=$("$@") || fail "request to $request_route failed"
    if [ "$expected_status" = '502-or-504' ]; then
        case "$response_status" in
            502|504)
                ;;
            *)
                fail "$request_method $request_route returned $response_status instead of 502 or 504"
                ;;
        esac
    elif [ "$response_status" != "$expected_status" ]; then
        fail "$request_method $request_route returned $response_status instead of $expected_status"
    fi
    response_content_type=$(header_value Content-Type "$response_headers" | tr '[:upper:]' '[:lower:]')
    if [ "${response_content_type%%;*}" != application/json ] ||
       [ "$(header_count Content-Type "$response_headers")" -ne 1 ]; then
        fail "$request_method $request_route did not return exactly one application/json Content-Type"
    fi
    if ! jq -e . "$response_body" >/dev/null 2>&1; then
        fail "$request_method $request_route did not return valid JSON"
    fi
    if [ "$(header_value Cache-Control "$response_headers")" != no-store ] ||
       [ "$(header_count Cache-Control "$response_headers")" -ne 1 ]; then
        fail "$request_method $request_route did not return exactly one Cache-Control: no-store"
    fi
    response_request_id=$(header_value X-Request-ID "$response_headers")
    if [ -z "$response_request_id" ] ||
       [ "$(header_count X-Request-ID "$response_headers")" -ne 1 ]; then
        fail "$request_method $request_route did not return exactly one request ID"
    fi
}

assert_error_response() {
    expected_code=$1
    expected_message=$2
    if ! jq -e \
        --arg code "$expected_code" \
        --arg message "$expected_message" \
        --arg request_id "$response_request_id" '
            . == {error: {code: $code, message: $message, request_id: $request_id}}
        ' "$response_body" >/dev/null; then
        fail "error response does not match $expected_code and its correlated request ID"
    fi
}

assert_multi_strategy_selection() {
    if ! jq -e '
        .selection.durability == "ephemeral" and
        .selection.strategy_id == "21003" and
        (
            .selection.award == {id: "1", name: "Reward", outcome: "reward"} or
            .selection.award == {id: "2", name: "No reward", outcome: "no_reward"}
        )
    ' "$response_body" >/dev/null; then
        fail "$1 returned an invalid multi-award selection"
    fi
}

identity_file_mode() {
    identity_mode_target=$1
    if identity_mode_value=$(stat -f '%Lp' "$identity_mode_target" 2>/dev/null); then
        printf '%s\n' "$identity_mode_value"
        return 0
    fi
    stat -c '%a' "$identity_mode_target" 2>/dev/null
}

assert_private_identity_file() {
    identity_private_target=$1
    identity_expected_bytes=$2
    if [ -L "$identity_private_target" ] || [ ! -f "$identity_private_target" ] ||
       [ ! -r "$identity_private_target" ]; then
        fail "private Identity artifact is not a readable non-symbolic regular file: $identity_private_target"
    fi
    identity_private_mode=$(identity_file_mode "$identity_private_target") ||
        fail "could not inspect private Identity artifact mode: $identity_private_target"
    if [ "$identity_private_mode" != 600 ]; then
        fail "private Identity artifact mode is $identity_private_mode instead of 0600: $identity_private_target"
    fi
    if [ "$identity_expected_bytes" != '-' ]; then
        identity_private_bytes=$(LC_ALL=C wc -c < "$identity_private_target" | awk '{ print $1 }') ||
            fail "could not inspect private Identity artifact size: $identity_private_target"
        if [ "$identity_private_bytes" != "$identity_expected_bytes" ]; then
            fail "private Identity artifact has $identity_private_bytes bytes instead of $identity_expected_bytes: $identity_private_target"
        fi
    fi
}

write_identity_login_body() {
    identity_body_login=$1
    identity_body_password_file=$2
    identity_body_target=$3
    assert_private_identity_file "$identity_body_password_file" -
    if ! jq -n \
        --arg login_name "$identity_body_login" \
        --rawfile password "$identity_body_password_file" \
        '{login_name: $login_name, password: $password}' \
        > "$identity_body_target"; then
        fail 'could not create the private file-backed login request body'
    fi
    chmod 0600 "$identity_body_target"
    assert_private_identity_file "$identity_body_target" -
}

identity_config_reset() {
    : > "$identity_curl_config"
    chmod 0600 "$identity_curl_config"
    printf '%s\n' 'header = "Accept: application/json"' >> "$identity_curl_config"
}

identity_config_add() {
    identity_config_header=$1
    case "$identity_config_header" in
        *'"'*|*'
'*)
            fail 'an internal Identity curl header contains unsafe configuration syntax'
            ;;
    esac
    printf 'header = "%s"\n' "$identity_config_header" >> "$identity_curl_config"
}

identity_config_add_file_header() {
    identity_secret_header_name=$1
    identity_secret_header_file=$2
    assert_private_identity_file "$identity_secret_header_file" -
    if ! awk -v name="$identity_secret_header_name" '
        BEGIN { valid_name = (name ~ /^[A-Za-z0-9-]+$/) }
        NR == 1 && $0 ~ /^[A-Za-z0-9._-]+$/ {
            printf "header = \"%s: %s\"\n", name, $0
            emitted++
        }
        END { exit (valid_name && NR == 1 && emitted == 1) ? 0 : 1 }
    ' "$identity_secret_header_file" >> "$identity_curl_config"; then
        fail 'the private CSRF header source is malformed'
    fi
}

identity_config_add_duplicate_cookie() {
    identity_cookie_token_file=$1
    assert_private_identity_file "$identity_cookie_token_file" -
    if ! awk '
        NR == 1 && length($0) == 43 && $0 ~ /^[A-Za-z0-9_-]+$/ {
            printf "header = \"Cookie: growthos_dev_session=%s\"\n", $0
            printf "header = \"Cookie: growthos_dev_session=%s\"\n", $0
            emitted = 2
        }
        END { exit (NR == 1 && emitted == 2) ? 0 : 1 }
    ' "$identity_cookie_token_file" >> "$identity_curl_config"; then
        fail 'the duplicate-Cookie token source is malformed'
    fi
}

identity_prepare_login_config() {
    identity_config_reset
    identity_config_add 'Content-Type: application/json'
    identity_config_add "Origin: $base_url"
    identity_config_add 'Sec-Fetch-Site: same-origin'
}

identity_prepare_current_config() {
    identity_config_reset
}

identity_prepare_logout_config() {
    identity_logout_csrf_file=$1
    identity_config_reset
    identity_config_add "Origin: $base_url"
    identity_config_add 'Sec-Fetch-Site: same-origin'
    identity_config_add_file_header X-CSRF-Token "$identity_logout_csrf_file"
}

identity_request() {
    identity_request_method=$1
    identity_request_route=$2
    identity_expected_status=$3
    identity_request_body_file=$4
    identity_cookie_input=$5
    identity_cookie_output=$6
    assert_private_identity_file "$identity_curl_config" -
    if [ "$identity_request_body_file" != '-' ]; then
        assert_private_identity_file "$identity_request_body_file" -
    fi
    if [ "$identity_cookie_input" != '-' ]; then
        assert_private_identity_file "$identity_cookie_input" -
    fi
    if [ "$identity_cookie_output" != '-' ] && [ "$identity_cookie_output" != "$identity_cookie_input" ]; then
        : > "$identity_cookie_output"
        chmod 0600 "$identity_cookie_output"
    fi

    response_number=$((response_number + 1))
    response_headers="$response_directory/headers-$response_number"
    response_body="$response_directory/body-$response_number"
    : > "$response_headers"
    : > "$response_body"
    chmod 0600 "$response_headers" "$response_body"

    set -- curl \
        --silent \
        --show-error \
        --globoff \
        --connect-timeout "$connect_timeout" \
        --max-time "$request_timeout" \
        --request "$identity_request_method" \
        --config "$identity_curl_config" \
        --dump-header "$response_headers" \
        --output "$response_body" \
        --write-out '%{http_code}'
    if [ "$identity_cookie_input" != '-' ]; then
        set -- "$@" --cookie "$identity_cookie_input"
    fi
    if [ "$identity_cookie_output" != '-' ]; then
        set -- "$@" --cookie-jar "$identity_cookie_output"
    fi
    if [ "$identity_request_body_file" != '-' ]; then
        set -- "$@" --data-binary @-
        identity_response_status=$("$@" "$base_url$identity_request_route" < "$identity_request_body_file") ||
            fail "Identity request to $identity_request_route failed"
    else
        identity_response_status=$("$@" "$base_url$identity_request_route") ||
            fail "Identity request to $identity_request_route failed"
    fi
    response_status=$identity_response_status
    if [ "$identity_response_status" != "$identity_expected_status" ]; then
        fail "$identity_request_method $identity_request_route returned $identity_response_status instead of $identity_expected_status"
    fi
    assert_private_identity_file "$response_headers" -
    assert_private_identity_file "$response_body" -
    if [ "$identity_cookie_output" != '-' ]; then
        chmod 0600 "$identity_cookie_output"
        assert_private_identity_file "$identity_cookie_output" -
    fi
    if [ "$(header_value Cache-Control "$response_headers")" != no-store ] ||
       [ "$(header_count Cache-Control "$response_headers")" -ne 1 ]; then
        fail "$identity_request_method $identity_request_route did not return exactly one Cache-Control: no-store"
    fi
    response_request_id=$(header_value X-Request-ID "$response_headers")
    if [ -z "$response_request_id" ] ||
       [ "$(header_count X-Request-ID "$response_headers")" -ne 1 ]; then
        fail "$identity_request_method $identity_request_route did not return exactly one request ID"
    fi
    if [ "$(header_count Content-Security-Policy "$response_headers")" -ne 1 ] ||
       [ "$(header_value Content-Security-Policy "$response_headers")" != "default-src 'none'; frame-ancestors 'none'; base-uri 'none'" ] ||
       [ "$(header_count Cross-Origin-Resource-Policy "$response_headers")" -ne 1 ] ||
       [ "$(header_value Cross-Origin-Resource-Policy "$response_headers")" != same-origin ] ||
       [ "$(header_count Permissions-Policy "$response_headers")" -ne 1 ] ||
       [ "$(header_value Permissions-Policy "$response_headers")" != 'camera=(), geolocation=(), microphone=()' ] ||
       [ "$(header_count Referrer-Policy "$response_headers")" -ne 1 ] ||
       [ "$(header_value Referrer-Policy "$response_headers")" != no-referrer ] ||
       [ "$(header_count X-Content-Type-Options "$response_headers")" -ne 1 ] ||
       [ "$(header_value X-Content-Type-Options "$response_headers")" != nosniff ] ||
       [ "$(header_count X-Frame-Options "$response_headers")" -ne 1 ] ||
       [ "$(header_value X-Frame-Options "$response_headers")" != DENY ]; then
        fail "$identity_request_method $identity_request_route did not return one exact canonical API security-header set"
    fi
    if [ "$identity_expected_status" = 204 ]; then
        if [ -s "$response_body" ] || [ "$(header_count Content-Type "$response_headers")" -ne 0 ]; then
            fail "$identity_request_method $identity_request_route did not return an exact zero-body 204"
        fi
    else
        identity_response_content_type=$(header_value Content-Type "$response_headers" | tr '[:upper:]' '[:lower:]')
        if [ "${identity_response_content_type%%;*}" != application/json ] ||
           [ "$(header_count Content-Type "$response_headers")" -ne 1 ]; then
            fail "$identity_request_method $identity_request_route did not return exactly one application/json Content-Type"
        fi
        if ! jq -e . "$response_body" >/dev/null 2>&1; then
            fail "$identity_request_method $identity_request_route did not return valid JSON"
        fi
    fi
}

assert_identity_error() {
    identity_expected_code=$1
    identity_expected_message=$2
    if ! jq -e \
        --arg code "$identity_expected_code" \
        --arg message "$identity_expected_message" \
        --arg request_id "$response_request_id" '
            . == {error: {code: $code, message: $message, request_id: $request_id}}
        ' "$response_body" >/dev/null; then
        fail "Identity error response does not match $identity_expected_code and its correlated request ID"
    fi
}

identity_error_signature() {
    jq -er '.error | [.code, .message] | join(":")' "$response_body"
}

assert_identity_session() {
    identity_expected_principal=$1
    if ! jq -e --arg principal "$identity_expected_principal" '
        def canonical_utc:
            type == "string" and
            test("^[0-9]{4}-(0[1-9]|1[0-2])-(0[1-9]|[12][0-9]|3[01])T([01][0-9]|2[0-3]):[0-5][0-9]:[0-5][0-9](\\.[0-9]{1,9})?Z$");
        (keys == ["data"]) and
        (.data | keys == [
            "absolute_expires_at",
            "authenticated",
            "csrf_token",
            "idle_expires_at",
            "principal"
        ]) and
        .data.authenticated == true and
        .data.principal == {kind: "human", id: $principal} and
        (.data.idle_expires_at | canonical_utc) and
        (.data.absolute_expires_at | canonical_utc) and
        (.data.idle_expires_at < .data.absolute_expires_at) and
        (.data.csrf_token | type == "string" and length > 32 and length <= 512 and test("^[A-Za-z0-9._-]+$")) and
        ([paths | .[-1] | strings] | all(
            . != "role" and . != "roles" and
            . != "scope" and . != "scopes" and
            . != "permission" and . != "permissions" and
            . != "account" and . != "account_id" and
            . != "login" and . != "login_name" and
            . != "token" and . != "session_token"
        ))
    ' "$response_body" >/dev/null; then
        fail 'Identity session DTO is not the exact authentication-only public shape'
    fi
}

assert_identity_set_cookie() {
    identity_cookie_jar=$1
    if [ "$(header_count Set-Cookie "$response_headers")" -ne 1 ] ||
       ! awk '
            {
                line = $0
                sub(/\r$/, "", line)
                if (line !~ /^[Ss]et-[Cc]ookie:[[:space:]]*growthos_dev_session=/) {
                    next
                }
                sub(/^[^:]*:[[:space:]]*/, "", line)
                found++
                if (split(line, field, "; ") != 6) {
                    next
                }
                token = field[1]
                sub(/^growthos_dev_session=/, "", token)
                canonical = (field[1] == "growthos_dev_session=" token)
                canonical = (canonical && length(token) == 43 && token ~ /^[A-Za-z0-9_-]+$/)
                canonical = (canonical && field[2] == "Path=/")
                canonical = (canonical && field[3] ~ /^Expires=[A-Z][a-z][a-z], [0-9][0-9] [A-Z][a-z][a-z] [0-9][0-9][0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9] GMT$/)
                canonical = (canonical && field[4] ~ /^Max-Age=[1-9][0-9]*$/)
                canonical = (canonical && field[5] == "HttpOnly")
                canonical = (canonical && field[6] == "SameSite=Strict")
                if (canonical) {
                    valid++
                }
            }
            END { exit (found == 1 && valid == 1) ? 0 : 1 }
        ' "$response_headers"; then
        fail 'development session Set-Cookie is not the exact six-field token, Path, expiry, Max-Age, HttpOnly, Strict, host-only, insecure-loopback tuple'
    fi
    assert_private_identity_file "$identity_cookie_jar" -
    if ! awk -F '\t' '
        $6 == "growthos_dev_session" {
            candidate = ($1 == "#HttpOnly_127.0.0.1" && $2 == "FALSE")
            candidate = (candidate && $3 == "/" && $4 == "FALSE")
            candidate = (candidate && $5 ~ /^[0-9]+$/)
            candidate = (candidate && length($7) == 43 && $7 ~ /^[A-Za-z0-9_-]+$/)
            if (candidate) {
                valid++
            }
            found++
        }
        END { exit (found == 1 && valid == 1) ? 0 : 1 }
    ' "$identity_cookie_jar"; then
        fail 'curl did not persist the development Cookie as one HttpOnly host-only non-Secure Path=/ credential'
    fi
}

assert_no_set_cookie() {
    if [ "$(header_count Set-Cookie "$response_headers")" -ne 0 ]; then
        fail 'the Identity response unexpectedly changed the session Cookie'
    fi
}

assert_identity_clear_cookie() {
    identity_cleared_cookie_jar=$1
    if [ "$(header_count Set-Cookie "$response_headers")" -ne 1 ] ||
       ! awk '
            {
                line = $0
                sub(/\r$/, "", line)
                if (line !~ /^[Ss]et-[Cc]ookie:[[:space:]]*growthos_dev_session=;/) {
                    next
                }
                sub(/^[^:]*:[[:space:]]*/, "", line)
                found++
                if (line == "growthos_dev_session=; Path=/; Expires=Thu, 01 Jan 1970 00:00:01 GMT; Max-Age=0; HttpOnly; SameSite=Strict") {
                    valid++
                }
            }
            END { exit (found == 1 && valid == 1) ? 0 : 1 }
        ' "$response_headers"; then
        fail 'logout did not emit the exact development Cookie deletion tuple'
    fi
    if awk -F '\t' '$6 == "growthos_dev_session" { found++ } END { exit found == 0 ? 0 : 1 }' \
        "$identity_cleared_cookie_jar"; then
        :
    else
        fail 'logout left the deleted session credential in the output Cookie jar'
    fi
}

extract_identity_cookie_token() {
    identity_extract_cookie_jar=$1
    identity_extract_token_target=$2
    assert_private_identity_file "$identity_extract_cookie_jar" -
    if ! awk -F '\t' '
        $6 == "growthos_dev_session" && length($7) == 43 && $7 ~ /^[A-Za-z0-9_-]+$/ {
            print $7
            found++
        }
        END { exit found == 1 ? 0 : 1 }
    ' "$identity_extract_cookie_jar" > "$identity_extract_token_target"; then
        fail 'could not extract exactly one canonical raw session token from the private Cookie jar'
    fi
    chmod 0600 "$identity_extract_token_target"
    assert_private_identity_file "$identity_extract_token_target" 44
}

extract_identity_csrf() {
    identity_extract_body=$1
    identity_extract_csrf_target=$2
    if ! jq -er '.data.csrf_token | select(type == "string" and length > 32 and test("^[A-Za-z0-9._-]+$"))' \
        "$identity_extract_body" > "$identity_extract_csrf_target"; then
        fail 'could not extract one canonical CSRF token from the private response body'
    fi
    chmod 0600 "$identity_extract_csrf_target"
    assert_private_identity_file "$identity_extract_csrf_target" -
}

record_identity_login_secrets() {
    identity_record_cookie=$1
    identity_record_body=$2
    identity_record_token_target=$3
    identity_record_csrf_target=$4
    extract_identity_cookie_token "$identity_record_cookie" "$identity_record_token_target"
    extract_identity_csrf "$identity_record_body" "$identity_record_csrf_target"
    cat "$identity_record_token_target" >> "$identity_sensitive_patterns"
    cat "$identity_record_csrf_target" >> "$identity_sensitive_patterns"
}

record_identity_current_csrf() {
    identity_record_current_body=$1
    identity_record_current_target=$2
    extract_identity_csrf "$identity_record_current_body" "$identity_record_current_target"
    cat "$identity_record_current_target" >> "$identity_sensitive_patterns"
}

# Session HTTP acceptance starts from an empty Identity data plane and creates
# its only credential through the real INSERT-only operations service. Raw
# password, Cookie, and CSRF bytes remain file-backed throughout the flow.
identity_state_before=$(mysql_root_execute '
    SELECT CONCAT(
        (SELECT COUNT(*) FROM identity_workforce_account), CHAR(58),
        (SELECT COUNT(*) FROM identity_session), CHAR(58),
        (SELECT COUNT(*) FROM identity_authentication_throttle)
    )
') || fail 'could not inspect the empty Identity state before HTTP acceptance'
if [ "$identity_state_before" != '0:0:0' ]; then
    fail "Identity HTTP acceptance did not start empty: $identity_state_before"
fi

identity_account_id="accept.http.$random_suffix.account"
identity_login_name="http_$random_suffix"
identity_unknown_login="unknown_$random_suffix"
identity_principal_id="accept.http.$random_suffix.principal"
identity_password_file="$identity_directory/enrollment-password"
identity_wrong_password_file="$identity_directory/wrong-password"
identity_password_snapshot="$identity_directory/enrollment-password-snapshot"
identity_login_body="$identity_directory/login-body"
identity_malformed_body="$identity_directory/malformed-body"
identity_curl_config="$identity_directory/curl.conf"
identity_sensitive_patterns="$identity_directory/sensitive-patterns"
identity_token_a="$identity_directory/token-a"
identity_token_b="$identity_directory/token-b"
identity_token_scratch="$identity_directory/token-scratch"
identity_csrf_a="$identity_directory/csrf-a"
identity_csrf_b="$identity_directory/csrf-b"
identity_csrf_current="$identity_directory/csrf-current"
identity_cookie_a="$identity_directory/cookie-a"
identity_cookie_b="$identity_directory/cookie-b"
identity_cookie_state="$identity_directory/cookie-state"
identity_provision_output="$identity_directory/provision-output"
identity_api_logs="$identity_directory/api-logs"
identity_web_logs="$identity_directory/web-logs"

openssl rand -hex 24 | tr -d '\n' > "$identity_password_file"
openssl rand -hex 24 | tr -d '\n' > "$identity_wrong_password_file"
chmod 0600 "$identity_password_file" "$identity_wrong_password_file"
assert_private_identity_file "$identity_password_file" 48
assert_private_identity_file "$identity_wrong_password_file" 48
if cmp -s "$identity_password_file" "$identity_wrong_password_file"; then
    fail 'the valid and wrong Identity passwords unexpectedly match'
else
    identity_comparison_status=$?
    if [ "$identity_comparison_status" -ne 1 ]; then
        fail 'could not compare the isolated Identity passwords'
    fi
fi
: > "$identity_sensitive_patterns"
{
    cat "$identity_password_file"
    printf '\n'
    cat "$identity_wrong_password_file"
    printf '\n'
} >> "$identity_sensitive_patterns"
chmod 0600 "$identity_sensitive_patterns"

identity_password_snapshot_bytes=48
cp "$identity_password_file" "$identity_password_snapshot"
identity_password_snapshot_bytes=$(LC_ALL=C wc -c < "$identity_password_snapshot" | awk '{ print $1 }')
if [ "$identity_password_snapshot_bytes" -ne 48 ] ||
   ! cmp -s "$identity_password_file" "$identity_password_snapshot"; then
    fail 'the bounded enrollment snapshot differs from its 0600 caller source'
fi
# uid 65532 needs the bind-mounted file read bit. The containing directory is
# still mode 0700; the exact snapshot is overwritten and unlinked immediately.
chmod 0444 "$identity_password_snapshot"
if ! compose --progress quiet --profile operations run \
    --rm \
    --no-deps \
    --no-tty \
    --volume "$identity_password_snapshot:/run/identity-enrollment/password:ro" \
    identity-provision \
    create \
    --account-id "$identity_account_id" \
    --login-name "$identity_login_name" \
    --principal-id "$identity_principal_id" \
    --password-file /run/identity-enrollment/password \
    > "$identity_provision_output" 2>&1; then
    fail 'the operations-only Identity provisioner did not create the HTTP fixture account'
fi
remove_identity_password_snapshot || fail 'could not securely remove the enrollment snapshot'
if [ -e "$identity_password_snapshot" ] || [ -L "$identity_password_snapshot" ]; then
    fail 'the enrollment snapshot remains after the one-shot provisioner returned'
fi
assert_private_identity_file "$identity_password_file" 48
if [ -n "$(compose --profile operations ps --all --quiet identity-provision)" ]; then
    fail 'the one-shot Identity provisioner left a project container behind'
fi
if ! jq -e -s \
    --arg service growth-identity-provision \
    --arg version lesson-32 '
        length == 1 and
        .[0].msg == "identity account provision completed" and
        .[0].service == $service and
        .[0].version == $version and
        .[0].environment == "development" and
        .[0].component == "identity_provisioning" and
        .[0].operation == "create" and
        .[0].result == "created" and
        (((.[0] | has("password")) or
          (.[0] | has("password_envelope")) or
          (.[0] | has("token"))) | not)
    ' "$identity_provision_output" >/dev/null; then
    fail 'the Identity provisioner did not emit its exact redacted success record'
fi
identity_account_state=$(mysql_root_execute "
    SELECT CONCAT_WS(CHAR(58), account_id, login_name, principal_id,
        account_status, credential_version, authentication_epoch)
    FROM identity_workforce_account
    WHERE account_id = '$identity_account_id'
") || fail 'could not inspect the provisioned Identity HTTP fixture'
if [ "$identity_account_state" != "$identity_account_id:$identity_login_name:$identity_principal_id:enabled:1:1" ]; then
    fail 'the provisioned Identity HTTP fixture differs from the reviewed identity and lifecycle state'
fi
ok 'the INSERT-only operations provisioner created the sole HTTP credential from a private bounded password file'

# Strict login vocabulary: media type, JSON grammar, query, alternate
# credential sources, Origin, and Fetch Metadata all reject before login.
write_identity_login_body "$identity_login_name" "$identity_password_file" "$identity_login_body"
identity_config_reset
identity_config_add 'Content-Type:'
identity_config_add "Origin: $base_url"
identity_config_add 'Sec-Fetch-Site: same-origin'
identity_request POST /api/v1/session 415 "$identity_login_body" - -
assert_identity_error unsupported_media_type 'unsupported media type'
assert_no_set_cookie

identity_config_reset
identity_config_add 'Content-Type: application/json; charset=utf-8'
identity_config_add "Origin: $base_url"
identity_config_add 'Sec-Fetch-Site: same-origin'
identity_request POST /api/v1/session 415 "$identity_login_body" - -
assert_identity_error unsupported_media_type 'unsupported media type'
assert_no_set_cookie

identity_config_reset
identity_config_add 'Content-Type: application/json'
identity_config_add 'Content-Type: application/json'
identity_config_add "Origin: $base_url"
identity_config_add 'Sec-Fetch-Site: same-origin'
identity_request POST /api/v1/session 415 "$identity_login_body" - -
assert_identity_error unsupported_media_type 'unsupported media type'
assert_no_set_cookie

printf '%s' '{"login_name":' > "$identity_malformed_body"
chmod 0600 "$identity_malformed_body"
identity_prepare_login_config
identity_request POST /api/v1/session 400 "$identity_malformed_body" - -
assert_identity_error invalid_request 'invalid request'
assert_no_set_cookie

printf '%s' '{"login_name":"operator","login_name":"duplicate","password":"rejected"}' > "$identity_malformed_body"
identity_prepare_login_config
identity_request POST /api/v1/session 400 "$identity_malformed_body" - -
assert_identity_error invalid_request 'invalid request'
assert_no_set_cookie

printf '%s' '{"login_name":"operator","password":"rejected"}{"trailing":true}' > "$identity_malformed_body"
# Fetch Metadata is optional for controlled non-browser clients; the exact
# Origin remains mandatory and the downstream JSON grammar still fails closed.
identity_config_reset
identity_config_add 'Content-Type: application/json'
identity_config_add "Origin: $base_url"
identity_request POST /api/v1/session 400 "$identity_malformed_body" - -
assert_identity_error invalid_request 'invalid request'
assert_no_set_cookie

# The edge is the last component that can observe transfer framing before
# nginx dechunks the body. It must reject both chunked transfer and declared
# trailers, while an ordinary declared body larger than the Identity adapter's
# 2 KiB limit must still reach Go and fail closed there.
identity_prepare_login_config
identity_config_add 'Transfer-Encoding: chunked'
identity_request POST /api/v1/session 400 "$identity_login_body" - -
assert_identity_error request_body_not_allowed 'request body is not allowed'
assert_no_set_cookie

identity_prepare_login_config
identity_config_add 'Trailer: X-Acceptance-Trailer'
identity_request POST /api/v1/session 400 "$identity_login_body" - -
assert_identity_error request_body_not_allowed 'request body is not allowed'
assert_no_set_cookie

awk 'BEGIN { for (i = 0; i < 2049; i++) printf "x" }' > "$identity_malformed_body"
chmod 0600 "$identity_malformed_body"
assert_private_identity_file "$identity_malformed_body" 2049
identity_prepare_login_config
identity_request POST /api/v1/session 400 "$identity_malformed_body" - -
assert_identity_error invalid_request 'invalid request'
assert_no_set_cookie

identity_prepare_login_config
identity_request POST '/api/v1/session?credential=forbidden' 400 "$identity_login_body" - -
assert_identity_error invalid_request 'invalid request'
assert_no_set_cookie

identity_prepare_login_config
identity_config_add 'Authorization: Basic YXR0YWNrZXI6c2VjcmV0'
identity_request POST /api/v1/session 400 "$identity_login_body" - -
assert_identity_error invalid_request 'invalid request'
assert_no_set_cookie

for identity_forbidden_header in X-Account-ID X-Principal-ID X-Role X-Permission X-Scope X-Tenant-ID; do
    identity_prepare_login_config
    identity_config_add "$identity_forbidden_header: attacker"
    identity_request POST /api/v1/session 400 "$identity_login_body" - -
    assert_identity_error invalid_request 'invalid request'
    assert_no_set_cookie
done

identity_config_reset
identity_config_add 'Content-Type: application/json'
identity_config_add 'Sec-Fetch-Site: same-origin'
identity_request POST /api/v1/session 403 "$identity_login_body" - -
assert_identity_error request_origin_rejected 'request origin rejected'
assert_no_set_cookie

identity_config_reset
identity_config_add 'Content-Type: application/json'
identity_config_add 'Origin: http://127.0.0.1:1'
identity_config_add 'Sec-Fetch-Site: same-origin'
identity_request POST /api/v1/session 403 "$identity_login_body" - -
assert_identity_error request_origin_rejected 'request origin rejected'
assert_no_set_cookie

identity_config_reset
identity_config_add 'Content-Type: application/json'
identity_config_add "Origin: $base_url"
identity_config_add "Origin: $base_url"
identity_config_add 'Sec-Fetch-Site: same-origin'
identity_request POST /api/v1/session 403 "$identity_login_body" - -
assert_identity_error request_origin_rejected 'request origin rejected'
assert_no_set_cookie

identity_config_reset
identity_config_add 'Content-Type: application/json'
identity_config_add "Origin: $base_url"
identity_config_add 'Sec-Fetch-Site: same-site'
identity_request POST /api/v1/session 403 "$identity_login_body" - -
assert_identity_error request_origin_rejected 'request origin rejected'
assert_no_set_cookie

identity_config_reset
identity_config_add 'Content-Type: application/json'
identity_config_add "Origin: $base_url"
identity_config_add 'Sec-Fetch-Site: same-origin'
identity_config_add 'Sec-Fetch-Site: same-origin'
identity_request POST /api/v1/session 403 "$identity_login_body" - -
assert_identity_error request_origin_rejected 'request origin rejected'
assert_no_set_cookie

# Wrong, unknown, and later disabled accounts intentionally share one public
# 401 contract. Their request bodies remain private files, never curl argv.
write_identity_login_body "$identity_login_name" "$identity_wrong_password_file" "$identity_login_body"
identity_prepare_login_config
identity_request POST /api/v1/session 401 "$identity_login_body" - -
assert_identity_error authentication_failed 'authentication failed'
assert_no_set_cookie
wrong_login_signature=$(identity_error_signature)

write_identity_login_body "$identity_unknown_login" "$identity_password_file" "$identity_login_body"
identity_prepare_login_config
identity_request POST /api/v1/session 401 "$identity_login_body" - -
assert_identity_error authentication_failed 'authentication failed'
assert_no_set_cookie
unknown_login_signature=$(identity_error_signature)

# Login -> current -> replacement proves both the normal DTO and fixation
# defense. No authorization vocabulary is allowed into the session DTO.
write_identity_login_body "$identity_login_name" "$identity_password_file" "$identity_login_body"
identity_prepare_login_config
identity_request POST /api/v1/session 201 "$identity_login_body" - "$identity_cookie_a"
assert_identity_session "$identity_principal_id"
assert_identity_set_cookie "$identity_cookie_a"
record_identity_login_secrets "$identity_cookie_a" "$response_body" "$identity_token_a" "$identity_csrf_a"

identity_prepare_current_config
identity_request GET /api/v1/session 200 - "$identity_cookie_a" -
assert_identity_session "$identity_principal_id"
assert_no_set_cookie
record_identity_current_csrf "$response_body" "$identity_csrf_current"

identity_prepare_login_config
identity_request POST /api/v1/session 201 "$identity_login_body" "$identity_cookie_a" "$identity_cookie_b"
assert_identity_session "$identity_principal_id"
assert_identity_set_cookie "$identity_cookie_b"
record_identity_login_secrets "$identity_cookie_b" "$response_body" "$identity_token_b" "$identity_csrf_b"
if cmp -s "$identity_token_a" "$identity_token_b"; then
    fail 'login fixation defense reused the incoming session token'
else
    identity_comparison_status=$?
    if [ "$identity_comparison_status" -ne 1 ]; then
        fail 'could not compare the login replacement tokens'
    fi
fi
identity_fixation_state=$(mysql_root_execute "
    SELECT CONCAT(
        COUNT(*), CHAR(58),
        SUM(revoked_at IS NULL), CHAR(58),
        SUM(revoke_reason = 'security_response')
    )
    FROM identity_session
    WHERE account_id = '$identity_account_id'
") || fail 'could not inspect the fixation replacement state'
if [ "$identity_fixation_state" != '2:1:1' ]; then
    fail "login fixation replacement state is $identity_fixation_state instead of 2:1:1"
fi

identity_config_reset
identity_config_add_duplicate_cookie "$identity_token_b"
identity_request GET /api/v1/session 401 - - "$identity_cookie_state"
assert_identity_error unauthenticated 'authentication required'
assert_identity_clear_cookie "$identity_cookie_state"

identity_prepare_current_config
identity_request GET /api/v1/session 401 - "$identity_cookie_a" "$identity_cookie_state"
assert_identity_error unauthenticated 'authentication required'
assert_identity_clear_cookie "$identity_cookie_state"

printf '%s' 'forbidden-body' > "$identity_malformed_body"
identity_prepare_current_config
identity_request GET /api/v1/session 400 "$identity_malformed_body" "$identity_cookie_b" -
assert_identity_error invalid_request 'invalid request'
assert_no_set_cookie

identity_prepare_logout_config "$identity_csrf_b"
identity_request DELETE '/api/v1/session?session=forbidden' 400 - "$identity_cookie_b" -
assert_identity_error invalid_request 'invalid request'
assert_no_set_cookie

identity_prepare_logout_config "$identity_csrf_b"
identity_request DELETE /api/v1/session 400 "$identity_malformed_body" "$identity_cookie_b" -
assert_identity_error invalid_request 'invalid request'
assert_no_set_cookie

identity_config_reset
identity_config_add "Origin: $base_url"
identity_config_add 'Sec-Fetch-Site: same-origin'
identity_request DELETE /api/v1/session 403 - "$identity_cookie_b" -
assert_identity_error request_origin_rejected 'request origin rejected'
assert_no_set_cookie

identity_config_reset
identity_config_add 'Sec-Fetch-Site: same-origin'
identity_config_add_file_header X-CSRF-Token "$identity_csrf_b"
identity_request DELETE /api/v1/session 403 - "$identity_cookie_b" -
assert_identity_error request_origin_rejected 'request origin rejected'
assert_no_set_cookie

identity_config_reset
identity_config_add 'Origin: http://127.0.0.1:1'
identity_config_add 'Sec-Fetch-Site: same-origin'
identity_config_add_file_header X-CSRF-Token "$identity_csrf_b"
identity_request DELETE /api/v1/session 403 - "$identity_cookie_b" -
assert_identity_error request_origin_rejected 'request origin rejected'
assert_no_set_cookie

identity_config_reset
identity_config_add "Origin: $base_url"
identity_config_add 'Sec-Fetch-Site: cross-site'
identity_config_add_file_header X-CSRF-Token "$identity_csrf_b"
identity_request DELETE /api/v1/session 403 - "$identity_cookie_b" -
assert_identity_error request_origin_rejected 'request origin rejected'
assert_no_set_cookie

identity_config_reset
identity_config_add "Origin: $base_url"
identity_config_add "Origin: $base_url"
identity_config_add 'Sec-Fetch-Site: same-origin'
identity_config_add_file_header X-CSRF-Token "$identity_csrf_b"
identity_request DELETE /api/v1/session 403 - "$identity_cookie_b" -
assert_identity_error request_origin_rejected 'request origin rejected'
assert_no_set_cookie

identity_config_reset
identity_config_add "Origin: $base_url"
identity_config_add 'Sec-Fetch-Site: same-origin'
identity_config_add 'Sec-Fetch-Site: same-origin'
identity_config_add_file_header X-CSRF-Token "$identity_csrf_b"
identity_request DELETE /api/v1/session 403 - "$identity_cookie_b" -
assert_identity_error request_origin_rejected 'request origin rejected'
assert_no_set_cookie

identity_config_reset
identity_config_add "Origin: $base_url"
identity_config_add 'Sec-Fetch-Site: same-origin'
identity_config_add 'X-CSRF-Token: invalid'
identity_request DELETE /api/v1/session 403 - "$identity_cookie_b" -
assert_identity_error request_origin_rejected 'request origin rejected'
assert_no_set_cookie

identity_config_reset
identity_config_add "Origin: $base_url"
identity_config_add 'Sec-Fetch-Site: same-origin'
identity_config_add_file_header X-CSRF-Token "$identity_csrf_b"
identity_config_add_file_header X-CSRF-Token "$identity_csrf_b"
identity_request DELETE /api/v1/session 403 - "$identity_cookie_b" -
assert_identity_error request_origin_rejected 'request origin rejected'
assert_no_set_cookie

identity_prepare_logout_config "$identity_csrf_a"
identity_request DELETE /api/v1/session 403 - "$identity_cookie_b" -
assert_identity_error request_origin_rejected 'request origin rejected'
assert_no_set_cookie

identity_prepare_logout_config "$identity_csrf_b"
identity_request DELETE /api/v1/session 204 - "$identity_cookie_b" "$identity_cookie_state"
assert_identity_clear_cookie "$identity_cookie_state"

identity_prepare_current_config
identity_request GET /api/v1/session 401 - "$identity_cookie_b" "$identity_cookie_state"
assert_identity_error unauthenticated 'authentication required'
assert_identity_clear_cookie "$identity_cookie_state"
identity_logout_state=$(mysql_root_execute "
    SELECT CONCAT(
        COUNT(*), CHAR(58), SUM(revoked_at IS NULL), CHAR(58),
        SUM(revoke_reason = 'security_response'), CHAR(58),
        SUM(revoke_reason = 'logout')
    )
    FROM identity_session
    WHERE account_id = '$identity_account_id'
") || fail 'could not inspect the exact logout state'
if [ "$identity_logout_state" != '2:0:1:1' ]; then
    fail "logout state is $identity_logout_state instead of 2:0:1:1"
fi
ok 'Session login/current/logout, token replacement, replay, Cookie shape, and CSRF boundaries passed through Nginx'

# Six independent browser logins retain exactly five active sessions and evict
# only the deterministic oldest one.
identity_cap_index=1
while [ "$identity_cap_index" -le 6 ]; do
    identity_cap_cookie="$identity_directory/cap-cookie-$identity_cap_index"
    identity_prepare_login_config
    identity_request POST /api/v1/session 201 "$identity_login_body" - "$identity_cap_cookie"
    assert_identity_session "$identity_principal_id"
    assert_identity_set_cookie "$identity_cap_cookie"
    record_identity_login_secrets \
        "$identity_cap_cookie" "$response_body" \
        "$identity_token_scratch" "$identity_csrf_current"
    identity_cap_index=$((identity_cap_index + 1))
done
identity_cap_state=$(mysql_root_execute "
    SELECT CONCAT(
        COUNT(*), CHAR(58), SUM(revoked_at IS NULL), CHAR(58),
        SUM(revoke_reason = 'concurrency_limit')
    )
    FROM identity_session
    WHERE account_id = '$identity_account_id'
") || fail 'could not inspect the active-session cap'
if [ "$identity_cap_state" != '8:5:1' ]; then
    fail "active-session cap state is $identity_cap_state instead of 8:5:1"
fi
identity_prepare_current_config
identity_request GET /api/v1/session 401 - "$identity_directory/cap-cookie-1" "$identity_cookie_state"
assert_identity_error unauthenticated 'authentication required'
assert_identity_clear_cookie "$identity_cookie_state"
identity_prepare_current_config
identity_request GET /api/v1/session 200 - "$identity_directory/cap-cookie-6" -
assert_identity_session "$identity_principal_id"
record_identity_current_csrf "$response_body" "$identity_csrf_current"
ok 'the sixth independent login preserved an exact maximum of five active sessions and evicted the oldest token'

# A valid Cookie remains an indeterminate server-side credential while MySQL
# is down: both current and login return 503, never anonymous 401, and the same
# Cookie recovers once the existing database container is healthy again.
resolve_container mysql
identity_mysql_container_id=$resolved_container_id
if ! compose stop mysql; then
    fail 'could not stop disposable MySQL for Identity dependency acceptance'
fi
if [ "$(docker inspect --format '{{.State.Status}}' "$identity_mysql_container_id")" != exited ]; then
    fail 'disposable MySQL did not stop for Identity dependency acceptance'
fi
identity_prepare_current_config
identity_request GET /api/v1/session 503 - "$identity_directory/cap-cookie-6" -
assert_identity_error authentication_unavailable 'authentication temporarily unavailable'
assert_no_set_cookie
identity_prepare_login_config
identity_request POST /api/v1/session 503 "$identity_login_body" - -
assert_identity_error authentication_unavailable 'authentication temporarily unavailable'
assert_no_set_cookie
if ! compose up --detach --wait --wait-timeout 120 mysql; then
    fail 'could not restore disposable MySQL after Identity dependency acceptance'
fi
resolve_container mysql
if [ "$resolved_container_id" != "$identity_mysql_container_id" ]; then
    fail 'Identity dependency recovery replaced the disposable MySQL container'
fi
assert_running_healthy api
identity_prepare_current_config
identity_request GET /api/v1/session 200 - "$identity_directory/cap-cookie-6" -
assert_identity_session "$identity_principal_id"
record_identity_current_csrf "$response_body" "$identity_csrf_current"
ok 'Identity returned exact 503 while MySQL was unavailable and the same server-side session recovered'

# Controlled root-only lifecycle mutations exercise inactive session states;
# credentials themselves still originate solely from the provisioner.
identity_expired_update=$(mysql_root_execute "
    SET @observed = UTC_TIMESTAMP(6);
    UPDATE identity_session AS target
    JOIN (
        SELECT session_ref
        FROM identity_session
        WHERE account_id = '$identity_account_id'
          AND revoked_at IS NULL
        ORDER BY issued_at DESC, session_ref DESC
        LIMIT 1
    ) AS selected ON selected.session_ref = target.session_ref
    SET target.issued_at = DATE_SUB(@observed, INTERVAL 3 MINUTE),
        target.last_seen_at = DATE_SUB(@observed, INTERVAL 2 MINUTE),
        target.idle_expires_at = DATE_SUB(@observed, INTERVAL 1 MINUTE),
        target.absolute_expires_at = DATE_ADD(@observed, INTERVAL 1 HOUR),
        target.updated_at = @observed;
    SELECT ROW_COUNT();
") || fail 'could not create the controlled expired-session state'
if [ "$identity_expired_update" != 1 ]; then
    fail 'the controlled expired-session mutation did not affect exactly one row'
fi
identity_prepare_current_config
identity_request GET /api/v1/session 401 - "$identity_directory/cap-cookie-6" "$identity_cookie_state"
assert_identity_error unauthenticated 'authentication required'
assert_identity_clear_cookie "$identity_cookie_state"
expired_current_signature=$(identity_error_signature)

identity_prepare_login_config
identity_request POST /api/v1/session 201 "$identity_login_body" - "$identity_cookie_state"
assert_identity_session "$identity_principal_id"
assert_identity_set_cookie "$identity_cookie_state"
record_identity_login_secrets \
    "$identity_cookie_state" "$response_body" \
    "$identity_token_scratch" "$identity_csrf_current"
identity_epoch_update=$(mysql_root_execute "
    UPDATE identity_workforce_account
    SET authentication_epoch = authentication_epoch + 1,
        updated_at = UTC_TIMESTAMP(6)
    WHERE account_id = '$identity_account_id'
      AND account_status = 'enabled'
      AND authentication_epoch = 1;
    SELECT ROW_COUNT();
") || fail 'could not create the controlled authentication-epoch mismatch'
if [ "$identity_epoch_update" != 1 ]; then
    fail 'the authentication-epoch mutation did not affect exactly one account'
fi
identity_prepare_current_config
identity_request GET /api/v1/session 401 - "$identity_cookie_state" "$identity_cookie_a"
assert_identity_error unauthenticated 'authentication required'
assert_identity_clear_cookie "$identity_cookie_a"
epoch_current_signature=$(identity_error_signature)

identity_prepare_login_config
identity_request POST /api/v1/session 201 "$identity_login_body" - "$identity_cookie_state"
assert_identity_session "$identity_principal_id"
assert_identity_set_cookie "$identity_cookie_state"
record_identity_login_secrets \
    "$identity_cookie_state" "$response_body" \
    "$identity_token_scratch" "$identity_csrf_current"
identity_disable_update=$(mysql_root_execute "
    UPDATE identity_workforce_account
    SET account_status = 'disabled', updated_at = UTC_TIMESTAMP(6)
    WHERE account_id = '$identity_account_id'
      AND account_status = 'enabled'
      AND authentication_epoch = 2;
    SELECT ROW_COUNT();
") || fail 'could not create the controlled disabled-account state'
if [ "$identity_disable_update" != 1 ]; then
    fail 'the disabled-account mutation did not affect exactly one account'
fi
identity_prepare_current_config
identity_request GET /api/v1/session 401 - "$identity_cookie_state" "$identity_cookie_a"
assert_identity_error unauthenticated 'authentication required'
assert_identity_clear_cookie "$identity_cookie_a"
disabled_current_signature=$(identity_error_signature)
identity_prepare_login_config
identity_request POST /api/v1/session 401 "$identity_login_body" - -
assert_identity_error authentication_failed 'authentication failed'
assert_no_set_cookie
disabled_login_signature=$(identity_error_signature)
if [ "$wrong_login_signature" != "$unknown_login_signature" ] ||
   [ "$wrong_login_signature" != "$disabled_login_signature" ]; then
    fail 'wrong, unknown, and disabled credentials did not share one exact public 401 contract'
fi
if [ "$expired_current_signature" != "$epoch_current_signature" ] ||
   [ "$expired_current_signature" != "$disabled_current_signature" ]; then
    fail 'expired, epoch-mismatched, and disabled sessions did not share one exact public 401 contract'
fi
ok 'wrong/unknown/disabled login and expired/epoch/disabled current-session states remained indistinguishable'

# Isolate the public persistent-throttle contract after all successful-login
# scenarios. The disposable data plane started empty and currently contains
# exactly the three rows asserted below, so this bounded reset cannot touch a
# pre-existing or unrelated tenant. Five failures on one login close only its
# login dimension (threshold 5); the source dimension remains open at 5 of 30.
# The sixth request must still return a stable 429 with no Cookie.
identity_throttle_reset=$(mysql_root_execute '
    DELETE FROM identity_authentication_throttle;
    SELECT ROW_COUNT();
') || fail 'could not reset the owned disposable throttle rows before the 429 probe'
if [ "$identity_throttle_reset" != 3 ]; then
    fail "Identity throttle reset removed $identity_throttle_reset rows instead of the exact owned three"
fi
identity_throttled_login="throttled_$random_suffix"
write_identity_login_body "$identity_throttled_login" "$identity_password_file" "$identity_login_body"
identity_throttle_attempt=1
while [ "$identity_throttle_attempt" -le 5 ]; do
    identity_prepare_login_config
    identity_request POST /api/v1/session 401 "$identity_login_body" - -
    assert_identity_error authentication_failed 'authentication failed'
    assert_no_set_cookie
    identity_throttle_attempt=$((identity_throttle_attempt + 1))
done
identity_prepare_login_config
identity_request POST /api/v1/session 429 "$identity_login_body" - -
assert_identity_error authentication_throttled 'authentication throttled'
assert_no_set_cookie
identity_throttle_state=$(mysql_root_execute '
    SELECT CONCAT(
        COUNT(*), CHAR(58),
        COALESCE(SUM(failure_count), 0), CHAR(58),
        COALESCE(SUM(inflight_count), 0), CHAR(58),
        COALESCE(SUM(blocked_until > UTC_TIMESTAMP(6)), 0)
    )
    FROM identity_authentication_throttle;
') || fail 'could not inspect the persistent HTTP throttle state'
if [ "$identity_throttle_state" != '2:10:0:1' ]; then
    fail "persistent login-throttle state is $identity_throttle_state instead of 2:10:0:1"
fi
ok 'five failures closed only the login dimension and the sixth request returned an exact Cookie-free 429'

# Reset the two owned rows, then distribute failures across thirty distinct
# login names. No per-login row reaches five, so only the shared source row can
# close; the thirty-first distinct login proves the independent source=30 gate.
identity_throttle_reset=$(mysql_root_execute '
    DELETE FROM identity_authentication_throttle;
    SELECT ROW_COUNT();
') || fail 'could not reset the owned login-throttle rows before the source probe'
if [ "$identity_throttle_reset" != 2 ]; then
    fail "Identity login-throttle reset removed $identity_throttle_reset rows instead of two"
fi
identity_source_attempt=1
while [ "$identity_source_attempt" -le 30 ]; do
    identity_source_login="source_${identity_source_attempt}_$random_suffix"
    write_identity_login_body "$identity_source_login" "$identity_password_file" "$identity_login_body"
    identity_prepare_login_config
    identity_request POST /api/v1/session 401 "$identity_login_body" - -
    assert_identity_error authentication_failed 'authentication failed'
    assert_no_set_cookie
    identity_source_attempt=$((identity_source_attempt + 1))
done
write_identity_login_body "source_blocked_$random_suffix" "$identity_password_file" "$identity_login_body"
identity_prepare_login_config
identity_request POST /api/v1/session 429 "$identity_login_body" - -
assert_identity_error authentication_throttled 'authentication throttled'
assert_no_set_cookie
identity_source_throttle_state=$(mysql_root_execute '
    SELECT CONCAT(
        COUNT(*), CHAR(58),
        COALESCE(SUM(dimension = 0x6c6f67696e), 0), CHAR(58),
        COALESCE(SUM(dimension = 0x736f75726365), 0), CHAR(58),
        COALESCE(SUM(failure_count), 0), CHAR(58),
        COALESCE(SUM(inflight_count), 0), CHAR(58),
        COALESCE(SUM(blocked_until > UTC_TIMESTAMP(6)), 0)
    )
    FROM identity_authentication_throttle;
') || fail 'could not inspect the persistent source-throttle state'
if [ "$identity_source_throttle_state" != '31:30:1:60:0:1' ]; then
    fail "persistent source-throttle state is $identity_source_throttle_state instead of 31:30:1:60:0:1"
fi
ok 'thirty distributed failures closed only the source dimension and the next distinct login returned an exact Cookie-free 429'

# Scan real provisioner/API/gateway output against every password, raw Cookie,
# and CSRF value observed above. grep output is suppressed so a regression
# cannot echo the secret to the acceptance console.
if ! awk 'length($0) == 0 { invalid = 1 } END { exit (NR > 10 && !invalid) ? 0 : 1 }' \
    "$identity_sensitive_patterns"; then
    fail 'the Identity sensitive-pattern set is incomplete or malformed'
fi
resolve_container api
docker logs "$resolved_container_id" > "$identity_api_logs" 2>&1 ||
    fail 'could not capture disposable API logs for Identity secret scanning'
resolve_container web
docker logs "$resolved_container_id" > "$identity_web_logs" 2>&1 ||
    fail 'could not capture disposable Web logs for Identity secret scanning'
chmod 0600 "$identity_api_logs" "$identity_web_logs" "$identity_provision_output"
if grep -F -f "$identity_sensitive_patterns" \
    "$identity_provision_output" "$identity_api_logs" "$identity_web_logs" >/dev/null 2>&1; then
    fail 'a password, raw session token, or CSRF token appeared in process or gateway logs'
else
    identity_secret_scan_status=$?
fi
if [ "$identity_secret_scan_status" -ne 1 ]; then
    fail 'the Identity process/gateway secret scan could not inspect every private input'
fi

identity_final_state=$(mysql_root_execute "
    SELECT CONCAT_WS(CHAR(58), account_status, authentication_epoch,
        (SELECT COUNT(*) FROM identity_session WHERE account_id = '$identity_account_id'),
        (SELECT COUNT(*) FROM identity_authentication_throttle))
    FROM identity_workforce_account
    WHERE account_id = '$identity_account_id'
") || fail 'could not inspect the final Identity HTTP fixture state'
if [ "$identity_final_state" != 'disabled:2:10:31' ]; then
    fail "final Identity HTTP fixture state is $identity_final_state instead of disabled:2:10:31"
fi
identity_fixture_cleanup=$(mysql_root_execute "
    START TRANSACTION;
    DELETE FROM identity_session WHERE account_id = '$identity_account_id';
    SET @deleted_sessions = ROW_COUNT();
    DELETE FROM identity_authentication_throttle;
    SET @deleted_throttles = ROW_COUNT();
    DELETE FROM identity_workforce_account WHERE account_id = '$identity_account_id';
    SET @deleted_accounts = ROW_COUNT();
    SELECT CONCAT(@deleted_sessions, CHAR(58), @deleted_throttles, CHAR(58), @deleted_accounts);
    COMMIT;
") || fail 'could not remove the exact Identity HTTP fixtures'
if [ "$identity_fixture_cleanup" != '10:31:1' ]; then
    fail "Identity HTTP fixture cleanup was $identity_fixture_cleanup instead of 10:31:1"
fi
identity_fixture_residue=$(mysql_root_execute '
    SELECT CONCAT(
        (SELECT COUNT(*) FROM identity_workforce_account), CHAR(58),
        (SELECT COUNT(*) FROM identity_session), CHAR(58),
        (SELECT COUNT(*) FROM identity_authentication_throttle)
    )
') || fail 'could not verify Identity HTTP fixture cleanup'
if [ "$identity_fixture_residue" != '0:0:0' ]; then
    fail "Identity HTTP fixture residue remains: $identity_fixture_residue"
fi
ok 'real Session HTTP acceptance left exact zero account/session/throttle residue and no secret-bearing logs'

request GET /health 200 - - -
if ! jq -e '.status == "ok" and .version == "lesson-32" and (.timestamp | type == "string" and length > 0)' "$response_body" >/dev/null; then
    fail '/health did not identify the lesson-32 build'
fi
request GET /ready 200 - - -
if ! jq -e '.status == "ready" and .version == "lesson-32" and (.timestamp | type == "string" and length > 0)' "$response_body" >/dev/null; then
    fail '/ready did not identify the ready lesson-32 build'
fi
ok 'health and readiness succeeded through the web proxy'

response_number=$((response_number + 1))
response_headers="$response_directory/headers-$response_number"
response_body="$response_directory/body-$response_number"
invalid_host_status=$(curl \
    --silent \
    --show-error \
    --connect-timeout "$connect_timeout" \
    --max-time "$request_timeout" \
    --header 'Host: attacker.example' \
    --dump-header "$response_headers" \
    --output "$response_body" \
    --write-out '%{http_code}' \
    "$base_url/health") || fail 'invalid-Host gateway request failed'
if [ "$invalid_host_status" != 421 ] ||
   [ "$(header_value Cache-Control "$response_headers")" != no-store ] ||
   [ "$(header_count Cache-Control "$response_headers")" -ne 1 ] ||
   [ -z "$(header_value X-Request-ID "$response_headers")" ] ||
   [ "$(header_count X-Request-ID "$response_headers")" -ne 1 ]; then
    fail 'the loopback gateway did not reject an arbitrary Host with correlated no-store 421'
fi
ok 'the loopback gateway rejected an arbitrary Host before proxying'

response_number=$((response_number + 1))
response_headers="$response_directory/headers-$response_number"
response_body="$response_directory/body-$response_number"
invalid_api_host_status=$(curl \
    --silent \
    --show-error \
    --connect-timeout "$connect_timeout" \
    --max-time "$request_timeout" \
    --header 'Host: attacker.example' \
    --dump-header "$response_headers" \
    --output "$response_body" \
    --write-out '%{http_code}' \
    "$base_url/api/v1/session") || fail 'invalid-Host Session request failed'
invalid_api_host_request_id=$(header_value X-Request-ID "$response_headers")
invalid_api_host_content_type=$(header_value Content-Type "$response_headers" | tr '[:upper:]' '[:lower:]')
if [ "$invalid_api_host_status" != 421 ] ||
   [ "$(header_count Content-Type "$response_headers")" -ne 1 ] ||
   [ "${invalid_api_host_content_type%%;*}" != application/json ] ||
   [ "$(header_count Cache-Control "$response_headers")" -ne 1 ] ||
   [ "$(header_value Cache-Control "$response_headers")" != no-store ] ||
   [ -z "$invalid_api_host_request_id" ] ||
   [ "$(header_count X-Request-ID "$response_headers")" -ne 1 ] ||
   [ "$(header_count Content-Security-Policy "$response_headers")" -ne 1 ] ||
   [ "$(header_value Content-Security-Policy "$response_headers")" != "default-src 'none'; frame-ancestors 'none'; base-uri 'none'" ] ||
   [ "$(header_count Cross-Origin-Resource-Policy "$response_headers")" -ne 1 ] ||
   [ "$(header_value Cross-Origin-Resource-Policy "$response_headers")" != same-origin ] ||
   [ "$(header_count Permissions-Policy "$response_headers")" -ne 1 ] ||
   [ "$(header_value Permissions-Policy "$response_headers")" != 'camera=(), geolocation=(), microphone=()' ] ||
   [ "$(header_count Referrer-Policy "$response_headers")" -ne 1 ] ||
   [ "$(header_value Referrer-Policy "$response_headers")" != no-referrer ] ||
   [ "$(header_count X-Content-Type-Options "$response_headers")" -ne 1 ] ||
   [ "$(header_value X-Content-Type-Options "$response_headers")" != nosniff ] ||
   [ "$(header_count X-Frame-Options "$response_headers")" -ne 1 ] ||
   [ "$(header_value X-Frame-Options "$response_headers")" != DENY ] ||
   [ "$(header_count Set-Cookie "$response_headers")" -ne 0 ]; then
    fail 'invalid-Host Session response escaped the canonical API edge contract'
fi
if ! jq -e \
    --arg request_id "$invalid_api_host_request_id" \
    '. == {error: {code: "misdirected_request", message: "request Host is not accepted", request_id: $request_id}}' \
    "$response_body" >/dev/null; then
    fail 'invalid-Host Session response did not return the correlated JSON error envelope'
fi
ok 'invalid-Host Session requests retain the canonical API security and error contract'

max_route=/api/v1/lottery/strategies/18446744073709551615/ephemeral-selections
request POST "$max_route" 200 - - ephemeral-selection
if ! jq -e '
    . == {
        selection: {
            durability: "ephemeral",
            strategy_id: "18446744073709551615",
            award: {
                id: "18446744073709551615",
                name: "Max reward",
                outcome: "reward"
            }
        }
    } and
    (.selection.strategy_id | type == "string") and
    (.selection.award.id | type == "string")
' "$response_body" >/dev/null; then
    fail 'MaxUint64 selection did not preserve public identities as decimal JSON strings'
fi
max_cache_key=growthos:development:lottery:strategy:projection:v1:18446744073709551615
cached_max_projection=$(redis_business getrange "$max_cache_key" 0 2097152) ||
    fail 'could not read the MaxUint64 Strategy projection through the business ACL'
if ! printf '%s' "$cached_max_projection" | jq -e '
    .schema == "growthos.lottery.strategy.projection" and
    .schema_version == 1 and
    .strategy.id == "18446744073709551615" and
    .strategy.awards == [{
        id: "18446744073709551615",
        name: "Max reward",
        weight: "18446744073709551615",
        outcome: "reward"
    }]
' >/dev/null; then
    fail 'the Redis projection did not preserve strict v1 MaxUint64 strings'
fi
ok 'the first API miss filled one strict Redis v1 Strategy projection'

no_reward_cache_key=growthos:development:lottery:strategy:projection:v1:21002
if [ "$(redis_business set "$no_reward_cache_key" '{"schema":"poison"}' EX 60)" != OK ]; then
    fail 'could not install the exact disposable poison cache fixture'
fi
request POST /api/v1/lottery/strategies/21002/ephemeral-selections 200 - - ephemeral-selection
if ! jq -e '
    .selection.durability == "ephemeral" and
    .selection.strategy_id == "21002" and
    .selection.award == {id: "1", name: "Try again", outcome: "no_reward"}
' "$response_body" >/dev/null; then
    fail 'the no_reward fixture did not return a successful no_reward selection'
fi
repaired_projection=$(redis_business getrange "$no_reward_cache_key" 0 2097152) ||
    fail 'could not read the repaired no_reward projection'
if ! printf '%s' "$repaired_projection" | jq -e '
    .schema == "growthos.lottery.strategy.projection" and
    .schema_version == 1 and
    .strategy.id == "21002" and
    (.strategy.awards | length) == 1
' >/dev/null; then
    fail 'the API did not replace the poison value with a strict projection'
fi
ok 'a poison cache value was deleted, reloaded from MySQL, and repaired'
request POST /api/v1/lottery/strategies/21002/ephemeral-selections 200 'X-Request-ID: acceptance.client:42' - ephemeral-selection
if [ "$response_request_id" != 'acceptance.client:42' ]; then
    fail 'the edge and Go process did not preserve one validated client request ID'
fi
request POST /api/v1/lottery/strategies/21002/ephemeral-selections 200 'X-Request-ID: unsafe request id' - ephemeral-selection
case "$response_request_id" in
    ''|'unsafe request id'|*[!A-Za-z0-9_.:-]*)
        fail 'the edge did not replace an unsafe client request ID with a bounded safe value'
        ;;
esac
if [ "${#response_request_id}" -gt 64 ]; then
    fail 'the edge-generated replacement request ID exceeds 64 bytes'
fi
ok 'max-string and no_reward success contracts passed through the web proxy'

request POST /api/v1/lottery/strategies/999999/ephemeral-selections 404 - - ephemeral-selection
assert_error_response lottery_strategy_not_found 'lottery strategy not found'
request POST /api/v1/lottery/strategies/01/ephemeral-selections 400 - - ephemeral-selection
assert_error_response invalid_strategy_id 'strategy_id must be a canonical decimal integer from 1 through 18446744073709551615'
request POST /api/v1/lottery/strategies/21003/ephemeral-selections 400 - - -
assert_error_response demo_mode_required 'X-GrowthOS-Demo-Mode must be ephemeral-selection'
request POST /api/v1/lottery/strategies/21003/ephemeral-selections 400 - - wrong-mode
assert_error_response demo_mode_required 'X-GrowthOS-Demo-Mode must be ephemeral-selection'
request POST /api/v1/lottery/strategies/21003/ephemeral-selections 400 'X-GrowthOS-Demo-Mode: ephemeral-selection' - ephemeral-selection
assert_error_response demo_mode_required 'X-GrowthOS-Demo-Mode must be ephemeral-selection'
request POST '/api/v1/lottery/strategies/21003/ephemeral-selections?seed=1' 400 - - ephemeral-selection
assert_error_response query_parameters_not_allowed 'query parameters are not allowed'
request GET /api/v1/lottery/strategies/21003/ephemeral-selections 405 - - ephemeral-selection
assert_error_response method_not_allowed 'method not allowed'
if [ "$(header_value Allow "$response_headers")" != POST ]; then
    fail 'method rejection did not return Allow: POST'
fi
request POST /api/v1/lottery/strategies/21003/ephemeral-selections/ 404 - - ephemeral-selection
assert_error_response route_not_found 'resource not found'
request POST /api/v1/lottery/strategies/21003/ephemeral-selections 400 - '{}' ephemeral-selection
assert_error_response request_body_not_allowed 'request body is not allowed'
request POST /api/v1/lottery/strategies/21003/ephemeral-selections 400 'Transfer-Encoding: chunked' '' ephemeral-selection
assert_error_response request_body_not_allowed 'request body is not allowed'
request POST /api/v1/lottery/strategies/21003/ephemeral-selections 400 'Trailer: X-Lottery-Ticket' - ephemeral-selection
assert_error_response request_body_not_allowed 'request body is not allowed'
request POST /api/v1/lottery/strategies/21003/ephemeral-selections 400 'Idempotency-Key: acceptance-retry' - ephemeral-selection
assert_error_response idempotency_not_supported 'idempotency is not supported for ephemeral selections'
ok 'missing, invalid, demo-header, query, method, path, body-framing, and idempotency rejection contracts passed'

gateway_oversize_request="$response_directory/gateway-oversize-request"
if ! openssl rand -out "$gateway_oversize_request" 16385; then
    fail 'could not create the disposable 16 KiB plus one byte gateway request body'
fi
gateway_oversize_bytes=$(wc -c < "$gateway_oversize_request" | tr -d '[:space:]')
if [ "$gateway_oversize_bytes" != 16385 ]; then
    fail "gateway request body is $gateway_oversize_bytes bytes instead of 16385"
fi
request POST /api/v1/lottery/strategies/21003/ephemeral-selections 413 - "@$gateway_oversize_request" ephemeral-selection
assert_error_response request_too_large 'request body exceeds the gateway limit'
ok 'the web gateway generated correlated JSON 413 with no-store for a 16385-byte Content-Length'

# The dollar-prefixed expressions belong to each xargs worker shell.
# shellcheck disable=SC2016
awk -v count="$concurrent_requests" 'BEGIN { for (i = 1; i <= count; i++) print i }' |
    xargs -n 1 -P "$concurrent_workers" sh -c '
        base_url=$1
        response_directory=$2
        connect_timeout=$3
        request_timeout=$4
        request_index=$5
        curl \
            --silent \
            --show-error \
            --globoff \
            --connect-timeout "$connect_timeout" \
            --max-time "$request_timeout" \
            --request POST \
            --header "Accept: application/json" \
            --header "X-GrowthOS-Demo-Mode: ephemeral-selection" \
            --dump-header "$response_directory/concurrent-headers-$request_index" \
            --output "$response_directory/concurrent-body-$request_index" \
            --write-out "%{http_code}" \
            "$base_url/api/v1/lottery/strategies/21003/ephemeral-selections" \
            > "$response_directory/concurrent-status-$request_index"
    ' sh "$base_url" "$response_directory" "$connect_timeout" "$request_timeout"

concurrent_index=1
while [ "$concurrent_index" -le "$concurrent_requests" ]; do
    concurrent_status=$(sed -e 's/\r$//' "$response_directory/concurrent-status-$concurrent_index")
    if [ "$concurrent_status" != 200 ]; then
        fail "concurrent request $concurrent_index returned $concurrent_status instead of 200"
    fi
    concurrent_headers="$response_directory/concurrent-headers-$concurrent_index"
    concurrent_body="$response_directory/concurrent-body-$concurrent_index"
    concurrent_content_type=$(header_value Content-Type "$concurrent_headers" | tr '[:upper:]' '[:lower:]')
    if [ "${concurrent_content_type%%;*}" != application/json ] ||
       [ "$(header_count Content-Type "$concurrent_headers")" -ne 1 ] ||
       [ "$(header_value Cache-Control "$concurrent_headers")" != no-store ] ||
       [ "$(header_count Cache-Control "$concurrent_headers")" -ne 1 ] ||
       [ -z "$(header_value X-Request-ID "$concurrent_headers")" ] ||
       [ "$(header_count X-Request-ID "$concurrent_headers")" -ne 1 ]; then
        fail "concurrent request $concurrent_index missed singular JSON, no-store, or request-ID headers"
    fi
    if ! jq -e '
        .selection.durability == "ephemeral" and
        .selection.strategy_id == "21003" and
        (
            .selection.award == {id: "1", name: "Reward", outcome: "reward"} or
            .selection.award == {id: "2", name: "No reward", outcome: "no_reward"}
        )
    ' "$concurrent_body" >/dev/null; then
        fail "concurrent request $concurrent_index returned an invalid multi-award selection"
    fi
    concurrent_index=$((concurrent_index + 1))
done
ok "$concurrent_requests multi-award requests at concurrency $concurrent_workers returned only configured outcomes"

multi_cache_key=growthos:development:lottery:strategy:projection:v1:21003
redis_business del "$multi_cache_key" >/dev/null
resolve_container redis
if ! compose stop redis; then
    fail 'could not stop the disposable Redis cache'
fi
if [ "$(docker inspect --format '{{.State.Status}}' "$resolved_container_id")" != exited ]; then
    fail 'disposable Redis did not reach exited state'
fi
request POST /api/v1/lottery/strategies/21003/ephemeral-selections 200 - - ephemeral-selection
assert_multi_strategy_selection 'the Redis-outage request'
request GET /ready 200 - - -
if ! compose up --detach --wait --wait-timeout 60 redis; then
    fail 'could not restore the disposable Redis cache'
fi
assert_running_healthy redis
request POST /api/v1/lottery/strategies/21003/ephemeral-selections 200 - - ephemeral-selection
assert_multi_strategy_selection 'the post-Redis-recovery request'
rebuilt_multi_projection=$(redis_business getrange "$multi_cache_key" 0 2097152) ||
    fail 'could not read the Strategy projection rebuilt after Redis recovery'
if ! printf '%s' "$rebuilt_multi_projection" | jq -e '
    .schema_version == 1 and .strategy.id == "21003" and (.strategy.awards | length) == 2
' >/dev/null; then
    fail 'Redis recovery did not rebuild the expected Strategy projection'
fi
ok 'Redis outage fell back to MySQL without changing readiness, then recovered and refilled'

resolve_container mysql
if ! compose stop mysql; then
    fail 'could not stop the disposable MySQL authority'
fi
if [ "$(docker inspect --format '{{.State.Status}}' "$resolved_container_id")" != exited ]; then
    fail 'disposable MySQL did not reach exited state'
fi
request POST /api/v1/lottery/strategies/21003/ephemeral-selections 200 - - ephemeral-selection
assert_multi_strategy_selection 'the warm-cache MySQL-outage request'
request GET /ready 503 - - -
assert_error_response dependency_unavailable 'service unavailable'
request POST /api/v1/lottery/strategies/999998/ephemeral-selections 503 - - ephemeral-selection
assert_error_response lottery_selection_unavailable 'lottery selection is temporarily unavailable'
if ! compose up --detach --wait --wait-timeout 120 mysql; then
    fail 'could not restore the disposable MySQL authority'
fi
assert_running_healthy mysql
request GET /ready 200 - - -
redis_business del "$multi_cache_key" >/dev/null
request POST /api/v1/lottery/strategies/21003/ephemeral-selections 200 - - ephemeral-selection
assert_multi_strategy_selection 'the post-MySQL-recovery cold request'
recovered_mysql_projection=$(redis_business getrange "$multi_cache_key" 0 2097152) ||
    fail 'could not read the Strategy projection rebuilt after MySQL recovery'
if ! printf '%s' "$recovered_mysql_projection" | jq -e '
    .schema_version == 1 and .strategy.id == "21003" and (.strategy.awards | length) == 2
' >/dev/null; then
    fail 'MySQL recovery did not rebuild the expected Strategy projection'
fi
ok 'warm Redis hit survived MySQL outage; readiness and cold reads failed safely; recovery refilled the cache'

# M1 uses one immutable fixture and one loopback Nginx -> Go endpoint for all
# scenarios. Latency is reported, never gated as a production SLO. Performance
# Schema account counters prove source reads independently of cache log events.
request POST /api/v1/lottery/strategies/21003/ephemeral-selections 200 - - ephemeral-selection
assert_multi_strategy_selection 'the M1 warm-up request'
recreate_api_for_cache_mode true
warm_selects_before=$(mysql_app_select_count) || fail 'could not read the pre-warm-cache MySQL SELECT counter'
assert_counter 'pre-warm-cache MySQL SELECT counter' "$warm_selects_before"
run_m1_load warm-cache
warm_completed=$baseline_completed
warm_report_summary=$baseline_summary
warm_selects_after=$(mysql_app_select_count) || fail 'could not read the post-warm-cache MySQL SELECT counter'
assert_counter 'post-warm-cache MySQL SELECT counter' "$warm_selects_after"
warm_select_delta=$((warm_selects_after - warm_selects_before))
warm_hits=$(cache_outcome_count "$baseline_api_container_id" hit)
assert_counter 'warm-cache hit count' "$warm_hits"
if [ "$warm_select_delta" -ne 0 ] || [ "$warm_hits" -ne "$warm_completed" ]; then
    fail "warm-cache evidence was mysql_select_executes=$warm_select_delta cache_hits=$warm_hits completed=$warm_completed"
fi
printf 'm1 - warm-cache %s mysql_select_executes=%s cache_hits=%s\n' \
    "$warm_report_summary" "$warm_select_delta" "$warm_hits"

recreate_api_for_cache_mode false
direct_selects_before=$(mysql_app_select_count) || fail 'could not read the pre-direct-MySQL SELECT counter'
assert_counter 'pre-direct-MySQL SELECT counter' "$direct_selects_before"
run_m1_load direct-mysql
direct_completed=$baseline_completed
direct_report_summary=$baseline_summary
direct_selects_after=$(mysql_app_select_count) || fail 'could not read the post-direct-MySQL SELECT counter'
assert_counter 'post-direct-MySQL SELECT counter' "$direct_selects_after"
direct_select_delta=$((direct_selects_after - direct_selects_before))
direct_expected_selects=$((direct_completed * 2))
direct_cache_events=$(cache_outcome_count "$baseline_api_container_id" '*')
assert_counter 'cache-disabled observation count' "$direct_cache_events"
if [ "$direct_select_delta" -ne "$direct_expected_selects" ] || [ "$direct_cache_events" -ne 0 ]; then
    fail "direct-MySQL evidence was mysql_select_executes=$direct_select_delta expected=$direct_expected_selects cache_events=$direct_cache_events"
fi
printf 'm1 - direct-mysql %s mysql_select_executes=%s source_loads=%s cache_events=%s\n' \
    "$direct_report_summary" "$direct_select_delta" "$direct_completed" "$direct_cache_events"

recreate_api_for_cache_mode true
redis_business del "$multi_cache_key" >/dev/null
resolve_container redis
if ! compose stop redis; then
    fail 'could not stop Redis for the M1 degraded baseline'
fi
if [ "$(docker inspect --format '{{.State.Status}}' "$resolved_container_id")" != exited ]; then
    fail 'Redis did not reach exited state for the M1 degraded baseline'
fi
down_selects_before=$(mysql_app_select_count) || fail 'could not read the pre-Redis-down MySQL SELECT counter'
assert_counter 'pre-Redis-down MySQL SELECT counter' "$down_selects_before"
run_m1_load redis-down
down_completed=$baseline_completed
down_report_summary=$baseline_summary
down_selects_after=$(mysql_app_select_count) || fail 'could not read the post-Redis-down MySQL SELECT counter'
assert_counter 'post-Redis-down MySQL SELECT counter' "$down_selects_after"
down_select_delta=$((down_selects_after - down_selects_before))
if [ $((down_select_delta % 2)) -ne 0 ]; then
    fail "Redis-down MySQL SELECT delta $down_select_delta is not two statements per source load"
fi
down_source_loads=$((down_select_delta / 2))
down_fill_leaders=$(cache_outcome_count "$baseline_api_container_id" fill_leader)
down_fill_joined=$(cache_outcome_count "$baseline_api_container_id" fill_joined)
down_read_error_logs=$(cache_outcome_count "$baseline_api_container_id" read_error)
assert_counter 'Redis-down fill leader count' "$down_fill_leaders"
assert_counter 'Redis-down fill joined count' "$down_fill_joined"
assert_counter 'Redis-down read-error log count' "$down_read_error_logs"
if [ "$down_source_loads" -lt 1 ] || [ "$down_source_loads" -gt "$down_completed" ] ||
   [ $((down_fill_leaders + down_fill_joined)) -ne "$down_completed" ] ||
   [ "$down_read_error_logs" -lt 1 ]; then
    fail "Redis-down evidence was completed=$down_completed source_loads=$down_source_loads leaders=$down_fill_leaders joined=$down_fill_joined read_error_logs=$down_read_error_logs"
fi
request GET /ready 200 - - -
if ! compose up --detach --wait --wait-timeout 60 redis; then
    fail 'could not restore Redis after the M1 degraded baseline'
fi
assert_running_healthy redis
request POST /api/v1/lottery/strategies/21003/ephemeral-selections 200 - - ephemeral-selection
assert_multi_strategy_selection 'the post-M1 Redis recovery request'
post_baseline_projection=$(redis_business getrange "$multi_cache_key" 0 2097152) ||
    fail 'could not read the Strategy projection rebuilt after the M1 baseline'
if ! printf '%s' "$post_baseline_projection" | jq -e '
    .schema_version == 1 and .strategy.id == "21003" and (.strategy.awards | length) == 2
' >/dev/null; then
    fail 'post-M1 recovery did not rebuild the expected Strategy projection'
fi
printf 'm1 - redis-down %s mysql_select_executes=%s source_loads=%s fill_leaders=%s fill_joined=%s read_error_logs=%s\n' \
    "$down_report_summary" "$down_select_delta" "$down_source_loads" \
    "$down_fill_leaders" "$down_fill_joined" "$down_read_error_logs"
ok 'M1 warm-cache, direct-MySQL, and Redis-down baselines have independent latency and source-load evidence'

# Stop only the label-resolved disposable API and prove nginx-generated
# upstream failures retain the same JSON/no-store/correlation contract. Then
# restore it before the final database and topology checks.
resolve_container api
if ! compose stop api; then
    fail 'could not stop the disposable API for the gateway failure check'
fi
stopped_state=$(docker inspect --format '{{.State.Status}}' "$resolved_container_id")
if [ "$stopped_state" != exited ]; then
    fail "disposable API state is $stopped_state instead of exited"
fi
request POST /api/v1/lottery/strategies/21003/ephemeral-selections 502-or-504 - - ephemeral-selection
case "$response_status" in
    502)
        assert_error_response bad_gateway 'upstream service is unavailable'
        ;;
    504)
        assert_error_response gateway_timeout 'upstream service timed out'
        ;;
esac
if ! compose up --detach --wait --wait-timeout 60 api; then
    fail 'could not restore the disposable API after the gateway failure check'
fi
assert_running_healthy api
ok "the gateway returned correlated JSON $response_status and the disposable API recovered healthy"

fingerprint_after=$(database_fingerprint) || fail 'could not fingerprint the fixtures after HTTP requests'
if [ "$fingerprint_after" != "$fingerprint_before" ]; then
    fail 'the Lottery API changed persisted business data during ephemeral selections'
fi
ok 'the complete business-table fingerprint remained unchanged across all HTTP requests'

# Recheck the version, exact grants, and host-port topology after concurrent
# traffic so a mid-run drift cannot hide behind the earlier preflight.
# shellcheck disable=SC2016
migration_state_after=$(compose exec -T mysql sh -c '
    export MYSQL_PWD="$(cat /run/secrets/mysql_migration_password)"
    mysql --protocol=tcp --host=127.0.0.1 --user=growthos_migrator --database=growthos \
        --batch --silent --skip-column-names \
        --execute="SELECT CONCAT(version, CHAR(58), dirty) FROM schema_migrations"
') || fail 'could not recheck migration state'
if [ "$migration_state_after" != '14:0' ]; then
    fail 'migration state drifted during HTTP acceptance'
fi
# shellcheck disable=SC2016
actual_app_grants_after=$(compose exec -T mysql sh -c '
    export MYSQL_PWD="$(cat /run/secrets/mysql_app_password)"
    mysql --protocol=tcp --host=127.0.0.1 --user=growthos_app --database=growthos \
        --batch --silent --skip-column-names --execute="SHOW GRANTS FOR CURRENT_USER"
' | LC_ALL=C sort) || fail 'could not recheck growthos_app grants'
if [ "$actual_app_grants_after" != "$expected_app_grants" ]; then
    fail 'growthos_app grants drifted during HTTP acceptance'
fi
# shellcheck disable=SC2016
actual_identity_grants_after=$(compose exec -T mysql sh -c '
    export MYSQL_PWD="$(cat /run/secrets/mysql_identity_password)"
    mysql --protocol=tcp --host=127.0.0.1 --user=growthos_identity --database=growthos \
        --batch --silent --skip-column-names --execute="SHOW GRANTS FOR CURRENT_USER"
' | LC_ALL=C sort) || fail 'could not recheck growthos_identity grants'
if [ "$actual_identity_grants_after" != "$expected_identity_grants" ]; then
    fail 'growthos_identity grants drifted during HTTP acceptance'
fi
# shellcheck disable=SC2016
actual_identity_provisioner_grants_after=$(compose exec -T mysql sh -c '
    export MYSQL_PWD="$(cat /run/secrets/mysql_identity_provisioner_password)"
    mysql --protocol=tcp --host=127.0.0.1 --user=growthos_identity_provisioner --database=growthos \
        --batch --silent --skip-column-names --execute="SHOW GRANTS FOR CURRENT_USER"
' | LC_ALL=C sort) || fail 'could not recheck growthos_identity_provisioner grants'
if [ "$actual_identity_provisioner_grants_after" != "$expected_identity_provisioner_grants" ]; then
    fail 'growthos_identity_provisioner grants drifted during HTTP acceptance'
fi
if ! docker inspect "$web_container_id" | jq -e --arg port "$web_port" '
    .[0].NetworkSettings.Ports == {
        "8080/tcp": [{"HostIp": "127.0.0.1", "HostPort": $port}]
    }
' >/dev/null; then
    fail 'the web host-port topology drifted during HTTP acceptance'
fi
published_container_ids_after=$(containers_publishing_loopback_port "$web_port")
if [ "$published_container_ids_after" != "$web_container_id" ]; then
    fail 'the disposable web port lost its unique ownership during HTTP acceptance'
fi
ok 'post-traffic migration, runtime/one-shot grant sets, and loopback port checks remained exact'
ok "lesson-32 schema plus lesson-24 cache isolated Compose acceptance passed for $compose_project"
