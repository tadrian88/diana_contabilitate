package postgres

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"diana-contabilitate/backend/internal/apperrors"
	"diana-contabilitate/backend/internal/llmusage"
	"diana-contabilitate/backend/internal/money"
)

// RecordLLMCall inserts one call. The client is taken from the run itself, so
// a call can never be billed to a client other than its run's.
func (s *Store) RecordLLMCall(ctx context.Context, event llmusage.Event) error {
	var httpStatus, latency any
	if event.HTTPStatus > 0 {
		httpStatus = event.HTTPStatus
	}
	if event.Latency >= 0 {
		latency = event.Latency.Milliseconds()
	}
	var detail any
	if len(event.Usage.Detail) > 0 {
		detail = string(event.Usage.Detail)
	}
	var providerStatus any
	if event.ProviderStatus != "" {
		providerStatus = event.ProviderStatus
	}
	usage := event.Usage
	result, err := s.DB.ExecContext(ctx, `
WITH run AS (
  SELECT r.client_id FROM accounting_analysis_runs r WHERE $3::text = 'ACCOUNTING_ANALYSIS' AND r.id = $4::text
  UNION ALL
  SELECT d.client_id FROM contract_extraction_attempts a JOIN contract_source_documents d ON d.id = a.document_id
   WHERE $3::text = 'CONTRACT_EXTRACTION' AND a.id = $4::text
)
INSERT INTO llm_usage_events(id, occurred_at, client_id, run_kind, accounting_analysis_run_id, contract_extraction_attempt_id,
  operation, outcome, source, provider, model, http_status, provider_status, latency_ms, usage_reported,
  input_tokens, output_tokens, thought_tokens, cached_tokens, tool_use_tokens, total_tokens, usage_detail,
  price_id, input_cost_usd, cached_input_cost_usd, output_cost_usd, cost_usd, cost_status)
SELECT $1::text, $2::timestamptz, run.client_id, $3::text,
  CASE WHEN $3::text = 'ACCOUNTING_ANALYSIS' THEN $4::text END,
  CASE WHEN $3::text = 'CONTRACT_EXTRACTION' THEN $4::text END,
  $5::text, $6::text, 'LIVE', $7::text, $8::text, $9::integer, $10::text, $11::bigint, $12::boolean,
  $13::bigint, $14::bigint, $15::bigint, $16::bigint, $17::bigint, $18::bigint, $19::jsonb,
  p.id, c.input_cost, c.cached_input_cost, c.output_cost, c.total_cost,
  CASE WHEN NOT $12::boolean THEN 'NO_USAGE' WHEN p.id IS NULL THEN 'NO_PRICE' ELSE 'PRICED' END
FROM run
LEFT JOIN LATERAL (
  SELECT lp.id, lp.input_usd_per_mtok, lp.cached_input_usd_per_mtok, lp.output_usd_per_mtok FROM llm_model_prices lp
  WHERE $12::boolean AND lp.provider = $7::text AND lp.model = $8::text AND lp.pricing_tier = 'STANDARD' AND lp.effective_from <= $2::timestamptz
  ORDER BY lp.effective_from DESC LIMIT 1
) p ON true
CROSS JOIN LATERAL llm_usage_cost($13::bigint, $16::bigint, $14::bigint, $15::bigint, p.input_usd_per_mtok, p.cached_input_usd_per_mtok, p.output_usd_per_mtok) c`,
		event.ID, event.OccurredAt, string(event.Scope.RunKind), event.Scope.RunID,
		event.Operation, event.Outcome, llmusage.NormalizeProvider(event.Provider), llmusage.NormalizeModel(event.Model), httpStatus, providerStatus, latency, usage.Reported,
		usage.InputTokens, usage.OutputTokens, usage.ThoughtTokens, usage.CachedTokens, usage.ToolUseTokens, usage.TotalTokens, detail)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err == nil && affected == 0 {
		return llmusage.ErrRunNotFound
	}
	return nil
}

