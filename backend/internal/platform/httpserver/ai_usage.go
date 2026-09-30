package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"diana-contabilitate/backend/internal/apperrors"
	"diana-contabilitate/backend/internal/llmusage"
	"diana-contabilitate/backend/internal/money"
)

type aiUsageTotalsDTO struct {
	Runs             int64    `json:"runs"`
	Calls            int64    `json:"calls"`
	FailedCalls      int64    `json:"failedCalls"`
	UnpricedCalls    int64    `json:"unpricedCalls"`
	UnreportedCalls  int64    `json:"unreportedCalls"`
	InputTokens      int64    `json:"inputTokens"`
	OutputTokens     int64    `json:"outputTokens"`
	ThoughtTokens    int64    `json:"thoughtTokens"`
	CachedTokens     int64    `json:"cachedTokens"`
	TotalTokens      int64    `json:"totalTokens"`
	Cost             moneyDTO `json:"cost"`
	IncludesBackfill bool     `json:"includesBackfill"`
}

type aiUsagePeriodDTO struct {
	From     string `json:"from"`
	To       string `json:"to"`
	TimeZone string `json:"timeZone"`
}

type aiUsageOverviewDTO struct {
	Period  aiUsagePeriodDTO         `json:"period"`
	Totals  aiUsageTotalsDTO         `json:"totals"`
	Clients []aiUsageClientTotalsDTO `json:"clients"`
}

type aiUsageClientTotalsDTO struct {
	ClientID   string           `json:"clientId"`
	ClientName string           `json:"clientName"`
	Totals     aiUsageTotalsDTO `json:"totals"`
}

type clientAIUsageDTO struct {
	ClientID    string                     `json:"clientId"`
	Period      aiUsagePeriodDTO           `json:"period"`
	Totals      aiUsageTotalsDTO           `json:"totals"`
	ByOperation []aiUsageOperationTotalDTO `json:"byOperation"`
	ByModel     []aiUsageModelTotalDTO     `json:"byModel"`
}

type aiUsageOperationTotalDTO struct {
	Operation string           `json:"operation"`
	Totals    aiUsageTotalsDTO `json:"totals"`
}

type aiUsageModelTotalDTO struct {
	Provider string           `json:"provider"`
	Model    string           `json:"model"`
	Totals   aiUsageTotalsDTO `json:"totals"`
}

type aiUsageRunDTO struct {
	RunKind       string           `json:"runKind"`
	RunID         string           `json:"runId"`
	Status        string           `json:"status"`
	Label         string           `json:"label"`
	InvoiceID     string           `json:"invoiceId,omitempty"`
	DocumentID    string           `json:"documentId,omitempty"`
	AttemptNumber int              `json:"attemptNumber,omitempty"`
	FirstCallAt   string           `json:"firstCallAt"`
	LastCallAt    string           `json:"lastCallAt"`
	Models        []string         `json:"models"`
	Backfill      bool             `json:"backfill"`
	Totals        aiUsageTotalsDTO `json:"totals"`
}

type aiUsageRunPageDTO struct {
	Items  []aiUsageRunDTO `json:"items"`
	Total  int             `json:"total"`
	Limit  int             `json:"limit"`
	Offset int             `json:"offset"`
}

type aiUsageCallDTO struct {
	Ordinal        int       `json:"ordinal"`
	ID             string    `json:"id"`
	OccurredAt     string    `json:"occurredAt"`
	Operation      string    `json:"operation"`
	Provider       string    `json:"provider"`
	Model          string    `json:"model"`
	Outcome        string    `json:"outcome"`
	ProviderStatus string    `json:"providerStatus,omitempty"`
	HTTPStatus     *int      `json:"httpStatus"`
	LatencyMS      *int64    `json:"latencyMs"`
	UsageReported  bool      `json:"usageReported"`
	InputTokens    int64     `json:"inputTokens"`
	OutputTokens   int64     `json:"outputTokens"`
	ThoughtTokens  int64     `json:"thoughtTokens"`
	CachedTokens   int64     `json:"cachedTokens"`
	ToolUseTokens  int64     `json:"toolUseTokens"`
	TotalTokens    int64     `json:"totalTokens"`
	Cost           *moneyDTO `json:"cost"`
	CostStatus     string    `json:"costStatus"`
	Source         string    `json:"source"`
}

