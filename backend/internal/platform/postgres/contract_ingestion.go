package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"

	"diana-contabilitate/backend/ent"
	"diana-contabilitate/backend/ent/activityevent"
	"diana-contabilitate/backend/ent/contractextractionattempt"
	"diana-contabilitate/backend/ent/contractsourcedocument"
	"diana-contabilitate/backend/ent/outboxentry"
	"diana-contabilitate/backend/internal/apperrors"
	"diana-contabilitate/backend/internal/contractingestion"
	"diana-contabilitate/backend/internal/contracts"
	"diana-contabilitate/backend/internal/outbox"
)

func (s *Store) CreateDocument(ctx context.Context, input contractingestion.Upload, id, hash string, now time.Time) (contractingestion.Document, bool, error) {
	existing, err := s.Client.ContractSourceDocument.Query().Where(contractsourcedocument.ClientIDEQ(input.ClientID), contractsourcedocument.Sha256EQ(hash)).Only(ctx)
	if err == nil {
		doc, convertErr := s.contractDocument(ctx, existing)
		return doc, true, convertErr
	}
	if !ent.IsNotFound(err) {
		return contractingestion.Document{}, false, err
	}
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return contractingestion.Document{}, false, err
	}
	rollback := func(cause error) (contractingestion.Document, bool, error) {
		_ = tx.Rollback()
		return contractingestion.Document{}, false, cause
	}
	if exists, lookupErr := tx.AccountingClient.Get(ctx, input.ClientID); lookupErr != nil || exists == nil {
		if ent.IsNotFound(lookupErr) {
			return rollback(apperrors.ErrNotFound)
		}
		return rollback(lookupErr)
	}
	create := tx.ContractSourceDocument.Create().SetID(id).SetClientID(input.ClientID).SetOriginalFilename(input.Filename).SetMimeType("application/pdf").SetSizeBytes(int64(len(input.Bytes))).SetSha256(hash).SetRawDocument(input.Bytes).SetUploadedAt(now).SetUpdatedAt(now)
	if input.Actor.ID != "" {
		create.SetUploadedByID(input.Actor.ID)
	}
	if input.Actor.Display != "" {
		create.SetUploadedByDisplay(input.Actor.Display)
	}
	row, err := create.Save(ctx)
	if ent.IsConstraintError(err) {
		_ = tx.Rollback()
		existing, err = s.Client.ContractSourceDocument.Query().Where(contractsourcedocument.ClientIDEQ(input.ClientID), contractsourcedocument.Sha256EQ(hash)).Only(ctx)
		if err != nil {
			return contractingestion.Document{}, false, err
		}
		doc, e := s.contractDocument(ctx, existing)
		return doc, true, e
	}
	if err != nil {
		return rollback(err)
	}
	payload, _ := json.Marshal(map[string]string{"document_id": id})
	_, err = tx.OutboxEntry.Create().SetID(stableID("outbox", "contract-extract:"+id)).SetEventType(outbox.EventContractExtractionRequested).SetAggregateType("CONTRACT_SOURCE_DOCUMENT").SetAggregateID(id).SetPayload(payload).SetIdempotencyKey("contract-extract:" + id).SetCorrelationID(input.Actor.CorrelationID).SetStatus(outboxentry.StatusPENDING).SetCreatedAt(now).SetAvailableAt(now).Save(ctx)
	if err != nil {
		return rollback(err)
	}
	err = createIngestionAudit(ctx, tx, input.ClientID, id, "CONTRACT_DOCUMENT_UPLOADED", "Documentul contractual PDF a fost încărcat și programat pentru extragere.", "contract-upload:"+id, input.Actor, now, nil, nil)
	if err != nil {
		return rollback(err)
	}
	if err = tx.Commit(); err != nil {
		return contractingestion.Document{}, false, err
	}
	return documentDomain(row, nil), false, nil
}