// LLMPriceConfigured reports whether a call made now would be priced.
func (s *Store) LLMPriceConfigured(ctx context.Context, provider, model string, at time.Time) (bool, error) {
	var exists bool
	err := s.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM llm_model_prices WHERE provider=$1 AND model=$2 AND pricing_tier='STANDARD' AND effective_from<=$3)`,
		llmusage.NormalizeProvider(provider), llmusage.NormalizeModel(model), at).Scan(&exists)
	return exists, err
}

// llmTotalsSQL aggregates the events aliased e; with a LEFT JOIN, missing
// events contribute nothing.
const llmTotalsSQL = `count(DISTINCT e.run_kind || ':' || e.run_id), count(e.id),
  count(e.id) FILTER (WHERE e.outcome <> 'COMPLETED'), count(e.id) FILTER (WHERE e.cost_status = 'NO_PRICE'),
  count(e.id) FILTER (WHERE e.cost_status = 'NO_USAGE'),
  COALESCE(sum(e.input_tokens), 0), COALESCE(sum(e.output_tokens), 0), COALESCE(sum(e.thought_tokens), 0),
  COALESCE(sum(e.cached_tokens), 0), COALESCE(sum(e.total_tokens), 0),
  COALESCE(trim_scale(sum(e.cost_usd)), 0)::text, COALESCE(bool_or(e.source = 'BACKFILL'), false)`

func totalsTargets(t *llmusage.Totals) []any {
	return []any{&t.Runs, &t.Calls, &t.FailedCalls, &t.UnpricedCalls, &t.UnreportedCalls,
		&t.InputTokens, &t.OutputTokens, &t.ThoughtTokens, &t.CachedTokens, &t.TotalTokens, &t.CostUSD, &t.IncludesBackfill}
}

func zeroTotals() llmusage.Totals { return llmusage.Totals{CostUSD: money.Amount("0")} }

func (s *Store) LLMUsageClientSummary(ctx context.Context, clientID string, period llmusage.Period) (llmusage.ClientSummary, error) {
	rows, err := s.DB.QueryContext(ctx, `
SELECT GROUPING(e.operation), GROUPING(e.provider, e.model), COALESCE(e.operation, ''), COALESCE(e.provider, ''), COALESCE(e.model, ''), `+llmTotalsSQL+`
FROM llm_usage_events e
WHERE e.client_id = $1 AND e.occurred_at >= $2 AND e.occurred_at < $3
GROUP BY GROUPING SETS ((e.operation), (e.provider, e.model), ())
ORDER BY 1, 2, COALESCE(sum(e.cost_usd), 0) DESC, 3, 4, 5`, clientID, period.Start, period.End)
	if err != nil {
		return llmusage.ClientSummary{}, err
	}
	defer rows.Close()
	summary := llmusage.ClientSummary{ClientID: clientID, Period: period, Totals: zeroTotals(), ByOperation: []llmusage.OperationTotals{}, ByModel: []llmusage.ModelTotals{}}
	for rows.Next() {
		var groupedOperation, groupedModel int
		var operation, provider, model string
		totals := llmusage.Totals{}
		if err = rows.Scan(append([]any{&groupedOperation, &groupedModel, &operation, &provider, &model}, totalsTargets(&totals)...)...); err != nil {
			return llmusage.ClientSummary{}, err
		}
		switch {
		case groupedOperation == 0:
			summary.ByOperation = append(summary.ByOperation, llmusage.OperationTotals{Operation: operation, Totals: totals})
		case groupedModel == 0:
			summary.ByModel = append(summary.ByModel, llmusage.ModelTotals{Provider: provider, Model: model, Totals: totals})
		default:
			summary.Totals = totals
		}
	}
	return summary, rows.Err()
}

func (s *Store) LLMUsageRuns(ctx context.Context, clientID string, period llmusage.Period, limit, offset int) (llmusage.RunPage, error) {
	page := llmusage.RunPage{Items: []llmusage.RunSummary{}, Limit: limit, Offset: offset}
	if err := s.DB.QueryRowContext(ctx, `SELECT count(DISTINCT run_kind || ':' || run_id) FROM llm_usage_events WHERE client_id=$1 AND occurred_at>=$2 AND occurred_at<$3`,
		clientID, period.Start, period.End).Scan(&page.Total); err != nil {
		return llmusage.RunPage{}, err
	}
	if page.Total == 0 || offset >= page.Total {
		return page, nil
	}
	items, err := s.llmUsageRunSummaries(ctx, clientID, &period, "", "", limit, offset)
	page.Items = items
	return page, err
}

func (s *Store) LLMUsageRunCalls(ctx context.Context, clientID string, kind llmusage.RunKind, runID string) (llmusage.RunDetail, error) {
	summaries, err := s.llmUsageRunSummaries(ctx, clientID, nil, kind, runID, 1, 0)
	if err != nil {
		return llmusage.RunDetail{}, err
	}
	if len(summaries) == 0 {
		return llmusage.RunDetail{}, apperrors.ErrNotFound
	}
	rows, err := s.DB.QueryContext(ctx, `
