#!/bin/sh
set -eu

read_hex_secret() {
    secret_path=$1

    if [ ! -r "$secret_path" ]; then
        printf '%s\n' 'mysql root password is unavailable' >&2
        exit 1
    fi

    secret_value=$(tr -d '\r\n' < "$secret_path")
    case "$secret_value" in
        ''|*[!0-9a-f]*)
            printf '%s\n' 'mysql root password must be lowercase hexadecimal' >&2
            exit 1
            ;;
    esac
    if [ "${#secret_value}" -ne 64 ]; then
        printf '%s\n' 'mysql root password must contain exactly 64 hexadecimal characters' >&2
        exit 1
    fi

    printf '%s' "$secret_value"
}

root_password=$(read_hex_secret /run/secrets/mysql_root_password)

mysql_root() {
    MYSQL_PWD="$root_password" mysql \
        --protocol=socket \
        --socket=/var/run/mysqld/mysqld.sock \
        --user=root \
        --batch \
        --silent \
        --skip-column-names \
        "$@"
}

# The Compose application identity is fully reconciled instead of receiving a
# schema wildcard. REVOKE ALL also removes stale table/column/routine grants
# from a reused local volume; it does not delete the account.
mysql_root --execute="
REVOKE IF EXISTS ALL PRIVILEGES, GRANT OPTION
    FROM 'growthos_app'@'%';
GRANT SELECT, INSERT
    ON \`growthos\`.\`lottery_strategy\` TO 'growthos_app'@'%';
GRANT SELECT, INSERT
    ON \`growthos\`.\`lottery_strategy_award\` TO 'growthos_app'@'%';
"

actual_grants=$(mysql_root --execute="SHOW GRANTS FOR 'growthos_app'@'%'" | LC_ALL=C sort)
expected_grants=$(LC_ALL=C sort <<'EOF'
GRANT SELECT, INSERT ON `growthos`.`lottery_strategy` TO `growthos_app`@`%`
GRANT SELECT, INSERT ON `growthos`.`lottery_strategy_award` TO `growthos_app`@`%`
GRANT USAGE ON *.* TO `growthos_app`@`%`
EOF
)
mandatory_roles=$(mysql_root --execute='SELECT @@GLOBAL.mandatory_roles')

if [ "$actual_grants" != "$expected_grants" ] || [ -n "$mandatory_roles" ]; then
    printf '%s\n' 'growthos_app effective grants differ from the current SELECT+INSERT allowlist' >&2
    unset root_password actual_grants expected_grants mandatory_roles
    exit 1
fi

unset root_password actual_grants expected_grants mandatory_roles
printf '%s\n' 'growthos_app grants match the current SELECT+INSERT allowlist'
