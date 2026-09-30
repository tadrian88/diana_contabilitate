-- Vânzări V1 (D-124, D-125, D-126). An invoice is received (INCOMING) or issued
-- by the client (OUTGOING). supplier_* keeps its factual UBL meaning; an issued
-- invoice also stores its customer. The identifier is kept raw (it may be a
-- CNP) and masked only at the presentation boundary. Existing rows are INCOMING
-- through the constant default, without rewriting the table.
ALTER TABLE invoices
  ADD COLUMN direction text NOT NULL DEFAULT 'INCOMING',
  ADD COLUMN customer_name text,
  ADD COLUMN customer_identifier text,
  ADD COLUMN normalized_customer_identifier text,
  ADD COLUMN customer_identifier_kind text,
  ADD CONSTRAINT invoices_direction_check CHECK (direction IN ('INCOMING', 'OUTGOING')),
  ADD CONSTRAINT invoices_customer_name_check CHECK (customer_name IS NULL OR length(btrim(customer_name)) > 0),
  ADD CONSTRAINT invoices_customer_identifier_kind_check CHECK (customer_identifier_kind IS NULL OR customer_identifier_kind IN ('CUI', 'CNP', 'OTHER')),
  ADD CONSTRAINT invoices_outgoing_customer_check CHECK (
    direction = 'INCOMING'
    OR (customer_name IS NOT NULL AND customer_identifier IS NOT NULL
        AND normalized_customer_identifier IS NOT NULL AND length(btrim(normalized_customer_identifier)) > 0
        AND customer_identifier_kind IS NOT NULL)
  );

CREATE INDEX invoices_client_direction_idx ON invoices (client_id, direction);
CREATE INDEX invoices_client_customer_idx ON invoices (client_id, normalized_customer_identifier) WHERE direction = 'OUTGOING';

-- The accounting client is the buyer of a purchase contract (BUYER, every
-- existing row) or the supplier of a sale contract such as a lease where it is
-- the Locator (SUPPLIER). For SUPPLIER contracts the counterparty is the buyer.
ALTER TABLE contracts
  ADD COLUMN client_role text NOT NULL DEFAULT 'BUYER',
  ADD COLUMN buyer_name text,
  ADD COLUMN buyer_cui text,
  ADD COLUMN normalized_buyer_cui text,
  ADD CONSTRAINT contracts_client_role_check CHECK (client_role IN ('BUYER', 'SUPPLIER')),
  ADD CONSTRAINT contracts_supplier_role_buyer_check CHECK (
    client_role = 'BUYER'
    OR (length(btrim(COALESCE(buyer_name, ''))) > 0 AND length(btrim(COALESCE(buyer_cui, ''))) > 0
        AND length(btrim(COALESCE(normalized_buyer_cui, ''))) > 0)
  );

CREATE INDEX contracts_client_buyer_idx ON contracts (client_id, normalized_buyer_cui) WHERE client_role = 'SUPPLIER';

-- The sent-invoice list (ANAF filter T) has its own last-success cursor, so the
-- first sync after this change imports the usual initial window of issued invoices.
ALTER TABLE spv_connections
  ADD COLUMN last_successful_sent_sync_at timestamptz;
