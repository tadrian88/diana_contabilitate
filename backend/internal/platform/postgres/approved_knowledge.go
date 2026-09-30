package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"diana-contabilitate/backend/internal/accounting"
	"diana-contabilitate/backend/internal/apperrors"
	"diana-contabilitate/backend/internal/classification"
)

type promotionRecord struct {
	preview                                                                                   classification.PromotionPreview
	clientName, supplierName, normalizedSupplier, currency, documentType, lineID, description string
	direction                                                                                 string
	lineFacts                                                                                 *accounting.LineFacts
	source, runID                                                                             string
	legalCitations                                                                            []accounting.LegalCitation
	originalProvenance                                                                        *accounting.ProposalProvenance
}

func (s *Store) loadPromotionRecord(ctx context.Context, clientID, invoiceID, classificationID string) (*promotionRecord, error) {
	var valueRaw, factsRaw, citationsRaw, provenanceRaw []byte
	var profileID sql.NullString
	var profileVersion sql.NullInt64
	var r promotionRecord
	err := s.DB.QueryRowContext(ctx, `SELECT c.name,`+counterpartyNameSQL+`,`+counterpartyIDSQL+`,i.direction,i.currency,i.document_type,i.current_classification_run_id,
		lc.classification_run_id,lc.dimension,lc.effective_typed_value,lc.revision,lc.source,COALESCE(lc.legal_citations,'[]'::jsonb),lc.proposal_provenance,
		l.id,l.description,l.source_facts,l.vat_rate,cr.profile_id,cr.profile_version
		FROM line_classifications lc JOIN invoices i ON i.id=lc.invoice_id AND i.client_id=lc.client_id
		JOIN clients c ON c.id=i.client_id JOIN invoice_lines l ON l.id=lc.invoice_line_id
		JOIN classification_runs cr ON cr.id=lc.classification_run_id
		WHERE lc.id=$1 AND lc.invoice_id=$2 AND lc.client_id=$3 AND lc.review_status IN ('ACCEPTED','CORRECTED')
		AND lc.effective_typed_value IS NOT NULL`, classificationID, invoiceID, clientID).Scan(
		&r.clientName, &r.supplierName, &r.normalizedSupplier, &r.direction, &r.currency, &r.documentType, &r.preview.ClassificationRunID,
		&r.runID, &r.preview.Dimension, &valueRaw, &r.preview.ClassificationRevision, &r.source, &citationsRaw, &provenanceRaw,
		&r.lineID, &r.description, &factsRaw, &r.preview.Scope.VATRate, &profileID, &profileVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperrors.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if r.preview.ClassificationRunID != r.runID {
		return nil, apperrors.ErrConflict
	}
	value, err := accounting.DecodeValue(valueRaw, string(r.preview.Dimension))
	if err != nil {
		return nil, apperrors.ErrValidation
	}
	r.preview.Value = *value
	if len(factsRaw) > 0 {
		var facts accounting.LineFacts
		if json.Unmarshal(factsRaw, &facts) == nil {
			r.lineFacts = &facts
		}
	}
	_ = json.Unmarshal(citationsRaw, &r.legalCitations)
	if len(provenanceRaw) > 0 {
		var p accounting.ProposalProvenance
		if json.Unmarshal(provenanceRaw, &p) == nil {
			r.originalProvenance = &p
		}
	}
	identity, ok := classification.PreferredServiceIdentity(classification.LineContext{SourceFacts: r.lineFacts, Description: r.description})
	if !ok || r.normalizedSupplier == "" {
		return nil, apperrors.ErrValidation
	}
	r.preview.Scope = classification.KnowledgeScope{ClientID: clientID, ClientDisplay: r.clientName, SupplierDisplay: r.supplierName, NormalizedSupplierID: r.normalizedSupplier, ServiceIdentityKind: string(identity.Kind), ServiceIdentityValue: identity.Value, NormalizerVersion: identity.NormalizerVersion, Currency: r.currency, DocumentType: r.documentType, VATRate: r.preview.Scope.VATRate, ProfileID: profileID.String, ProfileVersion: int(profileVersion.Int64), Direction: scopeDirection(r.direction)}
	r.preview.ClassificationID = classificationID
	return &r, nil
}

func (s *Store) PreviewApprovedKnowledge(ctx context.Context, clientID, invoiceID, classificationID string) (*classification.PromotionPreview, error) {
	r, err := s.loadPromotionRecord(ctx, clientID, invoiceID, classificationID)
	if err != nil {
		return nil, err
	}
	return &r.preview, nil
}

func knowledgeHashes(scope classification.KnowledgeScope, dimension classification.Dimension, value accounting.Value) (string, string) {
	valueRaw, _ := json.Marshal(value)
	vd := sha256.Sum256(valueRaw)
	identity := strings.Join([]string{scope.NormalizedSupplierID, scope.ServiceIdentityKind, scope.ServiceIdentityValue, scope.NormalizerVersion, scope.Currency, scope.DocumentType, scope.VATRate, scope.ProfileID, fmt.Sprint(scope.ProfileVersion), string(dimension)}, "\x00") + directionKeySuffix(scope.Direction)
	id := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(id[:]), hex.EncodeToString(vd[:])
}

func (s *Store) PromoteApprovedKnowledge(ctx context.Context, command classification.PromoteKnowledgeCommand, now time.Time) (*classification.KnowledgeItem, bool, error) {
	promoteKey := knowledgeCommandKey("promote", command.ClientID, command.CommandID)
	if exists, err := s.auditExists(ctx, promoteKey); err != nil {
		return nil, false, err
	} else if exists {
		items, e := s.ListApprovedKnowledge(ctx, command.ClientID)
		if e != nil {
			return nil, false, e
		}
		for i := range items {
			if items[i].SourceClassificationID == command.ClassificationID {
				return &items[i], false, nil
			}
		}
	}
	r, err := s.loadPromotionRecord(ctx, command.ClientID, command.InvoiceID, command.ClassificationID)
	if err != nil {
		return nil, false, err
	}
	currentInput, err := s.LoadClassificationInput(ctx, command.InvoiceID)
	if err != nil {
		return nil, false, err
	}
	if currentInput.ClientID != command.ClientID || currentInput.Snapshot == nil || currentInput.Snapshot.Profile == nil || currentInput.Snapshot.Profile.ID != r.preview.Scope.ProfileID || currentInput.Snapshot.Profile.Version != r.preview.Scope.ProfileVersion {
		return nil, false, apperrors.ErrConflict
	}
	if r.preview.Dimension == classification.DimensionAccount && (!currentInput.SelectableAccounts[r.preview.Value.Account] || !currentInput.Snapshot.Profile.AccountAllowed(r.preview.Value.Account)) {
		return nil, false, apperrors.ErrValidation
	}
	var invoiceRevision uint64
	var currentRun sql.NullString
	if err = s.DB.QueryRowContext(ctx, `SELECT revision,current_classification_run_id FROM invoices WHERE id=$1 AND client_id=$2`, command.InvoiceID, command.ClientID).Scan(&invoiceRevision, &currentRun); err != nil {
		return nil, false, err
	}
	if invoiceRevision != command.ExpectedInvoiceRevision || r.preview.ClassificationRevision != command.ExpectedClassificationRevision || !currentRun.Valid || currentRun.String != command.ExpectedClassificationRunID || r.runID != command.ExpectedClassificationRunID {
		return nil, false, apperrors.ErrConflict
	}
	if r.preview.Dimension == classification.DimensionAccount {
		return s.promoteAccountKnowledge(ctx, command, r, now)
	}
	if r.preview.Dimension != classification.DimensionVATreatment && r.preview.Dimension != classification.DimensionVATDeductibility && r.preview.Dimension != classification.DimensionExpenseTax {
		return nil, false, apperrors.ErrValidation
	}
	identityHash, valueHash := knowledgeHashes(r.preview.Scope, r.preview.Dimension, r.preview.Value)
	valueRaw, _ := json.Marshal(r.preview.Value)
	provenanceRaw, _ := json.Marshal(r.originalProvenance)
	versions := []string{}
	seen := map[string]bool{}
	for _, c := range r.legalCitations {
		if c.Verified && !seen[c.VersionID] {
			seen[c.VersionID] = true
			versions = append(versions, c.VersionID)
		}
	}
	versionsRaw, _ := json.Marshal(versions)
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback()
	existingRows, err := tx.QueryContext(ctx, `SELECT approved_value_hash FROM approved_accounting_knowledge WHERE client_id=$1 AND dimension=$2 AND identity_hash=$3 AND status='ACTIVE' FOR UPDATE`, command.ClientID, r.preview.Dimension, identityHash)
	if err != nil {
		return nil, false, err
	}
	hasSame, hasDifferent := false, false
	for existingRows.Next() {
		var hash string
		if err = existingRows.Scan(&hash); err != nil {
			existingRows.Close()
			return nil, false, err
		}
		if hash == valueHash {
			hasSame = true
		} else {
			hasDifferent = true
		}
	}
	if err = existingRows.Err(); err != nil {
		existingRows.Close()
		return nil, false, err
	}
	existingRows.Close()
	if hasDifferent {
		return nil, false, classification.ErrKnowledgeConflict
	}
	if hasSame {
		return nil, false, classification.ErrKnowledgeDuplicate
	}
	var previousID sql.NullString
	var version int
	err = tx.QueryRowContext(ctx, `SELECT id,version FROM approved_accounting_knowledge WHERE client_id=$1 AND dimension=$2 AND identity_hash=$3 ORDER BY version DESC,promoted_at DESC LIMIT 1 FOR UPDATE`, command.ClientID, r.preview.Dimension, identityHash).Scan(&previousID, &version)
	if errors.Is(err, sql.ErrNoRows) {
		version, err = 1, nil
	} else if err == nil {
		version++
	}
	if err != nil {
		return nil, false, err
	}
	id := stableID("approved-knowledge", command.ClientID+"\x00"+command.CommandID)
	_, err = tx.ExecContext(ctx, `INSERT INTO approved_accounting_knowledge(id,version,supersedes_id,client_id,dimension,decision,approved_value_hash,normalized_supplier_id,semantic_kind,semantic_value,normalizer_version,currency,document_type,vat_rate,profile_id,profile_version,legislation_version_ids,source_invoice_id,source_invoice_line_id,source_classification_id,source_classification_run_id,source_classification_revision,original_source,original_provenance,promoted_by_id,promoted_by_display,promoted_at,status,revision,identity_hash,command_key,effective_from,created_at,updated_at,direction)
		SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,'ACTIVE',1,$28,$29,i.issue_day,$27,$27,i.direction FROM invoices i WHERE i.id=$18 AND i.client_id=$4`, id, version, nullableString(previousID), command.ClientID, r.preview.Dimension, valueRaw, valueHash, r.preview.Scope.NormalizedSupplierID, r.preview.Scope.ServiceIdentityKind, r.preview.Scope.ServiceIdentityValue, r.preview.Scope.NormalizerVersion, r.preview.Scope.Currency, r.preview.Scope.DocumentType, r.preview.Scope.VATRate, r.preview.Scope.ProfileID, r.preview.Scope.ProfileVersion, versionsRaw, command.InvoiceID, r.lineID, command.ClassificationID, r.runID, r.preview.ClassificationRevision, r.source, provenanceRaw, optionalString(command.ActorID), command.ActorDisplay, now, identityHash, "knowledge:"+command.ClientID+":"+command.CommandID)
	if err != nil {
		return nil, false, err
	}
	if err = insertKnowledgeAudit(ctx, tx, promoteKey, command.ClientID, command.InvoiceID, id, "APPROVED_KNOWLEDGE_PROMOTED", "Accountant explicitly confirmed reusable scope for one dimension.", command.ActorID, command.ActorDisplay, command.CorrelationID, now); err != nil {
		return nil, false, err
	}
	if err = tx.Commit(); err != nil {
		return nil, false, err
	}
	item := knowledgeItemFromPromotion(id, r, command, versions, now)
	item.Version = version
	return &item, true, nil
}

func optionalString(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return v
}

func knowledgeCommandKey(operation, clientID, commandID string) string {
	return "knowledge-" + operation + ":" + clientID + ":" + commandID
}

func nullableString(v sql.NullString) any {
	if !v.Valid {
		return nil
	}
	return v.String
}

func knowledgeItemFromPromotion(id string, r *promotionRecord, c classification.PromoteKnowledgeCommand, versions []string, now time.Time) classification.KnowledgeItem {
	return classification.KnowledgeItem{ID: id, Version: 1, Dimension: r.preview.Dimension, Value: r.preview.Value, Scope: r.preview.Scope, Status: "ACTIVE", SourceInvoiceID: c.InvoiceID, SourceInvoiceLineID: r.lineID, SourceClassificationID: c.ClassificationID, SourceRunID: r.runID, OriginalSource: r.source, PromotedBy: c.ActorDisplay, PromotedAt: now, Revision: 1, LegislationVersionIDs: versions}
}

func (s *Store) promoteAccountKnowledge(ctx context.Context, command classification.PromoteKnowledgeCommand, r *promotionRecord, now time.Time) (*classification.KnowledgeItem, bool, error) {
	if r.preview.Value.Kind != "ACCOUNT" {
		return nil, false, apperrors.ErrValidation
	}
	promoteKey := knowledgeCommandKey("promote", command.ClientID, command.CommandID)
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback()
	var valid bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM accounts WHERE code=$1 AND is_active AND postable)`, r.preview.Value.Account).Scan(&valid); err != nil || !valid {
		if err == nil {
			err = apperrors.ErrValidation
		}
		return nil, false, err
	}
	mappingID := stableID("account-mapping", command.ClientID+"\x00"+r.preview.Scope.NormalizedSupplierID+"\x00"+r.preview.Scope.ServiceIdentityKind+"\x00"+r.preview.Scope.ServiceIdentityValue+"\x00"+r.preview.Scope.NormalizerVersion+directionKeySuffix(r.preview.Scope.Direction))
	var existingID, existingAccount, status string
	var existingVersion int
	var existingRevision uint64
	err = tx.QueryRowContext(ctx, `SELECT m.id,v.account_code,m.status,m.current_version,m.revision FROM account_mappings m JOIN account_mapping_versions v ON v.mapping_id=m.id AND v.version=m.current_version WHERE m.client_id=$1 AND m.normalized_supplier_id=$2 AND m.service_identity_kind=$3 AND m.service_identity_value=$4 AND m.normalizer_version=$5 AND m.direction=$6 FOR UPDATE`, command.ClientID, r.preview.Scope.NormalizedSupplierID, r.preview.Scope.ServiceIdentityKind, r.preview.Scope.ServiceIdentityValue, r.preview.Scope.NormalizerVersion, scopeDirection(r.preview.Scope.Direction)).Scan(&existingID, &existingAccount, &status, &existingVersion, &existingRevision)
	if err == nil {
		if status == "ACTIVE" && existingAccount == r.preview.Value.Account {
			return nil, false, classification.ErrKnowledgeDuplicate
		}
		if status != "INACTIVE" {
			return nil, false, classification.ErrKnowledgeConflict
		}
		nextVersion := existingVersion + 1
		if _, err = tx.ExecContext(ctx, `UPDATE account_mappings SET status='ACTIVE',current_version=$1,revision=revision+1,updated_at=$2 WHERE id=$3 AND status='INACTIVE'`, nextVersion, now, existingID); err != nil {
			return nil, false, err
		}
		versionID := stableID("account-mapping-version", existingID+fmt.Sprintf(":%d", nextVersion))
		if _, err = tx.ExecContext(ctx, `INSERT INTO account_mapping_versions(id,mapping_id,version,account_code,change_kind,source_classification_id,source_invoice_line_id,raw_description_snapshot,actor_id,actor_display,reason,created_at,command_key) SELECT $1,$2,$3,$4,'CORRECTION',$5,invoice_line_id,$6,$7,$8,'Explicit replacement after prior revocation',$9,$10 FROM line_classifications WHERE id=$5 AND client_id=$11`, versionID, existingID, nextVersion, r.preview.Value.Account, command.ClassificationID, r.description, optionalString(command.ActorID), command.ActorDisplay, now, "account-mapping:"+command.ClientID+":"+command.CommandID, command.ClientID); err != nil {
			return nil, false, err
		}
		if err = insertKnowledgeAudit(ctx, tx, promoteKey, command.ClientID, command.InvoiceID, existingID, "APPROVED_KNOWLEDGE_PROMOTED", "Accountant explicitly confirmed a replacement ACCOUNT version after revocation.", command.ActorID, command.ActorDisplay, command.CorrelationID, now); err != nil {
			return nil, false, err
		}
		if err = tx.Commit(); err != nil {
			return nil, false, err
		}
		item := knowledgeItemFromPromotion(existingID, r, command, nil, now)
		item.Version = nextVersion
		item.Revision = existingRevision + 1
		return &item, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, false, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO account_mappings(id,client_id,normalized_supplier_id,service_identity_kind,service_identity_value,normalizer_version,current_version,status,revision,created_at,updated_at,direction) VALUES($1,$2,$3,$4,$5,$6,1,'ACTIVE',1,$7,$7,$8)`, mappingID, command.ClientID, r.preview.Scope.NormalizedSupplierID, r.preview.Scope.ServiceIdentityKind, r.preview.Scope.ServiceIdentityValue, r.preview.Scope.NormalizerVersion, now, scopeDirection(r.preview.Scope.Direction))
	if err != nil {
		return nil, false, err
	}
	versionID := stableID("account-mapping-version", mappingID+":1")
	_, err = tx.ExecContext(ctx, `INSERT INTO account_mapping_versions(id,mapping_id,version,account_code,change_kind,source_classification_id,source_invoice_line_id,raw_description_snapshot,actor_id,actor_display,reason,created_at,command_key) SELECT $1,$2,1,$3,'CREATION',$4,invoice_line_id,$5,$6,$7,'Explicit opt-in after final decision',$8,$9 FROM line_classifications WHERE id=$4 AND client_id=$10`, versionID, mappingID, r.preview.Value.Account, command.ClassificationID, r.description, optionalString(command.ActorID), command.ActorDisplay, now, "account-mapping:"+command.ClientID+":"+command.CommandID, command.ClientID)
	if err != nil {
		return nil, false, err
	}
	if err = insertKnowledgeAudit(ctx, tx, promoteKey, command.ClientID, command.InvoiceID, mappingID, "APPROVED_KNOWLEDGE_PROMOTED", "Accountant explicitly confirmed reusable ACCOUNT scope.", command.ActorID, command.ActorDisplay, command.CorrelationID, now); err != nil {
		return nil, false, err
	}
	if err = tx.Commit(); err != nil {
		return nil, false, err
	}
	item := knowledgeItemFromPromotion(mappingID, r, command, nil, now)
	return &item, true, nil
}

