CREATE TABLE `lottery_strategy_routing_edge` (
    `graph_id` BIGINT UNSIGNED NOT NULL,
    `revision` VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `from_node_id` BIGINT UNSIGNED NOT NULL,
    `branch_code` VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `to_node_id` BIGINT UNSIGNED NOT NULL,
    `is_default` TINYINT UNSIGNED NOT NULL,
    `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),

    PRIMARY KEY (`graph_id`, `revision`, `from_node_id`, `branch_code`),
    KEY `idx_lottery_strategy_routing_edge_to_node`
        (`graph_id`, `revision`, `to_node_id`),

    CONSTRAINT `fk_lottery_strategy_routing_edge_from_node`
        FOREIGN KEY (`graph_id`, `revision`, `from_node_id`)
        REFERENCES `lottery_strategy_routing_node` (`graph_id`, `revision`, `node_id`)
        ON DELETE RESTRICT
        ON UPDATE RESTRICT,
    CONSTRAINT `fk_lottery_strategy_routing_edge_to_node`
        FOREIGN KEY (`graph_id`, `revision`, `to_node_id`)
        REFERENCES `lottery_strategy_routing_node` (`graph_id`, `revision`, `node_id`)
        ON DELETE RESTRICT
        ON UPDATE RESTRICT,
    CONSTRAINT `chk_lottery_strategy_routing_edge_nodes_positive`
        CHECK (`from_node_id` > 0 AND `to_node_id` > 0),
    CONSTRAINT `chk_lottery_strategy_routing_edge_branch_default`
        CHECK (
            (`branch_code` = 'premium_override' AND `is_default` = 0)
            OR
            (`branch_code` = 'baseline_default' AND `is_default` = 1)
        )
) ENGINE = InnoDB
  DEFAULT CHARACTER SET = utf8mb4
  COLLATE = utf8mb4_0900_bin;
