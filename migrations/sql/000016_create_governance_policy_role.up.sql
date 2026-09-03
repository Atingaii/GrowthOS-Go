CREATE TABLE `governance_policy_role` (
    `policy_id` VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `policy_revision` BIGINT UNSIGNED NOT NULL,
    `role_id` VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),

    PRIMARY KEY (`policy_id`, `policy_revision`, `role_id`),

    CONSTRAINT `fk_governance_policy_role_revision`
        FOREIGN KEY (`policy_id`, `policy_revision`)
        REFERENCES `governance_policy_revision` (`policy_id`, `policy_revision`)
        ON DELETE RESTRICT
        ON UPDATE RESTRICT,
    CONSTRAINT `chk_governance_policy_role_id`
        CHECK (
            `role_id` IN (
                'platform_administrator',
                'marketing_operator',
                'lottery_designer',
                'security_auditor',
                'growth_member'
            )
        )
) ENGINE = InnoDB
  DEFAULT CHARACTER SET = ascii
  COLLATE = ascii_bin;