type aiUsageRunDetailDTO struct {
	ClientID string           `json:"clientId"`
	Run      aiUsageRunDTO    `json:"run"`
	Calls    []aiUsageCallDTO `json:"calls"`
}

// runKindPath maps the URL form of a run kind to the domain value.
var runKindPath = map[string]llmusage.RunKind{
	"accounting-analysis": llmusage.RunAccountingAnalysis,
	"contract-extraction": llmusage.RunContractExtraction,
}

func (s *Server) aiUsageAvailable(w http.ResponseWriter, r *http.Request) bool {
	if s.aiUsage == nil {
		writeError(w, r, http.StatusServiceUnavailable, "FEATURE_DISABLED", "Evidența consumului AI nu este activată.")
		return false
	}
	return true
}

func (s *Server) aiUsagePeriod(w http.ResponseWriter, r *http.Request) (llmusage.Period, bool) {
	period, err := s.aiUsage.Period(r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Perioada selectată nu este validă.")
		return llmusage.Period{}, false
	}
	return period, true
}

func (s *Server) getAIUsageOverview(w http.ResponseWriter, r *http.Request) {
	if !s.aiUsageAvailable(w, r) {
		return
	}
	period, ok := s.aiUsagePeriod(w, r)
	if !ok {
		return
	}
	overview, err := s.aiUsage.Overview(r.Context(), period)
	if err != nil {
		s.writeAIUsageError(w, r, err)
		return
	}
	dto := aiUsageOverviewDTO{Period: toAIUsagePeriodDTO(overview.Period), Totals: toAIUsageTotalsDTO(overview.Totals), Clients: make([]aiUsageClientTotalsDTO, 0, len(overview.Clients))}
	for _, client := range overview.Clients {
		dto.Clients = append(dto.Clients, aiUsageClientTotalsDTO{ClientID: client.ClientID, ClientName: client.ClientName, Totals: toAIUsageTotalsDTO(client.Totals)})
	}
	writeJSON(w, http.StatusOK, dto)
}

func (s *Server) getClientAIUsage(w http.ResponseWriter, r *http.Request) {
	clientID := strings.TrimSpace(r.PathValue("clientId"))
	if !s.aiUsageAvailable(w, r) || !s.allowClient(w, r, clientID) {
		return
	}
	period, ok := s.aiUsagePeriod(w, r)
	if !ok {
		return
	}
	summary, err := s.aiUsage.ClientSummary(r.Context(), clientID, period)
	if err != nil {
		s.writeAIUsageError(w, r, err)
		return
	}
	dto := clientAIUsageDTO{ClientID: summary.ClientID, Period: toAIUsagePeriodDTO(summary.Period), Totals: toAIUsageTotalsDTO(summary.Totals), ByOperation: make([]aiUsageOperationTotalDTO, 0, len(summary.ByOperation)), ByModel: make([]aiUsageModelTotalDTO, 0, len(summary.ByModel))}
	for _, item := range summary.ByOperation {
		dto.ByOperation = append(dto.ByOperation, aiUsageOperationTotalDTO{Operation: item.Operation, Totals: toAIUsageTotalsDTO(item.Totals)})
	}
	for _, item := range summary.ByModel {
		dto.ByModel = append(dto.ByModel, aiUsageModelTotalDTO{Provider: item.Provider, Model: item.Model, Totals: toAIUsageTotalsDTO(item.Totals)})
	}
	writeJSON(w, http.StatusOK, dto)
}

func (s *Server) listClientAIUsageRuns(w http.ResponseWriter, r *http.Request) {
	clientID := strings.TrimSpace(r.PathValue("clientId"))
	if !s.aiUsageAvailable(w, r) || !s.allowClient(w, r, clientID) {
		return
	}
	period, ok := s.aiUsagePeriod(w, r)
	if !ok {
		return
	}
	limit, limitErr := optionalNonNegativeInt(r.URL.Query().Get("limit"))
	offset, offsetErr := optionalNonNegativeInt(r.URL.Query().Get("offset"))
	if limitErr != nil || offsetErr != nil {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Paginarea selectată nu este validă.")
		return
	}
	page, err := s.aiUsage.ClientRuns(r.Context(), clientID, period, limit, offset)
	if err != nil {
		s.writeAIUsageError(w, r, err)
		return
	}
	dto := aiUsageRunPageDTO{Items: make([]aiUsageRunDTO, 0, len(page.Items)), Total: page.Total, Limit: page.Limit, Offset: page.Offset}
	for _, run := range page.Items {
		dto.Items = append(dto.Items, toAIUsageRunDTO(run))
	}
	writeJSON(w, http.StatusOK, dto)
}