func documentMetadataFields() []string {
	fields := make([]string, 0, len(contractsourcedocument.Columns)-1)
	for _, name := range contractsourcedocument.Columns {
		if name != contractsourcedocument.FieldRawDocument {
			fields = append(fields, name)
		}
	}
	return fields
}

func (s *Store) ListDocuments(ctx context.Context, clientID string) ([]contractingestion.Document, error) {
	rows, err := s.Client.ContractSourceDocument.Query().Where(contractsourcedocument.ClientIDEQ(clientID)).Select(documentMetadataFields()...).Order(ent.Desc(contractsourcedocument.FieldUploadedAt)).All(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]contractingestion.Document, 0, len(rows))
	for _, row := range rows {
		doc, e := s.contractDocument(ctx, row)
		if e != nil {
			return nil, e
		}
		result = append(result, doc)
	}
	return result, nil
}
func (s *Store) GetDocument(ctx context.Context, clientID, id string) (contractingestion.Document, error) {
	row, err := s.Client.ContractSourceDocument.Query().Where(contractsourcedocument.IDEQ(id), contractsourcedocument.ClientIDEQ(clientID)).Select(documentMetadataFields()...).Only(ctx)
	if ent.IsNotFound(err) {
		return contractingestion.Document{}, apperrors.ErrNotFound
	}
	if err != nil {
		return contractingestion.Document{}, err
	}
	return s.contractDocument(ctx, row)
}
func (s *Store) GetSource(ctx context.Context, id string) (contractingestion.Source, error) {
	row, err := s.Client.ContractSourceDocument.Get(ctx, id)
	if ent.IsNotFound(err) {
		return contractingestion.Source{}, apperrors.ErrNotFound
	}
	if err != nil {
		return contractingestion.Source{}, err
	}
	doc, e := s.contractDocument(ctx, row)
	return contractingestion.Source{Document: doc, Bytes: append([]byte(nil), row.RawDocument...)}, e
}