SELECT row_number() OVER (ORDER BY e.occurred_at, e.created_at, e.id), e.id, e.occurred_at, e.operation, e.provider, e.model, e.outcome,
  COALESCE(e.provider_status, ''), e.cost_status, e.source, e.http_status, e.latency_ms, e.usage_reported,
  e.input_tokens, e.output_tokens, e.thought_tokens, e.cached_tokens, e.tool_use_tokens, e.total_tokens, trim_scale(e.cost_usd)::text
FROM llm_usage_events e
WHERE e.client_id = $1 AND e.run_kind = $2 AND e.run_id = $3
ORDER BY 1`, clientID, string(kind), runID)
	if err != nil {
		return llmusage.RunDetail{}, err
	}
	defer rows.Close()
	detail := llmusage.RunDetail{ClientID: clientID, Run: summaries[0], Calls: []llmusage.Call{}}
	for rows.Next() {
		var call llmusage.Call
		var httpStatus sql.NullInt32
		var latency sql.NullInt64
		var cost sql.NullString
		if err = rows.Scan(&call.Ordinal, &call.ID, &call.OccurredAt, &call.Operation, &call.Provider, &call.Model, &call.Outcome,
			&call.ProviderStatus, &call.CostStatus, &call.Source, &httpStatus, &latency, &call.UsageReported,
			&call.InputTokens, &call.OutputTokens, &call.ThoughtTokens, &call.CachedTokens, &call.ToolUseTokens, &call.TotalTokens, &cost); err != nil {
			return llmusage.RunDetail{}, err
		}
		if httpStatus.Valid {
			value := int(httpStatus.Int32)
			call.HTTPStatus = &value
		}
		if latency.Valid {
			call.LatencyMS = &latency.Int64
		}
		if cost.Valid {
			amount := money.Amount(cost.String)
			call.CostUSD = &amount
		}
		detail.Calls = append(detail.Calls, call)
	}
	return detail, rows.Err()
}

// llmUsageRunSummaries groups a client's calls by run, newest first, with the
// run's own status and a human label. A nil period means all time; a run
// filter selects one run.
func (s *Store) llmUsageRunSummaries(ctx context.Context, clientID string, period *llmusage.Period, kind llmusage.RunKind, runID string, limit, offset int) ([]llmusage.RunSummary, error) {
	var start, end any
	if period != nil {
		start, end = period.Start, period.End
	}
	var kindFilter, runFilter any
	if runID != "" {
		kindFilter, runFilter = string(kind), runID
	}
	rows, err := s.DB.QueryContext(ctx, `
