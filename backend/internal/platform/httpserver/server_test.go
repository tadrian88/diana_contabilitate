package httpserver

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"diana-contabilitate/backend/internal/apperrors"
	classificationdomain "diana-contabilitate/backend/internal/classification"
	"diana-contabilitate/backend/internal/clients"
	contractdomain "diana-contabilitate/backend/internal/contracts"
	"diana-contabilitate/backend/internal/invoicing"
	"diana-contabilitate/backend/internal/money"
	"diana-contabilitate/backend/internal/platform/requestactor"
	"diana-contabilitate/backend/internal/rules"
	"diana-contabilitate/backend/internal/validationtasks"
)

func timePointer(value time.Time) *time.Time { return &value }

type clientReader struct{ items []clients.Client }

func (s clientReader) ListClients(context.Context) ([]clients.Client, error) { return s.items, nil }

type invoiceReader struct{ item *invoicing.Invoice }

func (s invoiceReader) GetInvoice(_ context.Context, id string) (*invoicing.Invoice, error) {
	if s.item == nil || s.item.ID != id {
		return nil, apperrors.ErrNotFound
	}
	return s.item, nil
}

func (s invoiceReader) ListInvoices(context.Context, invoicing.Filter) ([]invoicing.Invoice, error) {
	if s.item == nil {
		return []invoicing.Invoice{}, nil
	}
	return []invoicing.Invoice{*s.item}, nil
}

type readyStub struct{}

func (readyStub) Ping(context.Context) error { return nil }

func testHandler(invoice *invoicing.Invoice) http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return withTestActor(New(
		clients.NewService(clientReader{items: []clients.Client{{ID: "client-alfa", Name: "Client Demo Alfa SRL", CUI: "RO-DEMO-ALFA-001"}}}),
		invoicing.NewService(invoiceReader{item: invoice}), nil, nil, nil, nil, readyStub{}, logger,
	), requestactor.Actor{ID: "test-accountant", Display: "Test", Persona: "CONTABIL", AllClients: true})
}

type contractStore struct {
	items         []contractdomain.Contract
	invoices      []contractdomain.AssociatedInvoice
	invoice       *invoicing.Invoice
	confirmations []contractdomain.ConfirmCommand
	committed     map[string]bool
}

func (s *contractStore) ListContracts(context.Context, contractdomain.Filter) ([]contractdomain.Contract, error) {
	return s.items, nil
}
func (s *contractStore) GetContract(_ context.Context, id string) (*contractdomain.Contract, error) {
	for index := range s.items {
		if s.items[index].ID == id {
			return &s.items[index], nil
		}
	}
	return nil, apperrors.ErrNotFound
}
func (s *contractStore) ListContractInvoices(context.Context, string) ([]contractdomain.AssociatedInvoice, error) {
	return s.invoices, nil
}
func (s *contractStore) MatchCommandCommitted(context.Context, string) (bool, error) {
	return false, nil
}
func (s *contractStore) LoadMatchingInput(context.Context, string) (contractdomain.InvoiceContext, []contractdomain.Contract, error) {
	return contractdomain.InvoiceContext{}, nil, nil
}
func (s *contractStore) ApplyMatchDecision(context.Context, contractdomain.MatchCommand, contractdomain.MatchDecision, time.Time) (bool, error) {
	return true, nil
}
func (s *contractStore) ConfirmContractMatch(_ context.Context, command contractdomain.ConfirmCommand, now time.Time) (bool, error) {
	s.confirmations = append(s.confirmations, command)
	if s.committed[command.CommandID] {
		return false, nil
	}
	if s.committed == nil {
		s.committed = make(map[string]bool)
	}
	s.committed[command.CommandID] = true
	s.invoice.PipelineStatus = invoicing.StatusDedupeChecked
	s.invoice.Revision++
	s.invoice.ActiveTask = nil
	s.invoice.ContractAssociation = &contractdomain.AssociationSnapshot{
		ContractID: command.ContractID, Reference: "CTR-1", SupplierName: "Furnizor", EffectiveFrom: now,
		EffectiveTo: timePointer(now.AddDate(0, 1, 0)), Value: money.Money{Amount: money.MustParse("100.0000"), Currency: "RON"},
		UnitType: "BUC", PaymentTerms: "30 zile", PolicyVersion: contractdomain.BaselinePolicyVersion,
	}
	return true, nil
}
func (s *contractStore) RecordContractAvailable(context.Context, contractdomain.AvailableCommand, time.Time) (bool, error) {
	return true, nil
}
func (s *contractStore) ListBlockedInvoicesForContract(context.Context, string, string, int) ([]contractdomain.BlockedInvoice, error) {
	return nil, nil
}
func (s *contractStore) ResumeCommandCommitted(context.Context, string) (bool, error) {
	return false, nil
}
func (s *contractStore) ApplyResumeDecision(context.Context, contractdomain.ResumeCommand, contractdomain.MatchDecision, time.Time) (bool, error) {
	return true, nil
}

