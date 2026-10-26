CREATE TABLE outbox (
    id BIGSERIAL PRIMARY KEY,

    event_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    topic TEXT NOT NULL,

    payload JSONB NOT NULL,

    status TEXT NOT NULL DEFAULT 'PENDING',

    attempts INT NOT NULL DEFAULT 0,

    last_error TEXT,

    next_retry_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    published_at TIMESTAMPTZ

);

CREATE UNIQUE INDEX idx_outbox_event_id
ON outbox(event_id);
