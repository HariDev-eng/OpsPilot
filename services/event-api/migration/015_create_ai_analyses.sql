
CREATE TABLE ai_analyses (
    id BIGSERIAL PRIMARY KEY,

    incident_id BIGINT NOT NULL
        REFERENCES incidents(id) ON DELETE CASCADE,

    status VARCHAR(20) NOT NULL DEFAULT 'PENDING'
        CHECK (status IN (
            'PENDING',
            'RUNNING',
            'COMPLETED',
            'FAILED'
        )),

    provider VARCHAR(50) NOT NULL,
    model VARCHAR(100) NOT NULL,

    analysis JSONB,

    error_message TEXT,

    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_ai_analyses_incident_created
ON ai_analyses (incident_id, created_at DESC);

CREATE INDEX idx_ai_analyses_status
ON ai_analyses (status);