func testHandlerWithContracts(invoice *invoicing.Invoice, store *contractStore) http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return withTestActor(New(
		clients.NewService(clientReader{}), invoicing.NewService(invoiceReader{item: invoice}), nil,
		contractdomain.NewService(store, nil, func() time.Time { return time.Date(2026, time.September, 15, 12, 0, 0, 0, time.UTC) }), nil, nil, readyStub{}, logger,
	), requestactor.Actor{ID: "test-accountant", Display: "Test", Persona: "CONTABIL", AllClients: true})
}

type validationTaskStore struct {
	items     []validationtasks.InboxItem
	invoice   *invoicing.Invoice
	requests  []validationtasks.RequestMissingContractCommand
	committed map[string]*validationtasks.Task
}

func (s *validationTaskStore) ListValidationTasks(context.Context, validationtasks.Filter) ([]validationtasks.InboxItem, error) {
	return s.items, nil
}

func (s *validationTaskStore) CreateBlockingTask(context.Context, validationtasks.CreationCommand, validationtasks.CreationDefinition, time.Time) (*validationtasks.Task, bool, error) {
	return nil, false, nil
}

func (s *validationTaskStore) RequestMissingContract(_ context.Context, command validationtasks.RequestMissingContractCommand, now time.Time) (*validationtasks.Task, bool, error) {
	s.requests = append(s.requests, command)
	if task, exists := s.committed[command.CommandID]; exists {
		return task, false, nil
	}
	task := s.invoice.ActiveTask
	task.Status = validationtasks.StatusWaiting
	task.Revision++
	task.WaitingSince = &now
	task.UpdatedAt = now
	if s.committed == nil {
		s.committed = make(map[string]*validationtasks.Task)
	}
	s.committed[command.CommandID] = task
	return task, true, nil
}

func testHandlerWithTasks(invoice *invoicing.Invoice, store *validationTaskStore) http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return withTestActor(New(
		clients.NewService(clientReader{items: []clients.Client{{ID: "client-alfa", Name: "Client Demo Alfa SRL", CUI: "RO-DEMO-ALFA-001"}}}),
		invoicing.NewService(invoiceReader{item: invoice}), validationtasks.NewService(store, func() time.Time { return time.Date(2026, time.September, 12, 12, 0, 0, 0, time.UTC) }), nil, nil, nil, readyStub{}, logger,
	), requestactor.Actor{ID: "test-accountant", Display: "Test", Persona: "CONTABIL", AllClients: true})
}

func TestMissingContractResumeHistoryLabelsAreRomanian(t *testing.T) {
	want := map[string]string{
		"MISSING_CONTRACT_REEVALUATED":   "Contract reevaluat automat",
		"MISSING_CONTRACT_RESOLVED":      "Contract lipsă rezolvat",
		"MISSING_CONTRACT_STILL_WAITING": "Contract încă indisponibil",
	}
	for eventType, label := range want {
		if got := activityLabel(eventType); got != label {
			t.Fatalf("label for %s=%q want=%q", eventType, got, label)
		}
	}
}

