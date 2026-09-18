package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"diana-contabilitate/backend/ent"
	"diana-contabilitate/backend/ent/accountingclient"
	"diana-contabilitate/backend/ent/activityevent"
	"diana-contabilitate/backend/ent/classificationrule"
	entcontract "diana-contabilitate/backend/ent/contract"
	"diana-contabilitate/backend/ent/contractmatchcandidate"
	"diana-contabilitate/backend/ent/contractmatchrun"
	"diana-contabilitate/backend/ent/invoice"
	"diana-contabilitate/backend/ent/invoicecontractassociation"
	"diana-contabilitate/backend/ent/invoiceline"
	"diana-contabilitate/backend/ent/lineclassification"
	"diana-contabilitate/backend/ent/outboxentry"
	"diana-contabilitate/backend/ent/ruleversion"
	"diana-contabilitate/backend/ent/sagaexportattempt"
	"diana-contabilitate/backend/ent/validationtask"
	"diana-contabilitate/backend/internal/accounting"
	"diana-contabilitate/backend/internal/accountingtest"
	classificationdomain "diana-contabilitate/backend/internal/classification"
	contractdomain "diana-contabilitate/backend/internal/contracts"
	"diana-contabilitate/backend/internal/invoicing"
	"diana-contabilitate/backend/internal/money"
	"diana-contabilitate/backend/internal/platform/config"
	"diana-contabilitate/backend/internal/platform/postgres"
	"diana-contabilitate/backend/internal/rules"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	if !demoSeedingAllowed(cfg.Environment) {
		slog.Error("devseed may load demo fixtures only in development/test")
		os.Exit(1)
	}
	store, err := postgres.Open(cfg.DatabaseURL)
	if err != nil {
		slog.Error("database setup failed", "error", err)
		os.Exit(1)
	}
	defer store.Close()
	if err := seed(context.Background(), store); err != nil {
		slog.Error("seed failed", "error", err)
		os.Exit(1)
	}
}

func seed(ctx context.Context, store *postgres.Store) error {
	now := time.Date(2026, time.September, 8, 14, 0, 0, 0, time.UTC)
	clients := []struct{ id, name, cui string }{
		{"client-alfa", "Client Demo Alfa SRL", "RO-DEMO-ALFA-001"},
		{"client-beta", "Client Demo Beta SRL", "RO-DEMO-BETA-002"},
	}
	for _, item := range clients {
		exists, err := store.Client.AccountingClient.Query().Where(accountingclient.IDEQ(item.id)).Exist(ctx)
		if err != nil {
			return err
		}
		if !exists {
			if _, err := store.Client.AccountingClient.Create().SetID(item.id).SetName(item.name).SetCui(item.cui).SetCreatedAt(now).SetUpdatedAt(now).Save(ctx); err != nil {
				return err
			}
		}
	}

	exists, err := store.Client.Invoice.Query().Where(invoice.IDEQ("inv-resolved")).Exist(ctx)
	if err != nil {
		return err
	}
	if !exists {
		due := time.Date(2026, time.October, 8, 14, 0, 0, 0, time.UTC)
		created, createErr := store.Client.Invoice.Create().
			SetID("inv-resolved").SetClientID("client-beta").
			SetSupplierName("Furnizor Demo Orizont SRL").SetSupplierCui("RO-DEMO-ORIZONT-707").
			SetNormalizedSupplierCui("RO-DEMO-ORIZONT-707").
			SetDocumentNumber("DEMO-RS-007").SetNormalizedDocumentNumber("DEMO-RS-007").
			SetIssueDate(now).SetIssueDay(time.Date(2026, time.September, 8, 0, 0, 0, 0, time.UTC)).SetDueDate(due).
			SetTotalAmount("1785.0000").SetCurrency("RON").SetSpvReference("SPV-DEMO-0007").
			SetIngestionSource("DEVELOPMENT_SEED").SetExternalDeliveryID("SPV-DEMO-0007").
			SetPipelineStatus(invoice.PipelineStatusEXPORTED).SetSagaStatus(invoice.SagaStatusEXPORTED).
			SetRevision(1).SetCreatedAt(now).SetUpdatedAt(now).Save(ctx)
		if createErr != nil {
			return createErr
		}
		actor := "Sistem demo"
		if _, createErr = store.Client.ActivityEvent.Create().
			SetID("activity-resolved-1").SetClientID("client-beta").SetInvoiceID(created.ID).
			SetAggregateType("INVOICE").SetAggregateID(created.ID).SetEventType("Export finalizat").
			SetOccurredAt(now.Add(10 * time.Minute)).SetActorKind("SYSTEM").SetActorDisplay(actor).
			SetAutomatic(true).SetDetail("Flux demonstrativ încheiat.").Save(ctx); createErr != nil {
			return createErr
		}
	}

	lineExists, err := store.Client.InvoiceLine.Query().Where(invoiceline.InvoiceIDEQ("inv-resolved"), invoiceline.PositionEQ(1)).Exist(ctx)
	if err != nil {
		return err
	}
	if !lineExists {
		_, err = store.Client.InvoiceLine.Create().SetID("line-resolved-1").SetInvoiceID("inv-resolved").SetPosition(1).
			SetDescription("Servicii conform contract").SetUnit("BUC").SetVatRate("19.0000").SetVatValue("285.0000").
			SetQuantity("1.0000").SetUnitPrice("1500.0000").SetNetValue("1500.0000").SetTotalValue("1785.0000").Save(ctx)
		if err != nil {
			return err
		}
	}
	if err = seedValidationTaskFixtures(ctx, store, now); err != nil {
		return err
	}
	if err = seedContractMatchingFixtures(ctx, store, now); err != nil {
		return err
	}
	if err = seedClassificationFixtures(ctx, store, now); err != nil {
		return err
	}
	if err = seedModule6Fixtures(ctx, store, now); err != nil {
		return err
	}
	if err = seedModule7Fixtures(ctx, store, now, os.Getenv("SEED_MODULE7") != "false"); err != nil {
		return err
	}
	if err = seedAccountingV2Fixture(ctx, store, now); err != nil {
		return err
	}
	if os.Getenv("SEED_CLIENT_ONBOARDING") == "true" {
		if err = seedClientOnboarding(ctx, store, now); err != nil {
			return err
		}
	}
	if os.Getenv("SEED_SAGA_UX") == "true" {
		return seedSagaUXFixture(ctx, store, now)
	}
	return nil
}

