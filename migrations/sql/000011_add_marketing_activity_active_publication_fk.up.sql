ALTER TABLE `marketing_activity`
    ADD CONSTRAINT `fk_marketing_activity_active_publication`
        FOREIGN KEY (`activity_id`, `active_version`)
        REFERENCES `marketing_activity_publication` (`activity_id`, `activity_version`)
        ON DELETE RESTRICT
        ON UPDATE RESTRICT;
