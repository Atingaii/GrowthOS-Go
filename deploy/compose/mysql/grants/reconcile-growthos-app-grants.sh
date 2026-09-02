#!/bin/sh
set -eu

read_hex_secret() {
    secret_name=$1
    secret_path=$2

    if [ ! -r "$secret_path" ]; then
        printf '%s is unavailable\n' "$secret_name" >&2
        exit 1
    fi

    secret_value=$(tr -d '\r\n' < "$secret_path")
    case "$secret_value" in
        ''|*[!0-9a-f]*)
            printf '%s must be lowercase hexadecimal\n' "$secret_name" >&2
            exit 1
            ;;
    esac
    if [ "${#secret_value}" -ne 64 ]; then
        printf '%s must contain exactly 64 hexadecimal characters\n' "$secret_name" >&2
        exit 1
    fi

    printf '%s' "$secret_value"
}

root_password=$(read_hex_secret mysql_root_password /run/secrets/mysql_root_password)
identity_password=$(read_hex_secret mysql_identity_password /run/secrets/mysql_identity_password)
identity_provisioner_password=$(read_hex_secret mysql_identity_provisioner_password /run/secrets/mysql_identity_provisioner_password)

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

# The runtime and one-shot identities are fully reconciled instead of receiving
# a schema wildcard. CREATE USER IF NOT EXISTS is the additive upgrade path for
# an existing volume; REVOKE ALL removes stale direct grants without deleting
# an account or changing an existing password.
mysql_root <<SQL
CREATE USER IF NOT EXISTS 'growthos_identity'@'%' IDENTIFIED BY '${identity_password}';
CREATE USER IF NOT EXISTS 'growthos_identity_provisioner'@'%' IDENTIFIED BY '${identity_provisioner_password}';
REVOKE IF EXISTS ALL PRIVILEGES, GRANT OPTION
    FROM 'growthos_app'@'%';
REVOKE IF EXISTS ALL PRIVILEGES, GRANT OPTION
    FROM 'growthos_identity'@'%';
REVOKE IF EXISTS ALL PRIVILEGES, GRANT OPTION
    FROM 'growthos_identity_provisioner'@'%';
GRANT SELECT
    ON \`growthos\`.\`lottery_strategy\` TO 'growthos_app'@'%';
GRANT SELECT
    ON \`growthos\`.\`lottery_strategy_award\` TO 'growthos_app'@'%';
GRANT SELECT
    ON \`growthos\`.\`identity_workforce_account\` TO 'growthos_identity'@'%';
GRANT UPDATE (\`updated_at\`)
    ON \`growthos\`.\`identity_workforce_account\` TO 'growthos_identity'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE
    ON \`growthos\`.\`identity_session\` TO 'growthos_identity'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE
    ON \`growthos\`.\`identity_authentication_throttle\` TO 'growthos_identity'@'%';
GRANT INSERT
    ON \`growthos\`.\`identity_workforce_account\` TO 'growthos_identity_provisioner'@'%';
SQL

actual_app_grants=$(mysql_root --execute="SHOW GRANTS FOR 'growthos_app'@'%'" | LC_ALL=C sort)
expected_app_grants=$(LC_ALL=C sort <<'EOF'
GRANT SELECT ON `growthos`.`lottery_strategy` TO `growthos_app`@`%`
GRANT SELECT ON `growthos`.`lottery_strategy_award` TO `growthos_app`@`%`
GRANT USAGE ON *.* TO `growthos_app`@`%`
EOF
)
actual_identity_grants=$(mysql_root --execute="SHOW GRANTS FOR 'growthos_identity'@'%'" | LC_ALL=C sort)
expected_identity_grants=$(LC_ALL=C sort <<'EOF'
GRANT SELECT, UPDATE (`updated_at`) ON `growthos`.`identity_workforce_account` TO `growthos_identity`@`%`
GRANT SELECT, INSERT, UPDATE, DELETE ON `growthos`.`identity_session` TO `growthos_identity`@`%`
GRANT SELECT, INSERT, UPDATE, DELETE ON `growthos`.`identity_authentication_throttle` TO `growthos_identity`@`%`
GRANT USAGE ON *.* TO `growthos_identity`@`%`
EOF
)
actual_identity_provisioner_grants=$(mysql_root --execute="SHOW GRANTS FOR 'growthos_identity_provisioner'@'%'" | LC_ALL=C sort)
expected_identity_provisioner_grants=$(LC_ALL=C sort <<'EOF'
GRANT INSERT ON `growthos`.`identity_workforce_account` TO `growthos_identity_provisioner`@`%`
GRANT USAGE ON *.* TO `growthos_identity_provisioner`@`%`
EOF
)
mandatory_roles=$(mysql_root --execute='SELECT @@GLOBAL.mandatory_roles')

if [ "$actual_app_grants" != "$expected_app_grants" ] || \
   [ "$actual_identity_grants" != "$expected_identity_grants" ] || \
   [ "$actual_identity_provisioner_grants" != "$expected_identity_provisioner_grants" ] || \
   [ -n "$mandatory_roles" ]; then
    printf '%s\n' 'runtime and one-shot MySQL grants differ from the exact allowlists' >&2
    unset root_password identity_password identity_provisioner_password \
        actual_app_grants expected_app_grants actual_identity_grants \
        expected_identity_grants actual_identity_provisioner_grants \
        expected_identity_provisioner_grants mandatory_roles
    exit 1
fi

# USAGE plus the table INSERT grant must also authenticate with the mounted
# one-shot credential. SELECT 1 reads no schema object and therefore does not
# broaden the provisioner's table boundary.
if ! MYSQL_PWD="$identity_provisioner_password" mysql \
    --protocol=socket \
    --socket=/var/run/mysqld/mysqld.sock \
    --user=growthos_identity_provisioner \
    --database=growthos \
    --batch \
    --silent \
    --skip-column-names \
    --execute='SELECT 1' | grep -qx 1; then
    printf '%s\n' 'growthos_identity_provisioner credential verification failed' >&2
    unset root_password identity_password identity_provisioner_password \
        actual_app_grants expected_app_grants actual_identity_grants \
        expected_identity_grants actual_identity_provisioner_grants \
        expected_identity_provisioner_grants mandatory_roles
    exit 1
fi

# Prove that the mounted Identity credential matches the account on both fresh
# and reused volumes. The fixed query reveals neither the password nor table
# contents and fails closed before the API can start.
if ! MYSQL_PWD="$identity_password" mysql \
    --protocol=socket \
    --socket=/var/run/mysqld/mysqld.sock \
    --user=growthos_identity \
    --database=growthos \
    --batch \
    --silent \
    --skip-column-names \
    --execute='SELECT 1' | grep -qx 1; then
    printf '%s\n' 'growthos_identity credential verification failed' >&2
    unset root_password identity_password identity_provisioner_password \
        actual_app_grants expected_app_grants actual_identity_grants \
        expected_identity_grants actual_identity_provisioner_grants \
        expected_identity_provisioner_grants mandatory_roles
    exit 1
fi

unset root_password identity_password identity_provisioner_password \
    actual_app_grants expected_app_grants actual_identity_grants \
    expected_identity_grants actual_identity_provisioner_grants \
    expected_identity_provisioner_grants mandatory_roles
printf '%s\n' 'runtime and one-shot MySQL grants match the exact allowlists'