func seedSagaUXFixture(ctx context.Context, store *postgres.Store, now time.Time) error {
	const invoiceID = "inv-saga-ux-real"
	if _, err := store.Client.SagaExportAttempt.Delete().Where(sagaexportattempt.InvoiceIDEQ(invoiceID)).Exec(ctx); err != nil {
		return err
	}
	if _, err := store.Client.ActivityEvent.Delete().Where(activityevent.InvoiceIDEQ(invoiceID)).Exec(ctx); err != nil {
		return err
	}
	if _, err := store.Client.OutboxEntry.Delete().Where(outboxentry.AggregateIDEQ(invoiceID)).Exec(ctx); err != nil {
		return err
	}
	if _, err := store.Client.LineClassification.Delete().Where(lineclassification.InvoiceIDEQ(invoiceID)).Exec(ctx); err != nil {
		return err
	}
	if _, err := store.Client.InvoiceLine.Delete().Where(invoiceline.InvoiceIDEQ(invoiceID)).Exec(ctx); err != nil {
		return err
	}
	if _, err := store.Client.Invoice.Delete().Where(invoice.IDEQ(invoiceID)).Exec(ctx); err != nil {
		return err
	}

	issue := time.Date(2026, time.September, 14, 9, 0, 0, 0, time.UTC)
	if _, err := store.Client.Invoice.Create().SetID(invoiceID).SetClientID("client-alfa").SetSupplierName("Furnizor SAGA UX Test SRL").SetSupplierCui("RO12345678").SetNormalizedSupplierCui("RO12345678").SetDocumentNumber("SAGA-UX-001").SetNormalizedDocumentNumber("SAGAUX001").SetIssueDate(issue).SetIssueDay(time.Date(2026, time.September, 14, 0, 0, 0, 0, time.UTC)).SetTotalAmount("119.0000").SetCurrency("RON").SetSpvReference("SPV-SAGA-UX-001").SetIngestionSource("SYNTHETIC_E2E").SetExternalDeliveryID("SYNTHETIC-E2E-SAGA-UX").SetDocumentType(invoice.DocumentTypeINVOICE).SetPipelineStatus(invoice.PipelineStatusREADY_FOR_SAGA).SetSagaStatus(invoice.SagaStatusREADY).SetRevision(1).SetCreatedAt(issue).SetUpdatedAt(issue).Save(ctx); err != nil {
		return err
	}
	const lineID = invoiceID + "-line-1"
	if _, err := store.Client.InvoiceLine.Create().SetID(lineID).SetInvoiceID(invoiceID).SetPosition(1).SetDescription("Serviciu sintetic pentru verificarea exportului").SetUnit("BUC").SetVatRate("19.0000").SetVatValue("19.0000").SetQuantity("1.0000").SetUnitPrice("100.0000").SetNetValue("100.0000").SetTotalValue("119.0000").Save(ctx); err != nil {
		return err
	}
	classifications := []struct {
		dimension lineclassification.Dimension
		value     string
	}{{lineclassification.DimensionACCOUNT, "628.01"}, {lineclassification.DimensionVAT, "19"}, {lineclassification.DimensionDEDUCTIBILITY, "SAGA_DEFAULT"}}
	for index, item := range classifications {
		if _, err := store.Client.LineClassification.Create().SetID(fmt.Sprintf("%s-classification-%d", invoiceID, index)).SetClientID("client-alfa").SetInvoiceID(invoiceID).SetInvoiceLineID(lineID).SetDimension(item.dimension).SetProposedValue(item.value).SetEffectiveValue(item.value).SetConfidenceDisplay("synthetic explicit E2E value").SetExplanation("Synthetic SAGA UX fixture; not accounting advice.").SetLegalBasis("Synthetic test only.").SetReviewedAt(issue).SetReviewedByDisplay("Synthetic fixture accountant").SetRequiredReview(true).SetReviewStatus(lineclassification.ReviewStatusACCEPTED).SetSource(lineclassification.SourceNO_MATCH).SetPolicyVersion("SAGA_UX_E2E_V1").SetRevision(1).SetCreatedAt(issue).SetUpdatedAt(issue).Save(ctx); err != nil {
			return err
		}
	}
	payload := []byte(`{"invoice_id":"inv-saga-ux-real"}`)
	_, err := store.Client.OutboxEntry.Create().SetID("out-saga-ux-real").SetEventType("INVOICE_CONTINUE").SetAggregateType("INVOICE").SetAggregateID(invoiceID).SetPayload(payload).SetIdempotencyKey("seed:inv-saga-ux-real:continue").SetCorrelationID("seed-saga-ux").SetStatus(outboxentry.StatusPENDING).SetCreatedAt(now).SetAvailableAt(now).Save(ctx)
	return err
}

func seedValidationTaskFixtures(ctx context.Context, store *postgres.Store, now time.Time) error {
	fixtures := []struct {
		invoiceID, clientID, supplier, supplierCUI, number, spv string
		pipeline                                                invoice.PipelineStatus
		taskID, title, reason                                   string
		taskType                                                validationtask.TaskType
		status                                                  validationtask.Status
	}{
		{"inv-task-missing-open", "client-alfa", "Furnizor Contract Absent SRL", "RO-TASK-101", "TASK-MC-OPEN", "SPV-TASK-101", invoice.PipelineStatusAWAITING_CONTRACT, "task-missing-open", "Contract furnizor indisponibil", "Nu există un contract disponibil pentru această factură.", validationtask.TaskTypeMISSING_CONTRACT, validationtask.StatusOPEN},
		{"inv-task-missing-waiting", "client-beta", "Furnizor Contract Solicitat SRL", "RO-TASK-102", "TASK-MC-WAIT", "SPV-TASK-102", invoice.PipelineStatusAWAITING_CONTRACT, "task-missing-waiting", "Contract furnizor solicitat", "Contractul a fost solicitat și factura așteaptă condiția externă.", validationtask.TaskTypeMISSING_CONTRACT, validationtask.StatusWAITING},
		{"inv-task-contract-open", "client-alfa", "Furnizor Asociere SRL", "RO-TASK-103", "TASK-CM-OPEN", "SPV-TASK-103", invoice.PipelineStatusAWAITING_MATCH_CONFIRM, "task-contract-open", "Confirmă asocierea contractului", "Asocierea contractuală necesită validarea contabilului.", validationtask.TaskTypeCONTRACT_MATCH, validationtask.StatusOPEN},
		{"inv-task-classification-open", "client-beta", "Furnizor Clasificare SRL", "RO-TASK-104", "TASK-CL-OPEN", "SPV-TASK-104", invoice.PipelineStatusAWAITING_REVIEW, "task-classification-open", "Revizuiește clasificarea", "Una sau mai multe dimensiuni de clasificare necesită validare.", validationtask.TaskTypeCLASSIFICATION, validationtask.StatusOPEN},
	}
	for index, fixture := range fixtures {
		invoiceExists, err := store.Client.Invoice.Query().Where(invoice.IDEQ(fixture.invoiceID)).Exist(ctx)
		if err != nil {
			return err
		}
		createdAt := now.Add(time.Duration(index+1) * time.Minute)
		if !invoiceExists {
			if _, err = store.Client.Invoice.Create().SetID(fixture.invoiceID).SetClientID(fixture.clientID).
				SetSupplierName(fixture.supplier).SetSupplierCui(fixture.supplierCUI).SetNormalizedSupplierCui(fixture.supplierCUI).
				SetDocumentNumber(fixture.number).SetNormalizedDocumentNumber(fixture.number).
				SetIssueDate(createdAt).SetIssueDay(time.Date(createdAt.Year(), createdAt.Month(), createdAt.Day(), 0, 0, 0, 0, time.UTC)).
				SetTotalAmount("119.0000").SetCurrency("RON").SetSpvReference(fixture.spv).
				SetIngestionSource("DEVELOPMENT_SEED").SetExternalDeliveryID(fixture.spv).
				SetPipelineStatus(fixture.pipeline).SetSagaStatus(invoice.SagaStatusNOT_READY).
				SetRevision(1).SetCreatedAt(createdAt).SetUpdatedAt(createdAt).Save(ctx); err != nil {
				return err
			}
		}
		taskExists, err := store.Client.ValidationTask.Query().Where(validationtask.IDEQ(fixture.taskID)).Exist(ctx)
		if err != nil {
			return err
		}
		if taskExists {
			update := store.Client.ValidationTask.UpdateOneID(fixture.taskID).SetStatus(fixture.status).SetRevision(1).SetUpdatedAt(createdAt).ClearResolvedAt()
			if fixture.status == validationtask.StatusWAITING {
				update.SetWaitingSince(createdAt)
			} else {
				update.ClearWaitingSince()
			}
			if _, err = update.Save(ctx); err != nil {
				return err
			}
			continue
		}
		create := store.Client.ValidationTask.Create().SetID(fixture.taskID).SetClientID(fixture.clientID).SetInvoiceID(fixture.invoiceID).
			SetTaskType(fixture.taskType).SetStatus(fixture.status).SetTitle(fixture.title).SetReason(fixture.reason).
			SetCreatedByKind(validationtask.CreatedByKindSYSTEM).SetCreatedByDisplay("Sistem demo").
			SetCreationKey("seed:" + fixture.taskID).SetRevision(1).SetCreatedAt(createdAt).SetUpdatedAt(createdAt)
		if fixture.status == validationtask.StatusWAITING {
			create.SetWaitingSince(createdAt)
		}
		if _, err = create.Save(ctx); err != nil {
			return err
		}
	}
	return nil
}

