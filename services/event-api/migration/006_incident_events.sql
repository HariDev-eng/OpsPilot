CREATE TABLE incident_events (
    incident_id BIGINT NOT NULL,
    event_id TEXT NOT NULL,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (incident_id, event_id),

    CONSTRAINT fk_incident_events_incident
        FOREIGN KEY (incident_id)
        REFERENCES incidents(id)
        ON DELETE CASCADE,

    CONSTRAINT fk_incident_events_event
        FOREIGN KEY (event_id)
        REFERENCES events(event_id)
        ON DELETE CASCADE
);

CREATE UNIQUE INDEX idx_incident_events_event_id
ON incident_events(event_id);

CREATE INDEX idx_incident_events_incident_id
ON incident_events(incident_id);
