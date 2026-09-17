-- Make terminal duplicate status inseparable from its canonical reference and
-- supplementary verification signals.
ALTER TABLE invoices DROP CONSTRAINT invoices_duplicate_verification_complete;

ALTER TABLE invoices ADD CONSTRAINT invoices_duplicate_verification_complete
CHECK (
    (
        pipeline_status <> 'DUPLICATE'
        AND duplicate_of_invoice_id IS NULL
        AND duplicate_amount_matches IS NULL
        AND duplicate_currency_matches IS NULL
    )
    OR
    (
        pipeline_status = 'DUPLICATE'
        AND duplicate_of_invoice_id IS NOT NULL
        AND duplicate_amount_matches IS NOT NULL
        AND duplicate_currency_matches IS NOT NULL
    )
);