func seedContractMatchingFixtures(ctx context.Context, store *postgres.Store, now time.Time) error {
	type contractFixture struct {
		id, clientID, supplier, cui, reference, value, currency, unitType, paymentTerms, source, metadata string
		from, to                                                                                          time.Time
	}
	from := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, time.December, 31, 0, 0, 0, 0, time.UTC)
	contracts := []contractFixture{
		{"contract-demo-100", "client-alfa", "Furnizor Demo Nord SRL", "RO-DEMO-NORD-101", "CTR-DEMO-100", "2400.0000", "RON", "Unitate demonstrativă", "30 zile — condiție demonstrativă", "SRC-DEMO-CTR-100", "Registru contractual demonstrativ · import 01.01.2026", from, to},
		{"contract-demo-201", "client-alfa", "Furnizor Demo Vest SRL", "RO-DEMO-VEST-202", "CTR-DEMO-201", "2400.0000", "RON", "Unitate demonstrativă", "30 zile — condiție demonstrativă", "SRC-DEMO-CTR-201", "Registru contractual demonstrativ · import 02.01.2026", from, to},
		{"contract-demo-202", "client-alfa", "Furnizor Demo Vest SRL", "RO-DEMO-VEST-202", "CTR-DEMO-202", "3200.0000", "RON", "Unitate alternativă demonstrativă", "45 zile — condiție demonstrativă", "SRC-DEMO-CTR-202", "Metadate sursă demonstrative parțiale", time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, time.November, 30, 0, 0, 0, 0, time.UTC)},
		{"contract-demo-401", "client-beta", "Furnizor Demo Est SRL", "RO-DEMO-EST-404", "CTR-DEMO-401", "1800.0000", "RON", "Unitate demonstrativă", "30 zile — condiție demonstrativă", "SRC-DEMO-CTR-401", "Registru contractual demonstrativ · import 04.01.2026", from, to},
		{"contract-demo-501", "client-beta", "Furnizor Demo Central SRL", "RO-DEMO-CENTRAL-505", "CTR-DEMO-501", "4000.0000", "EUR", "Unitate demonstrativă", "30 zile — condiție demonstrativă", "SRC-DEMO-CTR-501", "Politică baseline: monedă nealiniată", from, to},
		{"contract-demo-701", "client-beta", "Furnizor Demo Orizont SRL", "RO-DEMO-ORIZONT-707", "CTR-DEMO-701", "1800.0000", "RON", "Unitate demonstrativă", "30 zile — condiție demonstrativă", "SRC-DEMO-CTR-701", "Registru contractual demonstrativ · import 07.01.2026", from, to},
		{"contract-demo-801", "client-alfa", "Furnizor Demo Meridian SRL", "RO-DEMO-MERIDIAN-808", "CTR-DEMO-801", "2900.0000", "RON", "Unitate demonstrativă", "30 zile — condiție demonstrativă", "SRC-DEMO-CTR-801", "", from, to},
		{"contract-demo-1001", "client-beta", "Furnizor Demo Aurora SRL", "RO-DEMO-AURORA-1001", "CTR-DEMO-1001", "3300.0000", "RON", "Unitate demonstrativă", "30 zile — condiție demonstrativă", "SRC-DEMO-CTR-1001", "Registru contractual demonstrativ · import 10.01.2026", from, to},
	}
	for _, fixture := range contracts {
		normalized := fixture.cui
		existing, err := store.Client.Contract.Query().Where(entcontract.IDEQ(fixture.id)).Only(ctx)
		if err == nil {
			if existing.ClientID != fixture.clientID {
				return fmt.Errorf("contract fixture %s belongs to unexpected client %s", fixture.id, existing.ClientID)
			}
			update := store.Client.Contract.UpdateOne(existing).SetSupplierName(fixture.supplier).SetSupplierCui(fixture.cui).
				SetNormalizedSupplierCui(normalized).SetReference(fixture.reference).SetEffectiveFrom(fixture.from).SetEffectiveTo(fixture.to).
				SetTotalValue(fixture.value).SetCurrency(fixture.currency).SetUnitType(fixture.unitType).SetPaymentTerms(fixture.paymentTerms).
				SetSourceReference(fixture.source).SetRevision(1).SetUpdatedAt(now)
			if fixture.metadata != "" {
				update.SetSourceMetadata(fixture.metadata)
			} else {
				update.ClearSourceMetadata()
			}
			if _, err = update.Save(ctx); err != nil {
				return err
			}
			continue
		}
		if !ent.IsNotFound(err) {
			return err
		}
		create := store.Client.Contract.Create().SetID(fixture.id).SetClientID(fixture.clientID).SetSupplierName(fixture.supplier).
			SetSupplierCui(fixture.cui).SetNormalizedSupplierCui(normalized).SetReference(fixture.reference).
			SetEffectiveFrom(fixture.from).SetEffectiveTo(fixture.to).SetTotalValue(fixture.value).SetCurrency(fixture.currency).
			SetUnitType(fixture.unitType).SetPaymentTerms(fixture.paymentTerms).SetSourceReference(fixture.source).
			SetRevision(1).SetCreatedAt(now).SetUpdatedAt(now)
		if fixture.metadata != "" {
			create.SetSourceMetadata(fixture.metadata)
		}
		if _, err = create.Save(ctx); err != nil {
			return err
		}
	}

	type invoiceFixture struct{ id, clientID, supplier, cui, number, spv, currency string }
	invoices := []invoiceFixture{
		{"inv-contract-auto", "client-alfa", "Furnizor Demo Nord SRL", "RO-DEMO-NORD-101", "BACKEND-AUTO-001", "SPV-BACKEND-AUTO-001", "RON"},
		{"inv-contract-multiple", "client-alfa", "Furnizor Demo Vest SRL", "RO-DEMO-VEST-202", "BACKEND-MULTI-002", "SPV-BACKEND-MULTI-002", "RON"},
		{"inv-contract-multiple-alt", "client-alfa", "Furnizor Demo Vest SRL", "RO-DEMO-VEST-202", "BACKEND-MULTI-ALT-003", "SPV-BACKEND-MULTI-ALT-003", "RON"},
		{"inv-contract-incompatible", "client-beta", "Furnizor Demo Central SRL", "RO-DEMO-CENTRAL-505", "BACKEND-INCOMPAT-004", "SPV-BACKEND-INCOMPAT-004", "RON"},
		{"inv-contract-missing", "client-beta", "Furnizor Fără Contract SRL", "RO-DEMO-MISSING-909", "BACKEND-MISSING-005", "SPV-BACKEND-MISSING-005", "RON"},
	}
	ids := make([]string, 0, len(invoices))
	for _, fixture := range invoices {
		ids = append(ids, fixture.id)
	}
	if _, err := store.Client.ActivityEvent.Delete().Where(activityevent.InvoiceIDIn(ids...)).Exec(ctx); err != nil {
		return err
	}
	if _, err := store.Client.OutboxEntry.Delete().Where(outboxentry.AggregateIDIn(ids...)).Exec(ctx); err != nil {
		return err
	}
	if _, err := store.Client.InvoiceContractAssociation.Delete().Where(invoicecontractassociation.InvoiceIDIn(ids...)).Exec(ctx); err != nil {
		return err
	}
	runIDs, err := store.Client.ContractMatchRun.Query().Where(contractmatchrun.InvoiceIDIn(ids...)).IDs(ctx)
	if err != nil {
		return err
	}
	if _, err = store.Client.ValidationTask.Delete().Where(validationtask.InvoiceIDIn(ids...)).Exec(ctx); err != nil {
		return err
	}
	if len(runIDs) > 0 {
		if _, err = store.Client.ContractMatchCandidate.Delete().Where(contractmatchcandidate.MatchRunIDIn(runIDs...)).Exec(ctx); err != nil {
			return err
		}
	}
	if _, err = store.Client.ContractMatchRun.Delete().Where(contractmatchrun.InvoiceIDIn(ids...)).Exec(ctx); err != nil {
		return err
	}
	if _, err = store.Client.Invoice.Delete().Where(invoice.IDIn(ids...)).Exec(ctx); err != nil {
		return err
	}

	service := contractdomain.NewService(store, contractdomain.BaselinePolicy{}, func() time.Time { return now.Add(30 * time.Minute) })
	for index, fixture := range invoices {
		issue := time.Date(2026, time.September, 10+index, 9, 0, 0, 0, time.UTC)
		if _, err = store.Client.Invoice.Create().SetID(fixture.id).SetClientID(fixture.clientID).SetSupplierName(fixture.supplier).
			SetSupplierCui(fixture.cui).SetNormalizedSupplierCui(fixture.cui).SetDocumentNumber(fixture.number).SetNormalizedDocumentNumber(fixture.number).
			SetIssueDate(issue).SetIssueDay(time.Date(issue.Year(), issue.Month(), issue.Day(), 0, 0, 0, 0, time.UTC)).
			SetTotalAmount("119.0000").SetCurrency(fixture.currency).SetSpvReference(fixture.spv).
			SetIngestionSource("DEVELOPMENT_SEED").SetExternalDeliveryID(fixture.spv).SetPipelineStatus(invoice.PipelineStatusMATCHING).
			SetSagaStatus(invoice.SagaStatusNOT_READY).SetRevision(1).SetCreatedAt(issue).SetUpdatedAt(issue).Save(ctx); err != nil {
			return err
		}
		if _, _, err = service.MatchInvoice(ctx, contractdomain.MatchCommand{InvoiceID: fixture.id, ExpectedRevision: 1, CommandID: "seed:" + fixture.id, CorrelationID: "seed-module4"}); err != nil {
			return err
		}
	}
	return nil
}

