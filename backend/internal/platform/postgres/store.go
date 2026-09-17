package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"diana-contabilitate/backend/ent"
	"diana-contabilitate/backend/ent/activityevent"
	"diana-contabilitate/backend/ent/contractmatchcandidate"
	"diana-contabilitate/backend/ent/invoice"
	"diana-contabilitate/backend/ent/invoiceline"
	"diana-contabilitate/backend/ent/lineclassification"
	"diana-contabilitate/backend/ent/outboxentry"
	entvalidationtask "diana-contabilitate/backend/ent/validationtask"
	"diana-contabilitate/backend/internal/accounting"
	"diana-contabilitate/backend/internal/accountingdate"
	"diana-contabilitate/backend/internal/apperrors"
	"diana-contabilitate/backend/internal/audit"
	classificationdomain "diana-contabilitate/backend/internal/classification"
	"diana-contabilitate/backend/internal/clients"
	"diana-contabilitate/backend/internal/contracts"
	"diana-contabilitate/backend/internal/invoicing"
	"diana-contabilitate/backend/internal/money"
	"diana-contabilitate/backend/internal/outbox"
	"diana-contabilitate/backend/internal/rules"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/jackc/pgx/v5/stdlib"
)

type Store struct {
	AccountingReadinessObserver interface{ AccountingReadinessEvaluated(bool) }
	DB                          *sql.DB
	Client                      *ent.Client
}

type PoolConfig struct {
	MaxOpenConnections int
	MaxIdleConnections int
	ConnectionLifetime time.Duration
}

func Open(databaseURL string) (*Store, error) {
	return OpenWithPool(databaseURL, PoolConfig{MaxOpenConnections: 20, MaxIdleConnections: 5, ConnectionLifetime: 30 * time.Minute})
}

func OpenWithPool(databaseURL string, pool PoolConfig) (*Store, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	db.SetMaxOpenConns(pool.MaxOpenConnections)
	db.SetMaxIdleConns(pool.MaxIdleConnections)
	db.SetConnMaxLifetime(pool.ConnectionLifetime)
	driver := entsql.OpenDB(dialect.Postgres, db)
	return &Store{DB: db, Client: ent.NewClient(ent.Driver(driver))}, nil
}

func (s *Store) Close() error { return s.Client.Close() }

func (s *Store) Ping(ctx context.Context) error { return s.DB.PingContext(ctx) }

