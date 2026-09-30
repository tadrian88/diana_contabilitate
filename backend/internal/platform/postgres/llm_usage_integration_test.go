//go:build integration

package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"diana-contabilitate/backend/internal/llmusage"
)

// TestLLMUsageAgainstPostgreSQL uses contract extraction attempts as runs
// because they can be cleaned up; accounting analysis runs are immutable.
// Price rows are append-only, so each run leaves its test-only model prices.
func TestLLMUsageAgainstPostgreSQL(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	store, err := Open(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	model := "integration-model-" + suffix
	clientA, clientB := "llm-client-a-"+suffix, "llm-client-b-"+suffix
	attemptA, attemptB := "llm-attempt-a-"+suffix, "llm-attempt-b-"+suffix
	now := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	defer func() {
		_, _ = store.DB.ExecContext(ctx, `DELETE FROM llm_usage_events WHERE client_id IN ($1,$2)`, clientA, clientB)
		_, _ = store.DB.ExecContext(ctx, `DELETE FROM contract_extraction_attempts WHERE id IN ($1,$2)`, attemptA, attemptB)
		_, _ = store.DB.ExecContext(ctx, `DELETE FROM contract_source_documents WHERE client_id IN ($1,$2)`, clientA, clientB)
		_ = store.Client.AccountingClient.DeleteOneID(clientA).Exec(ctx)
		_ = store.Client.AccountingClient.DeleteOneID(clientB).Exec(ctx)
	}()
	for index, client := range []struct{ id, attempt string }{{clientA, attemptA}, {clientB, attemptB}} {
		if _, err := store.Client.AccountingClient.Create().SetID(client.id).SetName(fmt.Sprintf("Client LLM %d", index)).SetCui("RO-LLM-" + suffix + fmt.Sprint(index)).SetCreatedAt(now).SetUpdatedAt(now).Save(ctx); err != nil {
			t.Fatal(err)
		}
		documentID := "llm-doc-" + client.id
		if _, err := store.DB.ExecContext(ctx, `INSERT INTO contract_source_documents(id,client_id,original_filename,mime_type,size_bytes,sha256,raw_document,uploaded_at,status,updated_at)
VALUES($1,$2,'contract.pdf','application/pdf',4,$3,'\x25504446'::bytea,$4,'EXTRACTING',$4)`, documentID, client.id, "sha-"+client.id, now); err != nil {
			t.Fatal(err)
		}
		if _, err := store.DB.ExecContext(ctx, `INSERT INTO contract_extraction_attempts(id,document_id,provider,model,schema_version,prompt_version,status,started_at)
VALUES($1,$2,'GEMINI',$3,'v1','v1','STARTED',$4)`, client.attempt, documentID, model, now); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.DB.ExecContext(ctx, `INSERT INTO llm_model_prices(id,provider,model,effective_from,input_usd_per_mtok,cached_input_usd_per_mtok,output_usd_per_mtok,source_url) VALUES
($1,'GEMINI',$3,'2026-01-01T00:00:00Z',0.75,0.075,3.75,'https://example.test/prices'),
($2,'GEMINI',$3,'2027-01-01T00:00:00Z',1.50,0.15,7.50,'https://example.test/prices')`, "price-a-"+suffix, "price-b-"+suffix, model); err != nil {
		t.Fatal(err)
	}

	record := func(id, attempt string, occurredAt time.Time, eventModel, operation string, usage llmusage.Usage, outcome string, status int) error {
		return store.RecordLLMCall(ctx, llmusage.Event{ID: id + "-" + suffix, OccurredAt: occurredAt, Scope: llmusage.Scope{RunKind: llmusage.RunContractExtraction, RunID: attempt, ClientID: clientB},
			Provider: "gemini", Model: eventModel, Operation: operation, Outcome: outcome, HTTPStatus: status, Latency: 1500 * time.Millisecond, Usage: usage})
	}
	usage := llmusage.Usage{InputTokens: 1000, CachedTokens: 200, OutputTokens: 100, ThoughtTokens: 300, TotalTokens: 1400, Reported: true, Detail: []byte(`{"total_input_tokens":1000}`)}
	september := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	for _, call := range []struct {
		id, attempt string
		at          time.Time
		model, op   string
		usage       llmusage.Usage
		outcome     string
		status      int
	}{
		{"priced", attemptA, september, model, llmusage.OperationContractExtraction, usage, llmusage.OutcomeCompleted, 200},
		{"rate-limited", attemptA, september.Add(time.Minute), model, llmusage.OperationContractExtraction, llmusage.Usage{}, llmusage.OutcomeHTTPError, 429},
		{"unpriced", attemptA, september.Add(2 * time.Minute), "unknown-" + suffix, llmusage.OperationContractClauseNormalization, usage, llmusage.OutcomeCompleted, 200},
		{"new-operation", attemptA, september.Add(3 * time.Minute), model, "FUTURE_OPERATION", usage, llmusage.OutcomeCompleted, 200},
		{"next-year", attemptA, time.Date(2027, 1, 2, 0, 0, 0, 0, time.UTC), model, llmusage.OperationContractExtraction, usage, llmusage.OutcomeCompleted, 200},
	} {
		if err := record(call.id, call.attempt, call.at, call.model, call.op, call.usage, call.outcome, call.status); err != nil {
			t.Fatalf("%s: %v", call.id, err)
		}
	}

	var clientID, costStatus, cost, priceID string
	if err := store.DB.QueryRowContext(ctx, `SELECT client_id, cost_status, trim_scale(cost_usd)::text, price_id FROM llm_usage_events WHERE id=$1`, "priced-"+suffix).Scan(&clientID, &costStatus, &cost, &priceID); err != nil {
		t.Fatal(err)
	}
	// (800*0.75 + 200*0.075 + (100+300)*3.75) / 1e6; the client comes from the run, not from the scope.
	if clientID != clientA || costStatus != "PRICED" || cost != "0.002115" || priceID != "price-a-"+suffix {
		t.Fatalf("priced call client=%s status=%s cost=%s price=%s", clientID, costStatus, cost, priceID)
	}
	if err := store.DB.QueryRowContext(ctx, `SELECT trim_scale(cost_usd)::text FROM llm_usage_events WHERE id=$1`, "next-year-"+suffix).Scan(&cost); err != nil || cost != "0.00423" {
		t.Fatalf("2027 price cost=%s err=%v", cost, err)
	}
	for id, want := range map[string]string{"rate-limited": "NO_USAGE", "unpriced": "NO_PRICE"} {
		var status string
		var nullCost *string
		if err := store.DB.QueryRowContext(ctx, `SELECT cost_status, cost_usd::text FROM llm_usage_events WHERE id=$1`, id+"-"+suffix).Scan(&status, &nullCost); err != nil || status != want || nullCost != nil {
			t.Fatalf("%s status=%s cost=%v err=%v", id, status, nullCost, err)
		}
	}
	if err := record("unknown-run", "missing-"+suffix, september, model, "OP", usage, llmusage.OutcomeCompleted, 200); !errors.Is(err, llmusage.ErrRunNotFound) {
		t.Fatalf("unknown run err=%v", err)
	}
	if err := record("bad-operation", attemptA, september, model, "bad operation", usage, llmusage.OutcomeCompleted, 200); err == nil {
		t.Fatal("malformed operation accepted")
	}
	if _, err := store.DB.ExecContext(ctx, `UPDATE llm_model_prices SET input_usd_per_mtok=0 WHERE id=$1`, "price-a-"+suffix); err == nil {
		t.Fatal("prices must be append-only")
	}

	period, err := llmusage.ParsePeriod("2026-09-01", "2026-09-30", now)
	if err != nil {
		t.Fatal(err)
	}
	summary, err := store.LLMUsageClientSummary(ctx, clientA, period)
	if err != nil {
		t.Fatal(err)
	}
	totals := summary.Totals
	if totals.Runs != 1 || totals.Calls != 4 || totals.FailedCalls != 1 || totals.UnpricedCalls != 1 || totals.UnreportedCalls != 1 || totals.InputTokens != 3000 || totals.ThoughtTokens != 900 || totals.CostUSD != "0.00423" || totals.IncludesBackfill {
		t.Fatalf("september totals=%+v", totals)
	}
	if len(summary.ByOperation) != 3 || len(summary.ByModel) != 2 {
		t.Fatalf("breakdowns operation=%+v model=%+v", summary.ByOperation, summary.ByModel)
	}

	page, err := store.LLMUsageRuns(ctx, clientA, period, 10, 0)
	if err != nil || page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	run := page.Items[0]
	if run.RunID != attemptA || run.Label != "contract.pdf" || run.AttemptNumber != 1 || run.Status != "STARTED" || run.Totals.Calls != 4 || len(run.Models) != 2 {
		t.Fatalf("run=%+v", run)
	}
	detail, err := store.LLMUsageRunCalls(ctx, clientA, llmusage.RunContractExtraction, attemptA)
	if err != nil || len(detail.Calls) != 5 || detail.Calls[0].ID != "priced-"+suffix || detail.Calls[4].ID != "next-year-"+suffix || *detail.Calls[0].LatencyMS != 1500 {
		t.Fatalf("detail=%+v err=%v", detail, err)
	}
	if _, err := store.LLMUsageRunCalls(ctx, clientB, llmusage.RunContractExtraction, attemptA); err == nil {
		t.Fatal("a run must not be readable through another client")
	}

	overview, err := store.LLMUsageOverview(ctx, llmusage.Access{ClientIDs: []string{clientA, clientB}}, period)
	if err != nil || len(overview.Clients) != 2 || overview.Clients[0].ClientID != clientA || overview.Clients[1].Totals.Calls != 0 || overview.Totals.CostUSD != "0.00423" {
		t.Fatalf("overview=%+v err=%v", overview, err)
	}
	onlyB, err := store.LLMUsageOverview(ctx, llmusage.Access{ClientIDs: []string{clientB}}, period)
	if err != nil || len(onlyB.Clients) != 1 || onlyB.Totals.Calls != 0 || onlyB.Totals.CostUSD != "0" {
		t.Fatalf("grant-limited overview=%+v err=%v", onlyB, err)
	}
	all, err := store.LLMUsageOverview(ctx, llmusage.Access{All: true}, period)
	if err != nil || all.Totals.Calls < 4 {
		t.Fatalf("all-clients overview=%+v err=%v", all.Totals, err)
	}
}