func seedClassificationFixtures(ctx context.Context, store *postgres.Store, now time.Time) error {
	demoRule := classificationrule.Or(classificationrule.CreationKeyHasPrefix("seed:"), classificationrule.And(classificationrule.ClientIDIn("client-alfa", "client-beta"), classificationrule.HasParentWith(classificationrule.CreationKeyHasPrefix("seed:"))))
	invoiceIDs := []string{"inv-classification-auto-api", "inv-classification-review-api", "inv-classification-correct-api", "inv-classification-final-api"}
	if _, err := store.Client.ActivityEvent.Delete().Where(activityevent.Or(activityevent.InvoiceIDIn(invoiceIDs...), activityevent.And(activityevent.AggregateTypeEQ("CLASSIFICATION_RULE"), activityevent.AggregateIDIn("rule-account-global", "rule-vat-global", "rule-deductibility-global", "rule-vat-beta-override")))).Exec(ctx); err != nil {
		return err
	}
	if _, err := store.Client.OutboxEntry.Delete().Where(outboxentry.AggregateIDIn(invoiceIDs...)).Exec(ctx); err != nil {
		return err
	}
	if _, err := store.Client.ValidationTask.Delete().Where(validationtask.InvoiceIDIn(invoiceIDs...)).Exec(ctx); err != nil {
		return err
	}
	// The rule catalog is replaced as one deterministic seed set below. Clear all
	// decisions that reference its immutable versions before deleting the catalog,
	// including decisions produced by a previous Module 6 journey run.
	if _, err := store.Client.LineClassification.Delete().Where(lineclassification.ModelVersionEQ(accounting.LegacyVersion), lineclassification.HasInvoiceWith(invoice.IngestionSourceEQ("DEVELOPMENT_SEED"))).Exec(ctx); err != nil {
		return err
	}
	if _, err := store.Client.InvoiceLine.Delete().Where(invoiceline.InvoiceIDIn(invoiceIDs...)).Exec(ctx); err != nil {
		return err
	}
	if _, err := store.Client.Invoice.Delete().Where(invoice.IDIn(invoiceIDs...)).Exec(ctx); err != nil {
		return err
	}
	if _, err := store.Client.RuleVersion.Delete().Where(ruleversion.HasRuleWith(demoRule), ruleversion.ModelVersionEQ(accounting.LegacyVersion)).Exec(ctx); err != nil {
		return err
	}
	if _, err := store.Client.ClassificationRule.Delete().Where(classificationrule.ScopeEQ(classificationrule.ScopeCLIENT_OVERRIDE), demoRule).Exec(ctx); err != nil {
		return err
	}
	if _, err := store.Client.ClassificationRule.Delete().Where(demoRule, classificationrule.Not(classificationrule.HasVersions())).Exec(ctx); err != nil {
		return err
	}

	type ruleFixture struct {
		id, reference, name string
		category            classificationrule.Category
		versions            []struct {
			version          int
			criteria, result string
			kind             ruleversion.MatchKind
			match            *string
			from, to         time.Time
		}
	}
	containsService := "serviciu"
	from2026 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to2026 := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	fixtures := []ruleFixture{
		{"rule-account-global", "REG-DEMO-CONT-01", "Încadrare cont pentru servicii demonstrative", classificationrule.CategoryACCOUNT, []struct {
			version          int
			criteria, result string
			kind             ruleversion.MatchKind
			match            *string
			from, to         time.Time
		}{
			{1, "Descrierea conține un termen demonstrativ din categoria servicii generale.", "Cont demonstrativ 60X", ruleversion.MatchKindNO_AUTOMATION, nil, time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC)},
			{2, "Descrierea conține un semnal fictiv pentru servicii și moneda este RON.", "Cont demonstrativ 6XX", ruleversion.MatchKindDESCRIPTION_CONTAINS, &containsService, from2026, to2026},
		}},
		{"rule-vat-global", "REG-DEMO-TVA-01", "Încadrare TVA demonstrativă", classificationrule.CategoryVAT, []struct {
			version          int
			criteria, result string
			kind             ruleversion.MatchKind
			match            *string
			from, to         time.Time
		}{{1, "Linia sursă conține o etichetă TVA demonstrativă.", "Cotă demonstrativă din sursă", ruleversion.MatchKindALWAYS, nil, from2026, to2026}}},
		{"rule-deductibility-global", "REG-DEMO-DED-01", "Deductibilitate pentru categorii demonstrative", classificationrule.CategoryDEDUCTIBILITY, []struct {
			version          int
			criteria, result string
			kind             ruleversion.MatchKind
			match            *string
			from, to         time.Time
		}{{1, "Descrierea aparține unei categorii fictive configurate pentru demonstrație.", "Categorie demonstrativă", ruleversion.MatchKindDESCRIPTION_CONTAINS, &containsService, from2026, to2026}}},
	}
	for _, fixture := range fixtures {
		if _, err := store.Client.ClassificationRule.Create().SetID(fixture.id).SetReference(fixture.reference).SetName(fixture.name).SetCategory(fixture.category).SetScope(classificationrule.ScopeGLOBAL).SetCreationKey("seed:" + fixture.id).SetRevision(1).SetCreatedAt(now).SetUpdatedAt(now).Save(ctx); err != nil {
			return err
		}
		for _, version := range fixture.versions {
			create := store.Client.RuleVersion.Create().SetID(fmt.Sprintf("%s-v%d", fixture.id, version.version)).SetRuleID(fixture.id).SetVersion(version.version).
				SetCriteria(version.criteria).SetResult(version.result).SetExplanation("Regulă demonstrativă deterministă; nu reprezintă interpretare contabilă sau fiscală validată.").
				SetLegalBasis(rules.LegalBasisPlaceholder).SetMatchKind(version.kind).SetEffectiveFrom(version.from).SetEffectiveTo(version.to).
				SetCreatedByDisplay("Sistem demo").SetCommandKey(fmt.Sprintf("seed:%s:v%d", fixture.id, version.version)).SetCreatedAt(now.Add(time.Duration(version.version) * time.Minute))
			if version.match != nil {
				create.SetMatchValue(*version.match)
			}
			if _, err := create.Save(ctx); err != nil {
				return err
			}
		}
	}

	parentScope := classificationrule.ParentScopeGLOBAL
	if _, err := store.Client.ClassificationRule.Create().SetID("rule-vat-beta-override").SetReference("REG-DEMO-TVA-01-OVR-BETA").SetName("Variație TVA pentru Client Demo Beta").SetCategory(classificationrule.CategoryVAT).SetScope(classificationrule.ScopeCLIENT_OVERRIDE).SetClientID("client-beta").SetParentRuleID("rule-vat-global").SetParentScope(parentScope).SetCreationKey("seed:rule-vat-beta-override").SetRevision(1).SetCreatedAt(now).SetUpdatedAt(now).Save(ctx); err != nil {
		return err
	}
	if _, err := store.Client.RuleVersion.Create().SetID("rule-vat-beta-override-v1").SetRuleID("rule-vat-beta-override").SetVersion(1).SetCriteria("Categorie fictivă specifică exclusiv contextului Client Demo Beta.").SetResult("Rezultat TVA demonstrativ pentru Beta").SetExplanation("Override demonstrativ fără semantică fiscală validată.").SetLegalBasis(rules.LegalBasisPlaceholder).SetMatchKind(ruleversion.MatchKindNO_AUTOMATION).SetEffectiveFrom(from2026).SetEffectiveTo(to2026).SetCreatedByDisplay("Sistem demo").SetCommandKey("seed:rule-vat-beta-override:v1").SetCreatedAt(now).Save(ctx); err != nil {
		return err
	}

	type invoiceFixture struct {
		id, clientID string
		descriptions []string
	}
	invoices := []invoiceFixture{
		{"inv-classification-auto-api", "client-alfa", []string{"Serviciu demonstrativ standard"}},
		{"inv-classification-review-api", "client-beta", []string{"Articol atipic unu", "Element necunoscut doi"}},
		{"inv-classification-correct-api", "client-beta", []string{"Articol atipic pentru corecție"}},
		{"inv-classification-final-api", "client-beta", []string{"Articol atipic pentru rezolvare"}},
	}
	service := classificationdomain.NewService(store, classificationdomain.BaselinePolicy{}, func() time.Time { return now.Add(time.Hour) })
	for invoiceIndex, fixture := range invoices {
		issue := now.Add(time.Duration(invoiceIndex+1) * time.Hour)
		if _, err := store.Client.Invoice.Create().SetID(fixture.id).SetClientID(fixture.clientID).SetSupplierName("Furnizor clasificare demonstrativ").SetSupplierCui("RO-CLASS-DEMO").SetNormalizedSupplierCui("RO-CLASS-DEMO").SetDocumentNumber(fmt.Sprintf("CLASS-%02d", invoiceIndex+1)).SetNormalizedDocumentNumber(fmt.Sprintf("CLASS-%02d", invoiceIndex+1)).SetIssueDate(issue).SetIssueDay(time.Date(issue.Year(), issue.Month(), issue.Day(), 0, 0, 0, 0, time.UTC)).SetTotalAmount("119.0000").SetCurrency("RON").SetSpvReference("SPV-" + fixture.id).SetIngestionSource("DEVELOPMENT_SEED").SetExternalDeliveryID("SPV-" + fixture.id).SetPipelineStatus(invoice.PipelineStatusLINES_READ).SetSagaStatus(invoice.SagaStatusNOT_READY).SetRevision(1).SetCreatedAt(issue).SetUpdatedAt(issue).Save(ctx); err != nil {
			return err
		}
		for lineIndex, description := range fixture.descriptions {
			position := lineIndex + 1
			if _, err := store.Client.InvoiceLine.Create().SetID(fmt.Sprintf("%s-line-%d", fixture.id, position)).SetInvoiceID(fixture.id).SetPosition(position).SetDescription(description).SetUnit("buc.").SetVatRate("19.0000").SetVatValue("19.0000").SetQuantity("1.0000").SetUnitPrice("100.0000").SetNetValue("100.0000").SetTotalValue("119.0000").Save(ctx); err != nil {
				return err
			}
		}
		if _, _, err := service.ProcessInvoice(ctx, classificationdomain.ProcessCommand{InvoiceID: fixture.id, ExpectedRevision: 1, CommandID: "seed:" + fixture.id, CorrelationID: "seed-module5"}); err != nil {
			return err
		}
	}
	return nil
}

