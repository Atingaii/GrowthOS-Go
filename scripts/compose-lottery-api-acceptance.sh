#!/bin/sh
set -eu

# Destructive-to-self acceptance for the current Lesson 30 schema build and
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

for required_command in awk curl docker go jq mktemp openssl sed sort stat tr wc xargs; do
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
acceptance_redis_image="growthos/acceptance-redis:$compose_project"
acceptance_web_image="growthos/acceptance-web:$compose_project"
acceptance_images="$acceptance_api_image $acceptance_migrate_image $acceptance_redis_image $acceptance_web_image"
buildx_builder="${compose_project}builder"
buildkit_image=moby/buildkit:buildx-stable-1
expected_builder_container="buildx_buildkit_${buildx_builder}0"
expected_builder_volume="${expected_builder_container}_state"
export GROWTHOS_LESSON24_ACCEPTANCE_API_IMAGE="$acceptance_api_image"
export GROWTHOS_LESSON24_ACCEPTANCE_MIGRATE_IMAGE="$acceptance_migrate_image"
export GROWTHOS_LESSON24_ACCEPTANCE_REDIS_IMAGE="$acceptance_redis_image"
export GROWTHOS_LESSON24_ACCEPTANCE_WEB_IMAGE="$acceptance_web_image"
export GROWTHOS_LESSON24_ACCEPTANCE_CACHE_ENABLED=true
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
secret_directory_identity=
response_directory_identity=
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
            api|migrate|mysql|mysql-grants|redis|web)
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
    if [ -n "$response_directory" ] && { [ -e "$response_directory" ] || [ -L "$response_directory" ]; }; then
        if ! verify_temporary_directory "$response_directory" "$response_directory_identity" response; then
            temporary_cleanup_status=1
        else
            cleanup_index=1
            while [ "$cleanup_index" -le "$response_number" ]; do
                remove_regular_file "$response_directory/headers-$cleanup_index" || temporary_cleanup_status=1
                remove_regular_file "$response_directory/body-$cleanup_index" || temporary_cleanup_status=1
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
    if [ -n "$secret_directory" ] && { [ -e "$secret_directory" ] || [ -L "$secret_directory" ]; }; then
        if ! verify_temporary_directory "$secret_directory" "$secret_directory_identity" secret; then
            temporary_cleanup_status=1
        else
            for secret_name in mysql_root_password mysql_app_password mysql_migration_password mysql_identity_password redis_password; do
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
        printf 'preserved disposable secrets/responses at %s and %s for manual cleanup\n' "$secret_directory" "$response_directory" >&2
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
# neutral order: the api build populates the shared builder cache and migrate
# reuses it, while Redis and the web bundle never compete with the Go compiler.
for acceptance_build_service in api migrate redis web; do
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
ok 'all Compose services reached their expected states on the lesson-30 schema and lesson-24 cache snapshot'

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
    --arg cache "${compose_project}_cache" '
        (.[0].NetworkSettings.Networks | keys | sort) == ([$edge, $data, $cache] | sort) and
        ([.[0].Mounts[].Destination | select(startswith("/run/secrets/"))] | sort) ==
            (["/run/secrets/mysql_app_password", "/run/secrets/mysql_identity_password", "/run/secrets/redis_password"] | sort)
    ' >/dev/null; then
    fail 'api network or Secret mounts differ from the business/Identity MySQL plus cache contract'
fi
for mysql_secret_consumer in mysql migrate mysql-grants; do
    resolve_container "$mysql_secret_consumer"
    case "$mysql_secret_consumer" in
        mysql)
            expected_mysql_secret_mounts='["/run/secrets/mysql_app_password","/run/secrets/mysql_identity_password","/run/secrets/mysql_migration_password","/run/secrets/mysql_root_password"]'
            ;;
        migrate)
            expected_mysql_secret_mounts='["/run/secrets/mysql_migration_password"]'
            ;;
        mysql-grants)
            expected_mysql_secret_mounts='["/run/secrets/mysql_identity_password","/run/secrets/mysql_root_password"]'
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
web_binding=$(compose port web 8080) || fail 'could not resolve the web host port'
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
base_url="http://127.0.0.1:$web_port"
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

request GET /health 200 - - -
if ! jq -e '.status == "ok" and .version == "lesson-30" and (.timestamp | type == "string" and length > 0)' "$response_body" >/dev/null; then
    fail '/health did not identify the lesson-30 build'
fi
request GET /ready 200 - - -
if ! jq -e '.status == "ready" and .version == "lesson-30" and (.timestamp | type == "string" and length > 0)' "$response_body" >/dev/null; then
    fail '/ready did not identify the ready lesson-30 build'
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
ok 'post-traffic migration, both runtime grant sets, and loopback port checks remained exact'
ok "lesson-30 schema plus lesson-24 cache isolated Compose acceptance passed for $compose_project"
