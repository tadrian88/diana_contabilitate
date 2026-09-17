CREATE TABLE "contract_source_documents" (
  "id" varchar NOT NULL, "client_id" varchar NOT NULL,
  "original_filename" varchar NOT NULL, "mime_type" varchar NOT NULL,
  "size_bytes" bigint NOT NULL CHECK ("size_bytes" > 0), "sha256" varchar NOT NULL,
  "raw_document" bytea NOT NULL, "uploaded_by_id" varchar NULL,
  "uploaded_by_display" varchar NULL, "uploaded_at" timestamptz NOT NULL,
  "status" varchar NOT NULL DEFAULT 'UPLOADED', "lifecycle_state" varchar NOT NULL DEFAULT 'ACTIVE',
  "latest_extraction_id" varchar NULL, "confirmed_contract_id" varchar NULL,
  "confirmed_by_id" varchar NULL, "confirmed_by_display" varchar NULL,
  "confirmed_at" timestamptz NULL, "confirmed_values" jsonb NULL,
  "confirmation_key" varchar NULL, "confirmation_fingerprint" varchar NULL,
  "revision" bigint NOT NULL DEFAULT 1 CHECK ("revision" > 0),
  "updated_at" timestamptz NOT NULL, PRIMARY KEY ("id"),
  CONSTRAINT "contract_source_documents_clients_contract_source_documents" FOREIGN KEY ("client_id") REFERENCES "clients" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION,
  CONSTRAINT "contract_source_documents_status_check" CHECK ("status" IN ('UPLOADED','EXTRACTING','READY_FOR_REVIEW','EXTRACTION_FAILED','CONFIRMED')),
  CONSTRAINT "contract_source_documents_lifecycle_check" CHECK ("lifecycle_state" IN ('ACTIVE','SUPERSEDED'))
);
CREATE UNIQUE INDEX "contract_source_documents_client_id_sha256" ON "contract_source_documents" ("client_id", "sha256");
CREATE INDEX "contract_source_documents_client_id_status" ON "contract_source_documents" ("client_id", "status");
CREATE INDEX "contract_source_documents_latest_extraction_id" ON "contract_source_documents" ("latest_extraction_id");
CREATE UNIQUE INDEX "contract_source_documents_confirmed_contract_id" ON "contract_source_documents" ("confirmed_contract_id");

CREATE TABLE "contract_extraction_attempts" (
  "id" varchar NOT NULL, "document_id" varchar NOT NULL, "provider" varchar NOT NULL,
  "model" varchar NOT NULL, "schema_version" varchar NOT NULL, "prompt_version" varchar NOT NULL,
  "status" varchar NOT NULL, "proposal" jsonb NULL, "safe_error_category" varchar NULL,
  "input_tokens" bigint NULL, "output_tokens" bigint NULL, "started_at" timestamptz NOT NULL,
  "completed_at" timestamptz NULL, PRIMARY KEY ("id"),
  CONSTRAINT "contract_extraction_attempts_contract_source_documents_extraction_attempts" FOREIGN KEY ("document_id") REFERENCES "contract_source_documents" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION,
  CONSTRAINT "contract_extraction_attempts_status_check" CHECK ("status" IN ('STARTED','SUCCEEDED','FAILED'))
);
CREATE INDEX "contract_extraction_attempts_document_id_started_at" ON "contract_extraction_attempts" ("document_id", "started_at");
CREATE INDEX "contract_extraction_attempts_identity" ON "contract_extraction_attempts" ("document_id", "model", "schema_version", "prompt_version", "status");

ALTER TABLE "contracts" ADD COLUMN "source_document_id" varchar NULL;
ALTER TABLE "contracts" ADD COLUMN "extraction_attempt_id" varchar NULL;
CREATE UNIQUE INDEX "contracts_source_document_id" ON "contracts" ("source_document_id");
CREATE UNIQUE INDEX "contracts_extraction_attempt_id" ON "contracts" ("extraction_attempt_id");
