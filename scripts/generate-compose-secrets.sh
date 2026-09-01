#!/bin/sh
set -eu

repository_root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
secret_directory=${1:-"$repository_root/deploy/compose/secrets"}
compose_project=${GROWTHOS_COMPOSE_PROJECT:-growthos}
compose_web_port=${GROWTHOS_COMPOSE_WEB_PORT:-8088}
identity_csrf_active_key_id=${GROWTHOS_COMPOSE_IDENTITY_CSRF_ACTIVE_KEY_ID:-local-v1}
hex_secret_names="mysql_root_password mysql_app_password mysql_migration_password mysql_identity_password redis_password"
binary_secret_names="identity_throttle_hmac_key identity_csrf_active_key"
secret_names="$hex_secret_names $binary_secret_names"
legacy_secret_names="mysql_root_password mysql_app_password mysql_migration_password redis_password"
database_secret_names="$legacy_secret_names mysql_identity_password"

if ! command -v openssl >/dev/null 2>&1; then
    printf '%s\n' 'openssl is required to generate Compose development secrets' >&2
    exit 1
fi

case "$compose_web_port" in
    ''|*[!0-9]*|0|0*)
        printf '%s\n' 'GROWTHOS_COMPOSE_WEB_PORT must be a canonical integer from 1 through 65535' >&2
        exit 1
        ;;
esac
if [ "${#compose_web_port}" -gt 5 ] || [ "$compose_web_port" -gt 65535 ] || [ "$compose_web_port" -eq 80 ]; then
    printf '%s\n' 'GROWTHOS_COMPOSE_WEB_PORT must be a canonical non-default HTTP port from 1 through 65535' >&2
    exit 1
fi
case "$identity_csrf_active_key_id" in
    ''|*[!A-Za-z0-9_-]*)
        printf '%s\n' 'GROWTHOS_COMPOSE_IDENTITY_CSRF_ACTIVE_KEY_ID must use only letters, digits, underscore, or hyphen' >&2
        exit 1
        ;;
esac
if [ "${#identity_csrf_active_key_id}" -gt 16 ]; then
    printf '%s\n' 'GROWTHOS_COMPOSE_IDENTITY_CSRF_ACTIVE_KEY_ID must contain at most 16 characters' >&2
    exit 1
fi

umask 077
if [ -L "$secret_directory" ]; then
    printf '%s\n' "$secret_directory must not be a symbolic link" >&2
    exit 1
fi
mkdir -p "$secret_directory"
if [ ! -d "$secret_directory" ]; then
    printf '%s\n' "$secret_directory is not a directory" >&2
    exit 1
fi
chmod 0700 "$secret_directory"

lock_directory="$secret_directory/.generate.lock"
if ! mkdir "$lock_directory" 2>/dev/null; then
    printf '%s\n' 'another Compose secret generation may be running; refusing concurrent or stale-lock mutation' >&2
    exit 1
fi
temporary_directory=
cleanup() {
    if [ -n "${temporary_directory:-}" ] && [ -d "$temporary_directory" ]; then
        rm -rf "$temporary_directory"
    fi
    if [ -d "$lock_directory" ] && [ ! -L "$lock_directory" ]; then
        rmdir "$lock_directory" 2>/dev/null || true
    fi
}
trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

validate_hex_secret() {
    target=$1

    if [ -L "$target" ] || [ ! -f "$target" ] || [ ! -r "$target" ]; then
        printf '%s is not a readable regular file\n' "$target" >&2
        exit 1
    fi

    # Validate bytes directly. Shell command substitution cannot preserve NUL,
    # so reading an attacker-controlled file into a variable could turn
    # "64 hex bytes + NUL" into an apparently valid credential. If od fails
    # after partial output, append a nonnumeric sentinel that awk must reject;
    # this fails closed without pipefail or a reversible secret representation
    # in a traceable shell variable.
    if ! { LC_ALL=C od -An -v -tu1 "$target" || printf '%s\n' OD_FAILED; } | awk '
        {
            for (field = 1; field <= NF; field++) {
                if ($field !~ /^[0-9]+$/ || $field < 0 || $field > 255) {
                    invalid = 1
                }
                bytes[++count] = $field
            }
        }
        END {
            if (invalid) {
                exit 1
            }
            content_count = count
            if (count == 65 && bytes[65] == 10) {
                content_count = 64
            } else if (count == 66 && bytes[65] == 13 && bytes[66] == 10) {
                content_count = 64
            }
            if (content_count != 64) {
                exit 1
            }
            for (position = 1; position <= 64; position++) {
                if (!((bytes[position] >= 48 && bytes[position] <= 57) ||
                      (bytes[position] >= 97 && bytes[position] <= 102))) {
                    exit 1
                }
            }
        }
    '; then
        printf '%s must contain exactly 64 lowercase hexadecimal characters with at most one transport newline\n' "$target" >&2
        exit 1
    fi

    # Compose implements file-backed secrets as bind mounts. Docker Desktop
    # exposes those mounts as root-owned inside some images, so a non-root
    # process needs the read bit. The containing host directory remains 0700,
    # and each service receives only the secret it declares.
    chmod 0444 "$target"
}