func insertKnowledgeAudit(ctx context.Context, tx *sql.Tx, key, clientID, invoiceID, aggregateID, eventType, detail, actorID, actorDisplay, correlationID string, now time.Time) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO activity_events(id,client_id,invoice_id,aggregate_type,aggregate_id,event_type,occurred_at,actor_kind,actor_id,actor_display,automatic,detail,correlation_id,idempotency_key,trigger) VALUES($1,$2,$3,'APPROVED_KNOWLEDGE',$4,$5,$6,'USER',$7,$8,false,$9,$10,$11,'EXPLICIT_REUSE')`, stableID("evt", key), clientID, invoiceID, aggregateID, eventType, now, optionalString(actorID), actorDisplay, detail, optionalString(correlationID), key)
	return err
}

func (s *Store) ListApprovedKnowledge(ctx context.Context, clientID string) ([]classification.KnowledgeItem, error) {
	result := []classification.KnowledgeItem{}
	filter := ""
	args := []any{}
	if clientID != "" {
		filter = " WHERE k.client_id=$1"
		args = append(args, clientID)
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT k.id,k.version,k.client_id,c.name,k.dimension,k.decision,k.normalized_supplier_id,`+counterpartyNameSQL+`,k.direction,k.semantic_kind,k.semantic_value,k.normalizer_version,k.currency,k.document_type,k.vat_rate,k.profile_id,k.profile_version,k.legislation_version_ids,k.source_invoice_id,k.source_invoice_line_id,k.source_classification_id,k.source_classification_run_id,k.original_source,k.promoted_by_display,k.promoted_at,k.status,COALESCE(k.stale_reason,''),k.revision FROM approved_accounting_knowledge k JOIN clients c ON c.id=k.client_id JOIN invoices i ON i.id=k.source_invoice_id`+filter+func() string {
		if filter == "" {
			return " WHERE k.dimension IS NOT NULL"
		}
		return " AND k.dimension IS NOT NULL"
	}()+` ORDER BY k.promoted_at DESC,k.id`, args...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var item classification.KnowledgeItem
		var valueRaw, versionsRaw []byte
		if err = rows.Scan(&item.ID, &item.Version, &item.Scope.ClientID, &item.Scope.ClientDisplay, &item.Dimension, &valueRaw, &item.Scope.NormalizedSupplierID, &item.Scope.SupplierDisplay, &item.Scope.Direction, &item.Scope.ServiceIdentityKind, &item.Scope.ServiceIdentityValue, &item.Scope.NormalizerVersion, &item.Scope.Currency, &item.Scope.DocumentType, &item.Scope.VATRate, &item.Scope.ProfileID, &item.Scope.ProfileVersion, &versionsRaw, &item.SourceInvoiceID, &item.SourceInvoiceLineID, &item.SourceClassificationID, &item.SourceRunID, &item.OriginalSource, &item.PromotedBy, &item.PromotedAt, &item.Status, &item.StaleReason, &item.Revision); err != nil {
			rows.Close()
			return nil, err
		}
		value, e := accounting.DecodeValue(valueRaw, string(item.Dimension))
		if e != nil {
			rows.Close()
			return nil, e
		}
		item.Value = *value
		_ = json.Unmarshal(versionsRaw, &item.LegislationVersionIDs)
		result = append(result, item)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	accountFilter := ""
	accountArgs := []any{}
	if clientID != "" {
		accountFilter = " WHERE m.client_id=$1"
		accountArgs = append(accountArgs, clientID)
	}
	rows, err = s.DB.QueryContext(ctx, `SELECT m.id,v.version,m.client_id,c.name,v.account_code,m.normalized_supplier_id,`+counterpartyNameSQL+`,m.direction,m.service_identity_kind,m.service_identity_value,m.normalizer_version,i.currency,i.document_type,l.vat_rate,COALESCE(cr.profile_id,''),COALESCE(cr.profile_version,0),v.source_classification_id,v.source_invoice_line_id,lc.invoice_id,lc.classification_run_id,lc.source,v.actor_display,v.created_at,m.status,m.revision FROM account_mappings m JOIN account_mapping_versions v ON v.mapping_id=m.id AND v.version=m.current_version JOIN clients c ON c.id=m.client_id JOIN line_classifications lc ON lc.id=v.source_classification_id JOIN invoices i ON i.id=lc.invoice_id JOIN invoice_lines l ON l.id=v.source_invoice_line_id LEFT JOIN classification_runs cr ON cr.id=lc.classification_run_id`+accountFilter+` ORDER BY v.created_at DESC,m.id`, accountArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var item classification.KnowledgeItem
		var accountCode, status string
		if err = rows.Scan(&item.ID, &item.Version, &item.Scope.ClientID, &item.Scope.ClientDisplay, &accountCode, &item.Scope.NormalizedSupplierID, &item.Scope.SupplierDisplay, &item.Scope.Direction, &item.Scope.ServiceIdentityKind, &item.Scope.ServiceIdentityValue, &item.Scope.NormalizerVersion, &item.Scope.Currency, &item.Scope.DocumentType, &item.Scope.VATRate, &item.Scope.ProfileID, &item.Scope.ProfileVersion, &item.SourceClassificationID, &item.SourceInvoiceLineID, &item.SourceInvoiceID, &item.SourceRunID, &item.OriginalSource, &item.PromotedBy, &item.PromotedAt, &status, &item.Revision); err != nil {
			return nil, err
		}
		item.Dimension = classification.DimensionAccount
		item.Value = accounting.Value{Kind: "ACCOUNT", Account: accountCode}
		item.Status = status
		if status == "INACTIVE" {
			item.Status = "REVOKED"
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) RevokeApprovedKnowledge(ctx context.Context, command classification.RevokeKnowledgeCommand, now time.Time) (*classification.KnowledgeItem, bool, error) {
	revokeKey := knowledgeCommandKey("revoke", command.ClientID, command.CommandID)
	if exists, err := s.auditExists(ctx, revokeKey); err != nil {
		return nil, false, err
	} else if exists {
		items, e := s.ListApprovedKnowledge(ctx, command.ClientID)
		if e != nil {
			return nil, false, e
		}
		for i := range items {
			if items[i].ID == command.KnowledgeID {
				return &items[i], false, nil
			}
		}
	}
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE approved_accounting_knowledge SET status='REVOKED',revision=revision+1,revoked_by_id=$1,revoked_by_display=$2,revoked_at=$3,updated_at=$3 WHERE id=$4 AND client_id=$5 AND revision=$6 AND status IN ('ACTIVE','STALE')`, optionalString(command.ActorID), command.ActorDisplay, now, command.KnowledgeID, command.ClientID, command.ExpectedRevision)
	if err != nil {
		return nil, false, err
	}
	affected, _ := res.RowsAffected()
	invoiceID := ""
	if affected == 0 {
		res, err = tx.ExecContext(ctx, `UPDATE account_mappings SET status='INACTIVE',revision=revision+1,updated_at=$1 WHERE id=$2 AND client_id=$3 AND revision=$4 AND status='ACTIVE'`, now, command.KnowledgeID, command.ClientID, command.ExpectedRevision)
		if err != nil {
			return nil, false, err
		}
		affected, _ = res.RowsAffected()
	}
	if affected == 0 {
		return nil, false, apperrors.ErrConflict
	}
	_ = tx.QueryRowContext(ctx, `SELECT COALESCE(source_invoice_id,'') FROM approved_accounting_knowledge WHERE id=$1 UNION ALL SELECT COALESCE(lc.invoice_id,'') FROM account_mappings m JOIN account_mapping_versions v ON v.mapping_id=m.id AND v.version=m.current_version JOIN line_classifications lc ON lc.id=v.source_classification_id WHERE m.id=$1 LIMIT 1`, command.KnowledgeID).Scan(&invoiceID)
	if err = insertKnowledgeAudit(ctx, tx, revokeKey, command.ClientID, invoiceID, command.KnowledgeID, "APPROVED_KNOWLEDGE_REVOKED", "Accountant revoked reusable knowledge; history was retained.", command.ActorID, command.ActorDisplay, command.CorrelationID, now); err != nil {
		return nil, false, err
	}
	if err = tx.Commit(); err != nil {
		return nil, false, err
	}
	items, err := s.ListApprovedKnowledge(ctx, command.ClientID)
	if err != nil {
		return nil, false, err
	}
	for i := range items {
		if items[i].ID == command.KnowledgeID {
			return &items[i], true, nil
		}
	}
	return nil, false, apperrors.ErrNotFound
}

func (s *Store) loadApprovedKnowledgeCandidates(ctx context.Context, input *classification.InvoiceContext) error {
	if input.NormalizedSupplierID == "" || input.Snapshot == nil || input.Snapshot.Profile == nil {
		return nil
	}
	if err := s.markApprovedKnowledgeStale(ctx, input.ClientID, input.Snapshot.Profile.ID, input.Snapshot.Profile.Version); err != nil {
		return err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT k.id,k.version,k.dimension,k.decision,k.normalized_supplier_id,k.semantic_kind,k.semantic_value,k.normalizer_version,k.currency,k.document_type,k.vat_rate,k.profile_id,k.profile_version,k.source_invoice_id,k.source_invoice_line_id,k.source_classification_id,k.promoted_by_display,k.promoted_at,k.status
		FROM approved_accounting_knowledge k WHERE k.client_id=$1 AND k.normalized_supplier_id=$2 AND k.status='ACTIVE' AND k.direction=$6
		AND k.profile_id=$3 AND k.profile_version=$4
		AND k.dimension IS NOT NULL AND NOT EXISTS (SELECT 1 FROM jsonb_array_elements_text(k.legislation_version_ids) dep LEFT JOIN legislation_versions lv ON lv.id=dep WHERE lv.id IS NULL OR $5::date < lv.effective_from OR (lv.effective_to IS NOT NULL AND $5::date > lv.effective_to))
		AND NOT EXISTS (SELECT 1 FROM legislation_versions current_v JOIN legislation_versions depended_v ON depended_v.source_id=current_v.source_id WHERE depended_v.id IN (SELECT jsonb_array_elements_text(k.legislation_version_ids)) AND current_v.id NOT IN (SELECT jsonb_array_elements_text(k.legislation_version_ids)) AND $5::date>=current_v.effective_from AND (current_v.effective_to IS NULL OR $5::date<=current_v.effective_to))
		ORDER BY k.id`, input.ClientID, input.NormalizedSupplierID, input.Snapshot.Profile.ID, input.Snapshot.Profile.Version, string(input.IssueDate), scopeDirection(input.Direction))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var c classification.KnowledgeCandidate
		var raw []byte
		if err = rows.Scan(&c.ID, &c.Version, &c.Dimension, &raw, &c.NormalizedSupplierID, &c.ServiceIdentityKind, &c.ServiceIdentityValue, &c.NormalizerVersion, &c.Currency, &c.DocumentType, &c.VATRate, &c.ProfileID, &c.ProfileVersion, &c.SourceInvoiceID, &c.SourceInvoiceLineID, &c.SourceClassificationID, &c.PromotedBy, &c.PromotedAt, &c.Status); err != nil {
			return err
		}
		value, e := accounting.DecodeValue(raw, string(c.Dimension))
		if e != nil {
			return e
		}
		c.Value = *value
		input.Knowledge = append(input.Knowledge, c)
	}
	return rows.Err()
}