func seedModule6Fixtures(ctx context.Context, store *postgres.Store, now time.Time) error {
	invoiceIDs := []string{"inv-module6-journey", "inv-module6-ready", "inv-module6-exported", "inv-module6-duplicate", "inv-module6-saga-failed", "inv-module6-processing"}
	// Earlier module fixtures are already materialized at their intended UI state. Their
	// continuation messages are not part of the Module 6 journey and would make a shared
	// development seed timing-dependent when the normal runtime dispatcher starts.
	if _, err := store.Client.OutboxEntry.Delete().Where(outboxentry.StatusEQ(outboxentry.StatusPENDING)).Exec(ctx); err != nil {
		return err
	}
	if _, err := store.Client.ActivityEvent.Delete().Where(activityevent.Or(activityevent.InvoiceIDIn(invoiceIDs...), activityevent.AggregateIDIn("rule-module6-version", "rule-module6-override-parent"))).Exec(ctx); err != nil {
		return err
	}
	if _, err := store.Client.OutboxEntry.Delete().Where(outboxentry.AggregateIDIn(invoiceIDs...)).Exec(ctx); err != nil {
		return err
	}
	if _, err := store.Client.ValidationTask.Delete().Where(validationtask.InvoiceIDIn(invoiceIDs...)).Exec(ctx); err != nil {
		return err
	}
	if _, err := store.Client.LineClassification.Delete().Where(lineclassification.InvoiceIDIn(invoiceIDs...)).Exec(ctx); err != nil {
		return err
	}
	if _, err := store.Client.InvoiceLine.Delete().Where(invoiceline.InvoiceIDIn(invoiceIDs...)).Exec(ctx); err != nil {
		return err
	}
	if _, err := store.Client.InvoiceContractAssociation.Delete().Where(invoicecontractassociation.InvoiceIDIn(invoiceIDs...)).Exec(ctx); err != nil {
		return err
	}
	runIDs, err := store.Client.ContractMatchRun.Query().Where(contractmatchrun.InvoiceIDIn(invoiceIDs...)).IDs(ctx)
	if err != nil {
		return err
	}
	if len(runIDs) > 0 {
		if _, err = store.Client.ContractMatchCandidate.Delete().Where(contractmatchcandidate.MatchRunIDIn(runIDs...)).Exec(ctx); err != nil {
			return err
		}
	}
	if _, err = store.Client.ContractMatchRun.Delete().Where(contractmatchrun.InvoiceIDIn(invoiceIDs...)).Exec(ctx); err != nil {
		return err
	}
	if _, err = store.Client.Invoice.Delete().Where(invoice.IDIn(invoiceIDs...)).Exec(ctx); err != nil {
		return err
	}

	for _, ruleID := range []string{"rule-module6-version", "rule-module6-override-parent"} {
		if _, err = store.Client.RuleVersion.Delete().Where(ruleversion.RuleIDEQ(ruleID)).Exec(ctx); err != nil {
			return err
		}
		if _, err = store.Client.ClassificationRule.Delete().Where(classificationrule.IDEQ(ruleID)).Exec(ctx); err != nil {
			return err
		}
	}
	from := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, time.December, 31, 0, 0, 0, 0, time.UTC)
	for index, ruleID := range []string{"rule-module6-version", "rule-module6-override-parent"} {
		if _, err = store.Client.ClassificationRule.Create().SetID(ruleID).SetReference(fmt.Sprintf("REG-M6-%02d", index+1)).SetName([]string{"Regulă izolată pentru versiuni Module 6", "Regulă izolată pentru override Module 6"}[index]).SetCategory(classificationrule.CategoryACCOUNT).SetScope(classificationrule.ScopeGLOBAL).SetCreationKey("seed:" + ruleID).SetRevision(1).SetCreatedAt(now).SetUpdatedAt(now).Save(ctx); err != nil {
			return err
		}
		if _, err = store.Client.RuleVersion.Create().SetID(ruleID + "-v1").SetRuleID(ruleID).SetVersion(1).SetCriteria("Fixture izolată pentru integrarea frontend/API.").SetResult("Rezultat demonstrativ Module 6").SetExplanation("Regulă demonstrativă fără semantică contabilă validată.").SetLegalBasis(rules.LegalBasisPlaceholder).SetMatchKind(ruleversion.MatchKindNO_AUTOMATION).SetEffectiveFrom(from).SetEffectiveTo(to).SetCreatedByDisplay("Sistem demo").SetCommandKey("seed:" + ruleID + ":v1").SetCreatedAt(now).Save(ctx); err != nil {
			return err
		}
	}

	type fixture struct {
		id, clientID, supplier, cui, number, total string
		pipeline                                   invoice.PipelineStatus
		saga                                       invoice.SagaStatus
	}
	fixtures := []fixture{
		{"inv-module6-journey", "client-alfa", "Furnizor Demo Vest SRL", "RO-DEMO-VEST-202", "M6-JOURNEY-001", "238.0000", invoice.PipelineStatusMATCHING, invoice.SagaStatusNOT_READY},
		{"inv-module6-ready", "client-alfa", "Furnizor Module 6 Ready SRL", "RO-M6-READY", "M6-READY-002", "119.0000", invoice.PipelineStatusREADY_FOR_SAGA, invoice.SagaStatusREADY},
		{"inv-module6-exported", "client-beta", "Furnizor Module 6 Export SRL", "RO-M6-EXPORT", "M6-EXPORTED-003", "357.0000", invoice.PipelineStatusEXPORTED, invoice.SagaStatusEXPORTED},
		{"inv-module6-duplicate", "client-beta", "Furnizor Module 6 Export SRL", "RO-M6-EXPORT", "M6-EXPORTED-003", "357.0000", invoice.PipelineStatusDUPLICATE, invoice.SagaStatusNOT_READY},
		{"inv-module6-saga-failed", "client-beta", "Furnizor Module 6 SAGA SRL", "RO-M6-SAGA", "M6-SAGA-FAILED-005", "476.0000", invoice.PipelineStatusEXPORTING, invoice.SagaStatusFAILED},
		{"inv-module6-processing", "client-alfa", "Furnizor Module 6 Procesare SRL", "RO-M6-PROCESS", "M6-PROCESS-006", "595.0000", invoice.PipelineStatusHEADER_READ, invoice.SagaStatusNOT_READY},
	}
	for index, item := range fixtures {
		issue := time.Date(2026, time.September, 20+index, 9, 0, 0, 0, time.UTC)
		create := store.Client.Invoice.Create().SetID(item.id).SetClientID(item.clientID).SetSupplierName(item.supplier).SetSupplierCui(item.cui).SetNormalizedSupplierCui(item.cui).SetDocumentNumber(item.number).SetNormalizedDocumentNumber(item.number).SetIssueDate(issue).SetIssueDay(time.Date(issue.Year(), issue.Month(), issue.Day(), 0, 0, 0, 0, time.UTC)).SetTotalAmount(item.total).SetCurrency("RON").SetSpvReference("SPV-" + item.id).SetIngestionSource("DEVELOPMENT_SEED").SetExternalDeliveryID("SPV-" + item.id).SetPipelineStatus(item.pipeline).SetSagaStatus(item.saga).SetRevision(1).SetCreatedAt(issue).SetUpdatedAt(issue)
		if item.id == "inv-module6-duplicate" {
			create.SetDuplicateOfInvoiceID("inv-module6-exported").SetDuplicateAmountMatches(true).SetDuplicateCurrencyMatches(true)
		}
		if _, err = create.Save(ctx); err != nil {
			return err
		}
		if _, err = store.Client.ActivityEvent.Create().SetID("activity-" + item.id).SetClientID(item.clientID).SetInvoiceID(item.id).SetAggregateType("INVOICE").SetAggregateID(item.id).SetEventType("Factură pregătită pentru demonstrație").SetOccurredAt(issue).SetActorKind(activityevent.ActorKindSYSTEM).SetActorDisplay("Sistem demo").SetAutomatic(true).SetDetail("Eveniment de business persistent pentru scenariul Module 6.").Save(ctx); err != nil {
			return err
		}
	}

	if _, err = store.Client.InvoiceLine.Create().SetID("inv-module6-journey-line-1").SetInvoiceID("inv-module6-journey").SetPosition(1).SetDescription("Element necunoscut pentru validare completă").SetUnit("buc.").SetVatRate("19.0000").SetVatValue("38.0000").SetQuantity("2.0000").SetUnitPrice("100.0000").SetNetValue("200.0000").SetTotalValue("238.0000").Save(ctx); err != nil {
		return err
	}
	contractService := contractdomain.NewService(store, contractdomain.BaselinePolicy{}, func() time.Time { return now.Add(3 * time.Hour) })
	_, _, err = contractService.MatchInvoice(ctx, contractdomain.MatchCommand{InvoiceID: "inv-module6-journey", ExpectedRevision: 1, CommandID: "seed:inv-module6-journey:match", CorrelationID: "seed-module6"})
	return err
}

