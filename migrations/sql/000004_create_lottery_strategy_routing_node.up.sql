CREATE TABLE `lottery_strategy_routing_node` (
    `graph_id` BIGINT UNSIGNED NOT NULL,
    `revision` VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `node_id` BIGINT UNSIGNED NOT NULL,
    `node_kind` VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `rule_code` VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NULL,
    `strategy_id` BIGINT UNSIGNED NULL,
    `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),

    PRIMARY KEY (`graph_id`, `revision`, `node_id`),
    KEY `idx_lottery_strategy_routing_node_strategy` (`strategy_id`),

    CONSTRAINT `fk_lottery_strategy_routing_node_graph`
        FOREIGN KEY (`graph_id`, `revision`)
        REFERENCES `lottery_strategy_routing_graph` (`graph_id`, `revision`)
        ON DELETE RESTRICT
        ON UPDATE RESTRICT,
    CONSTRAINT `fk_lottery_strategy_routing_node_strategy`
        FOREIGN KEY (`strategy_id`)
        REFERENCES `lottery_strategy` (`strategy_id`)
        ON DELETE RESTRICT
        ON UPDATE RESTRICT,
    CONSTRAINT `chk_lottery_strategy_routing_node_id_positive`
        CHECK (`node_id` > 0),
    CONSTRAINT `chk_lottery_strategy_routing_node_shape`
        CHECK (
            (
                `node_kind` = 'decision'
                AND `rule_code` IS NOT NULL
                AND `rule_code` = 'lottery.membership_tier.route_strategy'
                AND `strategy_id` IS NULL
            )
            OR
            (
                `node_kind` = 'strategy_target'
                AND `rule_code` IS NULL
                AND `strategy_id` IS NOT NULL
                AND `strategy_id` > 0
            )
        )
) ENGINE = InnoDB
  DEFAULT CHARACTER SET = utf8mb4
  COLLATE = utf8mb4_0900_bin;
