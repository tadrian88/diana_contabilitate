-- Versioned legislation corpus and immutable accounting-analysis audit trail.
-- No normative text is seeded: official, licensed source material must be
-- supplied explicitly and its digest retained.
CREATE TABLE legislation_sources (
 id text PRIMARY KEY,
 kind text NOT NULL CHECK(kind IN ('LAW','ORDER','METHODOLOGICAL_NORMS','OTHER')),
 title text NOT NULL CHECK(length(btrim(title))>0),
 issuer text NOT NULL CHECK(length(btrim(issuer))>0),
 jurisdiction text NOT NULL DEFAULT 'RO',
 official_url text NOT NULL CHECK(length(btrim(official_url))>0),
 created_at timestamptz NOT NULL
);

CREATE TABLE legislation_versions (
 id text PRIMARY KEY,
 source_id text NOT NULL REFERENCES legislation_sources(id),
 label text NOT NULL CHECK(length(btrim(label))>0),
 effective_from date NOT NULL,
 effective_to date,
 content_hash text NOT NULL CHECK(content_hash ~ '^[0-9a-f]{64}$'),
 ingested_by text NOT NULL CHECK(length(btrim(ingested_by))>0),
 ingested_at timestamptz NOT NULL,
 UNIQUE(source_id,label),
 CHECK(effective_to IS NULL OR effective_to >= effective_from)
);

CREATE TABLE legislation_fragments (
 id text PRIMARY KEY,
 version_id text NOT NULL REFERENCES legislation_versions(id),
 citation_key text NOT NULL CHECK(length(btrim(citation_key))>0),
 heading text NOT NULL DEFAULT '',
 ordinal integer NOT NULL CHECK(ordinal>0),
 content text NOT NULL CHECK(length(btrim(content))>0),
 content_hash text NOT NULL CHECK(content_hash ~ '^[0-9a-f]{64}$'),
 search_vector tsvector GENERATED ALWAYS AS
   (to_tsvector('simple', coalesce(citation_key,'') || ' ' || coalesce(heading,'') || ' ' || content)) STORED,
 UNIQUE(version_id,citation_key),
 UNIQUE(version_id,ordinal)
);
CREATE INDEX legislation_fragments_search_idx ON legislation_fragments USING gin(search_vector);
CREATE INDEX legislation_versions_period_idx ON legislation_versions(effective_from,effective_to);

CREATE FUNCTION reject_legislation_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'Legislation corpus records are immutable; ingest a new version'; END; $$;
CREATE TRIGGER legislation_source_immutable BEFORE UPDATE OR DELETE ON legislation_sources FOR EACH ROW EXECUTE FUNCTION reject_legislation_mutation();
CREATE TRIGGER legislation_version_immutable BEFORE UPDATE OR DELETE ON legislation_versions FOR EACH ROW EXECUTE FUNCTION reject_legislation_mutation();
CREATE TRIGGER legislation_fragment_immutable BEFORE UPDATE OR DELETE ON legislation_fragments FOR EACH ROW EXECUTE FUNCTION reject_legislation_mutation();

CREATE TABLE accounting_analysis_runs (
 id text PRIMARY KEY,
 client_id text NOT NULL REFERENCES clients(id),
 invoice_id text NOT NULL,
 invoice_revision bigint NOT NULL CHECK(invoice_revision>0),
 schema_version text NOT NULL,
 prompt_version text NOT NULL,
 provider text NOT NULL,
 model text NOT NULL,
 status text NOT NULL CHECK(status IN ('RUNNING','PROPOSED','VALIDATION_FAILED','PROVIDER_FAILED')),
 input_snapshot jsonb NOT NULL CHECK(jsonb_typeof(input_snapshot)='object'),
 retrieved_fragment_ids jsonb NOT NULL CHECK(jsonb_typeof(retrieved_fragment_ids)='array'),
 approved_knowledge_ids jsonb NOT NULL CHECK(jsonb_typeof(approved_knowledge_ids)='array'),
 raw_structured_response jsonb,
 validation_results jsonb NOT NULL CHECK(jsonb_typeof(validation_results)='array'),
 input_tokens bigint,
 output_tokens bigint,
 started_at timestamptz NOT NULL,
 completed_at timestamptz,
 command_key text NOT NULL UNIQUE,
 FOREIGN KEY(invoice_id,client_id) REFERENCES invoices(id,client_id),
 UNIQUE(id,client_id,invoice_id),
 CHECK((status='RUNNING' AND completed_at IS NULL AND raw_structured_response IS NULL)
    OR (status<>'RUNNING' AND completed_at IS NOT NULL))
);
CREATE INDEX accounting_analysis_invoice_idx ON accounting_analysis_runs(client_id,invoice_id,started_at DESC);

