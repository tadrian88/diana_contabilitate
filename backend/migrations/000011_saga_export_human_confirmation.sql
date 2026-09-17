-- EXPORTED in file mode is an immutable human confirmation for the exact
-- generated artifact. Download remains an audit event, not confirmation.
ALTER TABLE saga_export_attempts
    ADD COLUMN confirmed_at timestamptz,
    ADD COLUMN confirmed_by_id text,
    ADD COLUMN confirmed_by_display text,
    ADD COLUMN confirmation_type text
        CHECK (confirmation_type IN ('HUMAN', 'LOCAL_AGENT', 'SAGA_API')),
    ADD COLUMN confirmation_note text,
    ADD CONSTRAINT saga_export_attempts_confirmation_shape CHECK (
        (confirmed_at IS NULL AND confirmed_by_id IS NULL AND confirmed_by_display IS NULL
            AND confirmation_type IS NULL AND confirmation_note IS NULL)
        OR
        (status = 'GENERATED' AND confirmed_at IS NOT NULL
            AND confirmation_type IS NOT NULL AND confirmed_by_id IS NOT NULL)
    );

CREATE INDEX saga_export_attempts_confirmed_at_idx
    ON saga_export_attempts (confirmed_at)
    WHERE confirmed_at IS NOT NULL;