func (s *Store) BeginExtraction(ctx context.Context, documentID, attemptID, provider, model string, now time.Time) (contractingestion.Document, bool, error) {
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return contractingestion.Document{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	row, err := tx.ContractSourceDocument.Get(ctx, documentID)
	if ent.IsNotFound(err) {
		return contractingestion.Document{}, false, apperrors.ErrNotFound
	}
	if err != nil {
		return contractingestion.Document{}, false, err
	}
	if row.Status == contractsourcedocument.StatusCONFIRMED {
		return documentDomain(row, nil), false, nil
	}
	if row.Status == contractsourcedocument.StatusREADY_FOR_REVIEW {
		return documentDomain(row, nil), false, nil
	}
	if row.Status == contractsourcedocument.StatusEXTRACTING && now.Sub(row.UpdatedAt) < 3*time.Minute {
		return contractingestion.Document{}, false, contractingestion.ErrExtractionBusy
	}
	expiredAttempt := row.LatestExtractionID
	wasExtracting := row.Status == contractsourcedocument.StatusEXTRACTING
	row, err = tx.ContractSourceDocument.UpdateOneID(documentID).Where(contractsourcedocument.RevisionEQ(row.Revision)).SetStatus(contractsourcedocument.StatusEXTRACTING).SetLatestExtractionID(attemptID).SetUpdatedAt(now).AddRevision(1).Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			err = contractingestion.ErrExtractionBusy
		}
		return contractingestion.Document{}, false, err
	}
	if wasExtracting && expiredAttempt != nil {
		if _, err = tx.ContractExtractionAttempt.Update().Where(contractextractionattempt.IDEQ(*expiredAttempt), contractextractionattempt.StatusEQ(contractextractionattempt.StatusSTARTED)).SetStatus(contractextractionattempt.StatusFAILED).SetSafeErrorCategory("LEASE_EXPIRED").SetCompletedAt(now).Save(ctx); err != nil {
			return contractingestion.Document{}, false, err
		}
	}
	attempt, err := tx.ContractExtractionAttempt.Create().SetID(attemptID).SetDocumentID(documentID).SetProvider(provider).SetModel(model).SetSchemaVersion(contractingestion.ExtractionSchemaVersion).SetPromptVersion(contractingestion.ExtractionPromptVersion).SetStatus(contractextractionattempt.StatusSTARTED).SetStartedAt(now).Save(ctx)
	if err != nil {
		return contractingestion.Document{}, false, err
	}
	if err = createIngestionAudit(ctx, tx, row.ClientID, documentID, "CONTRACT_EXTRACTION_STARTED", "Extragerea automată a documentului a început.", "contract-extraction-start:"+attemptID, contractingestion.Actor{}, now, nil, nil); err != nil {
		return contractingestion.Document{}, false, err
	}
	if err = tx.Commit(); err != nil {
		return contractingestion.Document{}, false, err
	}
	return documentDomain(row, attemptDomain(attempt)), true, nil
}
func (s *Store) CompleteExtraction(ctx context.Context, documentID, attemptID string, result contractingestion.ExtractionResult, now time.Time) error {
	proposal, err := json.Marshal(result.Proposal)
	if err != nil {
		return err
	}
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	update := tx.ContractExtractionAttempt.UpdateOneID(attemptID).Where(contractextractionattempt.StatusEQ(contractextractionattempt.StatusSTARTED)).SetStatus(contractextractionattempt.StatusSUCCEEDED).SetProposal(proposal).SetCompletedAt(now).ClearSafeErrorCategory()
	if result.InputTokens != nil {
		update.SetInputTokens(*result.InputTokens)
	}
	if result.OutputTokens != nil {
		update.SetOutputTokens(*result.OutputTokens)
	}
	if _, err = update.Save(ctx); err != nil {
		return err
	}
	row, err := tx.ContractSourceDocument.UpdateOneID(documentID).Where(contractsourcedocument.LatestExtractionIDEQ(attemptID), contractsourcedocument.StatusEQ(contractsourcedocument.StatusEXTRACTING)).SetStatus(contractsourcedocument.StatusREADY_FOR_REVIEW).SetUpdatedAt(now).AddRevision(1).Save(ctx)
	if ent.IsNotFound(err) {
		return apperrors.ErrConflict
	}
	if err != nil {
		return err
	}
	if err = createIngestionAudit(ctx, tx, row.ClientID, documentID, "CONTRACT_EXTRACTION_COMPLETED", "Propunerea structurată este pregătită pentru verificare umană.", "contract-extracted:"+attemptID, contractingestion.Actor{}, now, nil, nil); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) FailExtraction(ctx context.Context, documentID, attemptID, category string, now time.Time) error {
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ContractExtractionAttempt.UpdateOneID(attemptID).Where(contractextractionattempt.StatusEQ(contractextractionattempt.StatusSTARTED)).SetStatus(contractextractionattempt.StatusFAILED).SetSafeErrorCategory(category).SetCompletedAt(now).Save(ctx); err != nil {
		return err
	}
	row, err := tx.ContractSourceDocument.UpdateOneID(documentID).Where(contractsourcedocument.LatestExtractionIDEQ(attemptID), contractsourcedocument.StatusEQ(contractsourcedocument.StatusEXTRACTING)).SetStatus(contractsourcedocument.StatusEXTRACTION_FAILED).SetUpdatedAt(now).AddRevision(1).Save(ctx)
	if err != nil {
		return err
	}
	if err = createIngestionAudit(ctx, tx, row.ClientID, documentID, "CONTRACT_EXTRACTION_FAILED", "Extragerea automată nu a putut produce o propunere validă.", "contract-extraction-failed:"+attemptID, contractingestion.Actor{}, now, nil, nil); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ConfirmDocument(ctx context.Context, command contractingestion.ConfirmCommand, value contracts.Contract, now time.Time) (string, bool, error) {
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return "", false, err
	}
	defer func() { _ = tx.Rollback() }()
	confirmed, _ := json.Marshal(command.Contract)
	sum := sha256.Sum256(confirmed)
	fingerprint := hex.EncodeToString(sum[:])
	key := "ingestion-confirm:" + command.ClientID + ":" + command.DocumentID + ":" + command.CommandID
	doc, err := tx.ContractSourceDocument.Query().Where(contractsourcedocument.IDEQ(command.DocumentID), contractsourcedocument.ClientIDEQ(command.ClientID)).Only(ctx)
	if ent.IsNotFound(err) {
		return "", false, apperrors.ErrConflict
	}
	if err != nil {
		return "", false, err
	}
	if doc.Status == contractsourcedocument.StatusCONFIRMED && doc.ConfirmedContractID != nil {
		if doc.ConfirmationKey != nil && *doc.ConfirmationKey == key && doc.ConfirmationFingerprint != nil && *doc.ConfirmationFingerprint == fingerprint && doc.LatestExtractionID != nil && *doc.LatestExtractionID == command.ExtractionAttemptID {
			return *doc.ConfirmedContractID, false, nil
		}
		return "", false, apperrors.ErrConflict
	}
	if doc.Revision != command.ExpectedDocumentRevision || doc.Status != contractsourcedocument.StatusREADY_FOR_REVIEW || doc.LatestExtractionID == nil || *doc.LatestExtractionID != command.ExtractionAttemptID {
		return "", false, apperrors.ErrConflict
	}
	attempt, err := tx.ContractExtractionAttempt.Query().Where(contractextractionattempt.IDEQ(command.ExtractionAttemptID), contractextractionattempt.DocumentIDEQ(doc.ID), contractextractionattempt.StatusEQ(contractextractionattempt.StatusSUCCEEDED)).Only(ctx)
	if err != nil {
		return "", false, apperrors.ErrConflict
	}
	client, err := tx.AccountingClient.Get(ctx, doc.ClientID)
	if err != nil {
		return "", false, err
	}
	var proposal contractingestion.Proposal
	if json.Unmarshal(attempt.Proposal, &proposal) != nil {
		return "", false, apperrors.ErrConflict
	}
	if proposal.BuyerCUI.Value != nil && normalizeCUI(*proposal.BuyerCUI.Value) != normalizeCUI(client.Cui) {
		return "", false, contractingestion.ErrBuyerMismatch
	}
	if _, err = tx.ContractSourceDocument.UpdateOne(doc).Where(contractsourcedocument.RevisionEQ(command.ExpectedDocumentRevision), contractsourcedocument.StatusEQ(contractsourcedocument.StatusREADY_FOR_REVIEW)).AddRevision(1).SetUpdatedAt(now).Save(ctx); err != nil {
		if ent.IsNotFound(err) {
			err = apperrors.ErrConflict
		}
		return "", false, err
	}
	sourceRef := "Contract PDF: " + doc.OriginalFilename
	sourceMeta := fmt.Sprintf("sha256=%s; extraction=%s; schema=%s", doc.Sha256, attempt.ID, attempt.SchemaVersion)
	created, err := tx.Contract.Create().SetID(value.ID).SetClientID(value.ClientID).SetSupplierName(value.SupplierName).SetSupplierCui(value.SupplierCUI).SetNormalizedSupplierCui(value.NormalizedSupplierCUI).SetReference(value.Reference).SetEffectiveFrom(value.EffectiveFrom).SetEffectiveTo(value.EffectiveTo).SetTotalValue(value.Value.Amount.String()).SetCurrency(value.Value.Currency).SetUnitType(value.UnitType).SetPaymentTerms(value.PaymentTerms).SetSourceReference(sourceRef).SetSourceMetadata(sourceMeta).SetSourceDocumentID(doc.ID).SetExtractionAttemptID(attempt.ID).SetRevision(1).SetCreatedAt(now).SetUpdatedAt(now).Save(ctx)
	if ent.IsConstraintError(err) {
		return "", false, apperrors.ErrConflict
	}
	if err != nil {
		return "", false, err
	}
	doc, err = tx.ContractSourceDocument.UpdateOne(doc).SetStatus(contractsourcedocument.StatusCONFIRMED).SetConfirmedValues(confirmed).SetConfirmationKey(key).SetConfirmationFingerprint(fingerprint).SetConfirmedContractID(created.ID).SetConfirmedAt(now).SetNillableConfirmedByID(optional(command.Actor.ID)).SetNillableConfirmedByDisplay(optional(command.Actor.Display)).SetUpdatedAt(now).Save(ctx)
	if err != nil {
		return "", false, err
	}
	if err = createIngestionAudit(ctx, tx, doc.ClientID, doc.ID, "CONTRACT_CONFIRMED", "Datele revizuite au devenit contractul autoritativ folosit pentru asocierea facturilor.", key, command.Actor, now, nil, nil); err != nil {
		return "", false, err
	}
	activationKey := "contract-ingestion:" + doc.ID + ":available"
	payload, _ := json.Marshal(map[string]string{"contract_id": created.ID})
	if _, err = tx.OutboxEntry.Create().SetID(stableID("outbox", activationKey)).SetEventType(outbox.EventContractActivationRequested).SetAggregateType("CONTRACT").SetAggregateID(created.ID).SetPayload(payload).SetIdempotencyKey(activationKey).SetCorrelationID(command.Actor.CorrelationID).SetCreatedAt(now).SetAvailableAt(now).Save(ctx); err != nil {
		return "", false, err
	}
	if err = tx.Commit(); err != nil {
		return "", false, err
	}
	return created.ID, true, nil
}

