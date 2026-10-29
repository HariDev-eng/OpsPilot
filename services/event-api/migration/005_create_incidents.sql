CREATE TABLE incidents (
    id BIGSERIAL PRIMARY KEY,

    event_id TEXT NOT NULL UNIQUE,

    service TEXT NOT NULL,

    event_type TEXT NOT NULL,

    severity TEXT NOT NULL,

    status TEXT NOT NULL DEFAULT 'OPEN',

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
