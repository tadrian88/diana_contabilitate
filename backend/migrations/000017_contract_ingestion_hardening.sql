-- Contract ingestion hardening and typed service terms. Existing rows retain
-- their legacy value fields and become explicitly fixed-term active contracts.
UPDATE "clients" SET "normalized_identifier" = regexp_replace(upper("cui"), '^(RO)?[[:space:]]*', '')
WHERE upper("country") = 'RO' AND regexp_replace(upper("cui"), '[[:space:]]', '', 'g') ~ '^(RO)?[1-9][0-9]{1,9}$';
UPDATE "invoices" SET "normalized_supplier_cui" = regexp_replace(regexp_replace(upper("supplier_cui"), '[[:space:]]', '', 'g'), '^RO', '')
WHERE "supplier_cui" IS NOT NULL AND regexp_replace(upper("supplier_cui"), '[[:space:]]', '', 'g') ~ '^(RO)?[1-9][0-9]{1,9}$';
UPDATE "contracts" SET "normalized_supplier_cui" = regexp_replace(regexp_replace(upper("supplier_cui"), '[[:space:]]', '', 'g'), '^RO', '')
WHERE regexp_replace(upper("supplier_cui"), '[[:space:]]', '', 'g') ~ '^(RO)?[1-9][0-9]{1,9}$';

ALTER TABLE "contracts" ALTER COLUMN "effective_to" DROP NOT NULL;
ALTER TABLE "contracts" ADD COLUMN "period_type" varchar NOT NULL DEFAULT 'FIXED_TERM';
ALTER TABLE "contracts" ADD COLUMN "lifecycle_state" varchar NOT NULL DEFAULT 'ACTIVE';
ALTER TABLE "contracts" ADD COLUMN "has_legacy_total_value" boolean NOT NULL DEFAULT true;
ALTER TABLE "contracts" ADD CONSTRAINT "contracts_period_type_check" CHECK ("period_type" IN ('FIXED_TERM','INDEFINITE_TERM'));
ALTER TABLE "contracts" ADD CONSTRAINT "contracts_lifecycle_state_check" CHECK ("lifecycle_state" IN ('ACTIVE','ARCHIVED'));
ALTER TABLE "contracts" ADD CONSTRAINT "contracts_period_shape_check" CHECK (("period_type" = 'INDEFINITE_TERM' AND "effective_to" IS NULL) OR ("period_type" = 'FIXED_TERM' AND "effective_to" IS NOT NULL));

ALTER TABLE "invoice_contract_associations" ALTER COLUMN "effective_to" DROP NOT NULL;
ALTER TABLE "invoice_contract_associations" ADD COLUMN "period_type" varchar NOT NULL DEFAULT 'FIXED_TERM';
ALTER TABLE "invoice_contract_associations" ADD CONSTRAINT "invoice_contract_associations_period_type_check" CHECK ("period_type" IN ('FIXED_TERM','INDEFINITE_TERM'));

ALTER TABLE "contract_source_documents" DROP CONSTRAINT "contract_source_documents_lifecycle_check";
ALTER TABLE "contract_source_documents" ADD CONSTRAINT "contract_source_documents_lifecycle_check" CHECK ("lifecycle_state" IN ('ACTIVE','SUPERSEDED','DISCARDED'));

CREATE TABLE "contract_service_terms" (
  "id" varchar NOT NULL, "contract_id" varchar NOT NULL, "position" bigint NOT NULL CHECK ("position" > 0),
  "service_description" varchar NOT NULL, "pricing_model" varchar NOT NULL,
  "unit_price" numeric(20,4) NULL, "currency" varchar NOT NULL,
  "unit" varchar NULL, "quantity_source" varchar NOT NULL DEFAULT 'UNKNOWN',
  "quantity_value" numeric(20,4) NULL, "quantity_driver" varchar NULL,
  "billing_frequency" varchar NOT NULL DEFAULT 'UNKNOWN', "source_evidence" jsonb NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "contract_service_terms_contracts_service_terms" FOREIGN KEY ("contract_id") REFERENCES "contracts" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION,
  CONSTRAINT "contract_service_terms_pricing_model_check" CHECK ("pricing_model" IN ('FIXED_FEE','UNIT_RATE','FIXED_TOTAL')),
  CONSTRAINT "contract_service_terms_quantity_source_check" CHECK ("quantity_source" IN ('CONTRACT_FIXED_QUANTITY','INVOICE_REPORTED_QUANTITY','USER_CONFIRMED_QUANTITY','EXTERNAL_SOURCE_FUTURE','UNKNOWN')),
  CONSTRAINT "contract_service_terms_billing_frequency_check" CHECK ("billing_frequency" IN ('MONTHLY','QUARTERLY','ANNUAL','PER_OCCURRENCE','UNKNOWN'))
);
CREATE UNIQUE INDEX "contractserviceterm_contract_id_position" ON "contract_service_terms" ("contract_id", "position");