func (s *Store) contractDocument(ctx context.Context, row *ent.ContractSourceDocument) (contractingestion.Document, error) {
	var attempt *contractingestion.Attempt
	if row.LatestExtractionID != nil {
		a, err := s.Client.ContractExtractionAttempt.Get(ctx, *row.LatestExtractionID)
		if err == nil {
			attempt = attemptDomain(a)
		} else if !ent.IsNotFound(err) {
			return contractingestion.Document{}, err
		}
	}
	doc := documentDomain(row, attempt)
	if len(row.ConfirmedValues) > 0 {
		var values contractingestion.ReviewedContract
		if json.Unmarshal(row.ConfirmedValues, &values) == nil {
			doc.ConfirmedValues = &values
		}
	}
	history, err := s.Client.ContractExtractionAttempt.Query().Where(contractextractionattempt.DocumentIDEQ(row.ID)).Order(ent.Desc(contractextractionattempt.FieldStartedAt)).All(ctx)
	if err != nil {
		return contractingestion.Document{}, err
	}
	for _, a := range history {
		doc.Attempts = append(doc.Attempts, *attemptDomain(a))
	}
	if attempt != nil && attempt.Proposal != nil && attempt.Proposal.BuyerCUI.Value != nil {
		client, err := s.Client.AccountingClient.Get(ctx, row.ClientID)
		if err == nil {
			doc.BuyerMismatch = normalizeCUI(*attempt.Proposal.BuyerCUI.Value) != normalizeCUI(client.Cui)
		}
	}
	return doc, nil
}
func documentDomain(row *ent.ContractSourceDocument, attempt *contractingestion.Attempt) contractingestion.Document {
	return contractingestion.Document{ID: row.ID, ClientID: row.ClientID, OriginalFilename: row.OriginalFilename, MIMEType: row.MimeType, SizeBytes: row.SizeBytes, SHA256: row.Sha256, Status: contractingestion.Status(row.Status), LifecycleState: string(row.LifecycleState), LatestExtractionID: row.LatestExtractionID, ConfirmedContractID: row.ConfirmedContractID, UploadedByID: row.UploadedByID, UploadedByDisplay: row.UploadedByDisplay, UploadedAt: row.UploadedAt, UpdatedAt: row.UpdatedAt, ConfirmedByID: row.ConfirmedByID, ConfirmedByDisplay: row.ConfirmedByDisplay, ConfirmedAt: row.ConfirmedAt, Revision: row.Revision, LatestAttempt: attempt}
}
func attemptDomain(row *ent.ContractExtractionAttempt) *contractingestion.Attempt {
	a := &contractingestion.Attempt{ID: row.ID, Provider: row.Provider, Model: row.Model, SchemaVersion: row.SchemaVersion, PromptVersion: row.PromptVersion, Status: string(row.Status), SafeErrorCategory: valueOrEmpty(row.SafeErrorCategory), InputTokens: row.InputTokens, OutputTokens: row.OutputTokens, StartedAt: row.StartedAt, CompletedAt: row.CompletedAt}
	if len(row.Proposal) > 0 {
		var p contractingestion.Proposal
		if json.Unmarshal(row.Proposal, &p) == nil {
			a.Proposal = &p
		}
	}
	return a
}
func createIngestionAudit(ctx context.Context, tx *ent.Tx, clientID, documentID, eventType, detail, key string, actor contractingestion.Actor, now time.Time, before, after json.RawMessage) error {
	create := tx.ActivityEvent.Create().SetID(stableID("evt", eventType+":"+documentID+":"+key)).SetClientID(clientID).SetAggregateType("CONTRACT_SOURCE_DOCUMENT").SetAggregateID(documentID).SetEventType(eventType).SetOccurredAt(now).SetActorKind(activityevent.ActorKindUSER).SetAutomatic(false).SetDetail(detail).SetIdempotencyKey(key)
	if actor.ID == "" {
		create.SetActorKind(activityevent.ActorKindSYSTEM).SetAutomatic(true)
	} else {
		create.SetActorID(actor.ID)
	}
	if actor.Display != "" {
		create.SetActorDisplay(actor.Display)
	}
	if actor.CorrelationID != "" {
		create.SetCorrelationID(actor.CorrelationID)
	}
	if len(before) > 0 {
		create.SetBeforeSnapshot(before)
	}
	if len(after) > 0 {
		create.SetAfterSnapshot(after)
	}
	_, err := create.Save(ctx)
	return err
}
func normalizeCUI(value string) string {
	value = strings.ToUpper(strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, value))
	return strings.TrimPrefix(value, "RO")
}
func optional(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func (s *Store) RetryExtraction(ctx context.Context, clientID, documentID string, revision uint64, actor contractingestion.Actor, now time.Time) error {
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	row, err := tx.ContractSourceDocument.UpdateOneID(documentID).Where(contractsourcedocument.ClientIDEQ(clientID), contractsourcedocument.RevisionEQ(revision), contractsourcedocument.StatusIn(contractsourcedocument.StatusEXTRACTION_FAILED, contractsourcedocument.StatusREADY_FOR_REVIEW)).SetStatus(contractsourcedocument.StatusUPLOADED).SetUpdatedAt(now).AddRevision(1).Save(ctx)
	if ent.IsNotFound(err) {
		return apperrors.ErrConflict
	}
	if err != nil {
		return err
	}
	key := fmt.Sprintf("contract-extract:%s:revision:%d", documentID, row.Revision)
	payload, _ := json.Marshal(map[string]string{"document_id": documentID})
	if _, err = tx.OutboxEntry.Create().SetID(stableID("outbox", key)).SetEventType(outbox.EventContractExtractionRequested).SetAggregateType("CONTRACT_SOURCE_DOCUMENT").SetAggregateID(documentID).SetPayload(payload).SetIdempotencyKey(key).SetCorrelationID(actor.CorrelationID).SetCreatedAt(now).SetAvailableAt(now).Save(ctx); err != nil {
		return err
	}
	return tx.Commit()
}
