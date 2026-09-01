CREATE TABLE `identity_workforce_account` (
    `account_id` VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `login_name` VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `principal_id` VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `password_envelope` VARCHAR(256) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `account_status` VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `credential_version` BIGINT UNSIGNED NOT NULL,
    `authentication_epoch` BIGINT UNSIGNED NOT NULL,
    `created_at` DATETIME(6) NOT NULL,
    `updated_at` DATETIME(6) NOT NULL,

    PRIMARY KEY (`account_id`),
    UNIQUE KEY `uq_identity_workforce_account_login` (`login_name`),
    UNIQUE KEY `uq_identity_workforce_account_principal` (`principal_id`),

    CONSTRAINT `chk_identity_workforce_account_id`
        CHECK (
            CHAR_LENGTH(`account_id`) BETWEEN 1 AND 128
            AND `account_id` REGEXP '^[a-z0-9]([a-z0-9._:-]{0,126}[a-z0-9])?$'
        ),
    CONSTRAINT `chk_identity_workforce_account_login`
        CHECK (
            CHAR_LENGTH(`login_name`) BETWEEN 3 AND 64
            AND `login_name` REGEXP '^[a-z][a-z0-9._-]{2,63}$'
        ),
    CONSTRAINT `chk_identity_workforce_account_principal`
        CHECK (
            CHAR_LENGTH(`principal_id`) BETWEEN 1 AND 128
            AND `principal_id` REGEXP '^[a-z0-9]([a-z0-9._:-]{0,126}[a-z0-9])?$'
        ),
    CONSTRAINT `chk_identity_workforce_account_envelope`
        CHECK (
            CHAR_LENGTH(`password_envelope`) BETWEEN 1 AND 256
            AND LEFT(`password_envelope`, 10) = '$argon2id$'
        ),
    CONSTRAINT `chk_identity_workforce_account_status`
        CHECK (`account_status` IN ('enabled', 'disabled')),
    CONSTRAINT `chk_identity_workforce_account_versions`
        CHECK (`credential_version` > 0 AND `authentication_epoch` > 0),
    CONSTRAINT `chk_identity_workforce_account_times`
        CHECK (`created_at` <= `updated_at`)
) ENGINE = InnoDB
  DEFAULT CHARACTER SET = ascii
  COLLATE = ascii_bin;
