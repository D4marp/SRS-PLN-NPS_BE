ALTER TABLE feedbacks
    ADD COLUMN IF NOT EXISTS complaint_items TEXT NULL AFTER reason,
    ADD COLUMN IF NOT EXISTS complaint_other TEXT NULL AFTER complaint_items;