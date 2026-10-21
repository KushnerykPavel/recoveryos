-- +goose Up
ALTER TABLE payout_records ADD COLUMN origin text NOT NULL DEFAULT 'live'
    CHECK (origin IN ('live', 'precreated'));
ALTER TABLE payout_records DROP CONSTRAINT payout_records_creation_key_key;
ALTER TABLE payout_records DROP CONSTRAINT payout_records_sender_item_id_key;
ALTER TABLE payout_records DROP CONSTRAINT payout_records_batch_id_key;
ALTER TABLE payout_records DROP CONSTRAINT payout_records_item_id_key;

-- +goose Down
-- Existing backup sessions reuse provider IDs, so restoring uniqueness would
-- require deleting their records. Refuse that destructive rollback explicitly.
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM payout_records WHERE origin = 'precreated') THEN
        RAISE EXCEPTION 'cannot roll back backup payout migration while backup sessions exist';
    END IF;
END;
$$;
-- +goose StatementEnd
ALTER TABLE payout_records ADD CONSTRAINT payout_records_creation_key_key UNIQUE (creation_key);
ALTER TABLE payout_records ADD CONSTRAINT payout_records_sender_item_id_key UNIQUE (sender_item_id);
ALTER TABLE payout_records ADD CONSTRAINT payout_records_batch_id_key UNIQUE (batch_id);
ALTER TABLE payout_records ADD CONSTRAINT payout_records_item_id_key UNIQUE (item_id);
ALTER TABLE payout_records DROP COLUMN origin;
