CREATE TABLE `governance_policy_activation` (
    `policy_slot` VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `state_version` BIGINT UNSIGNED NOT NULL,
    `activation_reference` VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `policy_id` VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `policy_revision` BIGINT UNSIGNED NOT NULL,
    `policy_content_digest` BINARY(32) NOT NULL,
    `activated_at` DATETIME(6) NOT NULL,

    PRIMARY KEY (`policy_slot`, `state_version`),
    UNIQUE KEY `uq_governance_policy_activation_reference`
        (`activation_reference`),
    UNIQUE KEY `uq_governance_policy_activation_evidence`
        (
            `policy_slot`,
            `state_version`,
            `activation_reference`,
            `policy_id`,
            `policy_revision`,
            `policy_content_digest`
        ),
    KEY `idx_governance_policy_activation_revision`
        (`policy_id`, `policy_revision`, `policy_content_digest`),

    CONSTRAINT `fk_governance_policy_activation_revision`
        FOREIGN KEY (`policy_id`, `policy_revision`, `policy_content_digest`)
        REFERENCES `governance_policy_revision` (
            `policy_id`,
            `policy_revision`,
            `content_digest`
        )
        ON DELETE RESTRICT
        ON UPDATE RESTRICT,
    CONSTRAINT `chk_governance_policy_activation_shape`
        CHECK (
            `policy_slot` = 'workforce_http'
            AND `state_version` > 0
            AND CHAR_LENGTH(`activation_reference`) BETWEEN 1 AND 128
            AND `activation_reference` REGEXP '^[a-z0-9]([a-z0-9._:-]{0,126}[a-z0-9])?$'
            AND `policy_content_digest` <>
                X'0000000000000000000000000000000000000000000000000000000000000000'
        )
) ENGINE = InnoDB
  DEFAULT CHARACTER SET = ascii
  COLLATE = ascii_bin;
