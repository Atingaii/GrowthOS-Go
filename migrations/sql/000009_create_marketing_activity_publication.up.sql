CREATE TABLE `marketing_activity_publication` (
    `activity_id` BIGINT UNSIGNED NOT NULL,
    `activity_version` BIGINT UNSIGNED NOT NULL,
    `schema_version` SMALLINT UNSIGNED NOT NULL,
    `publication_kind` VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `rollback_of_version` BIGINT UNSIGNED NULL,
    `graph_id` BIGINT UNSIGNED NOT NULL,
    `graph_revision` VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `starts_at` DATETIME(6) NOT NULL,
    `ends_at` DATETIME(6) NOT NULL,
    `published_at` DATETIME(6) NOT NULL,
    `approval_reference` VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),

    PRIMARY KEY (`activity_id`, `activity_version`),
    KEY `idx_marketing_activity_publication_graph`
        (`graph_id`, `graph_revision`),
    KEY `idx_marketing_activity_publication_rollback`
        (`activity_id`, `rollback_of_version`),

    CONSTRAINT `fk_marketing_publication_activity`
        FOREIGN KEY (`activity_id`)
        REFERENCES `marketing_activity` (`activity_id`)
        ON DELETE RESTRICT
        ON UPDATE RESTRICT,
    CONSTRAINT `fk_marketing_publication_rollback`
        FOREIGN KEY (`activity_id`, `rollback_of_version`)
        REFERENCES `marketing_activity_publication` (`activity_id`, `activity_version`)
        ON DELETE RESTRICT
        ON UPDATE RESTRICT,
    CONSTRAINT `chk_marketing_publication_identity`
        CHECK (`activity_id` > 0 AND `activity_version` > 0),
    CONSTRAINT `chk_marketing_publication_schema_version`
        CHECK (`schema_version` = 1),
    CONSTRAINT `chk_marketing_publication_kind_shape`
        CHECK (
            (`publication_kind` = 'release' AND `rollback_of_version` IS NULL)
            OR
            (
                `publication_kind` = 'rollback'
                AND `rollback_of_version` IS NOT NULL
                AND `rollback_of_version` > 0
                AND `rollback_of_version` < `activity_version`
            )
        ),
    CONSTRAINT `chk_marketing_publication_graph_identity`
        CHECK (
            `graph_id` > 0
            AND CHAR_LENGTH(`graph_revision`) BETWEEN 1 AND 128
            AND `graph_revision` REGEXP '^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$'
        ),
    CONSTRAINT `chk_marketing_publication_window`
        CHECK (`starts_at` < `ends_at` AND `published_at` < `ends_at`),
    CONSTRAINT `chk_marketing_publication_approval_ref`
        CHECK (
            CHAR_LENGTH(`approval_reference`) BETWEEN 1 AND 128
            AND `approval_reference` REGEXP '^[A-Za-z0-9][A-Za-z0-9._:/-]{0,127}$'
        )
) ENGINE = InnoDB
  DEFAULT CHARACTER SET = utf8mb4
  COLLATE = utf8mb4_0900_bin;