func (s *Store) ListClients(ctx context.Context) ([]clients.Client, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+clientColumns+` FROM clients ORDER BY name,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []clients.Client{}
	for rows.Next() {
		c, e := scanClient(rows)
		if e != nil {
			return nil, e
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

func (s *Store) GetInvoice(ctx context.Context, id string) (*invoicing.Invoice, error) {
	row, err := s.Client.Invoice.Query().
		Where(invoice.IDEQ(id)).
		WithLines(func(query *ent.InvoiceLineQuery) {
			query.Order(ent.Asc(invoiceline.FieldPosition)).WithClassifications(func(classificationQuery *ent.LineClassificationQuery) {
				classificationQuery.Order(ent.Asc(lineclassification.FieldDimension)).WithRuleVersion(func(versionQuery *ent.RuleVersionQuery) { versionQuery.WithRule() })
			})
		}).
		WithActivityEvents(func(query *ent.ActivityEventQuery) {
			query.Order(ent.Asc(activityevent.FieldOccurredAt))
		}).
		WithValidationTasks(func(query *ent.ValidationTaskQuery) {
			query.Where(entvalidationtask.StatusNEQ(entvalidationtask.StatusRESOLVED)).Order(ent.Desc(entvalidationtask.FieldCreatedAt)).Limit(1).
				WithContractMatchRun(func(runQuery *ent.ContractMatchRunQuery) {
					runQuery.WithCandidates(func(candidateQuery *ent.ContractMatchCandidateQuery) {
						candidateQuery.Order(ent.Asc(contractmatchcandidate.FieldRank)).WithContract()
					})
				})
		}).
		WithContractAssociation().
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil, apperrors.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get invoice: %w", err)
	}
	result, err := invoiceDomain(row)
	if err != nil {
		return nil, err
	}
	for _, event := range row.Edges.ActivityEvents {
		result.Activity = append(result.Activity, audit.Event{
			ID: event.ID, ClientID: event.ClientID, InvoiceID: event.InvoiceID,
			ValidationTaskID: event.ValidationTaskID,
			AggregateType:    event.AggregateType, AggregateID: event.AggregateID,
			EventType: event.EventType, OccurredAt: event.OccurredAt,
			ActorKind: audit.ActorKind(event.ActorKind), ActorID: event.ActorID,
			ActorDisplay: event.ActorDisplay, Automatic: event.Automatic,
			Detail: event.Detail, BeforeSnapshot: event.BeforeSnapshot,
			AfterSnapshot: event.AfterSnapshot, CorrelationID: event.CorrelationID,
		})
	}
	if len(row.Edges.ValidationTasks) == 1 {
		result.ActiveTask = validationTaskDomain(row.Edges.ValidationTasks[0])
		if result.ActiveTask.Type == "CLASSIFICATION" {
			for _, lineRow := range row.Edges.Lines {
				for _, classificationRow := range lineRow.Edges.Classifications {
					if classificationRow.RequiredReview || classificationRow.ModelVersion == accounting.ModelVersion {
						result.ActiveTask.ClassificationItems = append(result.ActiveTask.ClassificationItems, lineClassificationDomain(classificationRow, lineRow))
					}
				}
			}
		}
	}
	if association := row.Edges.ContractAssociation; association != nil {
		value, parseErr := money.Parse(association.TotalValue)
		if parseErr != nil {
			return nil, fmt.Errorf("read contract association amount: %w", parseErr)
		}
		result.ContractAssociation = &contracts.AssociationSnapshot{
			ContractID: association.ContractID, Reference: association.ContractReference, SupplierName: association.SupplierName,
			EffectiveFrom: association.EffectiveFrom, EffectiveTo: association.EffectiveTo,
			Value: money.Money{Amount: value, Currency: association.Currency}, UnitType: association.UnitType,
			PaymentTerms: association.PaymentTerms, PolicyVersion: association.PolicyVersion,
			AssociationKind: contracts.AssociationKind(association.AssociationKind), AssociatedAt: association.AssociatedAt,
			AssociatedByID: association.AssociatedByID, AssociatedByName: association.AssociatedByDisplay,
		}
	}
	return result, nil
}

func (s *Store) ListInvoices(ctx context.Context, filter invoicing.Filter) ([]invoicing.Invoice, error) {
	query := s.Client.Invoice.Query()
	if filter.ClientID != "" {
		query.Where(invoice.ClientIDEQ(filter.ClientID))
	}
	ids, err := query.Order(ent.Desc(invoice.FieldIssueDate), ent.Asc(invoice.FieldID)).IDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list invoice identities: %w", err)
	}
	result := make([]invoicing.Invoice, 0, len(ids))
	for _, id := range ids {
		item, getErr := s.GetInvoice(ctx, id)
		if getErr != nil {
			return nil, getErr
		}
		result = append(result, *item)
	}
	return result, nil
}

func invoiceDomain(row *ent.Invoice) (*invoicing.Invoice, error) {
	amount, err := money.Parse(row.TotalAmount)
	if err != nil {
		return nil, fmt.Errorf("read invoice amount: %w", err)
	}
	result := &invoicing.Invoice{ModelVersion: row.ModelVersion, SourceFacts: row.SourceFacts, AccountingSnapshot: row.AccountingSnapshot, ReadinessReason: row.ReadinessReason,
		ID: row.ID, ClientID: row.ClientID, SupplierName: row.SupplierName,
		SupplierCUI: row.SupplierCui, NormalizedSupplierCUI: row.NormalizedSupplierCui,
		DocumentNumber: row.DocumentNumber, NormalizedDocumentNumber: row.NormalizedDocumentNumber,
		IssueDate: row.IssueDate, IssueDay: row.IssueDay, DueDate: row.DueDate,
		Total:        money.Money{Amount: amount, Currency: row.Currency},
		SPVReference: row.SpvReference, PipelineStatus: invoicing.PipelineStatus(row.PipelineStatus),
		SagaStatus: invoicing.SagaStatus(row.SagaStatus), Revision: row.Revision,
		IngestionSource: row.IngestionSource, ExternalDeliveryID: row.ExternalDeliveryID,
		DuplicateOfInvoiceID: row.DuplicateOfInvoiceID, DuplicateAmountMatches: row.DuplicateAmountMatches,
		DuplicateCurrencyMatches: row.DuplicateCurrencyMatches, DocumentType: invoicing.DocumentType(row.DocumentType),
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
	for _, lineRow := range row.Edges.Lines {
		values := make([]money.Amount, 0, 6)
		for _, raw := range []string{lineRow.VatRate, lineRow.VatValue, lineRow.Quantity, lineRow.UnitPrice, lineRow.NetValue, lineRow.TotalValue} {
			value, parseErr := money.Parse(raw)
			if parseErr != nil {
				return nil, fmt.Errorf("read invoice line decimal: %w", parseErr)
			}
			values = append(values, value)
		}
		line := invoicing.Line{SourceFacts: lineRow.SourceFacts, ID: lineRow.ID, Position: lineRow.Position, Description: lineRow.Description, Unit: lineRow.Unit, VATRate: values[0], VATValue: values[1], Quantity: values[2], UnitPrice: values[3], NetValue: values[4], TotalValue: values[5], AdditionalInfo: lineRow.AdditionalInfo}
		for _, classificationRow := range lineRow.Edges.Classifications {
			line.Classifications = append(line.Classifications, lineClassificationDomain(classificationRow, lineRow))
		}
		result.Lines = append(result.Lines, line)
	}
	return result, nil
}

func lineClassificationDomain(row *ent.LineClassification, line *ent.InvoiceLine) classificationdomain.Decision {
	result := classificationdomain.Decision{ModelVersion: row.ModelVersion, TypedValue: row.EffectiveTypedValue, ProposedTypedValue: row.ProposedTypedValue, Evidence: row.DecisionEvidence, ReviewReason: row.ReviewReason,
		ID: row.ID, ClientID: row.ClientID, InvoiceID: row.InvoiceID, InvoiceLineID: row.InvoiceLineID,
		LineLabel: fmt.Sprintf("Linia %d · %s", line.Position, line.Description), Dimension: classificationdomain.Dimension(row.Dimension),
		ProposedValue: row.ProposedValue, EffectiveValue: row.EffectiveValue, Confidence: row.ConfidenceDisplay,
		Explanation: row.Explanation, LegalBasis: row.LegalBasis, Status: classificationdomain.ReviewStatus(row.ReviewStatus),
		InvoiceDateUsed: dateFromOptional(row.InvoiceDateUsed), HumanReviewed: row.ReviewedAt != nil && row.ReviewedByDisplay != nil && *row.ReviewedByDisplay != "",
		Source: classificationdomain.Source(row.Source), PolicyVersion: row.PolicyVersion, Revision: row.Revision,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
	if version := row.Edges.RuleVersion; version != nil && version.Edges.Rule != nil {
		rule := version.Edges.Rule
		result.Rule = &classificationdomain.RuleReference{RuleID: rule.ID, RuleVersionID: version.ID, Reference: rule.Reference, Version: version.Version, Origin: rules.Scope(rule.Scope), ProductionEligible: version.ProductionEligible, RulePackVersion: version.RulePackVersion, Provenance: version.Provenance, EffectiveFrom: accountingdate.FromTime(version.EffectiveFrom), EffectiveTo: accountingDatePointer(version.EffectiveTo)}
	}
	return result
}

func (s *Store) IngestInvoice(ctx context.Context, input invoicing.IngestionInput, key string, now time.Time) (*invoicing.Invoice, bool, error) {
	existing, err := s.invoiceBySPVReference(ctx, input.ClientID, input.SPVReference)
	if err == nil {
		return existing, false, nil
	}
	if !errors.Is(err, apperrors.ErrNotFound) {
		return nil, false, err
	}
	normalizedDocumentNumber := invoicing.NormalizeBusinessIdentifier(input.DocumentNumber)
	issueDay := invoicing.InvoiceIssueDay(input.IssueDate)
	var normalizedSupplierCUI *string
	if input.SupplierCUI != nil {
		normalized := invoicing.NormalizeBusinessIdentifier(*input.SupplierCUI)
		if normalized != "" {
			normalizedSupplierCUI = &normalized
		}
	}
	canonical, err := s.canonicalByBusinessIdentity(ctx, input.ClientID, normalizedSupplierCUI, normalizedDocumentNumber, issueDay)
	if err != nil && !errors.Is(err, apperrors.ErrNotFound) {
		return nil, false, err
	}

	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return nil, false, err
	}
	rollback := func(cause error) (*invoicing.Invoice, bool, error) {
		_ = tx.Rollback()
		return nil, false, cause
	}
	invoiceID := stableID("inv", key)
	if input.ID != "" {
		invoiceID = input.ID
	}
	create := tx.Invoice.Create().SetID(invoiceID).SetClientID(input.ClientID).SetSupplierName(input.SupplierName).
		SetDocumentNumber(input.DocumentNumber).SetNormalizedDocumentNumber(normalizedDocumentNumber).
		SetModelVersion(ingestionModel(input)).SetSourceFacts(input.SourceFacts).SetIssueDate(input.IssueDate).SetIssueDay(issueDay).SetTotalAmount(input.Total.Amount.String()).
		SetCurrency(input.Total.Currency).SetSpvReference(input.SPVReference).SetIngestionSource(input.Source).
		SetExternalDeliveryID(input.ExternalDeliveryID).SetDocumentType(invoice.DocumentType(input.DocumentType)).SetPipelineStatus(invoice.PipelineStatusDOWNLOADED).
		SetSagaStatus(invoice.SagaStatusNOT_READY).SetRevision(1).SetCreatedAt(now).SetUpdatedAt(now)
	if input.SupplierCUI != nil {
		create.SetSupplierCui(*input.SupplierCUI)
	}
	if normalizedSupplierCUI != nil {
		create.SetNormalizedSupplierCui(*normalizedSupplierCUI)
	}
	if input.DueDate != nil {
		create.SetDueDate(*input.DueDate)
	}
	if canonical != nil {
		amountMatches := input.Total.Amount.Equal(canonical.Total.Amount)
		currencyMatches := input.Total.Currency == canonical.Total.Currency
		create.SetPipelineStatus(invoice.PipelineStatusDUPLICATE).
			SetDuplicateOfInvoiceID(canonical.ID).
			SetDuplicateAmountMatches(amountMatches).
			SetDuplicateCurrencyMatches(currencyMatches)
	}
	if _, err = create.Save(ctx); err != nil {
		_ = tx.Rollback()
		if IsConstraintError(err) {
			if item, lookupErr := s.invoiceBySPVReference(ctx, input.ClientID, input.SPVReference); lookupErr == nil {
				return item, false, nil
			}
			if canonical == nil {
				if racedCanonical, lookupErr := s.canonicalByBusinessIdentity(ctx, input.ClientID, normalizedSupplierCUI, normalizedDocumentNumber, issueDay); lookupErr == nil && racedCanonical != nil {
					return s.IngestInvoice(ctx, input, key, now)
				}
			}
		}
		return nil, false, err
	}
	for _, line := range input.Lines {
		createLine := tx.InvoiceLine.Create().SetID(stableID("line", fmt.Sprintf("%s:%d", key, line.Position))).SetInvoiceID(invoiceID).
			SetSourceFacts(line.SourceFacts).SetPosition(line.Position).SetDescription(line.Description).SetUnit(line.Unit).SetVatRate(line.VATRate.String()).
			SetVatValue(line.VATValue.String()).SetQuantity(line.Quantity.String()).SetUnitPrice(line.UnitPrice.String()).
			SetNetValue(line.NetValue.String()).SetTotalValue(line.TotalValue.String())
		if line.AdditionalInfo != nil {
			createLine.SetAdditionalInfo(*line.AdditionalInfo)
		}
		if _, err = createLine.Save(ctx); err != nil {
			return rollback(err)
		}
	}
	eventType := "INVOICE_INGESTED"
	to := invoicing.StatusDownloaded
	detail := "Invoice accepted using durable technical ingestion identity."
	if input.Source == "ANAF_SPV" {
		detail = "Invoice imported from ANAF/SPV using immutable source delivery " + input.ExternalDeliveryID + "."
	}
	if canonical != nil {
		eventType = "INVOICE_DUPLICATE_DETECTED"
		to = invoicing.StatusDuplicate
		detail = fmt.Sprintf("Business duplicate of %s; amount match=%t, currency match=%t. Duplicate is terminal.", canonical.ID, input.Total.Amount.Equal(canonical.Total.Amount), input.Total.Currency == canonical.Total.Currency)
	}
	if err = createAudit(tx, ctx, auditRecord{key: "ingest:" + key, invoiceID: invoiceID, clientID: input.ClientID, eventType: eventType, to: string(to), trigger: "TECHNICAL_INGESTION", detail: detail, actor: audit.ActorExternal, correlationID: input.ExternalDeliveryID, at: now}); err != nil {
		return rollback(err)
	}
	if canonical == nil {
		if err = createOutbox(tx, ctx, invoiceID, "ingest:"+key+":continue", input.ExternalDeliveryID, now); err != nil {
			return rollback(err)
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, false, err
	}
	item, err := s.GetInvoice(ctx, invoiceID)
	return item, true, err
}

func (s *Store) ApplyTransition(ctx context.Context, command invoicing.TransitionCommand, definition invoicing.TransitionDefinition, now time.Time) (*invoicing.Invoice, bool, error) {
	eventKey := "transition:" + command.CommandID
	if exists, err := s.auditExists(ctx, eventKey); err != nil {
		return nil, false, err
	} else if exists {
		item, getErr := s.GetInvoice(ctx, command.InvoiceID)
		return item, false, getErr
	}
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return nil, false, err
	}
	update := tx.Invoice.UpdateOneID(command.InvoiceID).Where(invoice.PipelineStatusEQ(invoice.PipelineStatus(command.From)), invoice.RevisionEQ(command.ExpectedRevision)).
		SetPipelineStatus(invoice.PipelineStatus(command.To)).AddRevision(1).SetUpdatedAt(now)
	switch command.To {
	case invoicing.StatusReadyForSAGA:
		update.SetSagaStatus(invoice.SagaStatusREADY)
	case invoicing.StatusExporting:
		update.SetSagaStatus(invoice.SagaStatusEXPORTING)
	case invoicing.StatusExported:
		update.SetSagaStatus(invoice.SagaStatusEXPORTED)
	case invoicing.StatusDuplicate:
		update.SetSagaStatus(invoice.SagaStatusNOT_READY)
	}
	row, err := update.Save(ctx)
	if err != nil {
		_ = tx.Rollback()
		if ent.IsNotFound(err) {
			if exists, checkErr := s.auditExists(ctx, eventKey); checkErr == nil && exists {
				item, getErr := s.GetInvoice(ctx, command.InvoiceID)
				return item, false, getErr
			}
			return nil, false, apperrors.ErrConflict
		}
		return nil, false, err
	}
	if err = createAudit(tx, ctx, auditRecord{key: eventKey, invoiceID: row.ID, clientID: row.ClientID, eventType: "INVOICE_PIPELINE_TRANSITION", from: string(command.From), to: string(command.To), trigger: string(command.Trigger), detail: fmt.Sprintf("Pipeline transitioned from %s to %s.", command.From, command.To), actor: command.Actor, actorDisplay: command.ActorDisplay, correlationID: command.CorrelationID, at: now}); err != nil {
		_ = tx.Rollback()
		return nil, false, err
	}
	if definition.ContinueAsynchronously {
		if err = createOutbox(tx, ctx, row.ID, command.CommandID+":continue", command.CorrelationID, now); err != nil {
			_ = tx.Rollback()
			return nil, false, err
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, false, err
	}
	item, err := s.GetInvoice(ctx, row.ID)
	return item, true, err
}

func (s *Store) RecordSagaFailure(ctx context.Context, id string, revision uint64, key, correlationID string, now time.Time) (*invoicing.Invoice, bool, error) {
	eventKey := "saga-failure:" + key
	if exists, err := s.auditExists(ctx, eventKey); err != nil {
		return nil, false, err
	} else if exists {
		item, getErr := s.GetInvoice(ctx, id)
		return item, false, getErr
	}
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return nil, false, err
	}
	row, err := tx.Invoice.UpdateOneID(id).Where(invoice.PipelineStatusEQ(invoice.PipelineStatusEXPORTING), invoice.RevisionEQ(revision)).SetSagaStatus(invoice.SagaStatusFAILED).AddRevision(1).SetUpdatedAt(now).Save(ctx)
	if err != nil {
		_ = tx.Rollback()
		if ent.IsNotFound(err) {
			return nil, false, apperrors.ErrConflict
		}
		return nil, false, err
	}
	if err = createAudit(tx, ctx, auditRecord{key: eventKey, invoiceID: id, clientID: row.ClientID, eventType: "SAGA_EXPORT_FAILED", from: string(invoicing.StatusExporting), to: string(invoicing.StatusExporting), trigger: "SAGA_EXPORT_FAILED", detail: "SAGA export failed permanently; no validation task was created.", actor: audit.ActorSystem, actorDisplay: "Sistem pipeline", correlationID: correlationID, at: now}); err != nil {
		_ = tx.Rollback()
		return nil, false, err
	}
	if err = tx.Commit(); err != nil {
		return nil, false, err
	}
	item, err := s.GetInvoice(ctx, id)
	return item, true, err
}

func (s *Store) PendingOutbox(ctx context.Context, limit int, now time.Time) ([]invoicing.OutboxEntry, error) {
	rows, err := s.Client.OutboxEntry.Query().Where(
		outboxentry.EventTypeEQ(outbox.EventInvoiceContinue),
		outboxentry.StatusEQ(outboxentry.StatusPENDING),
		outboxentry.AvailableAtLTE(now),
	).Order(ent.Asc(outboxentry.FieldCreatedAt)).Limit(limit).All(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]invoicing.OutboxEntry, 0, len(rows))
	for _, row := range rows {
		result = append(result, invoicing.OutboxEntry{ID: row.ID, EventType: row.EventType, AggregateID: row.AggregateID, IdempotencyKey: row.IdempotencyKey, Attempts: row.Attempts})
	}
	return result, nil
}

func (s *Store) MarkOutboxProcessed(ctx context.Context, id string, now time.Time) error {
	_, err := s.Client.OutboxEntry.UpdateOneID(id).Where(outboxentry.StatusEQ(outboxentry.StatusPENDING)).SetStatus(outboxentry.StatusPROCESSED).SetProcessedAt(now).AddAttempts(1).Save(ctx)
	if ent.IsNotFound(err) {
		return nil
	}
	return err
}

func (s *Store) invoiceBySPVReference(ctx context.Context, clientID, spvReference string) (*invoicing.Invoice, error) {
	row, err := s.Client.Invoice.Query().Where(invoice.ClientIDEQ(clientID), invoice.SpvReferenceEQ(spvReference)).Only(ctx)
	if ent.IsNotFound(err) {
		return nil, apperrors.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return s.GetInvoice(ctx, row.ID)
}

func (s *Store) canonicalByBusinessIdentity(ctx context.Context, clientID string, normalizedSupplierCUI *string, normalizedDocumentNumber string, issueDay time.Time) (*invoicing.Invoice, error) {
	if normalizedSupplierCUI == nil {
		return nil, apperrors.ErrNotFound
	}
	row, err := s.Client.Invoice.Query().Where(
		invoice.ClientIDEQ(clientID),
		invoice.NormalizedSupplierCuiEQ(*normalizedSupplierCUI),
		invoice.NormalizedDocumentNumberEQ(normalizedDocumentNumber),
		invoice.IssueDayEQ(issueDay),
		invoice.DuplicateOfInvoiceIDIsNil(),
	).Only(ctx)
	if ent.IsNotFound(err) {
		return nil, apperrors.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return s.GetInvoice(ctx, row.ID)
}

func (s *Store) auditExists(ctx context.Context, key string) (bool, error) {
	return s.Client.ActivityEvent.Query().Where(activityevent.IdempotencyKeyEQ(key)).Exist(ctx)
}

type auditRecord struct {
	key, invoiceID, taskID, clientID, aggregateType, aggregateID, eventType, from, to, trigger, detail, actorID, actorDisplay, correlationID string
	actor                                                                                                                                    audit.ActorKind
	at                                                                                                                                       time.Time
}

func createAudit(tx *ent.Tx, ctx context.Context, record auditRecord) error {
	before, _ := json.Marshal(record.from)
	after, _ := json.Marshal(record.to)
	aggregateType := "INVOICE"
	aggregateID := record.invoiceID
	if record.taskID != "" {
		aggregateType = "VALIDATION_TASK"
		aggregateID = record.taskID
	}
	if record.aggregateType != "" {
		aggregateType = record.aggregateType
		aggregateID = record.aggregateID
	}
	create := tx.ActivityEvent.Create().SetID(stableID("evt", record.key)).
		SetAggregateType(aggregateType).SetAggregateID(aggregateID).SetEventType(record.eventType).SetOccurredAt(record.at).
		SetActorKind(activityevent.ActorKind(record.actor)).SetAutomatic(record.actor != audit.ActorUser).SetDetail(record.detail).
		SetBeforeSnapshot(before).SetAfterSnapshot(after).SetIdempotencyKey(record.key)
	if record.clientID != "" {
		create.SetClientID(record.clientID)
	}
	if record.invoiceID != "" {
		create.SetInvoiceID(record.invoiceID)
	}
	if record.correlationID != "" {
		create.SetCorrelationID(record.correlationID)
	}
	if record.trigger != "" {
		create.SetTrigger(record.trigger)
	}
	if record.taskID != "" {
		create.SetValidationTaskID(record.taskID)
	} else if record.invoiceID != "" {
		create.SetPipelineTo(record.to)
		if record.from != "" {
			create.SetPipelineFrom(record.from)
		}
	}
	if record.actorDisplay != "" {
		create.SetActorDisplay(record.actorDisplay)
	}
	if record.actorID != "" {
		create.SetActorID(record.actorID)
	}
	_, err := create.Save(ctx)
	return err
}

func createOutbox(tx *ent.Tx, ctx context.Context, invoiceID, key, correlationID string, now time.Time) error {
	payload, _ := json.Marshal(map[string]string{"invoice_id": invoiceID})
	create := tx.OutboxEntry.Create().SetID(stableID("out", key)).SetEventType("INVOICE_CONTINUE").SetAggregateType("INVOICE").SetAggregateID(invoiceID).SetPayload(payload).SetIdempotencyKey(key).SetStatus(outboxentry.StatusPENDING).SetCreatedAt(now).SetAvailableAt(now)
	if correlationID != "" {
		create.SetCorrelationID(correlationID)
	}
	_, err := create.Save(ctx)
	return err
}

func stableID(prefix, value string) string {
	sum := sha256.Sum256([]byte(value))
	return prefix + "-" + hex.EncodeToString(sum[:12])
}

func IsConstraintError(err error) bool {
	var constraint *ent.ConstraintError
	return errors.As(err, &constraint)
}

func dateFromOptional(value *time.Time) accountingdate.Date {
	if value == nil {
		return ""
	}
	return accountingdate.FromTime(*value)
}

func ingestionModel(input invoicing.IngestionInput) string {
	if input.ModelVersion == accounting.ModelVersion {
		return accounting.ModelVersion
	}
	return accounting.LegacyVersion
}
