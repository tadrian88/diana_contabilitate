package llmusage_test

import (
	"context"
	"errors"
	"testing"

	"diana-contabilitate/backend/internal/apperrors"
	"diana-contabilitate/backend/internal/llmusage"
	"diana-contabilitate/backend/internal/platform/requestactor"
)

type readerStub struct {
	calls  int
	access llmusage.Access
	limit  int
	offset int
}

func (r *readerStub) LLMUsageClientSummary(_ context.Context, clientID string, period llmusage.Period) (llmusage.ClientSummary, error) {
	r.calls++
	return llmusage.ClientSummary{ClientID: clientID, Period: period}, nil
}
func (r *readerStub) LLMUsageRuns(_ context.Context, _ string, _ llmusage.Period, limit, offset int) (llmusage.RunPage, error) {
	r.calls++
	r.limit, r.offset = limit, offset
	return llmusage.RunPage{Limit: limit, Offset: offset}, nil
}
func (r *readerStub) LLMUsageRunCalls(context.Context, string, llmusage.RunKind, string) (llmusage.RunDetail, error) {
	r.calls++
	return llmusage.RunDetail{}, nil
}
func (r *readerStub) LLMUsageOverview(_ context.Context, access llmusage.Access, period llmusage.Period) (llmusage.Overview, error) {
	r.calls++
	r.access = access
	return llmusage.Overview{Period: period}, nil
}

func actorContext(actor requestactor.Actor) context.Context {
	return requestactor.WithActor(context.Background(), actor)
}

func TestServiceHidesClientsOutsideTheActorGrants(t *testing.T) {
	reader := &readerStub{}
	service := llmusage.NewService(reader)
	ctx := actorContext(requestactor.Actor{ID: "accountant-a", AuthorizedClientIDs: []string{"client-a"}})
	if _, err := service.ClientSummary(ctx, "client-b", llmusage.Period{}); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("foreign client summary err=%v", err)
	}
	if _, err := service.ClientRuns(ctx, "client-b", llmusage.Period{}, 10, 0); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("foreign client runs err=%v", err)
	}
	if _, err := service.RunCalls(ctx, "client-b", llmusage.RunAccountingAnalysis, "run"); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("foreign client run err=%v", err)
	}
	if _, err := service.ClientSummary(context.Background(), "client-a", llmusage.Period{}); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("anonymous err=%v", err)
	}
	if reader.calls != 0 {
		t.Fatalf("reader reached %d times for unauthorized requests", reader.calls)
	}
	if _, err := service.ClientSummary(ctx, "client-a", llmusage.Period{}); err != nil || reader.calls != 1 {
		t.Fatalf("own client err=%v calls=%d", err, reader.calls)
	}
}

func TestServiceOverviewUsesTheActorsOwnAccess(t *testing.T) {
	reader := &readerStub{}
	service := llmusage.NewService(reader)
	if _, err := service.Overview(actorContext(requestactor.Actor{ID: "accountant-a", AuthorizedClientIDs: []string{"client-a", "client-c"}}), llmusage.Period{}); err != nil {
		t.Fatal(err)
	}
	if reader.access.All || len(reader.access.ClientIDs) != 2 {
		t.Fatalf("grant-only access=%+v", reader.access)
	}
	if _, err := service.Overview(actorContext(requestactor.Actor{ID: "admin", AllClients: true}), llmusage.Period{}); err != nil || !reader.access.All {
		t.Fatalf("all-clients access=%+v err=%v", reader.access, err)
	}
	calls := reader.calls
	overview, err := service.Overview(actorContext(requestactor.Actor{ID: "new-accountant"}), llmusage.Period{})
	if err != nil || reader.calls != calls || overview.Totals.CostUSD != "0" || overview.Clients == nil {
		t.Fatalf("empty account overview=%+v err=%v calls=%d", overview, err, reader.calls)
	}
	if _, err := service.Overview(context.Background(), llmusage.Period{}); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("anonymous overview err=%v", err)
	}
}

func TestServiceBoundsRunPages(t *testing.T) {
	reader := &readerStub{}
	service := llmusage.NewService(reader)
	ctx := actorContext(requestactor.Actor{ID: "admin", AllClients: true})
	_, _ = service.ClientRuns(ctx, "client-a", llmusage.Period{}, 0, -5)
	if reader.limit != llmusage.DefaultRunPageSize || reader.offset != 0 {
		t.Fatalf("defaults limit=%d offset=%d", reader.limit, reader.offset)
	}
	_, _ = service.ClientRuns(ctx, "client-a", llmusage.Period{}, 5000, 40)
	if reader.limit != llmusage.MaxRunPageSize || reader.offset != 40 {
		t.Fatalf("bounded limit=%d offset=%d", reader.limit, reader.offset)
	}
	if _, err := service.RunCalls(ctx, "client-a", "OTHER", "run"); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("unknown run kind err=%v", err)
	}
}
