-- AI usage and cost tracking (D-121, D-122). One append-only row per provider
-- call, attributed to its AI run and therefore to the run's client. Prices are
-- versioned and append-only; the cost of a call is computed with the price
-- valid when the call happened and never recomputed.

CREATE TABLE llm_model_prices (
 id text PRIMARY KEY,
 provider text NOT NULL CHECK(provider ~ '^[A-Z][A-Z0-9_]{1,31}$'),
 model text NOT NULL CHECK(length(model) > 0 AND model = lower(btrim(model))),
 pricing_tier text NOT NULL DEFAULT 'STANDARD' CHECK(pricing_tier ~ '^[A-Z][A-Z0-9_]{1,31}$'),
 effective_from timestamptz NOT NULL,
 input_usd_per_mtok numeric(14,6) NOT NULL CHECK(input_usd_per_mtok >= 0),
 cached_input_usd_per_mtok numeric(14,6) CHECK(cached_input_usd_per_mtok >= 0),
 -- Output price includes thinking tokens, as the provider bills them.
 output_usd_per_mtok numeric(14,6) NOT NULL CHECK(output_usd_per_mtok >= 0),
 source_url text NOT NULL CHECK(source_url LIKE 'https://%'),
 source_note text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(provider, model, pricing_tier, effective_from)
);

CREATE FUNCTION reject_llm_price_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'LLM prices are append-only; add a row with a later effective_from'; END; $$;
CREATE TRIGGER llm_model_prices_immutable BEFORE UPDATE OR DELETE ON llm_model_prices FOR EACH ROW EXECUTE FUNCTION reject_llm_price_mutation();

-- Operation and outcome use a format check, not a closed list, so a new
-- operation can never make an insert fail and lose a billed call.
CREATE TABLE llm_usage_events (
 id text PRIMARY KEY,
 occurred_at timestamptz NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 client_id text NOT NULL REFERENCES clients(id),
 run_kind text NOT NULL CHECK(run_kind IN ('ACCOUNTING_ANALYSIS','CONTRACT_EXTRACTION')),
 accounting_analysis_run_id text REFERENCES accounting_analysis_runs(id),
 contract_extraction_attempt_id varchar REFERENCES contract_extraction_attempts(id),
 run_id text GENERATED ALWAYS AS (COALESCE(accounting_analysis_run_id, contract_extraction_attempt_id)) STORED,
 operation text NOT NULL CHECK(operation ~ '^[A-Z][A-Z0-9_]{1,63}$'),
 outcome text NOT NULL CHECK(outcome ~ '^[A-Z][A-Z0-9_]{1,63}$'),
 source text NOT NULL CHECK(source IN ('LIVE','BACKFILL')),
 provider text NOT NULL CHECK(provider ~ '^[A-Z][A-Z0-9_]{1,31}$'),
 model text NOT NULL CHECK(length(model) > 0 AND model = lower(btrim(model))),
 http_status integer CHECK(http_status BETWEEN 100 AND 599),
 provider_status text,
 latency_ms bigint CHECK(latency_ms >= 0),
 usage_reported boolean NOT NULL,
 input_tokens bigint NOT NULL DEFAULT 0 CHECK(input_tokens >= 0),
 output_tokens bigint NOT NULL DEFAULT 0 CHECK(output_tokens >= 0),
 thought_tokens bigint NOT NULL DEFAULT 0 CHECK(thought_tokens >= 0),
 cached_tokens bigint NOT NULL DEFAULT 0 CHECK(cached_tokens >= 0),
 tool_use_tokens bigint NOT NULL DEFAULT 0 CHECK(tool_use_tokens >= 0),
 total_tokens bigint NOT NULL DEFAULT 0 CHECK(total_tokens >= 0),
 usage_detail jsonb CHECK(usage_detail IS NULL OR jsonb_typeof(usage_detail) = 'object'),
 price_id text REFERENCES llm_model_prices(id),
 input_cost_usd numeric(20,10),
 cached_input_cost_usd numeric(20,10),
 output_cost_usd numeric(20,10),
 cost_usd numeric(20,10),
 cost_status text NOT NULL CHECK(cost_status IN ('PRICED','NO_PRICE','NO_USAGE')),
 CHECK((run_kind = 'ACCOUNTING_ANALYSIS' AND accounting_analysis_run_id IS NOT NULL AND contract_extraction_attempt_id IS NULL)
    OR (run_kind = 'CONTRACT_EXTRACTION' AND contract_extraction_attempt_id IS NOT NULL AND accounting_analysis_run_id IS NULL)),
 CHECK((cost_status = 'PRICED') = (price_id IS NOT NULL AND cost_usd IS NOT NULL)),
 CHECK((cost_status = 'NO_USAGE') = (NOT usage_reported))
);
CREATE INDEX llm_usage_events_client_time_idx ON llm_usage_events(client_id, occurred_at);
CREATE INDEX llm_usage_events_time_idx ON llm_usage_events(occurred_at);
CREATE INDEX llm_usage_events_analysis_run_idx ON llm_usage_events(accounting_analysis_run_id) WHERE accounting_analysis_run_id IS NOT NULL;
CREATE INDEX llm_usage_events_extraction_attempt_idx ON llm_usage_events(contract_extraction_attempt_id) WHERE contract_extraction_attempt_id IS NOT NULL;
-- A historical run is imported at most once.
CREATE UNIQUE INDEX llm_usage_events_analysis_backfill_uidx ON llm_usage_events(accounting_analysis_run_id) WHERE source = 'BACKFILL' AND accounting_analysis_run_id IS NOT NULL;
CREATE UNIQUE INDEX llm_usage_events_extraction_backfill_uidx ON llm_usage_events(contract_extraction_attempt_id) WHERE source = 'BACKFILL' AND contract_extraction_attempt_id IS NOT NULL;

