-- The invoice accounting snapshot mirrors the CURRENT classification run.
-- History stays immutable in classification_runs.snapshot; the invoice copy
-- may change only in the same update that moves the invoice to a new run, and
-- only to exactly that run's snapshot. Explicit reanalysis therefore uses the
-- current client configuration instead of the first (possibly profile-less)
-- snapshot.
CREATE OR REPLACE FUNCTION protect_accounting_source() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.source_facts IS DISTINCT FROM OLD.source_facts THEN RAISE EXCEPTION 'Source facts are immutable; explicit versioned reparse required'; END IF;
 IF TG_TABLE_NAME='invoices' THEN
  IF NEW.model_version IS DISTINCT FROM OLD.model_version THEN RAISE EXCEPTION 'Domain model version is immutable'; END IF;
  IF OLD.accounting_snapshot IS NOT NULL AND NEW.accounting_snapshot IS DISTINCT FROM OLD.accounting_snapshot THEN
   IF NEW.current_classification_run_id IS NOT DISTINCT FROM OLD.current_classification_run_id THEN
    RAISE EXCEPTION 'Classification snapshot is immutable for the current classification run';
   END IF;
   IF NEW.accounting_snapshot IS DISTINCT FROM (SELECT r.snapshot FROM classification_runs r WHERE r.id=NEW.current_classification_run_id AND r.invoice_id=NEW.id) THEN
    RAISE EXCEPTION 'Invoice classification snapshot must equal the current classification run snapshot';
   END IF;
  END IF;
 END IF;
 RETURN NEW;
END; $$;
