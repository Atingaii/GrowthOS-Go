CREATE TABLE `lottery_strategy` (
    `strategy_id` BIGINT UNSIGNED NOT NULL,
    `name` VARCHAR(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
    `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    `updated_at` DATETIME(6) NOT NULL
        DEFAULT CURRENT_TIMESTAMP(6)
        ON UPDATE CURRENT_TIMESTAMP(6),

    PRIMARY KEY (`strategy_id`),

    CONSTRAINT `chk_lottery_strategy_id_positive`
        CHECK (`strategy_id` > 0),
    CONSTRAINT `chk_lottery_strategy_name_basic`
        CHECK (
            CHAR_LENGTH(`name`) > 0
            AND `name` = TRIM(`name`)
        )
) ENGINE = InnoDB
  DEFAULT CHARACTER SET = utf8mb4
  COLLATE = utf8mb4_0900_bin;
