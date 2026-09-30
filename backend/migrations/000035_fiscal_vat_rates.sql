-- Legal Romanian VAT rates with validity periods. A contract clause that
-- follows "cota legală în vigoare" resolves applicable_vat_rate from this table
-- for the invoice issue date unless the dossier records its own value.
CREATE TABLE fiscal_vat_rates (
  id text PRIMARY KEY,
  country text NOT NULL,
  category text NOT NULL,
  rate numeric(7,4) NOT NULL,
  valid_from date NOT NULL,
  valid_to date,
  legal_source text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT fiscal_vat_rates_rate_check CHECK (rate >= 0 AND rate < 100),
  CONSTRAINT fiscal_vat_rates_period_check CHECK (valid_to IS NULL OR valid_to >= valid_from),
  CONSTRAINT fiscal_vat_rates_source_check CHECK (length(legal_source) > 0),
  CONSTRAINT fiscal_vat_rates_start_unique UNIQUE (country, category, valid_from)
);

INSERT INTO fiscal_vat_rates(id,country,category,rate,valid_from,valid_to,legal_source) VALUES
  ('ro-standard-2017-01-01','RO','STANDARD',19,'2017-01-01','2025-07-31','Legea nr. 227/2015 privind Codul fiscal, art. 291 alin. (1) lit. a)'),
  ('ro-standard-2025-08-01','RO','STANDARD',21,'2025-08-01',NULL,'Legea nr. 227/2015 privind Codul fiscal, art. 291 alin. (1) lit. a), modificat prin Legea nr. 141/2025');
