CREATE TABLE `lottery_strategy_snapshot_award` (
    `strategy_id` BIGINT UNSIGNED NOT NULL,
    `revision` VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `award_id` BIGINT UNSIGNED NOT NULL,
    `name` VARCHAR(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
    `weight` BIGINT UNSIGNED NOT NULL,
    `outcome` VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),

    PRIMARY KEY (`strategy_id`, `revision`, `award_id`),

    CONSTRAINT `fk_lottery_strategy_snapshot_award_snapshot`
        FOREIGN KEY (`strategy_id`, `revision`)
        REFERENCES `lottery_strategy_snapshot` (`strategy_id`, `revision`)
        ON DELETE RESTRICT
        ON UPDATE RESTRICT,
    CONSTRAINT `chk_lottery_strategy_snapshot_award_ids_positive`
        CHECK (`strategy_id` > 0 AND `award_id` > 0),
    CONSTRAINT `chk_lottery_strategy_snapshot_award_revision`
        CHECK (
            CHAR_LENGTH(`revision`) BETWEEN 1 AND 128
            AND `revision` REGEXP '^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$'
        ),
    CONSTRAINT `chk_lottery_strategy_snapshot_award_name_basic`
        CHECK (
            CHAR_LENGTH(`name`) > 0
            AND `name` = TRIM(`name`)
        ),
    CONSTRAINT `chk_lottery_strategy_snapshot_award_weight_positive`
        CHECK (`weight` > 0),
    CONSTRAINT `chk_lottery_strategy_snapshot_award_outcome`
        CHECK (`outcome` IN ('reward', 'no_reward'))
) ENGINE = InnoDB
  DEFAULT CHARACTER SET = utf8mb4
  COLLATE = utf8mb4_0900_bin;
