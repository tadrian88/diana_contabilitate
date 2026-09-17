-- Module 7 adds only operational delivery metadata. Business payload and
-- idempotency identity remain unchanged.
ALTER TABLE outbox_entries
    DROP CONSTRAINT outbox_entries_status_check,
    ADD CONSTRAINT outbox_entries_status_check
        CHECK (status IN ('PENDING', 'CLAIMED', 'DISPATCHED', 'FAILED', 'PROCESSED')),
    ADD COLUMN claim_owner text,
    ADD COLUMN claimed_at timestamptz,
    ADD COLUMN dispatched_at timestamptz,
    ADD COLUMN last_error text,
    ADD COLUMN correlation_id text;

CREATE INDEX outbox_entries_status_claimed_at_idx
    ON outbox_entries (status, claimed_at);
