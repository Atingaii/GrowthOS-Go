CREATE TABLE `governance_authorization_audit_match` (
    `evaluation_reference` VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `policy_id` VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `policy_revision` BIGINT UNSIGNED NOT NULL,
    `binding_id` VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `principal_kind` VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `principal_id` VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `role_id` VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `binding_effect` VARCHAR(8) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `scope_kind` VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `resource_kind` VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `resource_type` VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `action` VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,

    PRIMARY KEY (`evaluation_reference`, `binding_id`),
    KEY `idx_governance_audit_match_binding`
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
    KEY `idx_governance_audit_match_permission`
        (
            `policy_id`,
            `policy_revision`,
            `role_id`,
            `resource_kind`,
            `resource_type`,
            `action`
        ),

    CONSTRAINT `fk_governance_audit_match_audit`
        FOREIGN KEY (
            `evaluation_reference`,
            `policy_id`,
            `policy_revision`,
            `principal_kind`,
            `principal_id`,
            `resource_kind`,
            `resource_type`,
            `action`
        )
        REFERENCES `governance_authorization_audit` (
            `evaluation_reference`,
            `policy_id`,
            `policy_revision`,
            `principal_kind`,
            `principal_id`,
            `resource_kind`,
            `resource_type`,
            `action`
        )
        ON DELETE RESTRICT
        ON UPDATE RESTRICT,
    CONSTRAINT `fk_governance_audit_match_binding`
        FOREIGN KEY (
            `policy_id`,
            `policy_revision`,
            `binding_id`,
            `principal_kind`,
            `principal_id`,
            `role_id`,
            `binding_effect`,
            `scope_kind`
        )
        REFERENCES `governance_policy_role_binding` (
            `policy_id`,
            `policy_revision`,
            `binding_id`,
            `principal_kind`,
            `principal_id`,
            `role_id`,
            `binding_effect`,
            `scope_kind`
        )
        ON DELETE RESTRICT
        ON UPDATE RESTRICT,
    CONSTRAINT `fk_governance_audit_match_permission`
        FOREIGN KEY (
            `policy_id`,
            `policy_revision`,
            `role_id`,
            `resource_kind`,
            `resource_type`,
            `action`
        )
        REFERENCES `governance_policy_role_permission` (
            `policy_id`,
            `policy_revision`,
            `role_id`,
            `resource_kind`,
            `resource_type`,
            `action`
        )
        ON DELETE RESTRICT
        ON UPDATE RESTRICT
) ENGINE = InnoDB
  DEFAULT CHARACTER SET = ascii
  COLLATE = ascii_bin;
