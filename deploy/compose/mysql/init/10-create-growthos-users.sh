#!/bin/sh
set -eu

read_hex_secret() {
    variable_name=$1
    secret_path=$2

    if [ ! -r "$secret_path" ]; then
        printf '%s is unavailable\n' "$variable_name" >&2
        exit 1
    fi

    secret_value=$(tr -d '\r\n' < "$secret_path")
    case "$secret_value" in
        ''|*[!0-9a-f]*)
            printf '%s must be lowercase hexadecimal\n' "$variable_name" >&2
            exit 1
            ;;
    esac
    if [ "${#secret_value}" -ne 64 ]; then
        printf '%s must contain exactly 64 hexadecimal characters\n' "$variable_name" >&2
        exit 1
    fi

    printf '%s' "$secret_value"
}

app_password=$(read_hex_secret mysql_app_password /run/secrets/mysql_app_password)
migration_password=$(read_hex_secret mysql_migration_password /run/secrets/mysql_migration_password)
identity_password=$(read_hex_secret mysql_identity_password /run/secrets/mysql_identity_password)
identity_provisioner_password=$(read_hex_secret mysql_identity_provisioner_password /run/secrets/mysql_identity_provisioner_password)

MYSQL_PWD="${MYSQL_ROOT_PASSWORD:?mysql root password is unavailable}" mysql \
    --protocol=socket \
    --user=root <<SQL
CREATE USER 'growthos_app'@'%' IDENTIFIED BY '${app_password}';
CREATE USER 'growthos_migrator'@'%' IDENTIFIED BY '${migration_password}';
CREATE USER 'growthos_identity'@'%' IDENTIFIED BY '${identity_password}';
CREATE USER 'growthos_identity_provisioner'@'%' IDENTIFIED BY '${identity_provisioner_password}';

GRANT SELECT, INSERT, UPDATE, DELETE, CREATE, ALTER, DROP, INDEX, REFERENCES
    ON \`growthos\`.* TO 'growthos_migrator'@'%';
SQL

unset app_password migration_password identity_password identity_provisioner_password
