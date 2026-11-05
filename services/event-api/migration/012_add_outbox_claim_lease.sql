
ALTER TABLE outbox
ADD COLUMN claimed_at TIMESTAMPTZ;

-- Existing PUBLISHING records receive a fresh lease.
UPDATE outbox
SET claimed_at = NOW()
WHERE status = 'PUBLISHING';

-- Efficiently locate expired claims.
CREATE INDEX idx_outbox_stale_publishing
ON outbox (claimed_at)
WHERE status = 'PUBLISHING'
  AND claimed_at IS NOT NULL;
