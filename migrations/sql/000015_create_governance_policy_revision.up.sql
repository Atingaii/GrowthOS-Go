CREATE TABLE `governance_policy_revision` (
    `policy_id` VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `policy_revision` BIGINT UNSIGNED NOT NULL,
    `schema_version` SMALLINT UNSIGNED NOT NULL,
    `content_digest` BINARY(32) NOT NULL,
    `publication_reference` VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    `published_at` DATETIME(6) NOT NULL,

    PRIMARY KEY (`policy_id`, `policy_revision`),
    UNIQUE KEY `uq_governance_policy_revision_evidence`
        (`policy_id`, `policy_revision`, `content_digest`),
    UNIQUE KEY `uq_governance_policy_revision_publication`
        (`publication_reference`),

    CONSTRAINT `chk_governance_policy_revision_identity`
        CHECK (
            CHAR_LENGTH(`policy_id`) BETWEEN 1 AND 128
            AND `policy_id` REGEXP '^[a-z0-9]([a-z0-9._:-]{0,126}[a-z0-9])?$'
            AND `policy_revision` > 0
        ),
    CONSTRAINT `chk_governance_policy_revision_content`
        CHECK (
            `schema_version` = 1
            AND `content_digest` <>
                X'0000000000000000000000000000000000000000000000000000000000000000'
        ),
    CONSTRAINT `chk_governance_policy_revision_publication`
        CHECK (
            CHAR_LENGTH(`publication_reference`) BETWEEN 1 AND 128
            AND `publication_reference` REGEXP '^[a-z0-9]([a-z0-9._:-]{0,126}[a-z0-9])?$'
        )
) ENGINE = InnoDB
  DEFAULT CHARACTER SET = ascii
  COLLATE = ascii_bin;