func TestListClients(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/clients", nil)
	response := httptest.NewRecorder()
	testHandler(nil).ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Client Demo Alfa SRL") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if response.Header().Get("X-Request-ID") == "" {
		t.Fatal("missing request correlation header")
	}
}

func TestGetInvoiceSerializesMoneyAsJSONNumber(t *testing.T) {
	now := time.Date(2026, time.September, 8, 14, 0, 0, 0, time.UTC)
	item := &invoicing.Invoice{
		ID: "inv-resolved", ClientID: "client-beta", SupplierName: "Furnizor Demo",
		DocumentNumber: "DEMO-RS-007", IssueDate: now,
		Total:        money.Money{Amount: money.MustParse("1785.2500"), Currency: "RON"},
		SPVReference: "SPV-DEMO-0007", PipelineStatus: invoicing.StatusExported,
		SagaStatus: invoicing.SagaExported, Revision: 1, CreatedAt: now, UpdatedAt: now,
		Lines: []invoicing.Line{{ID: "line-1", Position: 1, Description: "Servicii", Unit: "BUC", VATRate: money.MustParse("19.0000"), VATValue: money.MustParse("285.0000"), Quantity: money.MustParse("1.0000"), UnitPrice: money.MustParse("1500.0000"), NetValue: money.MustParse("1500.0000"), TotalValue: money.MustParse("1785.0000")}},
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/invoices/inv-resolved", nil)
	response := httptest.NewRecorder()
	testHandler(item).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"amount":1785.2500`) {
		t.Fatalf("money is not a JSON number: %s", response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"vatRate":19.0000`) || !strings.Contains(response.Body.String(), `"quantity":1.0000`) {
		t.Fatalf("invoice lines are not serialized exactly: %s", response.Body.String())
	}
}

