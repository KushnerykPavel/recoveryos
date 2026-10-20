-- +goose Up
CREATE TABLE demo_sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    token_hash bytea NOT NULL UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL
);

CREATE TABLE daily_payout_counts (
    day_utc date PRIMARY KEY,
    count integer NOT NULL CHECK (count >= 0)
);

CREATE TABLE payout_records (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id uuid NOT NULL UNIQUE REFERENCES demo_sessions(id),
    creation_key text NOT NULL UNIQUE,
    sender_item_id text NOT NULL UNIQUE,
    batch_id text UNIQUE,
    item_id text UNIQUE,
    provider_status text,
    status text NOT NULL DEFAULT 'creating'
        CHECK (status IN ('creating', 'provider_unknown', 'pending', 'stale', 'corrected')),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE payout_events (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    session_id uuid NOT NULL REFERENCES demo_sessions(id),
    payout_id uuid NOT NULL REFERENCES payout_records(id),
    source text NOT NULL CHECK (source IN ('paypal', 'injected', 'system')),
    kind text NOT NULL,
    details text NOT NULL,
    occurred_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX payout_events_session_id_id_idx ON payout_events(session_id, id);

CREATE TABLE incidents (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id uuid NOT NULL REFERENCES demo_sessions(id),
    payout_id uuid NOT NULL UNIQUE REFERENCES payout_records(id),
    status text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'resolved')),
    created_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE incidents;
DROP TABLE payout_events;
DROP TABLE payout_records;
DROP TABLE daily_payout_counts;
DROP TABLE demo_sessions;
