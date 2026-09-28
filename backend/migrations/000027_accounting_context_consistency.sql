-- Versioned classification contexts and explicit immutable profile succession.
ALTER TABLE client_accounting_profiles
    ADD COLUMN supersedes_profile_id text,
    ADD CONSTRAINT client_accounting_profiles_supersedes_fk
        FOREIGN KEY (supersedes_profile_id, client_id)
        REFERENCES client_accounting_profiles(id, client_id);
CREATE UNIQUE INDEX client_accounting_profiles_one_successor_idx
    ON client_accounting_profiles(supersedes_profile_id)
    WHERE supersedes_profile_id IS NOT NULL;

CREATE TABLE classification_runs (
    id text PRIMARY KEY,
    client_id text NOT NULL REFERENCES clients(id),
    invoice_id text NOT NULL,
    invoice_revision bigint NOT NULL CHECK(invoice_revision > 0),
    snapshot jsonb NOT NULL CHECK(jsonb_typeof(snapshot) = 'object'),
    profile_id text,
    profile_version integer,
    context_fingerprint text NOT NULL CHECK(length(context_fingerprint) > 0),
    policy_version text NOT NULL CHECK(length(policy_version) > 0),
    status text NOT NULL CHECK(status IN ('COMPLETED','BLOCKED')),
    blocker_code text,
    supersedes_run_id text REFERENCES classification_runs(id),
    command_key text NOT NULL UNIQUE,
    actor_id text,
    actor_display text NOT NULL CHECK(length(btrim(actor_display)) > 0),
    created_at timestamptz NOT NULL,
    FOREIGN KEY(invoice_id, client_id) REFERENCES invoices(id, client_id) ON DELETE CASCADE,
    FOREIGN KEY(profile_id, client_id) REFERENCES client_accounting_profiles(id, client_id),
    UNIQUE(id, client_id, invoice_id),
    CHECK((profile_id IS NULL) = (profile_version IS NULL)),
    CHECK((status='BLOCKED') = (blocker_code IS NOT NULL))
);
CREATE INDEX classification_runs_invoice_idx
    ON classification_runs(client_id, invoice_id, created_at DESC);

CREATE TABLE classification_reanalysis_commands (
    command_key text PRIMARY KEY,
    client_id text NOT NULL REFERENCES clients(id),
    invoice_id text NOT NULL,
    requested_revision bigint NOT NULL CHECK(requested_revision > 0),
    prepared_revision bigint NOT NULL CHECK(prepared_revision > requested_revision),
    created_at timestamptz NOT NULL,
    FOREIGN KEY(invoice_id,client_id) REFERENCES invoices(id,client_id)
);

ALTER TABLE line_classifications ADD COLUMN classification_run_id text;
ALTER TABLE invoices ADD COLUMN current_classification_run_id text;
ALTER TABLE accounting_analysis_runs ADD COLUMN classification_run_id text;

INSERT INTO classification_runs(
    id,client_id,invoice_id,invoice_revision,snapshot,profile_id,profile_version,
    context_fingerprint,policy_version,status,command_key,actor_display,created_at
)
SELECT
    'classification-legacy-' || i.id,
    i.client_id,
    i.id,
    i.revision,
    COALESCE(i.accounting_snapshot,'{}'::jsonb),
    (SELECT cap.id FROM client_accounting_profiles cap
      WHERE cap.client_id=i.client_id AND cap.id=i.accounting_snapshot->'profile'->>'id' LIMIT 1),
    (SELECT cap.version FROM client_accounting_profiles cap
      WHERE cap.client_id=i.client_id AND cap.id=i.accounting_snapshot->'profile'->>'id' LIMIT 1),
    md5(COALESCE(i.accounting_snapshot,'{}'::jsonb)::text),
    COALESCE((SELECT lc.policy_version FROM line_classifications lc WHERE lc.invoice_id=i.id ORDER BY lc.created_at LIMIT 1),'LEGACY_UNKNOWN'),
    'COMPLETED',
    'legacy:' || i.id,
    'Migrare Capitolul A',
    COALESCE((SELECT min(lc.created_at) FROM line_classifications lc WHERE lc.invoice_id=i.id),i.updated_at)
