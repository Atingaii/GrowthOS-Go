#!/bin/sh
set -eu

repository_root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
secret_directory=${1:-"$repository_root/deploy/compose/secrets"}
compose_project=${GROWTHOS_COMPOSE_PROJECT:-growthos}
secret_names="mysql_root_password mysql_app_password mysql_migration_password mysql_identity_password redis_password"
legacy_secret_names="mysql_root_password mysql_app_password mysql_migration_password redis_password"

if ! command -v openssl >/dev/null 2>&1; then
    printf '%s\n' 'openssl is required to generate Compose development secrets' >&2
    exit 1
fi

umask 077
mkdir -p "$secret_directory"
chmod 0700 "$secret_directory"

validate_secret() {
    target=$1

    if [ ! -f "$target" ] || [ ! -r "$target" ]; then
        printf '%s is not a readable regular file\n' "$target" >&2
        exit 1
    fi

    secret_value=$(sed -e 's/\r$//' "$target")
    case "$secret_value" in
        ''|*[!0-9a-f]*)
            printf '%s must contain only lowercase hexadecimal characters\n' "$target" >&2
            exit 1
            ;;
    esac
    if [ "${#secret_value}" -ne 64 ]; then
        printf '%s must contain exactly 64 hexadecimal characters\n' "$target" >&2
        exit 1
    fi

    # Compose implements file-backed secrets as bind mounts. Docker Desktop
    # exposes those mounts as root-owned inside some images, so a non-root
    # process needs the read bit. The containing host directory remains 0700,
    # and each service receives only the secret it declares.
    chmod 0444 "$target"
    unset secret_value
}

present_count=0
expected_count=0
for name in $secret_names; do
    expected_count=$((expected_count + 1))
    if [ -e "$secret_directory/$name" ]; then
        present_count=$((present_count + 1))
    fi
done

if [ "$present_count" -eq "$expected_count" ]; then
    for name in $secret_names; do
        validate_secret "$secret_directory/$name"
    done
    printf '%s\n' "Compose development secrets are valid in $secret_directory"
    exit 0
fi

# Lesson 32 adds one new runtime identity without changing any pre-existing
# account password. The only accepted partial state is the exact four-file
# legacy set: validate it first, then atomically create only the new Identity
# secret. Any other partial set remains a hard failure.
legacy_set_complete=1
for name in $legacy_secret_names; do
    if [ ! -e "$secret_directory/$name" ]; then
        legacy_set_complete=0
    fi
done
if [ "$legacy_set_complete" -eq 1 ] && [ ! -e "$secret_directory/mysql_identity_password" ]; then
    for name in $legacy_secret_names; do
        validate_secret "$secret_directory/$name"
    done

    temporary_directory=$(mktemp -d "$secret_directory/.generate.XXXXXX")
    # shellcheck disable=SC2329 # invoked indirectly by this branch's trap
    cleanup() {
        if [ -n "${temporary_directory:-}" ] && [ -d "$temporary_directory" ]; then
            rm -rf "$temporary_directory"
        fi
    }
    trap cleanup EXIT HUP INT TERM

    openssl rand -hex 32 > "$temporary_directory/mysql_identity_password"
    validate_secret "$temporary_directory/mysql_identity_password"
    mv "$temporary_directory/mysql_identity_password" "$secret_directory/mysql_identity_password"
    rmdir "$temporary_directory"
    temporary_directory=
    trap - EXIT HUP INT TERM

    printf '%s\n' "Extended the validated legacy Compose secret set with the Identity secret in $secret_directory"
    exit 0
fi

if [ "$present_count" -ne 0 ]; then
    printf '%s\n' 'only part of the Compose secret set exists; refusing to create a mismatched set' >&2
    printf '%s\n' 'restore the missing files or explicitly remove the whole local set after checking the MySQL volume' >&2
    exit 1
fi

mysql_volume=${compose_project}_mysql_data
if [ "${GROWTHOS_COMPOSE_SKIP_VOLUME_CHECK:-0}" != "1" ] && \
   command -v docker >/dev/null 2>&1 && \
   docker volume inspect "$mysql_volume" >/dev/null 2>&1; then
    printf '%s\n' "Docker volume $mysql_volume already exists while its secret set is missing" >&2
    printf '%s\n' 'restore the original secrets or perform an explicit credential/data reset; new secrets would not match the stored accounts' >&2
    exit 1
fi

temporary_directory=$(mktemp -d "$secret_directory/.generate.XXXXXX")
cleanup() {
    if [ -n "${temporary_directory:-}" ] && [ -d "$temporary_directory" ]; then
        rm -rf "$temporary_directory"
    fi
}
trap cleanup EXIT HUP INT TERM

for name in $secret_names; do
    openssl rand -hex 32 > "$temporary_directory/$name"
    validate_secret "$temporary_directory/$name"
done

for name in $secret_names; do
    mv "$temporary_directory/$name" "$secret_directory/$name"
done

rmdir "$temporary_directory"
temporary_directory=
trap - EXIT HUP INT TERM

printf '%s\n' "Generated the complete Compose development secret set in $secret_directory"