func TestListInvoicesReturnsCompleteFrontendDTOs(t *testing.T) {
	now := time.Date(2026, time.September, 8, 14, 0, 0, 0, time.UTC)
	item := &invoicing.Invoice{ID: "inv-list", ClientID: "client-alfa", SupplierName: "Furnizor listă", DocumentNumber: "LIST-1", IssueDate: now, Total: money.Money{Amount: money.MustParse("119.0000"), Currency: "RON"}, SPVReference: "SPV-LIST", PipelineStatus: invoicing.StatusAwaitingContract, SagaStatus: invoicing.SagaNotReady, Revision: 2, CreatedAt: now, UpdatedAt: now, ActiveTask: &validationtasks.Task{ID: "task-list", Type: validationtasks.TypeMissingContract, Status: validationtasks.StatusOpen, Revision: 1, CreatedAt: now, UpdatedAt: now}}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/invoices?clientId=client-alfa", nil)
	response := httptest.NewRecorder()
	testHandler(item).ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"id":"inv-list"`) || !strings.Contains(response.Body.String(), `"task":{"id":"task-list"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestGetInvoiceNotFoundUsesErrorEnvelope(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/invoices/missing", nil)
	request.Header.Set("X-Request-ID", "correlation-test")
	response := httptest.NewRecorder()
	testHandler(nil).ServeHTTP(response, request)
	var envelope errorDTO
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusNotFound || envelope.Code != "NOT_FOUND" || envelope.CorrelationID != "correlation-test" {
		t.Fatalf("unexpected response: status=%d envelope=%+v", response.Code, envelope)
	}
}

func TestListValidationTasksReturnsCompactInvoiceContext(t *testing.T) {
	now := time.Date(2026, time.September, 12, 10, 0, 0, 0, time.UTC)
	store := &validationTaskStore{items: []validationtasks.InboxItem{{
		Task:    validationtasks.Task{ID: "task-1", ClientID: "client-alfa", InvoiceID: "inv-1", Type: validationtasks.TypeMissingContract, Status: validationtasks.StatusOpen, Title: "Contract lipsă", Reason: "Nu există contract.", Revision: 1, CreatedAt: now, UpdatedAt: now},
		Invoice: validationtasks.InvoiceSummary{ID: "inv-1", ClientID: "client-alfa", SupplierName: "Furnizor", DocumentNumber: "INV-1", IssueDate: now, TotalAmount: "119.0000", Currency: "RON", SPVReference: "SPV-1", PipelineStatus: "AWAITING_CONTRACT", SagaStatus: "NOT_READY"},
		Client:  validationtasks.ClientSummary{ID: "client-alfa", Name: "Client Alfa", CUI: "RO-ALFA"},
	}}}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/validation-tasks?status=OPEN&type=MISSING_CONTRACT", nil)
	response := httptest.NewRecorder()
	testHandlerWithTasks(nil, store).ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"id":"task-1"`) || !strings.Contains(response.Body.String(), `"amount":119.0000`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestRequestMissingContractReturnsWaitingInvoice(t *testing.T) {
	now := time.Date(2026, time.September, 12, 10, 0, 0, 0, time.UTC)
	item := &invoicing.Invoice{
		ID: "inv-1", ClientID: "client-alfa", SupplierName: "Furnizor", DocumentNumber: "INV-1", IssueDate: now,
		Total: money.Money{Amount: money.MustParse("119.0000"), Currency: "RON"}, SPVReference: "SPV-1",
		PipelineStatus: invoicing.StatusAwaitingContract, SagaStatus: invoicing.SagaNotReady, Revision: 1, CreatedAt: now, UpdatedAt: now,
		ActiveTask: &validationtasks.Task{ID: "task-1", ClientID: "client-alfa", InvoiceID: "inv-1", Type: validationtasks.TypeMissingContract, Status: validationtasks.StatusOpen, Title: "Contract lipsă", Reason: "Nu există contract.", Revision: 1, CreatedAt: now, UpdatedAt: now},
	}
	store := &validationTaskStore{invoice: item}
	handler := testHandlerWithTasks(item, store)
	requestContract := func() *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/invoices/inv-1/contract-requests", strings.NewReader(`{"taskId":"task-1","expectedRevision":1}`))
		request.Header.Set("Idempotency-Key", "request-1")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}
	response := requestContract()
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"WAITING"`) || !strings.Contains(response.Body.String(), `"pipelineStatus":"AWAITING_CONTRACT"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	replay := requestContract()
	if replay.Code != http.StatusOK || !strings.Contains(replay.Body.String(), `"revision":2`) || len(store.requests) != 2 || store.requests[1].CommandID != "request-1" {
		t.Fatalf("replay status=%d body=%s requests=%+v", replay.Code, replay.Body.String(), store.requests)
	}
}

func TestContractReadEndpointsReturnFrontendDTOs(t *testing.T) {
	now := time.Date(2026, time.September, 15, 10, 0, 0, 0, time.UTC)
	store := &contractStore{
		items: []contractdomain.Contract{{
			ID: "contract-1", ClientID: "client-alfa", SupplierName: "Furnizor", SupplierCUI: "RO-1", Reference: "CTR-1",
			EffectiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), EffectiveTo: timePointer(time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)),
			Value: money.Money{Amount: money.MustParse("100.0000"), Currency: "RON"}, UnitType: "BUC", PaymentTerms: "30 zile", Revision: 1,
		}},
		invoices: []contractdomain.AssociatedInvoice{{ID: "invoice-1", ClientID: "client-alfa", SupplierName: "Furnizor", DocumentNumber: "INV-1", IssueDate: now, Total: money.Money{Amount: money.MustParse("119.0000"), Currency: "RON"}, SPVReference: "SPV-1", PipelineStatus: "EXPORTED", SagaStatus: "EXPORTED"}},
	}
	handler := testHandlerWithContracts(nil, store)
	for _, path := range []string{"/api/v1/contracts", "/api/v1/contracts/contract-1", "/api/v1/contracts/contract-1/invoices"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "CTR-1") && path != "/api/v1/contracts/contract-1/invoices" {
			t.Fatalf("path=%s status=%d body=%s", path, response.Code, response.Body.String())
		}
	}
}

func TestConfirmContractMatchPassesBothRevisionsAndIsHTTPIdempotent(t *testing.T) {
	now := time.Date(2026, time.September, 15, 10, 0, 0, 0, time.UTC)
	item := &invoicing.Invoice{
		ID: "invoice-1", ClientID: "client-alfa", SupplierName: "Furnizor", DocumentNumber: "INV-1", IssueDate: now,
		Total: money.Money{Amount: money.MustParse("119.0000"), Currency: "RON"}, SPVReference: "SPV-1",
		PipelineStatus: invoicing.StatusAwaitingMatchConfirm, SagaStatus: invoicing.SagaNotReady, Revision: 2, CreatedAt: now, UpdatedAt: now,
		ActiveTask: &validationtasks.Task{ID: "task-1", Type: validationtasks.TypeContractMatch, Status: validationtasks.StatusOpen, Revision: 1, CreatedAt: now, UpdatedAt: now},
	}
	store := &contractStore{invoice: item}
	handler := testHandlerWithContracts(item, store)
	for range 2 {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/invoices/invoice-1/contract-confirmations", strings.NewReader(`{"taskId":"task-1","contractId":"contract-1","expectedInvoiceRevision":2,"expectedTaskRevision":1}`))
		request.Header.Set("Idempotency-Key", "confirm-once")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"selectedContractId":"contract-1"`) {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
	}
	if len(store.confirmations) != 2 || store.confirmations[0].ExpectedInvoiceRevision != 2 || store.confirmations[0].ExpectedTaskRevision != 1 || store.invoice.Revision != 3 {
		t.Fatalf("confirmations=%+v revision=%d", store.confirmations, store.invoice.Revision)
	}
}

type module5HTTPStore struct {
	rule            rules.Rule
	invoice         *invoicing.Invoice
	versionCommand  rules.CreateVersionCommand
	overrideCommand rules.CreateOverrideCommand
	versionErr      error
	overrideErr     error
	reviewCommand   classificationdomain.ReviewCommand
	reviewErr       error
	reviewKeys      map[string]bool
	reviewMutations int
}

func (s *module5HTTPStore) ListRules(context.Context, rules.Filter) ([]rules.Rule, error) {
	return []rules.Rule{s.rule}, nil
}
func (s *module5HTTPStore) GetRule(_ context.Context, id string) (*rules.Rule, error) {
	if id != s.rule.ID {
		return nil, apperrors.ErrNotFound
	}
	result := s.rule
	return &result, nil
}
func (s *module5HTTPStore) CreateRuleVersion(_ context.Context, command rules.CreateVersionCommand, now time.Time) (*rules.Rule, bool, error) {
	s.versionCommand = command
	if s.versionErr != nil {
		return nil, false, s.versionErr
	}
	result := s.rule
	result.Revision++
	result.Versions = append(result.Versions, rules.Version{Version: 2, Criteria: command.Criteria, Result: command.Result, LegalBasis: rules.LegalBasisPlaceholder, EffectiveFrom: command.EffectiveFrom, CreatedByDisplay: command.ActorDisplay, CreatedAt: now})
	s.rule = result
	return &result, true, nil
}
func (s *module5HTTPStore) CreateClientOverride(_ context.Context, command rules.CreateOverrideCommand, now time.Time) (*rules.Rule, bool, error) {
	s.overrideCommand = command
	if s.overrideErr != nil {
		return nil, false, s.overrideErr
	}
	result := s.rule
	result.ID = "override"
	result.Scope = rules.ScopeClientOverride
	result.ClientID = &command.ClientID
	result.ParentRuleID = &command.ParentRuleID
	result.Versions = []rules.Version{{Version: 1, Criteria: command.Criteria, Result: command.Result, LegalBasis: rules.LegalBasisPlaceholder, EffectiveFrom: command.EffectiveFrom, CreatedByDisplay: command.ActorDisplay, CreatedAt: now}}
	return &result, true, nil
}
func (*module5HTTPStore) ProcessCommandCommitted(context.Context, string) (bool, error) {
	return false, nil
}
func (*module5HTTPStore) LoadClassificationInput(context.Context, string) (classificationdomain.InvoiceContext, error) {
	return classificationdomain.InvoiceContext{}, nil
}
func (*module5HTTPStore) ApplyClassification(context.Context, classificationdomain.ProcessCommand, classificationdomain.Result, time.Time) (bool, error) {
	return true, nil
}
func (s *module5HTTPStore) ReviewClassification(_ context.Context, command classificationdomain.ReviewCommand, now time.Time) (bool, error) {
	s.reviewCommand = command
	if s.reviewErr != nil {
		return false, s.reviewErr
	}
	if s.reviewKeys == nil {
		s.reviewKeys = make(map[string]bool)
	}
	if s.reviewKeys[command.CommandID] {
		return false, nil
	}
	s.reviewKeys[command.CommandID] = true
	s.reviewMutations++
	item := s.invoice.ActiveTask.ClassificationItems[0]
	item.Status = classificationdomain.ReviewAccepted
	item.EffectiveValue = &item.ProposedValue
	item.Revision++
	s.invoice.ActiveTask.ClassificationItems[0] = item
	if len(s.invoice.Lines) > 0 && len(s.invoice.Lines[0].Classifications) > 0 {
		s.invoice.Lines[0].Classifications[0] = item
	}
	s.invoice.ActiveTask.Status = validationtasks.StatusResolved
	s.invoice.PipelineStatus = invoicing.StatusReadyForSAGA
	s.invoice.SagaStatus = invoicing.SagaReady
	s.invoice.Revision++
	return true, nil
}

func testHandlerWithModule5(store *module5HTTPStore) http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return withTestActor(New(clients.NewService(clientReader{}), invoicing.NewService(invoiceReader{item: store.invoice}), nil, nil, classificationdomain.NewService(store, nil, nil), rules.NewService(store, nil), readyStub{}, logger), requestactor.Actor{ID: "test-accountant", Display: "Test", Persona: "CONTABIL", AllClients: true})
}