func seedModule7Fixtures(ctx context.Context, store *postgres.Store, now time.Time, enabled bool) error {
	invoiceIDs := []string{"inv-module7-happy", "inv-module7-match-block", "inv-module7-classification-block", "inv-module7-missing-contract", "inv-module7-reload", "inv-module7-saga-failed"}
	if _, err := store.Client.ActivityEvent.Delete().Where(activityevent.InvoiceIDIn(invoiceIDs...)).Exec(ctx); err != nil {
		return err
	}
	if _, err := store.Client.OutboxEntry.Delete().Where(outboxentry.AggregateIDIn(invoiceIDs...)).Exec(ctx); err != nil {
		return err
	}
	if _, err := store.Client.ValidationTask.Delete().Where(validationtask.InvoiceIDIn(invoiceIDs...)).Exec(ctx); err != nil {
		return err
	}
	if _, err := store.Client.LineClassification.Delete().Where(lineclassification.InvoiceIDIn(invoiceIDs...)).Exec(ctx); err != nil {
		return err
	}
	if _, err := store.Client.InvoiceLine.Delete().Where(invoiceline.InvoiceIDIn(invoiceIDs...)).Exec(ctx); err != nil {
		return err
	}
	if _, err := store.Client.InvoiceContractAssociation.Delete().Where(invoicecontractassociation.InvoiceIDIn(invoiceIDs...)).Exec(ctx); err != nil {
		return err
	}
	runIDs, err := store.Client.ContractMatchRun.Query().Where(contractmatchrun.InvoiceIDIn(invoiceIDs...)).IDs(ctx)
	if err != nil {
		return err
	}
	if len(runIDs) > 0 {
		if _, err = store.Client.ContractMatchCandidate.Delete().Where(contractmatchcandidate.MatchRunIDIn(runIDs...)).Exec(ctx); err != nil {
			return err
		}
	}
	if _, err = store.Client.ContractMatchRun.Delete().Where(contractmatchrun.InvoiceIDIn(invoiceIDs...)).Exec(ctx); err != nil {
		return err
	}
	if _, err = store.Client.Invoice.Delete().Where(invoice.IDIn(invoiceIDs...)).Exec(ctx); err != nil {
		return err
	}
	if !enabled {
		return nil
	}

	pipeline := invoicing.NewPipelineService(store, invoicing.NewFakeSagaExporter(), func() time.Time { return now })
	fixtures := []struct {
		id, supplier, cui, number, description string
	}{
		{"inv-module7-happy", "Furnizor Demo Nord SRL", "RO-DEMO-NORD-101", "M7-HAPPY-001", "Serviciu demonstrativ automat Module 7"},
		{"inv-module7-match-block", "Furnizor Demo Vest SRL", "RO-DEMO-VEST-202", "M7-MATCH-002", "Serviciu demonstrativ după confirmare"},
		{"inv-module7-classification-block", "Furnizor Demo Nord SRL", "RO-DEMO-NORD-101", "M7-CLASS-003", "Element necunoscut pentru decizie umană"},
		{"inv-module7-missing-contract", "Furnizor Module 7 Fără Contract SRL", "RO-M7-MISSING", "M7-MISSING-004", "Serviciu fără contract"},
		{"inv-module7-reload", "Furnizor Demo Vest SRL", "RO-DEMO-VEST-202", "M7-RELOAD-005", "Serviciu demonstrativ după reload"},
	}
	for index, fixture := range fixtures {
		cui := fixture.cui
		issue := time.Date(2026, time.September, 25+index, 9, 0, 0, 0, time.UTC)
		_, _, err = pipeline.Ingest(ctx, invoicing.IngestionInput{
			ID: fixture.id, ClientID: "client-alfa", Source: "DEVELOPMENT_SEED", ExternalDeliveryID: "SPV-" + fixture.id,
			SupplierName: fixture.supplier, SupplierCUI: &cui, DocumentNumber: fixture.number, IssueDate: issue,
			Total: money.Money{Amount: money.MustParse("119.0000"), Currency: "RON"}, SPVReference: "SPV-" + fixture.id,
			Lines: []invoicing.Line{{Position: 1, Description: fixture.description, Unit: "buc.", VATRate: money.MustParse("19.0000"), VATValue: money.MustParse("19.0000"), Quantity: money.MustParse("1.0000"), UnitPrice: money.MustParse("100.0000"), NetValue: money.MustParse("100.0000"), TotalValue: money.MustParse("119.0000")}},
		})
		if err != nil {
			return err
		}
	}

	issue := time.Date(2026, time.September, 30, 9, 0, 0, 0, time.UTC)
	if _, err = store.Client.Invoice.Create().SetID("inv-module7-saga-failed").SetClientID("client-alfa").SetSupplierName("Furnizor Module 7 SAGA SRL").SetSupplierCui("RO-M7-SAGA").SetNormalizedSupplierCui("RO-M7-SAGA").SetDocumentNumber("M7-SAGA-FAILED-005").SetNormalizedDocumentNumber("M7-SAGA-FAILED-005").SetIssueDate(issue).SetIssueDay(time.Date(issue.Year(), issue.Month(), issue.Day(), 0, 0, 0, 0, time.UTC)).SetTotalAmount("119.0000").SetCurrency("RON").SetSpvReference("SPV-inv-module7-saga-failed").SetIngestionSource("DEVELOPMENT_SEED").SetExternalDeliveryID("SPV-inv-module7-saga-failed").SetPipelineStatus(invoice.PipelineStatusREADY_FOR_SAGA).SetSagaStatus(invoice.SagaStatusREADY).SetRevision(1).SetCreatedAt(issue).SetUpdatedAt(issue).Save(ctx); err != nil {
		return err
	}
	payload := []byte(`{"invoice_id":"inv-module7-saga-failed"}`)
	_, err = store.Client.OutboxEntry.Create().SetID("out-module7-saga-failed").SetEventType("INVOICE_CONTINUE").SetAggregateType("INVOICE").SetAggregateID("inv-module7-saga-failed").SetPayload(payload).SetIdempotencyKey("seed:inv-module7-saga-failed:continue").SetCorrelationID("seed-module7").SetStatus(outboxentry.StatusPENDING).SetCreatedAt(now).SetAvailableAt(now).Save(ctx)
	return err
}

