//go:build integration

package postgres

import (
	"archive/zip"
	"bytes"
	"diana-contabilitate/backend/ent/accountingrulepack"
	"diana-contabilitate/backend/ent/classificationrule"
	"diana-contabilitate/backend/ent/clientaccountingprofile"
	"diana-contabilitate/backend/ent/contract"
	"diana-contabilitate/backend/ent/contractmatchcandidate"
	"diana-contabilitate/backend/ent/contractmatchrun"
	"diana-contabilitate/backend/ent/invoice"
	"diana-contabilitate/backend/ent/invoicecontractassociation"
	"diana-contabilitate/backend/ent/ruleversion"
	"diana-contabilitate/backend/ent/sagaexportattempt"
	"diana-contabilitate/backend/ent/spvconnection"
	"diana-contabilitate/backend/ent/spvsourcedocument"
	"diana-contabilitate/backend/ent/validationtask"
	"diana-contabilitate/backend/internal/accounting"
	"diana-contabilitate/backend/internal/accountingtest"
	"diana-contabilitate/backend/internal/apperrors"
	"diana-contabilitate/backend/internal/classification"
	"diana-contabilitate/backend/internal/contracts"
	"diana-contabilitate/backend/internal/invoicing"
	"diana-contabilitate/backend/internal/saga"
	"diana-contabilitate/backend/internal/spv"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func domainReleaseFixture(t *testing.T, tc *module5TestContext, edit func(*accounting.Pack)) (*accounting.SourceFacts, *accounting.LineFacts, *accounting.Profile, *accounting.Pack) {
	t.Helper()
	f, l, p, pack := accountingtest.Fixture(tc.clientID)
	client, err := tc.store.Client.AccountingClient.Get(tc.ctx, tc.clientID)
	if err != nil {
		t.Fatal(err)
	}
	f.BuyerVATID = client.Cui
	if edit != nil {
		edit(pack)
	}
	for _, r := range pack.Rules {
		if _, err := tc.store.Client.ClassificationRule.Create().SetID(r.ID).SetReference(r.ID).SetName("TEST_ONLY " + r.Dimension).SetCategory(classificationrule.Category(r.Dimension)).SetScope(classificationrule.ScopeGLOBAL).SetCreationKey(r.ID).SetCreatedAt(tc.now).SetUpdatedAt(tc.now).Save(tc.ctx); err != nil {
			t.Fatal(err)
		}
		tc.rules = append(tc.rules, r.ID)
		if _, err := tc.store.Client.RuleVersion.Create().SetID(r.VersionID).SetRuleID(r.ID).SetVersion(r.Version).SetModelVersion(accounting.ModelVersion).SetDomainRule(&r).SetCriteria("TEST_ONLY exact predicate").SetResult(r.Result.Text()).SetExplanation(r.Explanation).SetLegalBasis(r.LegalBasis).SetMatchKind(ruleversion.MatchKindNO_AUTOMATION).SetRulePackVersion(pack.ID + "/1").SetEffectiveFrom(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)).SetCreatedByDisplay("TEST_ONLY").SetCommandKey(r.VersionID).SetCreatedAt(tc.now).Save(tc.ctx); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := tc.store.Client.ClientAccountingProfile.Create().SetID(p.ID).SetClientID(tc.clientID).SetVersion(1).SetPayload(p).SetCreatedAt(tc.now).Save(tc.ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := tc.store.Client.AccountingRulePack.Create().SetID(pack.ID).SetClientID(tc.clientID).SetVersion(1).SetPayload(pack).SetCreatedAt(tc.now).Save(tc.ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = tc.store.Client.SagaExportAttempt.Delete().Where(sagaexportattempt.ClientIDEQ(tc.clientID)).Exec(tc.ctx)
		_, _ = tc.store.Client.ValidationTask.Delete().Where(validationtask.ClientIDEQ(tc.clientID)).Exec(tc.ctx)
		_, _ = tc.store.Client.SPVSourceDocument.Delete().Where(spvsourcedocument.ClientIDEQ(tc.clientID)).Exec(tc.ctx)
		_, _ = tc.store.Client.SPVConnection.Delete().Where(spvconnection.ClientIDEQ(tc.clientID)).Exec(tc.ctx)
		_, _ = tc.store.Client.InvoiceContractAssociation.Delete().Where(invoicecontractassociation.ClientIDEQ(tc.clientID)).Exec(tc.ctx)
		runs, _ := tc.store.Client.ContractMatchRun.Query().Where(contractmatchrun.ClientIDEQ(tc.clientID)).IDs(tc.ctx)
		_, _ = tc.store.Client.ContractMatchCandidate.Delete().Where(contractmatchcandidate.MatchRunIDIn(runs...)).Exec(tc.ctx)
		_, _ = tc.store.Client.ContractMatchRun.Delete().Where(contractmatchrun.ClientIDEQ(tc.clientID)).Exec(tc.ctx)
		_, _ = tc.store.Client.Contract.Delete().Where(contract.ClientIDEQ(tc.clientID)).Exec(tc.ctx)
		_, _ = tc.store.Client.AccountingRulePack.Delete().Where(accountingrulepack.ClientIDEQ(tc.clientID)).Exec(tc.ctx)
		_, _ = tc.store.Client.ClientAccountingProfile.Delete().Where(clientaccountingprofile.ClientIDEQ(tc.clientID)).Exec(tc.ctx)
	})
	return f, l, p, pack
}
func domainPersistedInvoice(t *testing.T, tc *module5TestContext, f *accounting.SourceFacts, l *accounting.LineFacts, suffix string) string {
	t.Helper()
	id := "domain-" + tc.clientID + "-" + suffix
	if _, err := tc.store.Client.Invoice.Create().SetID(id).SetClientID(tc.clientID).SetSupplierName("TEST_ONLY supplier").SetSupplierCui("RO-TEST-SUPPLIER").SetNormalizedSupplierCui("RO-TEST-SUPPLIER").SetDocumentNumber(id).SetNormalizedDocumentNumber(id).SetIssueDate(tc.now).SetIssueDay(invoicing.InvoiceIssueDay(tc.now)).SetTotalAmount("121").SetCurrency("RON").SetSpvReference(id).SetIngestionSource("TEST_ONLY").SetExternalDeliveryID(id).SetModelVersion(accounting.ModelVersion).SetSourceFacts(f).SetPipelineStatus(invoice.PipelineStatusLINES_READ).SetSagaStatus(invoice.SagaStatusNOT_READY).SetCreatedAt(tc.now).SetUpdatedAt(tc.now).Save(tc.ctx); err != nil {
		t.Fatal(err)
	}
	tc.invoices = append(tc.invoices, id)
	if _, err := tc.store.Client.InvoiceLine.Create().SetID(id + "-line").SetInvoiceID(id).SetPosition(1).SetDescription("TEST_ONLY service").SetUnit("H87").SetQuantity("1").SetUnitPrice("100").SetNetValue("100").SetVatRate("21").SetVatValue("21").SetTotalValue("121").SetSourceFacts(l).Save(tc.ctx); err != nil {
		t.Fatal(err)
	}
	return id
}
func TestDomainPersistedFourDecisionReplayConcurrencyAndSnapshot(t *testing.T) {
	tc := newModule5TestContext(t)
	f, l, _, pack := domainReleaseFixture(t, tc, nil)
	id := domainPersistedInvoice(t, tc, f, l, "auto")
	service := classification.NewService(tc.store, classification.DomainPolicy{AllowTestOnly: true}, func() time.Time { return tc.now })
	command := classification.ProcessCommand{InvoiceID: id, ExpectedRevision: 1, CommandID: id + ":classify"}
	var count atomic.Int32
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, changed, err := service.ProcessInvoice(tc.ctx, command)
			if err != nil && !errors.Is(err, apperrors.ErrConflict) {
				t.Errorf("concurrent classification: %v", err)
			}
			if changed {
				count.Add(1)
			}
		}()
	}
	wg.Wait()
	if count.Load() != 1 {
		t.Fatal(count.Load())
	}
	item, err := tc.store.GetInvoice(tc.ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if item.PipelineStatus != invoicing.StatusReadyForSAGA || len(item.Lines[0].Classifications) != 4 || item.ActiveTask != nil {
		t.Fatal(item)
	}
	_, changed, err := service.ProcessInvoice(tc.ctx, command)
	if err != nil || changed {
		t.Fatal("replay", changed, err)
	}
	// New overlapping releases affect future selection, never this saved snapshot.
	next := *pack
	next.ID += "-v2"
	next.Version = 2
	if _, err := tc.store.Client.AccountingRulePack.Create().SetID(next.ID).SetClientID(tc.clientID).SetVersion(2).SetPayload(&next).SetCreatedAt(tc.now).Save(tc.ctx); err != nil {
		t.Fatal(err)
	}
	reloaded, _ := tc.store.GetInvoice(tc.ctx, id)
	if reloaded.AccountingSnapshot.Pack.ID != pack.ID {
		t.Fatal("historical release replaced")
	}
	exporter := saga.NewTestOnlyFileExporter(tc.store, func() time.Time { return tc.now })
	for range 2 {
		if _, err := exporter.Export(tc.ctx, reloaded, "replay"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := saga.NewFileExporter(tc.store, func() time.Time { return tc.now }).Export(tc.ctx, reloaded, "production-replay"); err == nil {
		t.Fatal("production reused a cached synthetic artifact")
	}
	attempts, _ := tc.store.Client.SagaExportAttempt.Query().Where(sagaexportattempt.InvoiceIDEQ(id)).Count(tc.ctx)
	if attempts != 1 {
		t.Fatal(attempts)
	}
	artifact, err := exporter.Artifact(tc.ctx, id, tc.clientID)
	if err != nil || strings.Contains(string(artifact.Payload), "TipDeducere") {
		t.Fatal(err)
	}
	// Database-level source and released snapshot are immutable, independently of Ent.
	if _, err := tc.store.DB.ExecContext(tc.ctx, "UPDATE invoices SET source_facts='{}'::jsonb WHERE id=$1", id); err == nil {
		t.Fatal("source mutation permitted")
	}
	if _, err := tc.store.DB.ExecContext(tc.ctx, "UPDATE invoices SET accounting_snapshot='{}'::jsonb WHERE id=$1", id); err == nil {
		t.Fatal("snapshot mutation permitted")
	}
	if _, err := tc.store.DB.ExecContext(tc.ctx, "UPDATE accounting_rule_packs SET payload='{}'::jsonb WHERE id=$1", pack.ID); err == nil {
		t.Fatal("release mutation permitted")
	}
}
func TestDomainHumanTypedValidationUnequalMappingAndRecovery(t *testing.T) {
	tc := newModule5TestContext(t)
	f, l, _, _ := domainReleaseFixture(t, tc, func(p *accounting.Pack) {
		p.Rules[3].Result = accounting.Value{Kind: "NONDEDUCTIBLE", Reason: "TEST_ONLY policy"}
	})
	id := domainPersistedInvoice(t, tc, f, l, "review")
	service := classification.NewService(tc.store, classification.DomainPolicy{AllowTestOnly: true}, func() time.Time { return tc.now })
	if _, _, err := service.ProcessInvoice(tc.ctx, classification.ProcessCommand{InvoiceID: id, ExpectedRevision: 1, CommandID: id}); err != nil {
		t.Fatal(err)
	}
	item, _ := tc.store.GetInvoice(tc.ctx, id)
	if item.PipelineStatus != invoicing.StatusAwaitingReview || item.ActiveTask == nil || item.ReadinessReason == "" || len(item.ActiveTask.ClassificationItems) != 4 {
		t.Fatal("false-ready or uneditable mapping task", item)
	}
	decision := classification.Decision{}
	for _, d := range item.Lines[0].Classifications {
		if d.Dimension == classification.DimensionExpenseTax {
			decision = d
		}
	}
	if decision.ID == "" {
		t.Fatal("expense dimension absent")
	}
	command := classification.ReviewCommand{InvoiceID: id, TaskID: item.ActiveTask.ID, ClassificationID: decision.ID, ExpectedInvoiceRevision: item.Revision, ExpectedTaskRevision: item.ActiveTask.Revision, ExpectedClassificationRevision: decision.Revision, CommandID: id + ":invalid", ActorDisplay: "TEST_ONLY accountant", Reason: "TEST_ONLY review", TypedValue: &accounting.Value{Kind: "arbitrary"}}
	if _, err := service.Review(tc.ctx, command); !errors.Is(err, apperrors.ErrValidation) {
		t.Fatal(err)
	}
	command.CommandID = id + ":valid-unsupported"
	command.TypedValue = &accounting.Value{Kind: "LIMITED", Percentage: accountingtest.Rate("50"), Basis: "TEST_ONLY utilization"}
	if _, err := service.Review(tc.ctx, command); err != nil {
		t.Fatal(err)
	}
	blocked, _ := tc.store.GetInvoice(tc.ctx, id)
	if blocked.PipelineStatus != invoicing.StatusAwaitingReview || blocked.ActiveTask == nil {
		t.Fatal("unsupported human result unlocked export")
	}
	if changed, err := tc.store.ReviewClassification(tc.ctx, command, tc.now); err != nil || changed {
		t.Fatal("review replay", changed, err)
	}
	command.CommandID = id + ":recover"
	command.ExpectedTaskRevision = blocked.ActiveTask.Revision
	for _, d := range blocked.Lines[0].Classifications {
		if d.ID == decision.ID {
			command.ExpectedClassificationRevision = d.Revision
		}
	}
	command.TypedValue = &accounting.Value{Kind: "FULLY_DEDUCTIBLE"}
	if _, err := service.Review(tc.ctx, command); err != nil {
		t.Fatal(err)
	}
	ready, _ := tc.store.GetInvoice(tc.ctx, id)
	if ready.PipelineStatus != invoicing.StatusReadyForSAGA || ready.ActiveTask != nil {
		t.Fatal("valid review failed readiness", ready)
	}
	if _, err := service.Review(tc.ctx, command); err != nil {
		t.Fatal("committed review retry must be harmless", err)
	}
}
func TestDomainReleaseRejectsSyntheticAndUnapproved(t *testing.T) {
	tc := newModule5TestContext(t)
	_, _, p, pack := accountingtest.Fixture(tc.clientID)
	if tc.store.ApproveAccountingProfile(tc.ctx, *p) == nil || tc.store.ReleaseAccountingPack(tc.ctx, *pack) == nil {
		t.Fatal("TEST_ONLY operator promotion")
	}
	p.TestOnly = false
	p.Approval.Evidence = nil
	if tc.store.ApproveAccountingProfile(tc.ctx, *p) == nil {
		t.Fatal("missing approval")
	}
}
func TestDomainPersistedMissingVersusZero(t *testing.T) {
	tc := newModule5TestContext(t)
	f, l, _, _ := domainReleaseFixture(t, tc, nil)
	l.Rate = nil
	missing := domainPersistedInvoice(t, tc, f, l, "missing")
	l2 := *l
	l2.Rate = accountingtest.Rate("0")
	zero := domainPersistedInvoice(t, tc, f, &l2, "zero")
	a, _ := tc.store.GetInvoice(tc.ctx, missing)
	b, _ := tc.store.GetInvoice(tc.ctx, zero)
	if a.Lines[0].SourceFacts.Rate != nil || b.Lines[0].SourceFacts.Rate == nil || !b.Lines[0].SourceFacts.Rate.Equal("0") {
		t.Fatal("rate presence lost")
	}
}
func TestDomainFullDeterministicSPVContractClassificationSAGAPipeline(t *testing.T) {
	tc := newModule5TestContext(t)
	domainReleaseFixture(t, tc, nil)
	if _, err := tc.store.Client.AccountingClient.UpdateOneID(tc.clientID).SetCui("RO-TEST-BUYER").Save(tc.ctx); err != nil {
		t.Fatal(err)
	}
	cid := "TEST_ONLY-connection-" + tc.clientID
	if _, err := tc.store.Client.SPVConnection.Create().SetID(cid).SetClientID(tc.clientID).SetCif("RO-TEST-BUYER").SetEnvironment(spvconnection.EnvironmentTEST).SetAccessTokenCiphertext("TEST_ONLY token").SetRefreshTokenCiphertext("TEST_ONLY refresh").SetAccessTokenExpiresAt(time.Now().Add(time.Hour)).SetCreatedAt(tc.now).SetUpdatedAt(tc.now).Save(tc.ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := tc.store.Client.Contract.Create().SetID("TEST_ONLY-contract-" + tc.clientID).SetClientID(tc.clientID).SetSupplierName("TEST_ONLY supplier").SetSupplierCui("RO-TEST-SUPPLIER").SetNormalizedSupplierCui("RO-TEST-SUPPLIER").SetReference("TEST_ONLY_CONTRACT").SetEffectiveFrom(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)).SetEffectiveTo(time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)).SetTotalValue("121").SetCurrency("RON").SetUnitType("H87").SetPaymentTerms("TEST_ONLY").SetCreatedAt(tc.now).SetUpdatedAt(tc.now).Save(tc.ctx); err != nil {
		t.Fatal(err)
	}
	var raw bytes.Buffer
	zw := zip.NewWriter(&raw)
	w, err := zw.Create("invoice.xml")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte(accountingtest.XML("<Percent>21</Percent>", "")))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/listaMesajePaginatieFactura":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"mesaje":[{"id":"900001","id_solicitare":"900002","tip":"FACTURA PRIMITA","data_creare":"202609151200"}],"numar_total_pagini":1}`))
		case "/descarcare":
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(raw.Bytes())
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	exporter := saga.NewTestOnlyFileExporter(tc.store, func() time.Time { return tc.now })
	pipeline := invoicing.NewPipelineService(tc.store, exporter, func() time.Time { return tc.now })
	pipeline.SetContractMatchingProcessor(contracts.NewService(tc.store, nil, func() time.Time { return tc.now }))
	pipeline.SetClassificationProcessor(classification.NewService(tc.store, classification.DomainPolicy{AllowTestOnly: true}, func() time.Time { return tc.now }))
	service := spv.NewService(tc.store, spv.NewHTTPClient(server.Client(), server.URL, server.URL), spv.UBLParser{}, pipeline, clearTestCipher{}, spv.ServiceConfig{InitialWindow: 60 * 24 * time.Hour, Overlap: 72 * time.Hour})
	result, err := service.Sync(tc.ctx, cid)
	if err != nil || len(result.Documents) != 1 {
		t.Fatal(err, result)
	}
	id, created, err := service.ProcessDocument(tc.ctx, result.Documents[0].ID, "TEST_ONLY worker")
	if err != nil || !created {
		t.Fatal(err)
	}
	tc.invoices = append(tc.invoices, id)
	// Dispatcher is scoped to this aggregate, exercising each native transition.
	scoped := invoicing.NewPipelineService(scopedPipelineStore{Store: tc.store, aggregateID: id}, exporter, func() time.Time { return tc.now.Add(24 * time.Hour) })
	scoped.SetContractMatchingProcessor(contracts.NewService(tc.store, nil, func() time.Time { return tc.now }))
	scoped.SetClassificationProcessor(classification.NewService(tc.store, classification.DomainPolicy{AllowTestOnly: true}, func() time.Time { return tc.now }))
	if err := scoped.Drain(tc.ctx, 20); err != nil {
		t.Fatal(err)
	}
	item, err := tc.store.GetInvoice(tc.ctx, id)
	if err != nil || item.PipelineStatus != invoicing.StatusExporting || item.AccountingSnapshot.ContractID == "" || len(item.Lines[0].Classifications) != 4 {
		t.Fatalf("full pipeline: %+v %v", item, err)
	}
	source, err := tc.store.Client.SPVSourceDocument.Get(tc.ctx, result.Documents[0].ID)
	if err != nil || !bytes.Equal(source.RawDocument, raw.Bytes()) || source.ContentSha256 == nil || item.SourceFacts.SourceHash != *source.ContentSha256 {
		t.Fatal("raw/source lineage", err)
	}
	artifact, err := exporter.Artifact(tc.ctx, id, tc.clientID)
	if err != nil || artifact.ClassificationSnapshot["model_version"] != accounting.ModelVersion {
		t.Fatal("final domain artifact", err)
	}
	if _, err := saga.Generate(item, saga.ClientIdentity{ID: tc.clientID, Name: "TEST_ONLY", CUI: "RO-TEST-BUYER"}); err == nil {
		t.Fatal("production adapter accepted TEST_ONLY pipeline")
	}
}

