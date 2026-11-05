
CREATE TABLE integrations (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    service_name TEXT NOT NULL,
    key_prefix TEXT NOT NULL UNIQUE,
    key_hash TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ
);

ALTER TABLE events
ADD COLUMN integration_id UUID REFERENCES integrations(id);

ALTER TABLE outbox
ADD COLUMN integration_id UUID REFERENCES integrations(id);

CREATE INDEX idx_events_integration_id
ON events(integration_id);

CREATE INDEX idx_outbox_integration_id
ON outbox(integration_id);
