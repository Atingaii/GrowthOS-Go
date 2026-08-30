CREATE TABLE `lottery_strategy_snapshot` (
    `strategy_id` BIGINT UNSIGNED NOT NULL,
    `revision` VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `schema_version` SMALLINT UNSIGNED NOT NULL,
    `name` VARCHAR(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
    `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),

    PRIMARY KEY (`strategy_id`, `revision`),

    CONSTRAINT `fk_lottery_strategy_snapshot_strategy`
        FOREIGN KEY (`strategy_id`)
        REFERENCES `lottery_strategy` (`strategy_id`)
        ON DELETE RESTRICT
        ON UPDATE RESTRICT,
    CONSTRAINT `chk_lottery_strategy_snapshot_id_positive`
        CHECK (`strategy_id` > 0),
    CONSTRAINT `chk_lottery_strategy_snapshot_revision`
        CHECK (
            CHAR_LENGTH(`revision`) BETWEEN 1 AND 128
            AND `revision` REGEXP '^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$'
        ),
    CONSTRAINT `chk_lottery_strategy_snapshot_schema_version`
        CHECK (`schema_version` = 1),
    CONSTRAINT `chk_lottery_strategy_snapshot_name_basic`
        CHECK (
            CHAR_LENGTH(`name`) > 0
            AND `name` = TRIM(`name`)
        )
) ENGINE = InnoDB
  DEFAULT CHARACTER SET = utf8mb4
  COLLATE = utf8mb4_0900_bin;