-- The single cost formula, used by live inserts and by the backfill. Cached
-- tokens are part of the prompt and billed at the cache price; thinking
-- tokens are billed as output. A missing price yields NULL costs.
CREATE FUNCTION llm_usage_cost(input_tokens bigint, cached_tokens bigint, output_tokens bigint, thought_tokens bigint,
  input_price numeric, cached_price numeric, output_price numeric,
  OUT input_cost numeric, OUT cached_input_cost numeric, OUT output_cost numeric, OUT total_cost numeric)
LANGUAGE sql IMMUTABLE AS $$
 SELECT a, b, c, a + b + c FROM (SELECT
  round(GREATEST(input_tokens - cached_tokens, 0) * input_price / 1000000, 10) AS a,
  round(LEAST(cached_tokens, input_tokens) * COALESCE(cached_price, input_price) / 1000000, 10) AS b,
  round((output_tokens + thought_tokens) * output_price / 1000000, 10) AS c) prices
$$;

-- Paid tier, standard (non-batch) prices from the Gemini pricing page, last
-- updated 2026-09-24. Google publishes no price history, so the first price is
-- applied to all earlier history.
INSERT INTO llm_model_prices(id, provider, model, pricing_tier, effective_from, input_usd_per_mtok, cached_input_usd_per_mtok, output_usd_per_mtok, source_url, source_note) VALUES
('llmprice-gemini-gemini-3.8-flash-standard-initial', 'GEMINI', 'gemini-3.8-flash', 'STANDARD', '2020-01-01T00:00:00Z', 0.750000, 0.075000, 3.750000, 'https://ai.google.dev/gemini-api/docs/pricing', 'Preț promoțional valabil până la 31.12.2026, aplicat retroactiv istoricului (Google nu publică istoricul prețurilor).'),
('llmprice-gemini-gemini-3.8-flash-standard-2027-01-01', 'GEMINI', 'gemini-3.8-flash', 'STANDARD', '2027-01-01T00:00:00Z', 1.500000, 0.150000, 7.500000, 'https://ai.google.dev/gemini-api/docs/pricing', 'Preț anunțat începând cu 01.01.2027.');

-- Backfill: the run tables kept the tokens of successful calls only, without
-- thinking tokens, retries or failed calls. These rows are marked BACKFILL and
-- the UI flags the resulting cost as partial. Re-running is idempotent.
INSERT INTO llm_usage_events(id, occurred_at, client_id, run_kind, accounting_analysis_run_id, operation, outcome, source, provider, model,
  usage_reported, input_tokens, output_tokens, total_tokens, price_id, input_cost_usd, cached_input_cost_usd, output_cost_usd, cost_usd, cost_status)
