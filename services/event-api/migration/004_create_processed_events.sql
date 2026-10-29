CREATE TABLE processed_events (
    event_id TEXT NOT NULL,
    consumer_name TEXT NOT NULL,

    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (event_id, consumer_name)
);