func TestRuleReadAndDomainMutationEndpoints(t *testing.T) {
	now := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
	store := &module5HTTPStore{rule: rules.Rule{ID: "rule-1", Reference: "REG-DEMO", Name: "Regulă demo", Category: rules.CategoryAccount, Scope: rules.ScopeGlobal, Revision: 1, Versions: []rules.Version{{Version: 1, Criteria: "Criteriu demo", Result: "Rezultat demo", Explanation: "Explicație demo", LegalBasis: rules.LegalBasisPlaceholder, EffectiveFrom: now, CreatedByDisplay: "Sistem", CreatedAt: now}}}}
	handler := testHandlerWithModule5(store)
	for _, path := range []string{"/api/v1/rules", "/api/v1/rules/rule-1"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), rules.LegalBasisPlaceholder) {
			t.Fatalf("path=%s status=%d body=%s", path, response.Code, response.Body.String())
		}
	}
	versionRequest := httptest.NewRequest(http.MethodPost, "/api/v1/rules/rule-1/versions", strings.NewReader(`{"expectedRevision":1,"criteria":"Criteriu nou","result":"Rezultat nou","effectiveFrom":"2027-01-01"}`))
	versionRequest.Header.Set("Idempotency-Key", "version-http")
	versionResponse := httptest.NewRecorder()
	handler.ServeHTTP(versionResponse, versionRequest)
	if versionResponse.Code != http.StatusOK || store.versionCommand.ExpectedRevision != 1 {
		t.Fatalf("status=%d body=%s command=%+v", versionResponse.Code, versionResponse.Body.String(), store.versionCommand)
	}
	overrideRequest := httptest.NewRequest(http.MethodPost, "/api/v1/rules/rule-1/client-overrides", strings.NewReader(`{"clientId":"client-alfa","criteria":"Criteriu override","result":"Rezultat override","effectiveFrom":"2027-01-01"}`))
	overrideRequest.Header.Set("Idempotency-Key", "override-http")
	overrideResponse := httptest.NewRecorder()
	handler.ServeHTTP(overrideResponse, overrideRequest)
	if overrideResponse.Code != http.StatusOK || store.overrideCommand.ClientID != "client-alfa" {
		t.Fatalf("status=%d body=%s command=%+v", overrideResponse.Code, overrideResponse.Body.String(), store.overrideCommand)
	}
}

