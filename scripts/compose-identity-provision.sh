#!/bin/sh
set -eu

# Runs exactly one workforce-account create through the operations-only
# provisioner. The enrollment password is never placed in an environment
# variable, shell variable containing its bytes, or process argument. A
# caller-owned 0600 file is copied into a short-lived 0444 snapshot so the
# container can remain uid 65532; its 0700 parent retains the host boundary.

repository_root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
compose_project=${GROWTHOS_COMPOSE_PROJECT:-growthos}
compose_file=${GROWTHOS_COMPOSE_FILE:-"$repository_root/deploy/compose/compose.yaml"}
secret_generator="$repository_root/scripts/generate-compose-secrets.sh"
umask 077

fail() {
    printf 'identity provision refused: %s\n' "$1" >&2
    exit 1
}

require_command() {
    if ! command -v "$1" >/dev/null 2>&1; then
        fail "$1 is required"
    fi
}

for required_command in awk chmod cmp cp dd docker id mktemp rmdir stat unlink wc; do
    require_command "$required_command"
done
if ! docker compose version >/dev/null 2>&1; then
    fail 'the Docker Compose plugin is unavailable'
fi
if [ ! -f "$compose_file" ] || [ -L "$compose_file" ] || [ ! -r "$compose_file" ]; then
    fail 'the configured Compose file must be a readable non-symbolic regular file'
fi
if [ ! -f "$secret_generator" ] || [ -L "$secret_generator" ] || [ ! -x "$secret_generator" ]; then
    fail 'the Compose secret generator is unavailable'
fi
case "$compose_project" in
    ''|*[!A-Za-z0-9_.-]*)
        fail 'GROWTHOS_COMPOSE_PROJECT contains unsupported characters'
        ;;
esac

if [ "$#" -ne 8 ]; then
    fail 'expected --account-id, --login-name, --principal-id, and --password-file exactly once'
fi

account_id=
login_name=
principal_id=
password_file=
while [ "$#" -gt 0 ]; do
    option_name=$1
    option_value=$2
    shift 2
    case "$option_name" in
        --account-id)
            [ -z "$account_id" ] || fail '--account-id was supplied more than once'
            account_id=$option_value
            ;;
        --login-name)
            [ -z "$login_name" ] || fail '--login-name was supplied more than once'
            login_name=$option_value
            ;;
        --principal-id)
            [ -z "$principal_id" ] || fail '--principal-id was supplied more than once'
            principal_id=$option_value
            ;;
        --password-file)
            [ -z "$password_file" ] || fail '--password-file was supplied more than once'
            password_file=$option_value
            ;;
        *)
            fail 'an unsupported option was supplied'
            ;;
    esac
done

if [ -z "$account_id" ] || [ -z "$login_name" ] ||
   [ -z "$principal_id" ] || [ -z "$password_file" ]; then
    fail 'every required option must contain a non-empty value'
fi
case "$password_file" in
    *:*)
        fail 'the enrollment password path must not contain a colon'
        ;;
    *'
'*)
        fail 'the enrollment password path must not contain a newline'
        ;;
esac
if [ -L "$password_file" ] || [ ! -f "$password_file" ] || [ ! -r "$password_file" ]; then
    fail 'the enrollment password must be a readable non-symbolic regular file'
fi

password_parent=$(CDPATH='' cd -- "$(dirname -- "$password_file")" && pwd -P) ||
    fail 'the enrollment password parent directory cannot be resolved'
password_name=$(basename -- "$password_file")
password_file="$password_parent/$password_name"

inspect_file_metadata() {
    inspected_path=$1
    if file_mode=$(stat -f '%Lp' "$inspected_path" 2>/dev/null); then
        file_owner=$(stat -f '%u' "$inspected_path") || return 1
        file_links=$(stat -f '%l' "$inspected_path") || return 1
    elif file_mode=$(stat -c '%a' "$inspected_path" 2>/dev/null); then
        file_owner=$(stat -c '%u' "$inspected_path") || return 1
        file_links=$(stat -c '%h' "$inspected_path") || return 1
    else
        return 1
    fi
}

