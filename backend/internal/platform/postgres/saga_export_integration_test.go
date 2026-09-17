//go:build integration

package postgres

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"diana-contabilitate/backend/ent/activityevent"
	"diana-contabilitate/backend/ent/invoice"
	"diana-contabilitate/backend/ent/lineclassification"
	"diana-contabilitate/backend/ent/sagaexportattempt"
	"diana-contabilitate/backend/internal/apperrors"
	"diana-contabilitate/backend/internal/invoicing"
	"diana-contabilitate/backend/internal/saga"
)

func TestRealSAGAArtifactPersistenceIsIdempotentUnderConcurrency(t *testing.T) {
	store, ctx, clientID, now := pipelineTestStore(t)
	invoiceID := "saga-real-" + clientID
	supplierCUI := "RO1234567"
	_, err := store.Client.Invoice.Create().SetID(invoiceID).SetClientID(clientID).
		SetSupplierName("Furnizor Știință SRL").SetSupplierCui(supplierCUI).SetNormalizedSupplierCui(supplierCUI).
		SetDocumentNumber("REAL/1").SetNormalizedDocumentNumber("REAL1").SetIssueDate(now).SetIssueDay(invoicing.InvoiceIssueDay(now)).
		SetTotalAmount("119.0000").SetCurrency("RON").SetSpvReference("SPV-" + invoiceID).
		SetIngestionSource("TEST").SetExternalDeliveryID("DELIVERY-" + invoiceID).SetDocumentType(invoice.DocumentTypeINVOICE).
		SetPipelineStatus(invoice.PipelineStatusEXPORTING).SetSagaStatus(invoice.SagaStatusEXPORTING).
		SetRevision(3).SetCreatedAt(now).SetUpdatedAt(now).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	lineID := invoiceID + "-line"
	if _, err = store.Client.InvoiceLine.Create().SetID(lineID).SetInvoiceID(invoiceID).SetPosition(1).
		SetDescription("Servicii analiză").SetUnit("H87").SetQuantity("1.0000").SetUnitPrice("100.0000").
		SetNetValue("100.0000").SetVatRate("19.0000").SetVatValue("19.0000").SetTotalValue("119.0000").Save(ctx); err != nil {
		t.Fatal(err)
	}
	for index, item := range []struct {
		dimension lineclassification.Dimension
		value     string
	}{{lineclassification.DimensionACCOUNT, "628.01"}, {lineclassification.DimensionVAT, "19"}, {lineclassification.DimensionDEDUCTIBILITY, "SAGA_DEFAULT"}} {
		if _, err = store.Client.LineClassification.Create().SetID(fmt.Sprintf("%s-classification-%d", invoiceID, index)).
			SetClientID(clientID).SetInvoiceID(invoiceID).SetInvoiceLineID(lineID).SetDimension(item.dimension).
			SetProposedValue(item.value).SetEffectiveValue(item.value).SetConfidenceDisplay("explicit test value").
			SetExplanation("synthetic SAGA integration fixture").SetLegalBasis("synthetic test only").
			SetReviewedAt(now).SetReviewedByDisplay("Fixture accountant").SetRequiredReview(true).SetReviewStatus(lineclassification.ReviewStatusACCEPTED).
			SetSource(lineclassification.SourceNO_MATCH).SetPolicyVersion("SAGA_TEST_POLICY_V1").SetRevision(1).
			SetCreatedAt(now).SetUpdatedAt(now).Save(ctx); err != nil {
			t.Fatal(err)
		}
	}
	loaded, err := store.GetInvoice(ctx, invoiceID)
	if err != nil {
		t.Fatal(err)
	}
	exporter := saga.NewFileExporter(store, func() time.Time { return now })
	const workers = 6
	var group sync.WaitGroup
	errorsSeen := make(chan error, workers)
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			result, exportErr := exporter.Export(ctx, loaded, "duplicate-delivery")
			if exportErr == nil && (result.Confirmed || result.Reference == "") {
				exportErr = errors.New("generated file must remain unconfirmed")
			}
			errorsSeen <- exportErr
		}()
	}
	group.Wait()
	close(errorsSeen)
	for exportErr := range errorsSeen {
		if exportErr != nil {
			t.Fatal(exportErr)
		}
	}
	attempts, _ := store.Client.SagaExportAttempt.Query().Where(sagaexportattempt.InvoiceIDEQ(invoiceID)).All(ctx)
	started, _ := store.Client.ActivityEvent.Query().Where(activityevent.InvoiceIDEQ(invoiceID), activityevent.EventTypeEQ("SAGA_EXPORT_STARTED")).Count(ctx)
	succeeded, _ := store.Client.ActivityEvent.Query().Where(activityevent.InvoiceIDEQ(invoiceID), activityevent.EventTypeEQ("SAGA_EXPORT_ARTIFACT_GENERATED")).Count(ctx)
	if len(attempts) != 1 || started != 1 || succeeded != 1 || attempts[0].PayloadSha256 == nil || len(attempts[0].Payload) == 0 {
		t.Fatalf("attempts=%d started=%d succeeded=%d", len(attempts), started, succeeded)
	}
	artifact, err := exporter.Artifact(ctx, invoiceID, clientID)
	if err != nil || len(artifact.Payload) == 0 {
		t.Fatalf("owned artifact unavailable: bytes=%d err=%v", len(artifact.Payload), err)
	}
	digest := sha256.Sum256(artifact.Payload)
	if artifact.SHA256 != hex.EncodeToString(digest[:]) || attempts[0].PayloadSha256 == nil || *attempts[0].PayloadSha256 != artifact.SHA256 {
		t.Fatalf("persisted artifact hash does not match exact bytes")
	}
	if _, err = exporter.Artifact(ctx, invoiceID, "different-client"); !errors.Is(err, saga.ErrAttemptNotFound) {
		t.Fatalf("cross-client artifact read must be impossible: %v", err)
	}
}

