CREATE TABLE `governance_authorization_audit` (
    `evaluation_reference` VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `correlation_reference` VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `policy_slot` VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `activation_state_version` BIGINT UNSIGNED NOT NULL,
    `activation_reference` VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `policy_id` VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `policy_revision` BIGINT UNSIGNED NOT NULL,
    `policy_content_digest` BINARY(32) NOT NULL,
    `principal_kind` VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `principal_id` VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `authentication_kind` VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `authentication_reference` VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `authentication_epoch` BIGINT UNSIGNED NOT NULL,
    `resource_kind` VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `resource_type` VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `resource_id` VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NULL,
    `resource_tenant_id` VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NULL,
    `resource_owner_kind` VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NULL,
    `resource_owner_id` VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NULL,
    `action` VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `decision_outcome` VARCHAR(8) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `decision_reason` VARCHAR(40) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `match_count` SMALLINT UNSIGNED NOT NULL,
    `evaluated_at` DATETIME(6) NOT NULL,
    `recorded_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),

    PRIMARY KEY (`evaluation_reference`),
    UNIQUE KEY `uq_governance_authorization_audit_evidence`
        (
            `evaluation_reference`,
            `policy_id`,
            `policy_revision`,
            `principal_kind`,
            `principal_id`,
            `resource_kind`,
            `resource_type`,
            `action`
        ),
    KEY `idx_governance_authorization_audit_principal`
        (`principal_kind`, `principal_id`, `evaluated_at`, `evaluation_reference`),
    KEY `idx_governance_authorization_audit_correlation`
        (`correlation_reference`, `evaluated_at`, `evaluation_reference`),
    KEY `idx_governance_authorization_audit_policy_revision`
        (`policy_id`, `policy_revision`, `evaluated_at`, `evaluation_reference`),
    KEY `idx_governance_authorization_audit_activation`
        (
            `policy_slot`,
            `activation_state_version`,
            `activation_reference`,
            `policy_id`,
            `policy_revision`,
            `policy_content_digest`
        ),

    CONSTRAINT `fk_governance_authorization_audit_activation`
        FOREIGN KEY (
            `policy_slot`,
            `activation_state_version`,
            `activation_reference`,
            `policy_id`,
            `policy_revision`,
            `policy_content_digest`
        )
        REFERENCES `governance_policy_activation` (
            `policy_slot`,
            `state_version`,
            `activation_reference`,
            `policy_id`,
            `policy_revision`,
            `policy_content_digest`
        )
        ON DELETE RESTRICT
        ON UPDATE RESTRICT,
    CONSTRAINT `chk_governance_authorization_audit_refs`
        CHECK (
            CHAR_LENGTH(`evaluation_reference`) BETWEEN 1 AND 128
            AND `evaluation_reference` REGEXP '^[a-z0-9]([a-z0-9._:-]{0,126}[a-z0-9])?$'
            AND CHAR_LENGTH(`correlation_reference`) BETWEEN 1 AND 128
            AND `correlation_reference` REGEXP '^[a-z0-9]([a-z0-9._:-]{0,126}[a-z0-9])?$'
        ),
    CONSTRAINT `chk_governance_authorization_audit_activation`
        CHECK (
            `policy_slot` = 'workforce_http'
            AND `activation_state_version` > 0
            AND CHAR_LENGTH(`activation_reference`) BETWEEN 1 AND 128
            AND `activation_reference` REGEXP '^[a-z0-9]([a-z0-9._:-]{0,126}[a-z0-9])?$'
            AND `policy_content_digest` <>
                X'0000000000000000000000000000000000000000000000000000000000000000'
        ),
    CONSTRAINT `chk_governance_authorization_audit_principal`
        CHECK (
            `principal_kind` IN ('human', 'service', 'agent')
            AND CHAR_LENGTH(`principal_id`) BETWEEN 1 AND 128
            AND `principal_id` REGEXP '^[a-z0-9]([a-z0-9._:-]{0,126}[a-z0-9])?$'
        ),
    CONSTRAINT `chk_governance_authorization_audit_authentication`
        CHECK (
            (
                (`principal_kind` = 'human' AND `authentication_kind` = 'workforce_session')
                OR
                (`principal_kind` = 'service' AND `authentication_kind` = 'service_credential')
                OR
                (`principal_kind` = 'agent' AND `authentication_kind` = 'agent_credential')
            )
            AND CHAR_LENGTH(`authentication_reference`) BETWEEN 1 AND 128
            AND `authentication_reference` REGEXP '^[a-z0-9]([a-z0-9._:-]{0,126}[a-z0-9])?$'
            AND `authentication_epoch` > 0
        ),
    CONSTRAINT `chk_governance_authorization_audit_resource_shape`
        CHECK (
            (
                `resource_kind` = 'collection'
                AND `resource_id` IS NULL
                AND `resource_owner_kind` IS NULL
                AND `resource_owner_id` IS NULL
            )
            OR
            (
                `resource_kind` = 'object'
                AND `resource_id` IS NOT NULL
                AND CHAR_LENGTH(`resource_id`) BETWEEN 1 AND 128
                AND `resource_id` REGEXP '^[a-z0-9]([a-z0-9._:-]{0,126}[a-z0-9])?$'
                AND (
                    (
                        `resource_owner_kind` IS NULL
                        AND `resource_owner_id` IS NULL
                    )
                    OR
                    (
                        `resource_owner_kind` IN ('human', 'service', 'agent')
                        AND `resource_owner_id` IS NOT NULL
                        AND CHAR_LENGTH(`resource_owner_id`) BETWEEN 1 AND 128
                        AND `resource_owner_id` REGEXP '^[a-z0-9]([a-z0-9._:-]{0,126}[a-z0-9])?$'
                    )
                )
            )
        ),
    CONSTRAINT `chk_governance_authorization_audit_tenant`
        CHECK (
            `resource_tenant_id` IS NULL
            OR (
                CHAR_LENGTH(`resource_tenant_id`) BETWEEN 1 AND 128
                AND `resource_tenant_id` REGEXP '^[a-z0-9]([a-z0-9._:-]{0,126}[a-z0-9])?$'
            )
        ),
    CONSTRAINT `chk_governance_authorization_audit_capability`
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
    CONSTRAINT `chk_governance_authorization_audit_decision`
        CHECK (
            (
                `decision_outcome` = 'allow'
                AND `decision_reason` = 'explicit_allow'
                AND `match_count` BETWEEN 1 AND 1024
            )
            OR
            (
                `decision_outcome` = 'deny'
                AND `decision_reason` = 'explicit_deny'
                AND `match_count` BETWEEN 1 AND 1024
            )
            OR
            (
                `decision_outcome` = 'deny'
                AND `decision_reason` = 'explicit_deny_overrode_allow'
                AND `match_count` BETWEEN 2 AND 1024
            )
            OR
            (
                `decision_outcome` = 'deny'
                AND `decision_reason` IN (
                    'no_binding',
                    'no_permission',
                    'scope_mismatch'
                )
                AND `match_count` = 0
            )
        )
) ENGINE = InnoDB
  DEFAULT CHARACTER SET = ascii
  COLLATE = ascii_bin;
