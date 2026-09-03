CREATE TABLE `governance_policy_role_binding` (
    `policy_id` VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `policy_revision` BIGINT UNSIGNED NOT NULL,
    `binding_id` VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `principal_kind` VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `principal_id` VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `role_id` VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `scope_kind` VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `scope_tenant_id` VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NULL,
    `scope_resource_type` VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NULL,
    `scope_resource_id` VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NULL,
    `scope_tenant_key` VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin
        GENERATED ALWAYS AS (COALESCE(`scope_tenant_id`, '')) STORED,
    `scope_resource_type_key` VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin
        GENERATED ALWAYS AS (COALESCE(`scope_resource_type`, '')) STORED,
    `scope_resource_id_key` VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin
        GENERATED ALWAYS AS (COALESCE(`scope_resource_id`, '')) STORED,
    `binding_effect` VARCHAR(8) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),

    PRIMARY KEY (`policy_id`, `policy_revision`, `binding_id`),
    UNIQUE KEY `uq_governance_role_binding_evidence`
        (
            `policy_id`,
            `policy_revision`,
            `binding_id`,
            `principal_kind`,
            `principal_id`,
            `role_id`,
            `binding_effect`,
            `scope_kind`
        ),
    UNIQUE KEY `uq_governance_role_binding_semantic`
        (
            `policy_id`,
            `policy_revision`,
            `principal_kind`,
            `principal_id`,
            `role_id`,
            `scope_kind`,
            `scope_tenant_key`,
            `scope_resource_type_key`,
            `scope_resource_id_key`,
            `binding_effect`
        ),
    KEY `idx_governance_role_binding_principal`
        (`principal_kind`, `principal_id`, `policy_id`, `policy_revision`),

    CONSTRAINT `fk_governance_role_binding_role`
        FOREIGN KEY (`policy_id`, `policy_revision`, `role_id`)
        REFERENCES `governance_policy_role` (`policy_id`, `policy_revision`, `role_id`)
        ON DELETE RESTRICT
        ON UPDATE RESTRICT,
    CONSTRAINT `chk_governance_role_binding_identity`
        CHECK (
            CHAR_LENGTH(`binding_id`) BETWEEN 1 AND 128
            AND `binding_id` REGEXP '^[a-z0-9]([a-z0-9._:-]{0,126}[a-z0-9])?$'
            AND `principal_kind` IN ('human', 'service', 'agent')
            AND CHAR_LENGTH(`principal_id`) BETWEEN 1 AND 128
            AND `principal_id` REGEXP '^[a-z0-9]([a-z0-9._:-]{0,126}[a-z0-9])?$'
            AND `binding_effect` IN ('allow', 'deny')
        ),
    CONSTRAINT `chk_governance_role_binding_scope_shape`
        CHECK (
            (
                `scope_kind` = 'system'
                AND `scope_tenant_id` IS NULL
                AND `scope_resource_type` IS NULL
                AND `scope_resource_id` IS NULL
            )
            OR
            (
                `scope_kind` IN ('tenant', 'owned')
                AND `scope_tenant_id` IS NOT NULL
                AND CHAR_LENGTH(`scope_tenant_id`) BETWEEN 1 AND 128
                AND `scope_tenant_id` REGEXP '^[a-z0-9]([a-z0-9._:-]{0,126}[a-z0-9])?$'
                AND `scope_resource_type` IS NULL
                AND `scope_resource_id` IS NULL
            )
            OR
            (
                `scope_kind` = 'resource'
                AND (
                    `scope_tenant_id` IS NULL
                    OR (
                        CHAR_LENGTH(`scope_tenant_id`) BETWEEN 1 AND 128
                        AND `scope_tenant_id` REGEXP '^[a-z0-9]([a-z0-9._:-]{0,126}[a-z0-9])?$'
                    )
                )
                AND `scope_resource_type` IN (
                    'marketing.activity',
                    'lottery.strategy',
                    'lottery.routing_graph',
                    'governance.policy',
                    'governance.audit'
                )
                AND `scope_resource_id` IS NOT NULL
                AND CHAR_LENGTH(`scope_resource_id`) BETWEEN 1 AND 128
                AND `scope_resource_id` REGEXP '^[a-z0-9]([a-z0-9._:-]{0,126}[a-z0-9])?$'
            )
        )
) ENGINE = InnoDB
  DEFAULT CHARACTER SET = ascii
  COLLATE = ascii_bin;
