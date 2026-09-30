package llmusage

import (
	"context"
	"strings"
	"time"

	"diana-contabilitate/backend/internal/apperrors"
	"diana-contabilitate/backend/internal/platform/requestactor"
)

const (
	DefaultRunPageSize = 20
	MaxRunPageSize     = 100
)

type Reader interface {
	LLMUsageClientSummary(ctx context.Context, clientID string, period Period) (ClientSummary, error)
	LLMUsageRuns(ctx context.Context, clientID string, period Period, limit, offset int) (RunPage, error)
	LLMUsageRunCalls(ctx context.Context, clientID string, kind RunKind, runID string) (RunDetail, error)
	LLMUsageOverview(ctx context.Context, access Access, period Period) (Overview, error)
}

type Service struct {
	reader Reader
	now    func() time.Time
}

func NewService(reader Reader) *Service {
	return &Service{reader: reader, now: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) Period(from, to string) (Period, error) { return ParsePeriod(from, to, s.now()) }

func (s *Service) ClientSummary(ctx context.Context, clientID string, period Period) (ClientSummary, error) {
	if err := authorizeClient(ctx, clientID); err != nil {
		return ClientSummary{}, err
	}
	return s.reader.LLMUsageClientSummary(ctx, clientID, period)
}

func (s *Service) ClientRuns(ctx context.Context, clientID string, period Period, limit, offset int) (RunPage, error) {
	if err := authorizeClient(ctx, clientID); err != nil {
		return RunPage{}, err
	}
	if limit <= 0 {
		limit = DefaultRunPageSize
	}
	limit = min(limit, MaxRunPageSize)
	offset = max(offset, 0)
	return s.reader.LLMUsageRuns(ctx, clientID, period, limit, offset)
}

func (s *Service) RunCalls(ctx context.Context, clientID string, kind RunKind, runID string) (RunDetail, error) {
	if err := authorizeClient(ctx, clientID); err != nil {
		return RunDetail{}, err
	}
	if !kind.Valid() || strings.TrimSpace(runID) == "" {
		return RunDetail{}, apperrors.ErrNotFound
	}
	return s.reader.LLMUsageRunCalls(ctx, clientID, kind, runID)
}

// Overview is the account total of the current accountant. It never takes a
// user parameter, so nobody can read another account's total.
func (s *Service) Overview(ctx context.Context, period Period) (Overview, error) {
	actor, ok := requestactor.FromContext(ctx)
	if !ok || actor.ID == "" {
		return Overview{}, apperrors.ErrNotFound
	}
	access := Access{All: actor.AllClients, ClientIDs: actor.AuthorizedClientIDs}
	if !access.All && len(access.ClientIDs) == 0 {
		return Overview{Period: period, Totals: Totals{CostUSD: "0"}, Clients: []ClientTotals{}}, nil
	}
	return s.reader.LLMUsageOverview(ctx, access, period)
}

func authorizeClient(ctx context.Context, clientID string) error {
	actor, ok := requestactor.FromContext(ctx)
	if !ok || strings.TrimSpace(clientID) == "" || !actor.AllowsClient(clientID) {
		return apperrors.ErrNotFound
	}
	return nil
}
