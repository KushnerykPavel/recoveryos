-- +goose Up
CREATE TABLE session_events (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    session_id uuid NOT NULL REFERENCES sessions(id),
    source text NOT NULL CHECK (source = 'system'),
    kind text NOT NULL,
    details text NOT NULL,
    occurred_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX session_events_session_id_id_idx ON session_events(session_id, id);

-- +goose StatementBegin
CREATE FUNCTION reject_session_event_mutation() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'session event history is immutable';
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER session_events_immutable
BEFORE UPDATE OR DELETE ON session_events
FOR EACH ROW EXECUTE FUNCTION reject_session_event_mutation();

-- +goose Down
DROP TABLE session_events;
DROP FUNCTION reject_session_event_mutation();
