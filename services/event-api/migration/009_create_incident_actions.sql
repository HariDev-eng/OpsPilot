
CREATE TABLE incident_actions (
    id BIGSERIAL PRIMARY KEY,

    incident_id BIGINT NOT NULL,

    description TEXT NOT NULL,

    action_type TEXT NOT NULL,

    status TEXT NOT NULL,

    requires_approval BOOLEAN NOT NULL,

    idempotency_key TEXT NOT NULL UNIQUE,

    approved_by TEXT,
    rejected_by TEXT,

    approved_at TIMESTAMPTZ,
    rejected_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_incident_actions_incident
        FOREIGN KEY (incident_id)
        REFERENCES incidents(id)
        ON DELETE CASCADE,

    CONSTRAINT chk_incident_action_type
        CHECK (
            action_type IN (
                'INVESTIGATION',
                'MITIGATION',
                'DESTRUCTIVE'
            )
        ),

    CONSTRAINT chk_incident_action_status
        CHECK (
            status IN (
                'PROPOSED',
                'PENDING_APPROVAL',
                'APPROVED',
                'REJECTED',
                'EXECUTING',
                'EXECUTED',
                'FAILED'
            )
        ),

    CONSTRAINT chk_action_approval_requirement
        CHECK (
            (action_type = 'INVESTIGATION' AND requires_approval = FALSE)
            OR
            (action_type IN ('MITIGATION', 'DESTRUCTIVE')
                AND requires_approval = TRUE)
        )
);

CREATE INDEX idx_incident_actions_incident_id
ON incident_actions(incident_id);

CREATE INDEX idx_incident_actions_status
ON incident_actions(status);


CREATE TABLE action_audit_logs (
    id BIGSERIAL PRIMARY KEY,

    action_id BIGINT NOT NULL,

    actor_id TEXT NOT NULL,

    action TEXT NOT NULL,

    details JSONB NOT NULL DEFAULT '{}'::jsonb,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_action_audit_logs_action
        FOREIGN KEY (action_id)
        REFERENCES incident_actions(id)
        ON DELETE CASCADE
);

CREATE INDEX idx_action_audit_logs_action_id
ON action_audit_logs(action_id);