func TestDomainReleaseDuringEvaluationRetainsSelectedSnapshot(t *testing.T) {
	tc := newModule5TestContext(t)
	f, l, _, pack := domainReleaseFixture(t, tc, nil)
	id := domainPersistedInvoice(t, tc, f, l, "snapshot")
	input, err := tc.store.LoadClassificationInput(tc.ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	result, err := (classification.DomainPolicy{AllowTestOnly: true}).Evaluate(input)
	if err != nil {
		t.Fatal(err)
	}
	newer := *pack
	newer.ID += "-concurrent"
	newer.Version = 2
	if _, err := tc.store.Client.AccountingRulePack.Create().SetID(newer.ID).SetClientID(tc.clientID).SetVersion(2).SetPayload(&newer).SetCreatedAt(tc.now).Save(tc.ctx); err != nil {
		t.Fatal(err)
	}
	if changed, err := tc.store.ApplyClassification(tc.ctx, classification.ProcessCommand{InvoiceID: id, ExpectedRevision: 1, CommandID: id}, result, tc.now); err != nil || !changed {
		t.Fatal(err)
	}
	item, _ := tc.store.GetInvoice(tc.ctx, id)
	if item.AccountingSnapshot.Pack.ID != pack.ID || item.PipelineStatus != invoicing.StatusReadyForSAGA {
		t.Fatal("release changed in-flight evaluation")
	}
	fresh := domainPersistedInvoice(t, tc, f, l, "overlap")
	input, err = tc.store.LoadClassificationInput(tc.ctx, fresh)
	if err != nil {
		t.Fatal(err)
	}
	if input.Snapshot.Pack != nil {
		t.Fatal("overlapping release selected arbitrarily")
	}
}
