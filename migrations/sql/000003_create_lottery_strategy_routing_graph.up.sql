CREATE TABLE `lottery_strategy_routing_graph` (
    `graph_id` BIGINT UNSIGNED NOT NULL,
    `revision` VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `schema_version` SMALLINT UNSIGNED NOT NULL,
    `root_node_id` BIGINT UNSIGNED NOT NULL,
    `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),

    PRIMARY KEY (`graph_id`, `revision`),

    CONSTRAINT `chk_lottery_strategy_routing_graph_id_positive`
        CHECK (`graph_id` > 0),
    CONSTRAINT `chk_lottery_strategy_routing_graph_revision`
        CHECK (
            CHAR_LENGTH(`revision`) BETWEEN 1 AND 128
            AND `revision` REGEXP '^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$'
        ),
    CONSTRAINT `chk_lottery_strategy_routing_graph_schema_version`
        CHECK (`schema_version` = 1),
    CONSTRAINT `chk_lottery_strategy_routing_graph_root_positive`
        CHECK (`root_node_id` > 0)
) ENGINE = InnoDB
  DEFAULT CHARACTER SET = utf8mb4
  COLLATE = utf8mb4_0900_bin;