func TestClassificationDecisionEndpointPassesAllRevisions(t *testing.T) {
	now := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
	decision := classificationdomain.Decision{ID: "classification-1", InvoiceLineID: "line-1", LineLabel: "Linia 1", Dimension: classificationdomain.DimensionAccount, ProposedValue: "Demo", Confidence: "Opaque", Explanation: "Demo", LegalBasis: rules.LegalBasisPlaceholder, Status: classificationdomain.ReviewPending, Revision: 1}
	item := &invoicing.Invoice{ID: "invoice-1", ClientID: "client-alfa", SupplierName: "Furnizor", DocumentNumber: "INV", IssueDate: now, Total: money.Money{Amount: money.MustParse("119.0000"), Currency: "RON"}, SPVReference: "SPV", PipelineStatus: invoicing.StatusAwaitingReview, SagaStatus: invoicing.SagaNotReady, Revision: 3, CreatedAt: now, UpdatedAt: now, Lines: []invoicing.Line{{ID: "line-1", Position: 1, Description: "Demo", Unit: "buc.", VATRate: money.MustParse("19"), VATValue: money.MustParse("19"), Quantity: money.MustParse("1"), UnitPrice: money.MustParse("100"), NetValue: money.MustParse("100"), TotalValue: money.MustParse("119"), Classifications: []classificationdomain.Decision{decision}}}, ActiveTask: &validationtasks.Task{ID: "task-1", Type: validationtasks.TypeClassification, Status: validationtasks.StatusOpen, Revision: 1, CreatedAt: now, UpdatedAt: now, ClassificationItems: []classificationdomain.Decision{decision}}}
	store := &module5HTTPStore{invoice: item}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/invoices/invoice-1/classification-decisions", strings.NewReader(`{"taskId":"task-1","classificationId":"classification-1","expectedInvoiceRevision":3,"expectedTaskRevision":1,"expectedClassificationRevision":1}`))
	request.Header.Set("Idempotency-Key", "review-http")
	response := httptest.NewRecorder()
	testHandlerWithModule5(store).ServeHTTP(response, request)
	if response.Code != http.StatusOK || store.reviewCommand.ExpectedInvoiceRevision != 3 || store.reviewCommand.ExpectedTaskRevision != 1 || store.reviewCommand.ExpectedClassificationRevision != 1 || !strings.Contains(response.Body.String(), `"pipelineStatus":"READY_FOR_SAGA"`) || !strings.Contains(response.Body.String(), rules.LegalBasisPlaceholder) {
		t.Fatalf("status=%d body=%s command=%+v", response.Code, response.Body.String(), store.reviewCommand)
	}
	request = httptest.NewRequest(http.MethodPost, "/api/v1/invoices/invoice-1/classification-decisions", strings.NewReader(`{"taskId":"task-1","classificationId":"classification-1","expectedInvoiceRevision":3,"expectedTaskRevision":1,"expectedClassificationRevision":1}`))
	request.Header.Set("Idempotency-Key", "review-http")
	response = httptest.NewRecorder()
	testHandlerWithModule5(store).ServeHTTP(response, request)
	if response.Code != http.StatusOK || store.reviewMutations != 1 {
		t.Fatalf("replay status=%d mutations=%d body=%s", response.Code, store.reviewMutations, response.Body.String())
	}
}