func (s *Server) getClientAIUsageRun(w http.ResponseWriter, r *http.Request) {
	clientID := strings.TrimSpace(r.PathValue("clientId"))
	if !s.aiUsageAvailable(w, r) || !s.allowClient(w, r, clientID) {
		return
	}
	kind, known := runKindPath[r.PathValue("runKind")]
	if !known {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "Rularea nu a fost găsită.")
		return
	}
	detail, err := s.aiUsage.RunCalls(r.Context(), clientID, kind, strings.TrimSpace(r.PathValue("runId")))
	if err != nil {
		s.writeAIUsageError(w, r, err)
		return
	}
	dto := aiUsageRunDetailDTO{ClientID: detail.ClientID, Run: toAIUsageRunDTO(detail.Run), Calls: make([]aiUsageCallDTO, 0, len(detail.Calls))}
	for _, call := range detail.Calls {
		item := aiUsageCallDTO{Ordinal: call.Ordinal, ID: call.ID, OccurredAt: call.OccurredAt.UTC().Format(time.RFC3339), Operation: call.Operation, Provider: call.Provider, Model: call.Model, Outcome: call.Outcome, ProviderStatus: call.ProviderStatus,
			HTTPStatus: call.HTTPStatus, LatencyMS: call.LatencyMS, UsageReported: call.UsageReported, InputTokens: call.InputTokens, OutputTokens: call.OutputTokens, ThoughtTokens: call.ThoughtTokens,
			CachedTokens: call.CachedTokens, ToolUseTokens: call.ToolUseTokens, TotalTokens: call.TotalTokens, CostStatus: call.CostStatus, Source: call.Source}
		if call.CostUSD != nil {
			cost := usdDTO(*call.CostUSD)
			item.Cost = &cost
		}
		dto.Calls = append(dto.Calls, item)
	}
	writeJSON(w, http.StatusOK, dto)
}

func (s *Server) writeAIUsageError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, apperrors.ErrNotFound):
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "Consumul AI nu a fost găsit.")
	case errors.Is(err, apperrors.ErrValidation):
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Perioada selectată nu este validă.")
	default:
		s.internalError(w, r, err)
	}
}

func optionalNonNegativeInt(raw string) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value < 0 {
		return 0, apperrors.ErrValidation
	}
	return value, nil
}

func toAIUsagePeriodDTO(period llmusage.Period) aiUsagePeriodDTO {
	return aiUsagePeriodDTO{From: string(period.From), To: string(period.To), TimeZone: llmusage.TimeZone}
}

func toAIUsageTotalsDTO(totals llmusage.Totals) aiUsageTotalsDTO {
	return aiUsageTotalsDTO{Runs: totals.Runs, Calls: totals.Calls, FailedCalls: totals.FailedCalls, UnpricedCalls: totals.UnpricedCalls, UnreportedCalls: totals.UnreportedCalls,
		InputTokens: totals.InputTokens, OutputTokens: totals.OutputTokens, ThoughtTokens: totals.ThoughtTokens, CachedTokens: totals.CachedTokens, TotalTokens: totals.TotalTokens,
		Cost: usdDTO(totals.CostUSD), IncludesBackfill: totals.IncludesBackfill}
}

func toAIUsageRunDTO(run llmusage.RunSummary) aiUsageRunDTO {
	models := run.Models
	if models == nil {
		models = []string{}
	}
	return aiUsageRunDTO{RunKind: string(run.RunKind), RunID: run.RunID, Status: run.Status, Label: run.Label, InvoiceID: run.InvoiceID, DocumentID: run.DocumentID, AttemptNumber: run.AttemptNumber,
		FirstCallAt: run.FirstCallAt.UTC().Format(time.RFC3339), LastCallAt: run.LastCallAt.UTC().Format(time.RFC3339), Models: models, Backfill: run.Backfill, Totals: toAIUsageTotalsDTO(run.Totals)}
}

// usdDTO renders an exact decimal as a JSON number; an invalid value (never
// produced by the store) degrades to zero rather than invalid JSON.
func usdDTO(amount money.Amount) moneyDTO {
	value := amount.String()
	if !json.Valid([]byte(value)) {
		value = "0"
	}
	return moneyDTO{Amount: json.RawMessage(value), Currency: "USD"}
}