func demoSeedingAllowed(environment string) bool {
	return environment == "development" || environment == "test"
}

// Four visible typed decisions with a deliberately unapproved mapping. The
// production worker cannot export this TEST_ONLY configuration. Legacy fixtures
// keep their model and separate values; repeated seed preserves this history.
func seedAccountingV2Fixture(ctx context.Context, store *postgres.Store, now time.Time) error {
	const cid = "TEST_ONLY-accounting-client"
	const id = "inv-accounting-v2-test-only"
	exists, err := store.Client.Invoice.Query().Where(invoice.IDEQ(id)).Exist(ctx)
	if err != nil || exists {
		return err
	}
	f, l, p, pack := accountingtest.Fixture(cid)
	pack.Mapping.Approved = false
	if _, err := store.Client.AccountingClient.Create().SetID(cid).SetName("TEST_ONLY — Accounting V2 synthetic client").SetCui(accountingtest.BuyerCUI).SetNormalizedIdentifier(accountingtest.BuyerNormalizedCUI).SetCreatedAt(now).SetUpdatedAt(now).Save(ctx); err != nil {
		return err
	}
	for _, r := range pack.Rules {
		if _, err := store.Client.ClassificationRule.Create().SetID(r.ID).SetReference(r.ID).SetName("TEST_ONLY " + r.Dimension).SetCategory(classificationrule.Category(r.Dimension)).SetScope(classificationrule.ScopeGLOBAL).SetCreationKey("TEST_ONLY:" + r.ID).SetCreatedAt(now).SetUpdatedAt(now).Save(ctx); err != nil {
			return err
		}
		if _, err := store.Client.RuleVersion.Create().SetID(r.VersionID).SetRuleID(r.ID).SetVersion(1).SetModelVersion(accounting.ModelVersion).SetDomainRule(&r).SetCriteria("TEST_ONLY exact synthetic policy").SetResult(r.Result.Text()).SetExplanation(r.Explanation).SetLegalBasis(r.LegalBasis).SetMatchKind(ruleversion.MatchKindNO_AUTOMATION).SetRulePackVersion(pack.ID + "/1").SetEffectiveFrom(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)).SetCreatedByDisplay("TEST_ONLY").SetCommandKey("TEST_ONLY:" + r.VersionID).SetCreatedAt(now).Save(ctx); err != nil {
			return err
		}
	}
	if _, err := store.Client.ClientAccountingProfile.Create().SetID(p.ID).SetClientID(cid).SetVersion(1).SetPayload(p).SetCreatedAt(now).Save(ctx); err != nil {
		return err
	}
	if _, err := store.Client.AccountingRulePack.Create().SetID(pack.ID).SetClientID(cid).SetVersion(1).SetPayload(pack).SetCreatedAt(now).Save(ctx); err != nil {
		return err
	}
	if _, err := store.Client.Invoice.Create().SetID(id).SetClientID(cid).SetSupplierName("TEST_ONLY supplier").SetSupplierCui(accountingtest.SupplierCUI).SetNormalizedSupplierCui(accountingtest.SupplierNormalizedCUI).SetDocumentNumber("TEST_ONLY-001").SetNormalizedDocumentNumber("TEST_ONLY-001").SetIssueDate(now).SetIssueDay(invoicing.InvoiceIssueDay(now)).SetTotalAmount("121").SetCurrency("RON").SetSpvReference(id).SetIngestionSource("TEST_ONLY_ACCOUNTING_V2").SetExternalDeliveryID(id).SetModelVersion(accounting.ModelVersion).SetSourceFacts(f).SetPipelineStatus(invoice.PipelineStatusLINES_READ).SetSagaStatus(invoice.SagaStatusNOT_READY).SetCreatedAt(now).SetUpdatedAt(now).Save(ctx); err != nil {
		return err
	}
	if _, err := store.Client.InvoiceLine.Create().SetID(id + "-line").SetInvoiceID(id).SetPosition(1).SetDescription("TEST_ONLY service").SetUnit("H87").SetQuantity("1").SetUnitPrice("100").SetNetValue("100").SetVatRate("21").SetVatValue("21").SetTotalValue("121").SetSourceFacts(l).Save(ctx); err != nil {
		return err
	}
	_, _, err = classificationdomain.NewService(store, classificationdomain.DomainPolicy{AllowTestOnly: true}, func() time.Time { return now }).ProcessInvoice(ctx, classificationdomain.ProcessCommand{InvoiceID: id, ExpectedRevision: 1, CommandID: "TEST_ONLY:" + id})
	return err
}

