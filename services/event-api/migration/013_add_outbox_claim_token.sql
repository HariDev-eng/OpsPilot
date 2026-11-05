
ALTER TABLE outbox
ADD COLUMN claim_token UUID;

-- Find active claims efficiently.
CREATE INDEX idx_outbox_claim_token
ON outbox (claim_token)
WHERE claim_token IS NOT NULL;