validate_binary_secret() {
    target=$1

    if [ -L "$target" ] || [ ! -f "$target" ] || [ ! -r "$target" ]; then
        printf '%s is not a readable regular file\n' "$target" >&2
        exit 1
    fi

    if ! { LC_ALL=C od -An -v -tu1 "$target" || printf '%s\n' OD_FAILED; } | awk '
        {
            for (field = 1; field <= NF; field++) {
                if ($field !~ /^[0-9]+$/ || $field < 0 || $field > 255) {
                    invalid = 1
                }
                count++
                if ($field != 0) {
                    found_nonzero = 1
                }
            }
        }
        END { exit (!invalid && count == 32 && found_nonzero) ? 0 : 1 }
    '; then
        printf '%s must contain exactly 32 raw, nonzero secret bytes\n' "$target" >&2
        exit 1
    fi

    # These files intentionally contain binary key material. Never place their
    # bytes in a shell variable: command substitution cannot preserve NULs.
    chmod 0444 "$target"
}

validate_secret() {
    name=$1
    target=$2
    case "$name" in
        identity_throttle_hmac_key|identity_csrf_active_key)
            validate_binary_secret "$target"
            ;;
        *)
            validate_hex_secret "$target"
            ;;
    esac
}

generate_secret() {
    name=$1
    target=$2
    case "$name" in
        identity_throttle_hmac_key|identity_csrf_active_key)
            openssl rand 32 > "$target"
            ;;
        *)
            openssl rand -hex 32 > "$target"
            ;;
    esac
    validate_secret "$name" "$target"
}

validate_identity_key_separation() {
    directory=$1
    if LC_ALL=C cmp -s \
        "$directory/identity_throttle_hmac_key" \
        "$directory/identity_csrf_active_key"; then
        printf '%s\n' 'Identity throttle and active CSRF keys must contain different bytes' >&2
        exit 1
    else
        comparison_status=$?
        if [ "$comparison_status" -ne 1 ]; then
            printf '%s\n' 'could not verify separation of the Identity throttle and active CSRF keys' >&2
            exit 1
        fi
        unset comparison_status
    fi
}

present_count=0
expected_count=0
for name in $secret_names; do
    expected_count=$((expected_count + 1))
    if [ -e "$secret_directory/$name" ] || [ -L "$secret_directory/$name" ]; then
        present_count=$((present_count + 1))
    fi
done

if [ "$present_count" -eq "$expected_count" ]; then
    for name in $secret_names; do
        validate_secret "$name" "$secret_directory/$name"
    done
    validate_identity_key_separation "$secret_directory"
    printf '%s\n' "Compose development secrets are valid in $secret_directory"
    exit 0
fi

# Lesson 32 has two supported upgrade sources. A pre-Lesson-32 four-file set
# needs the Identity database password and both runtime keys; an intermediate
# five-file set already has the database password and needs only the keys. The
# old bytes are validated first and never overwritten. Every other partial set
# remains a hard failure.
upgrade_existing_names=
upgrade_missing_names=
if [ "$present_count" -eq 4 ]; then
    upgrade_existing_names=$legacy_secret_names
    upgrade_missing_names="mysql_identity_password $binary_secret_names"
elif [ "$present_count" -eq 5 ]; then
    upgrade_existing_names=$database_secret_names
    upgrade_missing_names=$binary_secret_names
fi

upgrade_source_complete=1
for name in $upgrade_existing_names; do
    if [ ! -e "$secret_directory/$name" ]; then
        upgrade_source_complete=0
    fi
done
for name in $upgrade_missing_names; do
    if [ -e "$secret_directory/$name" ] || [ -L "$secret_directory/$name" ]; then
        upgrade_source_complete=0
    fi
done

if [ -n "$upgrade_existing_names" ] && [ "$upgrade_source_complete" -eq 1 ]; then
    for name in $upgrade_existing_names; do
        validate_secret "$name" "$secret_directory/$name"
    done

    temporary_directory=$(mktemp -d "$secret_directory/.generate.XXXXXX")

    for name in $upgrade_missing_names; do
        generate_secret "$name" "$temporary_directory/$name"
    done
    validate_identity_key_separation "$temporary_directory"
    for name in $upgrade_missing_names; do
        mv "$temporary_directory/$name" "$secret_directory/$name"
    done
    rmdir "$temporary_directory"
    temporary_directory=

    printf '%s\n' "Extended the validated Compose secret set with the missing Identity secrets in $secret_directory"
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

for name in $secret_names; do
    generate_secret "$name" "$temporary_directory/$name"
done
validate_identity_key_separation "$temporary_directory"

for name in $secret_names; do
    mv "$temporary_directory/$name" "$secret_directory/$name"
done

rmdir "$temporary_directory"
temporary_directory=

printf '%s\n' "Generated the complete Compose development secret set in $secret_directory"