func (s *Store) markApprovedKnowledgeStale(ctx context.Context, clientID, profileID string, profileVersion int) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	mark := func(reason, predicate string, args ...any) error {
		query := `UPDATE approved_accounting_knowledge k SET status='STALE',stale_reason=$2,revision=revision+1,updated_at=NOW() WHERE k.client_id=$1 AND k.dimension IS NOT NULL AND k.status='ACTIVE' AND ` + predicate + ` RETURNING k.id,COALESCE(k.source_invoice_id,''),k.revision`
		rows, e := tx.QueryContext(ctx, query, append([]any{clientID, reason}, args...)...)
		if e != nil {
			return e
		}
		type staleItem struct {
			id, invoiceID string
			revision      uint64
		}
		items := []staleItem{}
		for rows.Next() {
			var item staleItem
			if e = rows.Scan(&item.id, &item.invoiceID, &item.revision); e != nil {
				rows.Close()
				return e
			}
			items = append(items, item)
		}
		if e = rows.Err(); e != nil {
			rows.Close()
			return e
		}
		rows.Close()
		for _, item := range items {
			key := fmt.Sprintf("knowledge-stale:%s:%d", item.id, item.revision)
			if _, e = tx.ExecContext(ctx, `INSERT INTO activity_events(id,client_id,invoice_id,aggregate_type,aggregate_id,event_type,occurred_at,actor_kind,actor_display,automatic,detail,idempotency_key,trigger) VALUES($1,$2,NULLIF($3,''),'APPROVED_KNOWLEDGE',$4,'APPROVED_KNOWLEDGE_STALE',NOW(),'SYSTEM','Sistem contabil',true,$5,$6,'KNOWLEDGE_INVALIDATED') ON CONFLICT(idempotency_key) DO NOTHING`, stableID("evt", key), clientID, item.invoiceID, item.id, reason, key); e != nil {
				return e
			}
		}
		return nil
	}
	if err = mark("Profilul contabil a fost înlocuit; decizia necesită promovare din nou.", `(k.profile_id,k.profile_version) IS DISTINCT FROM ($3,$4)`, profileID, profileVersion); err != nil {
		return err
	}
	if err = mark("Versiunea legislativă citată nu mai este versiunea efectivă curentă.", `EXISTS (
		SELECT 1 FROM jsonb_array_elements_text(k.legislation_version_ids) dep
		JOIN legislation_versions old_v ON old_v.id=dep
		JOIN legislation_versions current_v ON current_v.source_id=old_v.source_id AND current_v.id<>old_v.id
		WHERE CURRENT_DATE>=current_v.effective_from AND (current_v.effective_to IS NULL OR CURRENT_DATE<=current_v.effective_to)
	)`); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ListLegislationSources(ctx context.Context) ([]classification.LegislationSourceView, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT s.id,s.kind,s.title,s.issuer,s.official_url,v.id,v.label,v.effective_from,v.effective_to,CASE WHEN CURRENT_DATE<v.effective_from THEN 'FUTURE' WHEN v.effective_to IS NOT NULL AND CURRENT_DATE>v.effective_to THEN 'EXPIRED' ELSE 'ACTIVE' END,count(f.id),v.test_only FROM legislation_sources s JOIN legislation_versions v ON v.source_id=s.id LEFT JOIN legislation_fragments f ON f.version_id=v.id GROUP BY s.id,s.kind,s.title,s.issuer,s.official_url,v.id,v.label,v.effective_from,v.effective_to,v.test_only ORDER BY s.title,v.effective_from DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []classification.LegislationSourceView{}
	for rows.Next() {
		var item classification.LegislationSourceView
		var from time.Time
		var to sql.NullTime
		if err = rows.Scan(&item.ID, &item.Kind, &item.Title, &item.Issuer, &item.OfficialURL, &item.VersionID, &item.VersionLabel, &from, &to, &item.Status, &item.FragmentCount, &item.TestOnly); err != nil {
			return nil, err
		}
		item.EffectiveFrom = from.Format("2006-01-02")
		if to.Valid {
			value := to.Time.Format("2006-01-02")
			item.EffectiveTo = &value
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// Learning scope helpers (D-129). The counterparty of an issued invoice is its
// customer; purchase keys, hashes and mapping IDs stay exactly as before.
const (
	counterpartyNameSQL = `CASE WHEN i.direction='OUTGOING' THEN COALESCE(i.customer_name,'') ELSE i.supplier_name END`
	counterpartyIDSQL   = `CASE WHEN i.direction='OUTGOING' THEN COALESCE(i.normalized_customer_identifier,'') ELSE COALESCE(i.normalized_supplier_cui,'') END`
)

func scopeDirection(direction string) string {
	if direction == "OUTGOING" {
		return "OUTGOING"
	}
	return "INCOMING"
}

func directionKeySuffix(direction string) string {
	if direction == "OUTGOING" {
		return "\x00OUTGOING"
	}
	return ""
}