// Opt-in synthetic company fixtures; never install a production pack or real credentials.
func seedClientOnboarding(ctx context.Context, store *postgres.Store, now time.Time) error {
	for _, c := range []struct {
		id, name, cui, lifecycle string
		enabled                  bool
	}{{"client-onboarding-partial", "Client configurare parțială SRL", "99000100", "ONBOARDING", false}, {"client-onboarding-action-required", "Client ANAF de reautorizat SRL", "99000101", "ACTIVE", true}} {
		exists, err := store.Client.AccountingClient.Query().Where(accountingclient.IDEQ(c.id)).Exist(ctx)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		tx, err := store.DB.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO clients(id,name,cui,normalized_identifier,lifecycle,created_at,updated_at) VALUES($1,$2,$3,$3,$4,$5,$5)`, c.id, c.name, c.cui, c.lifecycle, now)
		if err == nil {
			_, err = tx.ExecContext(ctx, `INSERT INTO client_saga_configurations(client_id,enabled,updated_at) VALUES($1,$2,$3)`, c.id, c.enabled, now)
		}
		if err == nil && c.enabled {
			_, err = tx.ExecContext(ctx, `INSERT INTO spv_connections(id,client_id,cif,environment,status,access_token_ciphertext,refresh_token_ciphertext,access_token_expires_at,created_at,updated_at) VALUES($1,$2,$3,'TEST','EXPIRED','','',$4,$4,$4)`, "spv-"+c.id, c.id, c.cui, now)
		}
		if err != nil {
			tx.Rollback()
			return err
		}
		if err = tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}