func TestSagaDownloadAuditsWithoutExportAndHumanConfirmationIsAtomic(t *testing.T) {
	store, ctx, clientID, now := pipelineTestStore(t)
	invoiceID := "saga-handoff-" + clientID
	_, err := store.Client.Invoice.Create().SetID(invoiceID).SetClientID(clientID).
		SetSupplierName("Furnizor handoff").SetDocumentNumber("HANDOFF/1").SetNormalizedDocumentNumber("HANDOFF1").
		SetIssueDate(now).SetIssueDay(invoicing.InvoiceIssueDay(now)).SetTotalAmount("119.0000").SetCurrency("RON").
		SetSpvReference("SPV-" + invoiceID).SetIngestionSource("TEST").SetExternalDeliveryID("DELIVERY-" + invoiceID).
		SetDocumentType(invoice.DocumentTypeINVOICE).SetPipelineStatus(invoice.PipelineStatusEXPORTING).
		SetSagaStatus(invoice.SagaStatusEXPORTING).SetRevision(3).SetCreatedAt(now).SetUpdatedAt(now).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	attempt, created, err := store.SaveAttempt(ctx, saga.Attempt{ID: "attempt-" + invoiceID, InvoiceID: invoiceID, ClientID: clientID, InvoiceRevision: 3, ExporterVersion: saga.ExporterVersion, Status: saga.AttemptGenerated, Artifact: &saga.Artifact{Filename: "handoff.xml", ContentType: saga.ContentType, Payload: []byte("<Facturi/>"), SHA256: "hash"}, StartedAt: now, CompletedAt: now})
	if err != nil || !created {
		t.Fatalf("attempt created=%v err=%v", created, err)
	}

	download, err := store.DownloadExportArtifact(ctx, clientID, invoiceID, saga.Actor{ID: "accountant", Display: "Contabil", CorrelationID: "download-1"}, now.Add(time.Minute))
	if err != nil || string(download.Artifact.Payload) != "<Facturi/>" {
		t.Fatalf("download=%#v err=%v", download, err)
	}
	item, _ := store.GetInvoice(ctx, invoiceID)
	if item.PipelineStatus != invoicing.StatusExporting {
		t.Fatalf("download exported the invoice: %s", item.PipelineStatus)
	}
	if _, err = store.DownloadExportArtifact(ctx, "different-client", invoiceID, saga.Actor{ID: "accountant", CorrelationID: "download-cross"}, now); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("cross-client download: %v", err)
	}

	confirmed, view, changed, err := store.ConfirmSagaImport(ctx, saga.ConfirmCommand{ClientID: clientID, InvoiceID: invoiceID, AttemptID: attempt.ID, ExpectedInvoiceRevision: 3, CommandID: "confirm-1", Actor: saga.Actor{ID: "accountant", Display: "Contabil", CorrelationID: "request-1"}}, now.Add(2*time.Minute))
	if err != nil || !changed || confirmed.PipelineStatus != invoicing.StatusExported || confirmed.SagaStatus != invoicing.SagaExported {
		t.Fatalf("confirmation changed=%v invoice=%#v err=%v", changed, confirmed, err)
	}
	if view.ConfirmationType == nil || *view.ConfirmationType != saga.ConfirmationHuman || view.ConfirmedAt == nil {
		t.Fatalf("human confirmation missing: %#v", view)
	}
	confirmedAgain, _, changedAgain, err := store.ConfirmSagaImport(ctx, saga.ConfirmCommand{ClientID: clientID, InvoiceID: invoiceID, AttemptID: attempt.ID, ExpectedInvoiceRevision: 3, CommandID: "confirm-1", Actor: saga.Actor{ID: "accountant"}}, now.Add(3*time.Minute))
	if err != nil || changedAgain || confirmedAgain.Revision != confirmed.Revision {
		t.Fatalf("idempotent confirmation changed=%v revision=%d err=%v", changedAgain, confirmedAgain.Revision, err)
	}

	downloadEvents, _ := store.Client.ActivityEvent.Query().Where(activityevent.InvoiceIDEQ(invoiceID), activityevent.EventTypeEQ("SAGA_EXPORT_DOWNLOADED")).Count(ctx)
	confirmEvents, _ := store.Client.ActivityEvent.Query().Where(activityevent.InvoiceIDEQ(invoiceID), activityevent.EventTypeEQ("SAGA_IMPORT_CONFIRMED")).Count(ctx)
	if downloadEvents != 1 || confirmEvents != 1 {
		t.Fatalf("download events=%d confirmation events=%d", downloadEvents, confirmEvents)
	}
}

