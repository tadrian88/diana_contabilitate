//go:build integration

package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"diana-contabilitate/backend/ent/account"
	"diana-contabilitate/backend/ent/accountmapping"
	"diana-contabilitate/backend/ent/accountmappingversion"
	"diana-contabilitate/backend/ent/invoice"
	"diana-contabilitate/backend/internal/accounting"
	"diana-contabilitate/backend/internal/accountingtest"
	"diana-contabilitate/backend/internal/apperrors"
	"diana-contabilitate/backend/internal/classification"
	"diana-contabilitate/backend/internal/invoicing"
)

func TestAccountCatalogSeedAndSearch(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	store, err := Open(url)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	exact, err := store.GetSelectableAccount(t.Context(), "6281")
	if err != nil || exact.Name != "Cheltuieli cu serviciile IT" || !exact.Postable || exact.Synthetic {
		t.Fatal(exact, err)
	}
	byCode, err := store.SearchAccounts(t.Context(), "628", 100)
	if err != nil || len(byCode) == 0 {
		t.Fatal(len(byCode), err)
	}
	for _, account := range byCode {
		if !account.Active || !account.Postable || account.Synthetic {
			t.Fatalf("search returned a non-selectable account: %+v", account)
		}
	}
	exactSearch, err := store.SearchAccounts(t.Context(), "6281", 25)
	if err != nil || len(exactSearch) == 0 || exactSearch[0].Code != "6281" {
		t.Fatalf("exact 6281 search failed: %+v, %v", exactSearch, err)
	}
	byName, err := store.SearchAccounts(t.Context(), "telecomunicațiile", 25)
	if err != nil || len(byName) == 0 || byName[0].Code != "6262" {
		t.Fatal(byName, err)
	}
	if _, err = store.GetSelectableAccount(t.Context(), "628"); !errors.Is(err, apperrors.ErrValidation) {
		t.Fatalf("synthetic account accepted: %v", err)
	}
	if _, err = store.GetSelectableAccount(t.Context(), "999999"); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("unknown account accepted: %v", err)
	}
	inactiveCode := fmt.Sprintf("99%d", time.Now().UnixNano())
	inactiveID := "inactive-account-" + inactiveCode
	if _, err = store.DB.ExecContext(t.Context(), `INSERT INTO accounts(id,code,name,account_class,account_type,level,is_synthetic,is_active) VALUES($1,$2,'Inactive integration account',9,'test',3,false,false)`, inactiveID, inactiveCode); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = store.DB.ExecContext(t.Context(), `DELETE FROM accounts WHERE id=$1`, inactiveID) })
	if _, err = store.GetSelectableAccount(t.Context(), inactiveCode); !errors.Is(err, apperrors.ErrValidation) {
		t.Fatalf("inactive account accepted: %v", err)
	}
	inactiveSearch, err := store.SearchAccounts(t.Context(), inactiveCode, 25)
	if err != nil || len(inactiveSearch) != 0 {
		t.Fatalf("inactive account returned by search: %+v, %v", inactiveSearch, err)
	}
}

