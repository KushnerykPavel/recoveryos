-- +goose Up
CREATE TABLE investigations (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    session_id uuid NOT NULL REFERENCES sessions(id),
    incident_id uuid NOT NULL REFERENCES incidents(id),
    result jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX investigations_session_incident_id_idx ON investigations(session_id, incident_id, id DESC);

-- +goose Down
DROP TABLE investigations;
