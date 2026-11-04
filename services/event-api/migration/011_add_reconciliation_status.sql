
BEGIN;

-- Replace the existing action status constraint.
ALTER TABLE incident_actions
DROP CONSTRAINT chk_incident_action_status;

ALTER TABLE incident_actions
ADD CONSTRAINT chk_incident_action_status
CHECK (
    status IN (
        'PROPOSED',
        'PENDING_APPROVAL',
        'APPROVED',
        'REJECTED',
        'EXECUTING',
        'EXECUTED',
        'FAILED',
        'RECONCILIATION_REQUIRED'
    )
);

-- Include uncertain executions in the execution-status index.
DROP INDEX IF EXISTS idx_incident_actions_execution_status;

CREATE INDEX idx_incident_actions_execution_status
ON incident_actions(status)
WHERE status IN (
    'APPROVED',
    'EXECUTING',
    'FAILED',
    'RECONCILIATION_REQUIRED'
);

COMMIT;
