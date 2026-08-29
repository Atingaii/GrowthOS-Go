CREATE TABLE `lottery_strategy_award` (
    `strategy_id` BIGINT UNSIGNED NOT NULL,
    `award_id` BIGINT UNSIGNED NOT NULL,
    `name` VARCHAR(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
    `weight` BIGINT UNSIGNED NOT NULL,
    `outcome` VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    `updated_at` DATETIME(6) NOT NULL
        DEFAULT CURRENT_TIMESTAMP(6)
        ON UPDATE CURRENT_TIMESTAMP(6),

    PRIMARY KEY (`strategy_id`, `award_id`),

    CONSTRAINT `fk_lottery_strategy_award_strategy`
        FOREIGN KEY (`strategy_id`)
        REFERENCES `lottery_strategy` (`strategy_id`)
        ON DELETE RESTRICT
        ON UPDATE RESTRICT,
    CONSTRAINT `chk_lottery_strategy_award_id_positive`
        CHECK (`award_id` > 0),
    CONSTRAINT `chk_lottery_strategy_award_name_basic`
        CHECK (
            CHAR_LENGTH(`name`) > 0
            AND `name` = TRIM(`name`)
        ),
    CONSTRAINT `chk_lottery_strategy_award_weight_positive`
        CHECK (`weight` > 0),
    CONSTRAINT `chk_lottery_strategy_award_outcome`
        CHECK (`outcome` IN ('reward', 'no_reward'))
) ENGINE = InnoDB
  DEFAULT CHARACTER SET = utf8mb4
  COLLATE = utf8mb4_0900_bin;