CREATE TABLE accounting_analysis_reviews (
 id text PRIMARY KEY,
 analysis_id text NOT NULL REFERENCES accounting_analysis_runs(id),
 client_id text NOT NULL,
 invoice_id text NOT NULL,
 action text NOT NULL CHECK(action IN ('APPROVE','EDIT','REJECT')),
 ai_proposal jsonb NOT NULL CHECK(jsonb_typeof(ai_proposal)='object'),
 final_decision jsonb,
 reason text NOT NULL DEFAULT '',
 actor_id text,
 actor_display text NOT NULL CHECK(length(btrim(actor_display))>0),
 created_at timestamptz NOT NULL,
 command_key text NOT NULL UNIQUE,
 FOREIGN KEY(analysis_id,client_id,invoice_id) REFERENCES accounting_analysis_runs(id,client_id,invoice_id),
 CHECK((action='REJECT' AND final_decision IS NULL) OR (action IN ('APPROVE','EDIT') AND jsonb_typeof(final_decision)='object')),
	UNIQUE(id,client_id)
);

ALTER TABLE client_accounting_profiles ADD CONSTRAINT client_accounting_profiles_id_client_key UNIQUE(id,client_id);
CREATE TABLE approved_accounting_knowledge (
 id text PRIMARY KEY,
 client_id text NOT NULL REFERENCES clients(id),
 normalized_supplier_id text NOT NULL CHECK(length(btrim(normalized_supplier_id))>0),
 semantic_kind text NOT NULL CHECK(semantic_kind IN ('SELLER_ITEM_ID','STANDARD_ITEM_ID','NORMALIZED_DESCRIPTION','REVIEWED_CATEGORY')),
 semantic_value text NOT NULL CHECK(length(btrim(semantic_value))>0),
 profile_id text NOT NULL,
 legislation_version_ids jsonb NOT NULL CHECK(jsonb_typeof(legislation_version_ids)='array'),
 decision jsonb NOT NULL CHECK(jsonb_typeof(decision)='object'),
 source_review_id text NOT NULL,
 effective_from date NOT NULL,
 effective_to date,
 status text NOT NULL CHECK(status IN ('ACTIVE','STALE','INACTIVE')),
 created_at timestamptz NOT NULL,
 CHECK(effective_to IS NULL OR effective_to >= effective_from),
 FOREIGN KEY(profile_id,client_id) REFERENCES client_accounting_profiles(id,client_id),
 FOREIGN KEY(source_review_id,client_id) REFERENCES accounting_analysis_reviews(id,client_id),
 UNIQUE(client_id,normalized_supplier_id,semantic_kind,semantic_value,profile_id,effective_from)
);
CREATE INDEX approved_accounting_knowledge_lookup_idx
 ON approved_accounting_knowledge(client_id,normalized_supplier_id,status,semantic_kind,semantic_value);

CREATE FUNCTION reject_accounting_analysis_history_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'Accounting analysis history is immutable'; END; $$;
CREATE TRIGGER accounting_analysis_review_immutable BEFORE UPDATE OR DELETE ON accounting_analysis_reviews FOR EACH ROW EXECUTE FUNCTION reject_accounting_analysis_history_mutation();

-- Runs may transition from RUNNING exactly once; completed evidence is frozen.
CREATE FUNCTION protect_accounting_analysis_run() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
	IF TG_OP='DELETE' THEN RAISE EXCEPTION 'Accounting analysis history is immutable'; END IF;
 IF OLD.status <> 'RUNNING' THEN RAISE EXCEPTION 'Completed accounting analysis is immutable'; END IF;
 IF NEW.status='RUNNING' OR NEW.completed_at IS NULL THEN RAISE EXCEPTION 'Accounting analysis run must complete in one transition'; END IF;
 IF NEW.id<>OLD.id OR NEW.client_id<>OLD.client_id OR NEW.invoice_id<>OLD.invoice_id OR NEW.invoice_revision<>OLD.invoice_revision
    OR NEW.input_snapshot<>OLD.input_snapshot OR NEW.retrieved_fragment_ids<>OLD.retrieved_fragment_ids
    OR NEW.approved_knowledge_ids<>OLD.approved_knowledge_ids OR NEW.provider<>OLD.provider OR NEW.model<>OLD.model
    OR NEW.schema_version<>OLD.schema_version OR NEW.prompt_version<>OLD.prompt_version OR NEW.command_key<>OLD.command_key
 THEN RAISE EXCEPTION 'Accounting analysis input/provenance is immutable'; END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER accounting_analysis_run_guard BEFORE UPDATE OR DELETE ON accounting_analysis_runs FOR EACH ROW EXECUTE FUNCTION protect_accounting_analysis_run();
