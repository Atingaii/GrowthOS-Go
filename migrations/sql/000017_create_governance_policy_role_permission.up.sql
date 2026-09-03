CREATE TABLE `governance_policy_role_permission` (
    `policy_id` VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `policy_revision` BIGINT UNSIGNED NOT NULL,
    `role_id` VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `resource_kind` VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `resource_type` VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `action` VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),

    PRIMARY KEY (
        `policy_id`,
        `policy_revision`,
        `role_id`,
        `resource_kind`,
        `resource_type`,
        `action`
    ),

    CONSTRAINT `fk_governance_role_permission_role`
        FOREIGN KEY (`policy_id`, `policy_revision`, `role_id`)
        REFERENCES `governance_policy_role` (`policy_id`, `policy_revision`, `role_id`)
        ON DELETE RESTRICT
        ON UPDATE RESTRICT,
    CONSTRAINT `chk_governance_role_permission_capability`
        CHECK (
            (
                `resource_type` = 'marketing.activity'
                AND (
                    (`resource_kind` = 'collection' AND `action` IN ('create', 'read'))
                    OR
                    (
                        `resource_kind` = 'object'
                        AND `action` IN ('read', 'publish', 'rollback', 'retire')
                    )
                )
            )
            OR
            (
                `resource_type` = 'lottery.strategy'
                AND (
                    (`resource_kind` = 'collection' AND `action` IN ('create', 'read'))
                    OR
                    (`resource_kind` = 'object' AND `action` IN ('read', 'simulate'))
                )
            )
            OR
            (
                `resource_type` = 'lottery.routing_graph'
                AND (
                    (`resource_kind` = 'collection' AND `action` IN ('create', 'read'))
                    OR
                    (`resource_kind` = 'object' AND `action` = 'read')
                )
            )
            OR
            (
                `resource_type` = 'governance.policy'
                AND (
                    (`resource_kind` = 'collection' AND `action` = 'read')
                    OR
                    (`resource_kind` = 'object' AND `action` IN ('read', 'change'))
                )
            )
            OR
            (
                `resource_type` = 'governance.audit'
                AND `resource_kind` = 'collection'
                AND `action` = 'read'
            )
        ),
    CONSTRAINT `chk_governance_role_permission_ceiling`
        CHECK (
            `role_id` = 'platform_administrator'
            OR
            (
                `role_id` = 'marketing_operator'
                AND (
                    (
                        `resource_type` = 'marketing.activity'
                        AND (
                            (`resource_kind` = 'collection' AND `action` IN ('create', 'read'))
                            OR
                            (
                                `resource_kind` = 'object'
                                AND `action` IN ('read', 'publish', 'rollback', 'retire')
                            )
                        )
                    )
                    OR
                    (
                        `resource_type` IN ('lottery.strategy', 'lottery.routing_graph')
                        AND `action` = 'read'
                        AND `resource_kind` IN ('collection', 'object')
                    )
                )
            )
            OR
            (
                `role_id` = 'lottery_designer'
                AND (
                    (
                        `resource_type` = 'marketing.activity'
                        AND `resource_kind` IN ('collection', 'object')
                        AND `action` = 'read'
                    )
                    OR
                    (
                        `resource_type` = 'lottery.strategy'
                        AND (
                            (`resource_kind` = 'collection' AND `action` IN ('create', 'read'))
                            OR
                            (`resource_kind` = 'object' AND `action` IN ('read', 'simulate'))
                        )
                    )
                    OR
                    (
                        `resource_type` = 'lottery.routing_graph'
                        AND (
                            (`resource_kind` = 'collection' AND `action` IN ('create', 'read'))
                            OR
                            (`resource_kind` = 'object' AND `action` = 'read')
                        )
                    )
                )
            )
            OR
            (
                `role_id` = 'security_auditor'
                AND (
                    (
                        `resource_type` IN (
                            'marketing.activity',
                            'lottery.strategy',
                            'lottery.routing_graph'
                        )
                        AND `resource_kind` IN ('collection', 'object')
                        AND `action` = 'read'
                    )
                    OR
                    (
                        `resource_type` = 'governance.policy'
                        AND `resource_kind` IN ('collection', 'object')
                        AND `action` = 'read'
                    )
                    OR
                    (
                        `resource_type` = 'governance.audit'
                        AND `resource_kind` = 'collection'
                        AND `action` = 'read'
                    )
                )
            )
        )
) ENGINE = InnoDB
  DEFAULT CHARACTER SET = ascii
  COLLATE = ascii_bin;