FROM invoices i
WHERE EXISTS(SELECT 1 FROM line_classifications lc WHERE lc.invoice_id=i.id);

UPDATE line_classifications lc
SET classification_run_id='classification-legacy-' || lc.invoice_id;
UPDATE invoices i SET current_classification_run_id='classification-legacy-' || i.id
WHERE EXISTS(SELECT 1 FROM classification_runs r WHERE r.id='classification-legacy-' || i.id);

ALTER TABLE line_classifications ALTER COLUMN classification_run_id SET NOT NULL;
ALTER TABLE line_classifications
    ADD CONSTRAINT line_classifications_run_fk FOREIGN KEY(classification_run_id) REFERENCES classification_runs(id);
ALTER TABLE invoices
    ADD CONSTRAINT invoices_current_classification_run_fk
        FOREIGN KEY(current_classification_run_id,client_id,id)
        REFERENCES classification_runs(id,client_id,invoice_id);
ALTER TABLE accounting_analysis_runs
    ADD CONSTRAINT accounting_analysis_classification_run_fk
        FOREIGN KEY(classification_run_id,client_id,invoice_id)
        REFERENCES classification_runs(id,client_id,invoice_id);

ALTER TABLE accounting_analysis_runs DISABLE TRIGGER accounting_analysis_run_guard;
UPDATE accounting_analysis_runs a
SET classification_run_id=i.current_classification_run_id
FROM invoices i, classification_runs r
WHERE i.id=a.invoice_id AND i.client_id=a.client_id
  AND r.id=i.current_classification_run_id
  AND a.input_snapshot->'Profile'->>'id'=r.profile_id
  AND a.input_snapshot->'Profile'->>'version'=r.profile_version::text;
ALTER TABLE accounting_analysis_runs ENABLE TRIGGER accounting_analysis_run_guard;

CREATE OR REPLACE FUNCTION protect_accounting_analysis_run() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'Accounting analysis history is immutable'; END IF;
 IF OLD.status <> 'RUNNING' THEN RAISE EXCEPTION 'Completed accounting analysis is immutable'; END IF;
 IF NEW.status='RUNNING' OR NEW.completed_at IS NULL THEN RAISE EXCEPTION 'Accounting analysis run must complete in one transition'; END IF;
 IF NEW.id<>OLD.id OR NEW.client_id<>OLD.client_id OR NEW.invoice_id<>OLD.invoice_id OR NEW.invoice_revision<>OLD.invoice_revision
    OR NEW.input_snapshot<>OLD.input_snapshot OR NEW.retrieved_fragment_ids<>OLD.retrieved_fragment_ids
    OR NEW.approved_knowledge_ids<>OLD.approved_knowledge_ids OR NEW.provider<>OLD.provider OR NEW.model<>OLD.model
    OR NEW.schema_version<>OLD.schema_version OR NEW.prompt_version<>OLD.prompt_version OR NEW.command_key<>OLD.command_key
    OR NEW.classification_run_id IS DISTINCT FROM OLD.classification_run_id
 THEN RAISE EXCEPTION 'Accounting analysis input/provenance is immutable'; END IF;
 RETURN NEW;
END; $$;

ALTER TABLE line_classifications DROP CONSTRAINT line_classifications_line_dimension_model_key;
ALTER TABLE line_classifications
    ADD CONSTRAINT line_classifications_run_line_dimension_model_key
    UNIQUE(classification_run_id,invoice_line_id,dimension,model_version);

CREATE FUNCTION protect_classification_run() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'Classification run history is immutable'; END; $$;
CREATE TRIGGER classification_run_immutable
    BEFORE UPDATE ON classification_runs
    FOR EACH ROW EXECUTE FUNCTION protect_classification_run();