func TestSagaHumanConfirmationIsIdempotentUnderConcurrency(t *testing.T) {
	store, ctx, clientID, now := pipelineTestStore(t)
	invoiceID := "saga-confirm-race-" + clientID
	_, err := store.Client.Invoice.Create().SetID(invoiceID).SetClientID(clientID).SetSupplierName("Furnizor concurent").
		SetDocumentNumber("RACE/1").SetNormalizedDocumentNumber("RACE1").SetIssueDate(now).SetIssueDay(invoicing.InvoiceIssueDay(now)).
		SetTotalAmount("10.0000").SetCurrency("RON").SetSpvReference("SPV-" + invoiceID).SetIngestionSource("TEST").
		SetExternalDeliveryID("DELIVERY-" + invoiceID).SetDocumentType(invoice.DocumentTypeINVOICE).
		SetPipelineStatus(invoice.PipelineStatusEXPORTING).SetSagaStatus(invoice.SagaStatusEXPORTING).SetRevision(3).SetCreatedAt(now).SetUpdatedAt(now).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	attemptID := "attempt-" + invoiceID
	if _, _, err = store.SaveAttempt(ctx, saga.Attempt{ID: attemptID, InvoiceID: invoiceID, ClientID: clientID, InvoiceRevision: 3, ExporterVersion: saga.ExporterVersion, Status: saga.AttemptGenerated, Artifact: &saga.Artifact{Filename: "race.xml", ContentType: saga.ContentType, Payload: []byte("<Facturi/>"), SHA256: "race-hash"}, StartedAt: now, CompletedAt: now}); err != nil {
		t.Fatal(err)
	}

	const workers = 6
	var group sync.WaitGroup
	errorsSeen := make(chan error, workers)
	for index := range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			_, _, _, confirmErr := store.ConfirmSagaImport(ctx, saga.ConfirmCommand{ClientID: clientID, InvoiceID: invoiceID, AttemptID: attemptID, ExpectedInvoiceRevision: 3, CommandID: fmt.Sprintf("confirm-race-%d", index), Actor: saga.Actor{ID: fmt.Sprintf("actor-%d", index)}}, now.Add(time.Minute))
			errorsSeen <- confirmErr
		}()
	}
	group.Wait()
	close(errorsSeen)
	for confirmErr := range errorsSeen {
		if confirmErr != nil {
			t.Fatal(confirmErr)
		}
	}
	item, _ := store.GetInvoice(ctx, invoiceID)
	confirmEvents, _ := store.Client.ActivityEvent.Query().Where(activityevent.InvoiceIDEQ(invoiceID), activityevent.EventTypeEQ("SAGA_IMPORT_CONFIRMED")).Count(ctx)
	if item.PipelineStatus != invoicing.StatusExported || item.Revision != 4 || confirmEvents != 1 {
		t.Fatalf("status=%s revision=%d events=%d", item.PipelineStatus, item.Revision, confirmEvents)
	}
}
