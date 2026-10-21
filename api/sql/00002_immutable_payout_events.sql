-- +goose Up
-- +goose StatementBegin
CREATE FUNCTION reject_payout_event_mutation() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'payout event history is immutable';
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER payout_events_immutable
BEFORE UPDATE OR DELETE ON payout_events
FOR EACH ROW EXECUTE FUNCTION reject_payout_event_mutation();

-- +goose Down
DROP TRIGGER payout_events_immutable ON payout_events;
DROP FUNCTION reject_payout_event_mutation();
