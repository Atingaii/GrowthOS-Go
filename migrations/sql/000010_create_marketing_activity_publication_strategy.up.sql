CREATE TABLE `marketing_activity_publication_strategy` (
    `activity_id` BIGINT UNSIGNED NOT NULL,
    `activity_version` BIGINT UNSIGNED NOT NULL,
    `strategy_id` BIGINT UNSIGNED NOT NULL,
    `strategy_revision` VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),

    PRIMARY KEY (`activity_id`, `activity_version`, `strategy_id`),
    KEY `idx_marketing_publication_strategy_snapshot`
        (`strategy_id`, `strategy_revision`),

    CONSTRAINT `fk_marketing_publication_strategy_publication`
        FOREIGN KEY (`activity_id`, `activity_version`)
        REFERENCES `marketing_activity_publication` (`activity_id`, `activity_version`)
        ON DELETE RESTRICT
        ON UPDATE RESTRICT,
    CONSTRAINT `chk_marketing_publication_strategy_identity`
        CHECK (
            `activity_id` > 0
            AND `activity_version` > 0
            AND `strategy_id` > 0
            AND CHAR_LENGTH(`strategy_revision`) BETWEEN 1 AND 128
            AND `strategy_revision` REGEXP '^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$'
        )
) ENGINE = InnoDB
  DEFAULT CHARACTER SET = utf8mb4
  COLLATE = utf8mb4_0900_bin;
