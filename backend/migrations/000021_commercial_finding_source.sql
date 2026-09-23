-- Preserve the invoice-side provenance of values compared with contract rules.
ALTER TABLE invoice_commercial_findings ADD COLUMN actual_source text;