WITH runs AS (
  SELECT e.run_kind, e.run_id, min(e.occurred_at) AS first_call_at, max(e.occurred_at) AS last_call_at,
    string_agg(DISTINCT e.model, ',' ORDER BY e.model) AS models, `+llmTotalsSQL+`
  FROM llm_usage_events e
  WHERE e.client_id = $1
    AND ($2::timestamptz IS NULL OR e.occurred_at >= $2::timestamptz) AND ($3::timestamptz IS NULL OR e.occurred_at < $3::timestamptz)
    AND ($4::text IS NULL OR e.run_kind = $4::text) AND ($5::text IS NULL OR e.run_id = $5::text)
  GROUP BY e.run_kind, e.run_id
)
SELECT runs.*,
  COALESCE(ar.status, ca.status, ''),
  COALESCE(ar.invoice_id, ''), COALESCE(ca.document_id, ''),
  CASE WHEN runs.run_kind = 'ACCOUNTING_ANALYSIS' THEN COALESCE(i.document_number || ' · ' || i.supplier_name, '') ELSE COALESCE(d.original_filename, '') END,
  CASE WHEN ca.id IS NULL THEN 0 ELSE (SELECT count(*) FROM contract_extraction_attempts x WHERE x.document_id = ca.document_id AND (x.started_at, x.id) <= (ca.started_at, ca.id)) END
FROM runs
LEFT JOIN accounting_analysis_runs ar ON runs.run_kind = 'ACCOUNTING_ANALYSIS' AND ar.id = runs.run_id
LEFT JOIN invoices i ON i.id = ar.invoice_id AND i.client_id = ar.client_id
LEFT JOIN contract_extraction_attempts ca ON runs.run_kind = 'CONTRACT_EXTRACTION' AND ca.id = runs.run_id
LEFT JOIN contract_source_documents d ON d.id = ca.document_id
ORDER BY runs.last_call_at DESC, runs.run_id
LIMIT $6 OFFSET $7`, clientID, start, end, kindFilter, runFilter, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []llmusage.RunSummary{}
	for rows.Next() {
		var run llmusage.RunSummary
		var kindValue, models string
		targets := append([]any{&kindValue, &run.RunID, &run.FirstCallAt, &run.LastCallAt, &models}, totalsTargets(&run.Totals)...)
		targets = append(targets, &run.Status, &run.InvoiceID, &run.DocumentID, &run.Label, &run.AttemptNumber)
		if err = rows.Scan(targets...); err != nil {
			return nil, err
		}
		run.RunKind = llmusage.RunKind(kindValue)
		run.Backfill = run.Totals.IncludesBackfill
		run.Models = strings.Split(models, ",")
		result = append(result, run)
	}
	return result, rows.Err()
}

// LLMUsageOverview lists every accessible client, including those without
// usage in the period, and the account total as the empty grouping set.
func (s *Store) LLMUsageOverview(ctx context.Context, access llmusage.Access, period llmusage.Period) (llmusage.Overview, error) {
	ids := access.ClientIDs
	if ids == nil {
		ids = []string{}
	}
	rows, err := s.DB.QueryContext(ctx, `
SELECT GROUPING(c.id, c.name), COALESCE(c.id, ''), COALESCE(c.name, ''), `+llmTotalsSQL+`
FROM clients c
LEFT JOIN llm_usage_events e ON e.client_id = c.id AND e.occurred_at >= $1 AND e.occurred_at < $2
WHERE $3::boolean OR c.id = ANY($4::text[])
GROUP BY GROUPING SETS ((c.id, c.name), ())
ORDER BY 1 DESC, COALESCE(sum(e.cost_usd), 0) DESC, COALESCE(sum(e.total_tokens), 0) DESC, 3, 2`, period.Start, period.End, access.All, ids)
	if err != nil {
		return llmusage.Overview{}, err
	}
	defer rows.Close()
	overview := llmusage.Overview{Period: period, Totals: zeroTotals(), Clients: []llmusage.ClientTotals{}}
	for rows.Next() {
		var grouped int
		var client llmusage.ClientTotals
		if err = rows.Scan(append([]any{&grouped, &client.ClientID, &client.ClientName}, totalsTargets(&client.Totals)...)...); err != nil {
			return llmusage.Overview{}, err
		}
		if grouped != 0 {
			overview.Totals = client.Totals
			continue
		}
		overview.Clients = append(overview.Clients, client)
	}
	return overview, rows.Err()
}