SELECT 'llmbackfill-' || md5('ACCOUNTING_ANALYSIS:' || r.id), COALESCE(r.completed_at, r.started_at), r.client_id, 'ACCOUNTING_ANALYSIS', r.id,
  'ACCOUNTING_ANALYSIS', 'COMPLETED', 'BACKFILL', upper(btrim(r.provider)), m.model,
  true, t.input_tokens, t.output_tokens, t.input_tokens + t.output_tokens,
  p.id, c.input_cost, c.cached_input_cost, c.output_cost, c.total_cost, CASE WHEN p.id IS NULL THEN 'NO_PRICE' ELSE 'PRICED' END
FROM accounting_analysis_runs r
CROSS JOIN LATERAL (SELECT regexp_replace(lower(btrim(r.model)), '^models/', '') AS model) m
CROSS JOIN LATERAL (SELECT GREATEST(COALESCE(r.input_tokens, 0), 0) AS input_tokens, GREATEST(COALESCE(r.output_tokens, 0), 0) AS output_tokens) t
LEFT JOIN LATERAL (SELECT lp.* FROM llm_model_prices lp WHERE lp.provider = upper(btrim(r.provider)) AND lp.model = m.model AND lp.pricing_tier = 'STANDARD' AND lp.effective_from <= COALESCE(r.completed_at, r.started_at) ORDER BY lp.effective_from DESC LIMIT 1) p ON true
CROSS JOIN LATERAL llm_usage_cost(t.input_tokens, 0, t.output_tokens, 0, p.input_usd_per_mtok, p.cached_input_usd_per_mtok, p.output_usd_per_mtok) c
WHERE (r.input_tokens IS NOT NULL OR r.output_tokens IS NOT NULL)
  AND length(m.model) > 0 AND upper(btrim(r.provider)) ~ '^[A-Z][A-Z0-9_]{1,31}$'
  AND NOT EXISTS (SELECT 1 FROM llm_usage_events e WHERE e.accounting_analysis_run_id = r.id);

INSERT INTO llm_usage_events(id, occurred_at, client_id, run_kind, contract_extraction_attempt_id, operation, outcome, source, provider, model,
  usage_reported, input_tokens, output_tokens, total_tokens, price_id, input_cost_usd, cached_input_cost_usd, output_cost_usd, cost_usd, cost_status)
SELECT 'llmbackfill-' || md5('CONTRACT_EXTRACTION:' || a.id), COALESCE(a.completed_at, a.started_at), d.client_id, 'CONTRACT_EXTRACTION', a.id,
  'CONTRACT_EXTRACTION', 'COMPLETED', 'BACKFILL', upper(btrim(a.provider)), m.model,
  true, t.input_tokens, t.output_tokens, t.input_tokens + t.output_tokens,
  p.id, c.input_cost, c.cached_input_cost, c.output_cost, c.total_cost, CASE WHEN p.id IS NULL THEN 'NO_PRICE' ELSE 'PRICED' END
FROM contract_extraction_attempts a
JOIN contract_source_documents d ON d.id = a.document_id
CROSS JOIN LATERAL (SELECT regexp_replace(lower(btrim(a.model)), '^models/', '') AS model) m
CROSS JOIN LATERAL (SELECT GREATEST(COALESCE(a.input_tokens, 0), 0) AS input_tokens, GREATEST(COALESCE(a.output_tokens, 0), 0) AS output_tokens) t
LEFT JOIN LATERAL (SELECT lp.* FROM llm_model_prices lp WHERE lp.provider = upper(btrim(a.provider)) AND lp.model = m.model AND lp.pricing_tier = 'STANDARD' AND lp.effective_from <= COALESCE(a.completed_at, a.started_at) ORDER BY lp.effective_from DESC LIMIT 1) p ON true
CROSS JOIN LATERAL llm_usage_cost(t.input_tokens, 0, t.output_tokens, 0, p.input_usd_per_mtok, p.cached_input_usd_per_mtok, p.output_usd_per_mtok) c
WHERE (a.input_tokens IS NOT NULL OR a.output_tokens IS NOT NULL)
  AND length(m.model) > 0 AND upper(btrim(a.provider)) ~ '^[A-Z][A-Z0-9_]{1,31}$'
  AND NOT EXISTS (SELECT 1 FROM llm_usage_events e WHERE e.contract_extraction_attempt_id = a.id);
