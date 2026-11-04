
-- Add server-owned operation metadata.
ALTER TABLE incident_actions
ADD COLUMN operation_key TEXT;

-- Structured parameters passed to the registered executor.
ALTER TABLE incident_actions
ADD COLUMN parameters JSONB NOT NULL DEFAULT '{}'::jsonb;

-- Execution lifecycle metadata.
ALTER TABLE incident_actions
ADD COLUMN execution_started_at TIMESTAMPTZ;

ALTER TABLE incident_actions
ADD COLUMN executed_at TIMESTAMPTZ;

ALTER TABLE incident_actions
ADD COLUMN execution_error TEXT;

-- Track the number of execution attempts.
ALTER TABLE incident_actions
ADD COLUMN execution_attempts INT NOT NULL DEFAULT 0;

-- An operation key must not be blank when provided.
ALTER TABLE incident_actions
ADD CONSTRAINT chk_incident_action_operation_key
CHECK (
    operation_key IS NULL
    OR LENGTH(BTRIM(operation_key)) > 0
);

-- Execution attempts cannot be negative.
ALTER TABLE incident_actions
ADD CONSTRAINT chk_incident_action_execution_attempts
CHECK (execution_attempts >= 0);

-- Useful for finding actions requiring execution processing.
CREATE INDEX idx_incident_actions_execution_status
ON incident_actions(status)
WHERE status IN ('APPROVED', 'EXECUTING', 'FAILED');
