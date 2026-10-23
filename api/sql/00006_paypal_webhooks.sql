-- +goose Up
ALTER TABLE payout_events DROP CONSTRAINT payout_events_source_check;
ALTER TABLE payout_events ADD CONSTRAINT payout_events_source_check
    CHECK (source IN ('paypal', 'paypal_webhook', 'injected', 'system'));
ALTER TABLE payout_events ADD COLUMN paypal_event_id text UNIQUE;
CREATE UNIQUE INDEX payout_records_live_item_id_uidx ON payout_records(item_id)
    WHERE origin = 'live' AND item_id IS NOT NULL;

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM payout_events WHERE source = 'paypal_webhook') THEN
        RAISE EXCEPTION 'cannot roll back PayPal webhooks while their immutable events exist';
    END IF;
END;
$$;
-- +goose StatementEnd
ALTER TABLE payout_events DROP COLUMN paypal_event_id;
DROP INDEX payout_records_live_item_id_uidx;
ALTER TABLE payout_events DROP CONSTRAINT payout_events_source_check;
ALTER TABLE payout_events ADD CONSTRAINT payout_events_source_check
    CHECK (source IN ('paypal', 'injected', 'system'));