func TestClassificationDecisionEndpointMapsStaleAndInvalidOwnership(t *testing.T) {
	now := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
	invoiceItem := &invoicing.Invoice{ID: "invoice-1", ClientID: "client-alfa", SupplierName: "Furnizor", DocumentNumber: "INV", IssueDate: now, Total: money.Money{Amount: money.MustParse("119"), Currency: "RON"}, SPVReference: "SPV", PipelineStatus: invoicing.StatusAwaitingReview, SagaStatus: invoicing.SagaNotReady, Revision: 3, CreatedAt: now, UpdatedAt: now}
	for _, testCase := range []struct {
		name string
		err  error
		code int
	}{
		{name: "stale revisions", err: classificationdomain.ErrStaleReview, code: http.StatusConflict},
		{name: "cross-client context", err: apperrors.ErrValidation, code: http.StatusBadRequest},
		{name: "missing context", err: apperrors.ErrNotFound, code: http.StatusNotFound},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			store := &module5HTTPStore{invoice: invoiceItem, reviewErr: testCase.err}
			request := httptest.NewRequest(http.MethodPost, "/api/v1/invoices/invoice-1/classification-decisions", strings.NewReader(`{"taskId":"task-other","classificationId":"classification-other","expectedInvoiceRevision":3,"expectedTaskRevision":1,"expectedClassificationRevision":1}`))
			request.Header.Set("Idempotency-Key", "invalid-review")
			response := httptest.NewRecorder()
			testHandlerWithModule5(store).ServeHTTP(response, request)
			if response.Code != testCase.code {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestRuleMutationEndpointsMapStaleAndDuplicateCommands(t *testing.T) {
	now := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
	rule := rules.Rule{ID: "rule-1", Reference: "REG-DEMO", Name: "Demo", Category: rules.CategoryAccount, Scope: rules.ScopeGlobal, Revision: 2, Versions: []rules.Version{{Version: 1, Criteria: "Demo", Result: "Demo", Explanation: "Demo", LegalBasis: rules.LegalBasisPlaceholder, EffectiveFrom: now, CreatedByDisplay: "System", CreatedAt: now}}}
	for _, testCase := range []struct {
		name string
		path string
		body string
		set  func(*module5HTTPStore)
	}{
		{name: "stale version", path: "/api/v1/rules/rule-1/versions", body: `{"expectedRevision":1,"criteria":"Nou","result":"Nou","effectiveFrom":"2027-01-01"}`, set: func(store *module5HTTPStore) { store.versionErr = rules.ErrRuleVersionStale }},
		{name: "duplicate override", path: "/api/v1/rules/rule-1/client-overrides", body: `{"clientId":"client-alfa","criteria":"Nou","result":"Nou","effectiveFrom":"2027-01-01"}`, set: func(store *module5HTTPStore) { store.overrideErr = rules.ErrOverrideAlreadyExists }},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			store := &module5HTTPStore{rule: rule}
			testCase.set(store)
			request := httptest.NewRequest(http.MethodPost, testCase.path, strings.NewReader(testCase.body))
			request.Header.Set("Idempotency-Key", "rule-conflict")
			response := httptest.NewRecorder()
			testHandlerWithModule5(store).ServeHTTP(response, request)
			if response.Code != http.StatusConflict {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}
