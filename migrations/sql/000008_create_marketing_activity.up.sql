CREATE TABLE `marketing_activity` (
    `activity_id` BIGINT UNSIGNED NOT NULL,
    `name` VARCHAR(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
    `lifecycle_state` VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `state_version` BIGINT UNSIGNED NOT NULL,
    `active_version` BIGINT UNSIGNED NULL,
    `retired_at` DATETIME(6) NULL,
    `retirement_reference` VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NULL,
    `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    `updated_at` DATETIME(6) NOT NULL
        DEFAULT CURRENT_TIMESTAMP(6)
        ON UPDATE CURRENT_TIMESTAMP(6),

    PRIMARY KEY (`activity_id`),

    CONSTRAINT `chk_marketing_activity_id_positive`
        CHECK (`activity_id` > 0),
    CONSTRAINT `chk_marketing_activity_name_basic`
        CHECK (
            CHAR_LENGTH(`name`) BETWEEN 1 AND 128
            AND `name` = TRIM(`name`)
        ),
    CONSTRAINT `chk_marketing_activity_state_shape`
        CHECK (
            (
                `lifecycle_state` = 'draft'
                AND `state_version` = 0
                AND `active_version` IS NULL
                AND `retired_at` IS NULL
                AND `retirement_reference` IS NULL
            )
            OR
            (
                `lifecycle_state` = 'published'
                AND `state_version` > 0
                AND `active_version` IS NOT NULL
                AND `active_version` > 0
                AND `state_version` = `active_version`
                AND `retired_at` IS NULL
                AND `retirement_reference` IS NULL
            )
            OR
            (
                `lifecycle_state` = 'retired'
                AND `state_version` > 0
                AND `active_version` IS NOT NULL
                AND `active_version` > 0
                AND `state_version` = `active_version` + 1
                AND `retired_at` IS NOT NULL
                AND `retirement_reference` IS NOT NULL
            )
        ),
    CONSTRAINT `chk_marketing_activity_retirement_ref`
        CHECK (
            `retirement_reference` IS NULL
            OR (
                CHAR_LENGTH(`retirement_reference`) BETWEEN 1 AND 128
                AND `retirement_reference` REGEXP '^[A-Za-z0-9][A-Za-z0-9._:/-]{0,127}$'
            )
        )
) ENGINE = InnoDB
  DEFAULT CHARACTER SET = utf8mb4
  COLLATE = utf8mb4_0900_bin;
