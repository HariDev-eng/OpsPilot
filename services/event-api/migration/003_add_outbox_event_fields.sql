ALTER TABLE outbox
ADD COLUMN service TEXT,
ADD COLUMN timestamp TIMESTAMPTZ;

UPDATE outbox o
SET
    service = e.service,
    timestamp = e.timestamp
FROM events e
WHERE o.event_id = e.event_id;

ALTER TABLE outbox
ALTER COLUMN service SET NOT NULL,
ALTER COLUMN timestamp SET NOT NULL;
