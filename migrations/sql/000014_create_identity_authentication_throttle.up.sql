CREATE TABLE `identity_authentication_throttle` (
    `dimension` VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `subject_digest` BINARY(32) NOT NULL,
    `window_started_at` DATETIME(6) NOT NULL,
    `window_expires_at` DATETIME(6) NOT NULL,
    `failure_count` INT UNSIGNED NOT NULL,
    `inflight_count` INT UNSIGNED NOT NULL,
    `admission_epoch` BIGINT UNSIGNED NOT NULL,
    `inflight_expires_at` DATETIME(6) NULL,
    `blocked_until` DATETIME(6) NULL,
    `updated_at` DATETIME(6) NOT NULL,
    `row_expires_at` DATETIME(6) NOT NULL,

    PRIMARY KEY (`dimension`, `subject_digest`),
    KEY `idx_identity_throttle_cleanup`
        (`row_expires_at`, `dimension`, `subject_digest`),

    CONSTRAINT `chk_identity_throttle_dimension`
        CHECK (`dimension` IN ('login', 'source')),
    CONSTRAINT `chk_identity_throttle_digest`
        CHECK (
            `subject_digest` <> X'0000000000000000000000000000000000000000000000000000000000000000'
        ),
    CONSTRAINT `chk_identity_throttle_window`
        CHECK (
            `window_started_at` < `window_expires_at`
            AND `window_started_at` <= `updated_at`
            AND `row_expires_at` > `updated_at`
            AND `row_expires_at` >= `window_expires_at`
        ),
    CONSTRAINT `chk_identity_throttle_epoch`
        CHECK (`admission_epoch` > 0),
    CONSTRAINT `chk_identity_throttle_aggregate`
        CHECK (
            `failure_count` <= 4294967295 - `inflight_count`
        ),
    CONSTRAINT `chk_identity_throttle_inflight_shape`
        CHECK (
            (
                `inflight_count` = 0
                AND `inflight_expires_at` IS NULL
            )
            OR
            (
                `inflight_count` > 0
                AND `inflight_expires_at` IS NOT NULL
                AND `inflight_expires_at` > `window_started_at`
                AND `row_expires_at` >= `inflight_expires_at`
            )
        ),
    CONSTRAINT `chk_identity_throttle_block_shape`
        CHECK (
            `blocked_until` IS NULL
            OR
            (
                `failure_count` > 0
                AND `blocked_until` > `window_started_at`
                AND `blocked_until` <= `window_expires_at`
                AND `row_expires_at` >= `blocked_until`
            )
        )
) ENGINE = InnoDB
  DEFAULT CHARACTER SET = ascii
  COLLATE = ascii_bin;
