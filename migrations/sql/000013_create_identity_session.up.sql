CREATE TABLE `identity_session` (
    `session_ref` VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `issue_operation_ref` VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `account_id` VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `token_digest` BINARY(32) NOT NULL,
    `authentication_epoch` BIGINT UNSIGNED NOT NULL,
    `issued_at` DATETIME(6) NOT NULL,
    `last_seen_at` DATETIME(6) NOT NULL,
    `idle_expires_at` DATETIME(6) NOT NULL,
    `absolute_expires_at` DATETIME(6) NOT NULL,
    `revoked_at` DATETIME(6) NULL,
    `revoke_reason` VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NULL,
    `revoke_operation_ref` VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NULL,
    `updated_at` DATETIME(6) NOT NULL,

    PRIMARY KEY (`session_ref`),
    UNIQUE KEY `uq_identity_session_issue_operation` (`issue_operation_ref`),
    UNIQUE KEY `uq_identity_session_token_digest` (`token_digest`),
    UNIQUE KEY `uq_identity_session_revoke_operation` (`revoke_operation_ref`),
    KEY `idx_identity_session_account_active` (
        `account_id`,
        `authentication_epoch`,
        `revoked_at`,
        `absolute_expires_at`,
        `idle_expires_at`,
        `last_seen_at`,
        `issued_at`,
        `session_ref`
    ),
    KEY `idx_identity_session_absolute_cleanup` (`absolute_expires_at`, `session_ref`),
    KEY `idx_identity_session_revoked_cleanup` (`revoked_at`, `session_ref`),

    CONSTRAINT `fk_identity_session_account`
        FOREIGN KEY (`account_id`)
        REFERENCES `identity_workforce_account` (`account_id`)
        ON DELETE RESTRICT
        ON UPDATE RESTRICT,
    CONSTRAINT `chk_identity_session_ref`
        CHECK (
            CHAR_LENGTH(`session_ref`) BETWEEN 1 AND 128
            AND `session_ref` REGEXP '^[a-z0-9]([a-z0-9._:-]{0,126}[a-z0-9])?$'
        ),
    CONSTRAINT `chk_identity_session_issue_operation`
        CHECK (
            CHAR_LENGTH(`issue_operation_ref`) BETWEEN 1 AND 128
            AND `issue_operation_ref` REGEXP '^[a-z0-9]([a-z0-9._:-]{0,126}[a-z0-9])?$'
        ),
    CONSTRAINT `chk_identity_session_token_digest`
        CHECK (
            `token_digest` <> X'0000000000000000000000000000000000000000000000000000000000000000'
        ),
    CONSTRAINT `chk_identity_session_epoch`
        CHECK (`authentication_epoch` > 0),
    CONSTRAINT `chk_identity_session_times`
        CHECK (
            `issued_at` <= `last_seen_at`
            AND `last_seen_at` < `idle_expires_at`
            AND `idle_expires_at` <= `absolute_expires_at`
            AND `updated_at` >= `issued_at`
            AND `updated_at` >= `last_seen_at`
            AND (`revoked_at` IS NULL OR `updated_at` >= `revoked_at`)
        ),
    CONSTRAINT `chk_identity_session_revocation_shape`
        CHECK (
            (
                `revoked_at` IS NULL
                AND `revoke_reason` IS NULL
                AND `revoke_operation_ref` IS NULL
            )
            OR
            (
                `revoked_at` IS NOT NULL
                AND `revoked_at` >= `last_seen_at`
                AND `revoke_reason` IN (
                    'logout',
                    'concurrency_limit',
                    'authentication_epoch_changed',
                    'account_disabled',
                    'security_response'
                )
                AND `revoke_operation_ref` IS NOT NULL
                AND CHAR_LENGTH(`revoke_operation_ref`) BETWEEN 1 AND 128
                AND `revoke_operation_ref` REGEXP '^[a-z0-9]([a-z0-9._:-]{0,126}[a-z0-9])?$'
            )
        )
) ENGINE = InnoDB
  DEFAULT CHARACTER SET = ascii
  COLLATE = ascii_bin;