func TestAccountLearningRealPersistencePath(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	store, err := Open(url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	suffix := fmt.Sprintf("%x", time.Now().UnixNano())
	tc := &module5TestContext{ctx: t.Context(), store: store, clientID: "learning-client-" + suffix, now: time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)}
	if _, err = store.Client.AccountingClient.Create().SetID(tc.clientID).SetName("Generic learning client").SetCui("RO" + suffix).SetCreatedAt(tc.now).SetUpdatedAt(tc.now).Save(tc.ctx); err != nil {
		t.Fatal(err)
	}
	facts, lineFacts, _, _ := domainReleaseFixture(t, tc, func(pack *accounting.Pack) { pack.Rules[0].Predicate.SellerItemID = "NON_MATCHING_ACCOUNT_RULE" })
	service := classification.NewService(store, classification.DomainPolicy{AllowTestOnly: true}, func() time.Time { return tc.now })

	createInvoice := func(name, description string, lf *accounting.LineFacts) string {
		id := "learning-" + suffix + "-" + name
		if _, e := store.Client.Invoice.Create().SetID(id).SetClientID(tc.clientID).SetSupplierName("Generic supplier").SetSupplierCui(accountingtest.SupplierCUI).SetNormalizedSupplierCui(accountingtest.SupplierNormalizedCUI).SetDocumentNumber(id).SetNormalizedDocumentNumber(id).SetIssueDate(tc.now).SetIssueDay(tc.now).SetTotalAmount("121").SetCurrency("RON").SetSpvReference(id).SetIngestionSource("TEST_ONLY").SetExternalDeliveryID(id).SetModelVersion(accounting.ModelVersion).SetSourceFacts(facts).SetPipelineStatus(invoice.PipelineStatusCOMMERCIALLY_VALIDATED).SetSagaStatus(invoice.SagaStatusNOT_READY).SetRevision(1).SetCreatedAt(tc.now).SetUpdatedAt(tc.now).Save(tc.ctx); e != nil {
			t.Fatal(e)
		}
		tc.invoices = append(tc.invoices, id)
		if _, e := store.Client.InvoiceLine.Create().SetID(id + "-line").SetInvoiceID(id).SetPosition(1).SetDescription(description).SetUnit("H87").SetQuantity("1").SetUnitPrice("100").SetNetValue("100").SetVatRate("21").SetVatValue("21").SetTotalValue("121").SetSourceFacts(lf).Save(tc.ctx); e != nil {
			t.Fatal(e)
		}
		if _, _, e := service.ProcessInvoice(tc.ctx, classification.ProcessCommand{InvoiceID: id, ExpectedRevision: 1, CommandID: id + ":classify"}); e != nil {
			t.Fatal(e)
		}
		return id
	}
	accountDecision := func(id string) (*classification.Decision, uint64, uint64) {
		item, e := store.GetInvoice(tc.ctx, id)
		if e != nil {
			t.Fatal(e)
		}
		if item.ActiveTask == nil {
			t.Fatalf("%s has no review task", id)
		}
		for i := range item.ActiveTask.ClassificationItems {
			if item.ActiveTask.ClassificationItems[i].Dimension == classification.DimensionAccount {
				return &item.ActiveTask.ClassificationItems[i], item.Revision, item.ActiveTask.Revision
			}
		}
		t.Fatalf("%s has no account decision", id)
		return nil, 0, 0
	}
	review := func(id string, decision *classification.Decision, invoiceRevision, taskRevision uint64, accountCode, action, commandID, reason string) classification.ReviewCommand {
		command := classification.ReviewCommand{InvoiceID: id, TaskID: mustInvoice(t, store, id).ActiveTask.ID, ClassificationID: decision.ID, ExpectedInvoiceRevision: invoiceRevision, ExpectedTaskRevision: taskRevision, ExpectedClassificationRevision: decision.Revision, CommandID: commandID, ActorID: "accountant", ActorDisplay: "Generic accountant", Reason: reason, TypedValue: &accounting.Value{Kind: "ACCOUNT", Account: accountCode}, MappingAction: action}
		if decision.Mapping != nil {
			command.ExpectedMappingRevision = decision.Mapping.Revision
		}
		if _, e := service.Review(tc.ctx, command); e != nil {
			t.Fatal(e)
		}
		return command
	}

	i1 := createInvoice("i1", "Recurring technical service 2026-09", lineFacts)
	d, ir, tr := accountDecision(i1)
	if d.Source != classification.SourceNoMatch {
		t.Fatal(d.Source)
	}
	if d.MappingScope == nil || d.MappingScope.ClientDisplay != "Generic learning client" || d.MappingScope.SupplierDisplay == "" || d.MappingScope.ServiceIdentityKind != "SELLER_ITEM_ID" || d.MappingScope.ServiceIdentityValue != lineFacts.SellerItemID {
		t.Fatalf("missing reusable mapping scope preview: %+v", d.MappingScope)
	}
	invalidAccount := classification.ReviewCommand{InvoiceID: i1, TaskID: mustInvoice(t, store, i1).ActiveTask.ID, ClassificationID: d.ID, ExpectedInvoiceRevision: ir, ExpectedTaskRevision: tr, ExpectedClassificationRevision: d.Revision, CommandID: i1 + ":invalid-account", ActorDisplay: "Generic accountant", Reason: "Invalid catalogue selection", TypedValue: &accounting.Value{Kind: "ACCOUNT", Account: "999999"}, MappingAction: "OCCURRENCE_ONLY"}
	if _, e := service.Review(tc.ctx, invalidAccount); !errors.Is(e, apperrors.ErrValidation) {
		t.Fatalf("invalid account was confirmed: %v", e)
	}
	createCommand := review(i1, d, ir, tr, "6281", "CREATE", i1+":review", "Reusable accountant decision")
	if _, e := service.Review(tc.ctx, createCommand); e != nil {
		t.Fatal("idempotent replay", e)
	}
	historicalI1, _, _ := accountDecision(i1)
	if historicalI1.Status != classification.ReviewCorrected || !historicalI1.HumanReviewed || historicalI1.TypedValue == nil || historicalI1.TypedValue.Account != "6281" || historicalI1.EffectiveValue == nil || *historicalI1.EffectiveValue != "6281" {
		t.Fatalf("unexpected historical I1 decision after CREATE: %+v", historicalI1)
	}
	historicalI1Revision := historicalI1.Revision
	count, e := store.Client.AccountMappingVersion.Query().Count(tc.ctx)
	if e != nil || count < 1 {
		t.Fatal(count, e)
	}
	mappingRow, e := store.Client.AccountMapping.Query().Where(
		accountmapping.ClientIDEQ(tc.clientID),
		accountmapping.NormalizedSupplierIDEQ(accountingtest.SupplierNormalizedCUI),
	).Only(tc.ctx)
	if e != nil {
		t.Fatal(e)
	}
	accountRow, e := store.Client.Account.Query().Where(account.CodeEQ("6281")).Only(tc.ctx)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		_, _ = store.Client.Account.UpdateOneID(accountRow.ID).SetIsActive(true).SetIsSynthetic(false).Save(context.Background())
	})
	assertUnusableMappingRequiresReview := func(name string) {
		t.Helper()
		invoiceID := createInvoice(name, "Recurring technical service 2026-09", lineFacts)
		unresolved, _, _ := accountDecision(invoiceID)
		if unresolved.Source != classification.SourceNoMatch || unresolved.Mapping != nil || unresolved.ProposedTypedValue != nil || unresolved.Status != classification.ReviewPending {
			t.Fatalf("%s reused an account that is no longer selectable: %+v", name, unresolved)
		}
		if _, err := store.Client.AccountMapping.Get(tc.ctx, mappingRow.ID); err != nil {
			t.Fatalf("%s removed historical mapping: %v", name, err)
		}
		if _, err := store.Client.AccountMappingVersion.Query().Where(accountmappingversion.MappingIDEQ(mappingRow.ID), accountmappingversion.VersionEQ(1)).Only(tc.ctx); err != nil {
			t.Fatalf("%s removed historical mapping version: %v", name, err)
		}
		historical := mustInvoice(t, store, i1)
		for _, item := range historical.Lines[0].Classifications {
			if item.Dimension == classification.DimensionAccount {
				if item.Status != classification.ReviewCorrected || !item.HumanReviewed || item.Revision != historicalI1Revision || item.TypedValue == nil || item.TypedValue.Account != "6281" || item.EffectiveValue == nil || *item.EffectiveValue != "6281" {
					t.Fatalf("%s changed the historical I1 decision: %+v", name, item)
				}
				return
			}
		}
		t.Fatalf("%s lost the historical I1 ACCOUNT decision", name)
	}

	if _, e = store.Client.Account.UpdateOneID(accountRow.ID).SetIsActive(false).Save(tc.ctx); e != nil {
		t.Fatal(e)
	}
	assertUnusableMappingRequiresReview("inactive-account-i2")
	if _, e = store.Client.Account.UpdateOneID(accountRow.ID).SetIsActive(true).Save(tc.ctx); e != nil {
		t.Fatal(e)
	}

	if _, e = store.Client.Account.UpdateOneID(accountRow.ID).SetIsSynthetic(true).Save(tc.ctx); e != nil {
		t.Fatal(e)
	}
	assertUnusableMappingRequiresReview("non-postable-account-i2")
	if _, e = store.Client.Account.UpdateOneID(accountRow.ID).SetIsSynthetic(false).Save(tc.ctx); e != nil {
		t.Fatal(e)
	}

	i2 := createInvoice("i2", "Recurring technical service 2026-09", lineFacts)
	d, ir, tr = accountDecision(i2)
	if d.Source != classification.SourceLearnedMapping || d.ProposedTypedValue.Account != "6281" || d.Mapping == nil || d.Mapping.Version != 1 {
		t.Fatal(d)
	}
	mappingID := d.Mapping.MappingID
	review(i2, d, ir, tr, "6281", "VALIDATE", i2+":review", "Current occurrence validated")
	validated := mustInvoice(t, store, i2)
	foundValidationAudit := false
	for _, event := range validated.Activity {
		foundValidationAudit = foundValidationAudit || event.EventType == "ACCOUNT_MAPPING_SUGGESTION_VALIDATED"
	}
	if !foundValidationAudit {
		t.Fatal("learned mapping validation audit is missing")
	}
	for index := range validated.Lines[0].Classifications {
		candidate := validated.Lines[0].Classifications[index]
		if candidate.Dimension == classification.DimensionAccount && candidate.Status != classification.ReviewAccepted {
			t.Fatalf("learned suggestion validation was not accepted: %s", candidate.Status)
		}
	}
	versionCount, e := store.Client.AccountMappingVersion.Query().Where(accountmappingversion.MappingIDEQ(mappingID)).Count(tc.ctx)
	if e != nil || versionCount != 1 {
		t.Fatalf("validation created a mapping version: %d, %v", versionCount, e)
	}

	i3 := createInvoice("i3", "Recurring technical service 2026-09", lineFacts)
	d, ir, tr = accountDecision(i3)
	review(i3, d, ir, tr, "6262", "OCCURRENCE_ONLY", i3+":review", "One-off exception")
	i4 := createInvoice("i4", "Recurring technical service 2026-09", lineFacts)
	d, _, _ = accountDecision(i4)
	if d.ProposedTypedValue.Account != "6281" || d.Mapping.Version != 1 {
		t.Fatal("occurrence-only mutated mapping", d)
	}

	i5 := createInvoice("i5", "Recurring technical service 2026-09", lineFacts)
	d, ir, tr = accountDecision(i5)
	correctCommand := review(i5, d, ir, tr, "6262", "CORRECT", i5+":review", "Correction of reusable knowledge")
	if _, e := service.Review(tc.ctx, correctCommand); e != nil {
		t.Fatal("correction replay", e)
	}
	i6 := createInvoice("i6", "Recurring technical service 2026-09", lineFacts)
	d, _, _ = accountDecision(i6)
	if d.ProposedTypedValue.Account != "6262" || d.Mapping.Version != 2 {
		t.Fatal("correction not reused", d)
	}

	unknownFacts := *lineFacts
	unknownFacts.SellerItemID = "UNRELATED-ITEM"
	unknownFacts.StandardItemID = ""
	i7 := createInvoice("i7", "Unrelated manual service 2031-04", &unknownFacts)
	d, _, _ = accountDecision(i7)
	if d.Source == classification.SourceLearnedMapping {
		t.Fatal("unrelated mapping applied", d)
	}

	// Two occurrences classified at version 2: only the first correction may win.
	i8 := createInvoice("i8", "Recurring technical service 2026-09", lineFacts)
	i9 := createInvoice("i9", "Recurring technical service 2026-09", lineFacts)
	d8, ir8, tr8 := accountDecision(i8)
	d9, ir9, tr9 := accountDecision(i9)
	review(i8, d8, ir8, tr8, "6281", "CORRECT", i8+":review", "Concurrent correction winner")
	stale := classification.ReviewCommand{InvoiceID: i9, TaskID: mustInvoice(t, store, i9).ActiveTask.ID, ClassificationID: d9.ID, ExpectedInvoiceRevision: ir9, ExpectedTaskRevision: tr9, ExpectedClassificationRevision: d9.Revision, ExpectedMappingRevision: d9.Mapping.Revision, CommandID: i9 + ":review", ActorDisplay: "Generic accountant", Reason: "Concurrent correction loser", TypedValue: &accounting.Value{Kind: "ACCOUNT", Account: "6281"}, MappingAction: "CORRECT"}
	if _, e := service.Review(tc.ctx, stale); !errors.Is(e, apperrors.ErrConflict) {
		t.Fatalf("stale correction: %v", e)
	}

	i10 := createInvoice("i10", "Recurring technical service 2026-09", lineFacts)
	d, ir, tr = accountDecision(i10)
	review(i10, d, ir, tr, "6262", "POLICY_CHANGE", i10+":review", "Approved policy changed immediately")
	latest, e := store.Client.AccountMappingVersion.Query().Where(accountmappingversion.MappingIDEQ(d.Mapping.MappingID), accountmappingversion.VersionEQ(4)).Only(tc.ctx)
	if e != nil || latest.ChangeKind != accountmappingversion.ChangeKindPOLICY_CHANGE || latest.Reason == "" {
		t.Fatal(latest, e)
	}

	// Supplier and client are both part of lookup scope.
	supplierIsolated := createInvoice("supplier-isolated", "Recurring technical service 2026-09", lineFacts)
	if _, e = store.Client.Invoice.UpdateOneID(supplierIsolated).SetNormalizedSupplierCui("DIFFERENT-SUPPLIER").Save(tc.ctx); e != nil {
		t.Fatal(e)
	}
	input, e := store.LoadClassificationInput(tc.ctx, supplierIsolated)
	if e != nil || len(input.Mappings) != 0 {
		t.Fatal("supplier isolation", len(input.Mappings), e)
	}
	otherClient := "learning-other-" + suffix
	if _, e = store.Client.AccountingClient.Create().SetID(otherClient).SetName("Other generic client").SetCui("OTHER" + suffix).SetCreatedAt(tc.now).SetUpdatedAt(tc.now).Save(tc.ctx); e != nil {
		t.Fatal(e)
	}
	otherInvoice := "learning-other-invoice-" + suffix
	if _, e = store.Client.Invoice.Create().SetID(otherInvoice).SetClientID(otherClient).SetSupplierName("Generic supplier").SetSupplierCui(accountingtest.SupplierCUI).SetNormalizedSupplierCui(accountingtest.SupplierNormalizedCUI).SetDocumentNumber(otherInvoice).SetNormalizedDocumentNumber(otherInvoice).SetIssueDate(tc.now).SetIssueDay(tc.now).SetTotalAmount("121").SetCurrency("RON").SetSpvReference(otherInvoice).SetIngestionSource("TEST_ONLY").SetExternalDeliveryID(otherInvoice).SetModelVersion(accounting.ModelVersion).SetSourceFacts(facts).SetPipelineStatus(invoice.PipelineStatusCOMMERCIALLY_VALIDATED).SetSagaStatus(invoice.SagaStatusNOT_READY).SetRevision(1).SetCreatedAt(tc.now).SetUpdatedAt(tc.now).Save(tc.ctx); e != nil {
		t.Fatal(e)
	}
	if _, e = store.Client.InvoiceLine.Create().SetID(otherInvoice + "-line").SetInvoiceID(otherInvoice).SetPosition(1).SetDescription("Recurring technical service 2026-09").SetUnit("H87").SetQuantity("1").SetUnitPrice("100").SetNetValue("100").SetVatRate("21").SetVatValue("21").SetTotalValue("121").SetSourceFacts(lineFacts).Save(tc.ctx); e != nil {
		t.Fatal(e)
	}
	input, e = store.LoadClassificationInput(tc.ctx, otherInvoice)
	if e != nil || len(input.Mappings) != 0 {
		t.Fatal("client isolation", len(input.Mappings), e)
	}
}

func mustInvoice(t *testing.T, store *Store, id string) *invoicing.Invoice {
	t.Helper()
	item, err := store.GetInvoice(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	return item
}