validate_enrollment_source() {
    if [ -L "$password_file" ] || [ ! -f "$password_file" ] || [ ! -r "$password_file" ]; then
        fail 'the enrollment password changed into an unsafe file'
    fi
    inspect_file_metadata "$password_file" || fail 'the enrollment password metadata is unavailable'
    if [ "$file_mode" != 600 ]; then
        fail 'the enrollment password file mode must be exactly 0600'
    fi
    if [ "$file_owner" != "$(id -u)" ]; then
        fail 'the enrollment password file must be owned by the invoking user'
    fi
    if [ "$file_links" != 1 ]; then
        fail 'the enrollment password file must have exactly one hard link'
    fi
    password_bytes=$(LC_ALL=C wc -c < "$password_file" | awk '{ print $1 }') ||
        fail 'the enrollment password size cannot be measured'
    case "$password_bytes" in
        ''|*[!0-9]*)
            fail 'the enrollment password size is invalid'
            ;;
    esac
    if [ "$password_bytes" -eq 0 ] || [ "$password_bytes" -gt 514 ]; then
        fail 'the enrollment password transport must contain from 1 through 514 bytes'
    fi
}

validate_enrollment_source

GROWTHOS_COMPOSE_PROJECT="$compose_project" \
GROWTHOS_COMPOSE_WEB_PORT="${GROWTHOS_COMPOSE_WEB_PORT:-8088}" \
GROWTHOS_COMPOSE_IDENTITY_CSRF_ACTIVE_KEY_ID="${GROWTHOS_COMPOSE_IDENTITY_CSRF_ACTIVE_KEY_ID:-local-v1}" \
    "$secret_generator"

compose() {
    docker compose \
        --project-name "$compose_project" \
        --file "$compose_file" \
        --profile operations \
        "$@"
}

compose config --quiet
compose build identity-provision
compose up --detach --build --wait --wait-timeout 180 mysql-grants

# Revalidate immediately before taking the private snapshot. Copying avoids the
# uid-501/uid-65532 bind-mount mismatch without relaxing the caller's source
# file. cmp proves the bounded snapshot contains exactly the reviewed bytes.
validate_enrollment_source
temporary_directory=$(mktemp -d "${TMPDIR:-/tmp}/growthos-identity-provision.XXXXXX") ||
    fail 'the private enrollment snapshot directory cannot be created'
if [ -L "$temporary_directory" ] || [ ! -d "$temporary_directory" ]; then
    fail 'the private enrollment snapshot directory is unsafe'
fi
chmod 0700 "$temporary_directory"
password_snapshot="$temporary_directory/enrollment-password"
snapshot_bytes=$password_bytes

cleanup() {
    cleanup_status=$?
    trap - 0 HUP INT TERM
    snapshot_cleanup_failed=0
    if [ -n "${password_snapshot:-}" ] && { [ -e "$password_snapshot" ] || [ -L "$password_snapshot" ]; }; then
        if [ -L "$password_snapshot" ] || [ ! -f "$password_snapshot" ]; then
            printf '%s\n' 'identity provision cleanup preserved an unexpected snapshot path for inspection' >&2
            snapshot_cleanup_failed=1
        else
            chmod 0600 "$password_snapshot" 2>/dev/null || snapshot_cleanup_failed=1
            dd if=/dev/zero of="$password_snapshot" bs=1 count="${snapshot_bytes:-0}" conv=notrunc \
                >/dev/null 2>&1 || snapshot_cleanup_failed=1
            unlink "$password_snapshot" 2>/dev/null || snapshot_cleanup_failed=1
        fi
    fi
    if [ -n "${temporary_directory:-}" ] && [ -d "$temporary_directory" ] && [ ! -L "$temporary_directory" ]; then
        rmdir "$temporary_directory" 2>/dev/null || snapshot_cleanup_failed=1
    fi
    if [ "$snapshot_cleanup_failed" -ne 0 ]; then
        printf '%s\n' 'identity provision cleanup could not remove the private password snapshot' >&2
        exit 1
    fi
    exit "$cleanup_status"
}
trap cleanup 0
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

cp "$password_file" "$password_snapshot" || fail 'the private enrollment snapshot cannot be copied'
chmod 0444 "$password_snapshot"
copied_bytes=$(LC_ALL=C wc -c < "$password_snapshot" | awk '{ print $1 }') ||
    fail 'the private enrollment snapshot size cannot be measured'
if [ "$copied_bytes" != "$snapshot_bytes" ]; then
    fail 'the enrollment password changed while its private snapshot was copied'
fi
if cmp -s "$password_file" "$password_snapshot"; then
    :
else
    comparison_status=$?
    if [ "$comparison_status" -eq 1 ]; then
        fail 'the enrollment password changed while its private snapshot was copied'
    fi
    fail 'the enrollment password snapshot cannot be verified'
fi

compose run \
    --rm \
    --no-deps \
    --no-tty \
    --volume "$password_snapshot:/run/identity-enrollment/password:ro" \
    identity-provision \
    create \
    --account-id "$account_id" \
    --login-name "$login_name" \
    --principal-id "$principal_id" \
    --password-file /run/identity-enrollment/password
