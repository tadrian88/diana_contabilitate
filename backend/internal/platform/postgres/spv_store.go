package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"diana-contabilitate/backend/ent"
	"diana-contabilitate/backend/ent/accountingclient"
	"diana-contabilitate/backend/ent/activityevent"
	"diana-contabilitate/backend/ent/spvconnection"
	"diana-contabilitate/backend/ent/spvoauthstate"
	"diana-contabilitate/backend/ent/spvsourcedocument"
	"diana-contabilitate/backend/internal/apperrors"
	"diana-contabilitate/backend/internal/spv"
)

func (s *Store) ListActiveConnections(ctx context.Context) ([]spv.Connection, error) {
	rows, err := s.Client.SPVConnection.Query().Where(spvconnection.StatusEQ(spvconnection.StatusACTIVE), spvconnection.HasClientWith(accountingclient.LifecycleNEQ("INACTIVE"))).Order(ent.Asc(spvconnection.FieldID)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active SPV connections: %w", err)
	}
	result := make([]spv.Connection, 0, len(rows))
	for _, row := range rows {
		result = append(result, spvConnectionDomain(row))
	}
	return result, nil
}

func (s *Store) ClientCUI(ctx context.Context, clientID string) (string, error) {
	row, err := s.Client.AccountingClient.Get(ctx, clientID)
	if ent.IsNotFound(err) {
		return "", apperrors.ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return row.Cui, nil
}

func (s *Store) ConnectionByClient(ctx context.Context, clientID string) (*spv.Connection, error) {
	row, err := s.Client.SPVConnection.Query().Where(spvconnection.ClientIDEQ(clientID)).Only(ctx)
	if ent.IsNotFound(err) {
		return nil, apperrors.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	value := spvConnectionDomain(row)
	return &value, nil
}

func (s *Store) CreateOAuthState(ctx context.Context, state spv.OAuthState) error {
	_, err := s.Client.SPVOAuthState.Create().SetID(state.ID).SetStateHash(state.StateHash).SetClientID(state.ClientID).SetEnvironment(spvoauthstate.Environment(state.Environment)).SetReturnPath(state.ReturnPath).SetExpiresAt(state.ExpiresAt).SetCreatedAt(state.CreatedAt).Save(ctx)
	if ent.IsConstraintError(err) {
		return apperrors.ErrConflict
	}
	return err
}

func (s *Store) ConsumeOAuthState(ctx context.Context, stateHash string, now time.Time) (spv.OAuthState, error) {
	count, err := s.Client.SPVOAuthState.Update().Where(spvoauthstate.StateHashEQ(stateHash), spvoauthstate.ConsumedAtIsNil(), spvoauthstate.ExpiresAtGT(now)).SetConsumedAt(now).Save(ctx)
	if err != nil {
		return spv.OAuthState{}, err
	}
	if count != 1 {
		return spv.OAuthState{}, spv.ErrInvalidOAuthState
	}
	row, err := s.Client.SPVOAuthState.Query().Where(spvoauthstate.StateHashEQ(stateHash)).Only(ctx)
	if err != nil {
		return spv.OAuthState{}, err
	}
	return spv.OAuthState{ID: row.ID, StateHash: row.StateHash, ClientID: row.ClientID, Environment: string(row.Environment), ReturnPath: row.ReturnPath, ExpiresAt: row.ExpiresAt, ConsumedAt: row.ConsumedAt, CreatedAt: row.CreatedAt}, nil
}

func (s *Store) CompleteOAuthConnection(ctx context.Context, state spv.OAuthState, access, refresh string, accessExpires time.Time, refreshExpires *time.Time, actor spv.Actor, now time.Time) (spv.Connection, bool, error) {
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return spv.Connection{}, false, err
	}
	rollback := func(cause error) (spv.Connection, bool, error) {
		_ = tx.Rollback()
		return spv.Connection{}, false, cause
	}
	client, err := tx.AccountingClient.Get(ctx, state.ClientID)
	if ent.IsNotFound(err) {
		return rollback(apperrors.ErrNotFound)
	}
	if err != nil {
		return rollback(err)
	}
	if client.Lifecycle == "INACTIVE" {
		return rollback(spv.ErrConnectionInactive)
	}
	existing, err := tx.SPVConnection.Query().Where(spvconnection.ClientIDEQ(state.ClientID)).Only(ctx)
	reconnected := err == nil
	var row *ent.SPVConnection
	if ent.IsNotFound(err) {
		create := tx.SPVConnection.Create().SetID(stableID("spvconn", state.Environment+":"+client.Cui)).SetClientID(client.ID).SetCif(client.Cui).SetEnvironment(spvconnection.Environment(state.Environment)).SetAccessTokenCiphertext(access).SetRefreshTokenCiphertext(refresh).SetAccessTokenExpiresAt(accessExpires).SetStatus(spvconnection.StatusACTIVE).SetConnectedAt(now).SetLastSyncStatus(spvconnection.LastSyncStatusNEVER).SetCreatedAt(now).SetUpdatedAt(now)
		if refreshExpires != nil {
			create.SetRefreshTokenExpiresAt(*refreshExpires)
		}
		row, err = create.Save(ctx)
	} else if err == nil {
		if string(existing.Environment) != state.Environment {
			return rollback(apperrors.ErrValidation)
		}
		update := tx.SPVConnection.UpdateOne(existing).SetCif(client.Cui).SetAccessTokenCiphertext(access).SetRefreshTokenCiphertext(refresh).SetAccessTokenExpiresAt(accessExpires).SetStatus(spvconnection.StatusACTIVE).SetConnectedAt(now).SetUpdatedAt(now).AddRevision(1).ClearLastError()
		if refreshExpires != nil {
			update.SetRefreshTokenExpiresAt(*refreshExpires)
		} else {
			update.ClearRefreshTokenExpiresAt()
		}
		row, err = update.Save(ctx)
	}
	if err != nil {
		return rollback(err)
	}
	eventType, detail := "SPV_CONNECTION_CONNECTED", "Conexiunea ANAF/SPV a fost activată pentru client."
	if reconnected {
		eventType, detail = "SPV_CONNECTION_RECONNECTED", "Conexiunea ANAF/SPV a fost reautorizată pentru client."
	}
	if err = createSPVActivity(ctx, tx.ActivityEvent.Create(), row.ClientID, row.ID, eventType, detail, "spv-oauth:"+state.ID, actor, now); err != nil {
		return rollback(err)
	}
	if err = tx.Commit(); err != nil {
		return spv.Connection{}, false, err
	}
	return spvConnectionDomain(row), reconnected, nil
}

func (s *Store) DisconnectSPVConnection(ctx context.Context, clientID, commandID string, actor spv.Actor, now time.Time) (spv.Connection, bool, error) {
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return spv.Connection{}, false, err
	}
	rollback := func(cause error) (spv.Connection, bool, error) {
		_ = tx.Rollback()
		return spv.Connection{}, false, cause
	}
	row, err := tx.SPVConnection.Query().Where(spvconnection.ClientIDEQ(clientID)).Only(ctx)
	if ent.IsNotFound(err) {
		return rollback(apperrors.ErrNotFound)
	}
	if err != nil {
		return rollback(err)
	}
	if commandID != "" {
		exists, lookupErr := tx.ActivityEvent.Query().Where(activityevent.IdempotencyKeyEQ(commandID)).Exist(ctx)
		if lookupErr != nil {
			return rollback(lookupErr)
		}
		if exists {
			_ = tx.Rollback()
			value := spvConnectionDomain(row)
			return value, false, nil
		}
	}
	if row.Status != spvconnection.StatusREVOKED {
		row, err = tx.SPVConnection.UpdateOne(row).SetStatus(spvconnection.StatusREVOKED).SetAccessTokenCiphertext("").SetRefreshTokenCiphertext("").SetAccessTokenExpiresAt(now).SetUpdatedAt(now).AddRevision(1).Save(ctx)
		if err != nil {
			return rollback(err)
		}
	}
	if err = createSPVActivity(ctx, tx.ActivityEvent.Create(), clientID, row.ID, "SPV_CONNECTION_DISCONNECTED", "Conexiunea ANAF/SPV a fost dezactivată; documentele deja importate sunt păstrate.", commandID, actor, now); err != nil {
		return rollback(err)
	}
	if err = tx.Commit(); err != nil {
		return spv.Connection{}, false, err
	}
	return spvConnectionDomain(row), true, nil
}

func (s *Store) RecordManualSyncRequested(ctx context.Context, connection spv.Connection, commandID string, actor spv.Actor, now time.Time) (bool, error) {
	if commandID != "" {
		exists, err := s.Client.ActivityEvent.Query().Where(activityevent.IdempotencyKeyEQ(commandID)).Exist(ctx)
		if err != nil {
			return false, err
		}
		if exists {
			return false, nil
		}
	}
	err := createSPVActivity(ctx, s.Client.ActivityEvent.Create(), connection.ClientID, connection.ID, "SPV_MANUAL_SYNC_REQUESTED", "Sincronizarea manuală ANAF/SPV a fost solicitată.", commandID, actor, now)
	if ent.IsConstraintError(err) {
		return false, nil
	}
	return err == nil, err
}

func createSPVActivity(ctx context.Context, create *ent.ActivityEventCreate, clientID, connectionID, eventType, detail, key string, actor spv.Actor, now time.Time) error {
	create.SetID(stableID("evt", eventType+":"+connectionID+":"+key)).SetClientID(clientID).SetAggregateType("SPV_CONNECTION").SetAggregateID(connectionID).SetEventType(eventType).SetOccurredAt(now).SetActorKind(activityevent.ActorKindUSER).SetAutomatic(false).SetDetail(detail)
	if actor.ID != "" {
		create.SetActorID(actor.ID)
	}
	if actor.Display != "" {
		create.SetActorDisplay(actor.Display)
	}
	if actor.CorrelationID != "" {
		create.SetCorrelationID(actor.CorrelationID)
	}
	if key != "" {
		create.SetIdempotencyKey(key)
	}
	_, err := create.Save(ctx)
	return err
}

func (s *Store) GetConnection(ctx context.Context, id string) (spv.Connection, error) {
	row, err := s.Client.SPVConnection.Get(ctx, id)
	if ent.IsNotFound(err) {
		return spv.Connection{}, apperrors.ErrNotFound
	}
	if err != nil {
		return spv.Connection{}, err
	}
	return spvConnectionDomain(row), nil
}

func (s *Store) InstallSPVConnection(ctx context.Context, clientID, environment, access, refresh string, accessExpires time.Time, refreshExpires *time.Time, now time.Time) (spv.Connection, error) {
	client, err := s.Client.AccountingClient.Get(ctx, clientID)
	if ent.IsNotFound(err) {
		return spv.Connection{}, apperrors.ErrNotFound
	}
	if err != nil {
		return spv.Connection{}, err
	}
	create := s.Client.SPVConnection.Create().SetID(stableID("spvconn", environment+":"+client.Cui)).SetClientID(client.ID).SetCif(client.Cui).SetEnvironment(spvconnection.Environment(environment)).SetAccessTokenCiphertext(access).SetRefreshTokenCiphertext(refresh).SetAccessTokenExpiresAt(accessExpires).SetStatus(spvconnection.StatusACTIVE).SetConnectedAt(now).SetCreatedAt(now).SetUpdatedAt(now)
	if refreshExpires != nil {
		create.SetRefreshTokenExpiresAt(*refreshExpires)
	}
	row, err := create.Save(ctx)
	if err != nil {
		return spv.Connection{}, err
	}
	return spvConnectionDomain(row), nil
}

func spvConnectionDomain(row *ent.SPVConnection) spv.Connection {
	return spv.Connection{ID: row.ID, ClientID: row.ClientID, CIF: row.Cif, Environment: string(row.Environment), AccessTokenCiphertext: row.AccessTokenCiphertext, RefreshTokenCiphertext: row.RefreshTokenCiphertext, AccessTokenExpiresAt: row.AccessTokenExpiresAt, RefreshTokenExpiresAt: row.RefreshTokenExpiresAt, ConnectedAt: row.ConnectedAt, LastSuccessfulSyncAt: row.LastSuccessfulSyncAt, LastSyncStartedAt: row.LastSyncStartedAt, LastSyncFinishedAt: row.LastSyncFinishedAt, LastSyncStatus: string(row.LastSyncStatus), LastError: valueOrEmpty(row.LastError), Status: string(row.Status), Revision: row.Revision}
}

func (s *Store) SaveTokens(ctx context.Context, id string, revision uint64, access, refresh string, expires time.Time, refreshExpires *time.Time, now time.Time) (spv.Connection, error) {
	update := s.Client.SPVConnection.UpdateOneID(id).Where(spvconnection.RevisionEQ(revision)).SetAccessTokenCiphertext(access).SetRefreshTokenCiphertext(refresh).SetAccessTokenExpiresAt(expires).SetStatus(spvconnection.StatusACTIVE).SetUpdatedAt(now).AddRevision(1).ClearLastError()
	if refreshExpires != nil {
		update.SetRefreshTokenExpiresAt(*refreshExpires)
	}
	row, err := update.Save(ctx)
	if ent.IsNotFound(err) {
		return spv.Connection{}, apperrors.ErrConflict
	}
	if err != nil {
		return spv.Connection{}, err
	}
	return spvConnectionDomain(row), nil
}

func (s *Store) MarkSyncStarted(ctx context.Context, id string, now time.Time) error {
	_, err := s.Client.SPVConnection.UpdateOneID(id).SetLastSyncStartedAt(now).SetLastSyncStatus(spvconnection.LastSyncStatusRUNNING).SetUpdatedAt(now).Save(ctx)
	return err
}
func (s *Store) MarkSyncFinished(ctx context.Context, id string, now time.Time, cause error) error {
	update := s.Client.SPVConnection.UpdateOneID(id).SetUpdatedAt(now).SetLastSyncFinishedAt(now)
	if cause == nil {
		update.SetLastSuccessfulSyncAt(now).SetLastSyncStatus(spvconnection.LastSyncStatusSUCCEEDED).ClearLastError()
	} else {
		update.SetLastSyncStatus(spvconnection.LastSyncStatusFAILED).SetLastError(safeStoreError(cause))
		if errors.Is(cause, spv.ErrReauthenticationRequired) {
			update.SetStatus(spvconnection.StatusEXPIRED)
		}
	}
	_, err := update.Save(ctx)
	return err
}

func (s *Store) Discover(ctx context.Context, connection spv.Connection, message spv.Message, now time.Time) (spv.SourceDocument, bool, error) {
	existing, err := s.Client.SPVSourceDocument.Query().Where(spvsourcedocument.ConnectionIDEQ(connection.ID), spvsourcedocument.ExternalMessageIDEQ(message.ID)).Only(ctx)
	if err == nil {
		return spvDocumentDomain(existing), false, nil
	}
	if !ent.IsNotFound(err) {
		return spv.SourceDocument{}, false, err
	}
	create := s.Client.SPVSourceDocument.Create().SetID(stableID("spvdoc", connection.ID+":"+message.ID)).SetConnectionID(connection.ID).SetClientID(connection.ClientID).SetExternalMessageID(message.ID).SetDiscoveredAt(now).SetAvailableAt(now).SetUpdatedAt(now)
	if message.UploadID != "" {
		create.SetExternalUploadID(message.UploadID)
	}
	if message.RequestID != "" {
		create.SetExternalRequestID(message.RequestID)
	}
	if message.Type != "" {
		create.SetMessageType(message.Type)
	}
	if message.CreatedRaw != "" {
		create.SetSourceCreatedRaw(message.CreatedRaw)
	}
	if message.CreatedAt != nil {
		create.SetSourceCreatedAt(*message.CreatedAt)
	}
	row, err := create.Save(ctx)
	if err != nil && IsConstraintError(err) {
		row, err = s.Client.SPVSourceDocument.Query().Where(spvsourcedocument.ConnectionIDEQ(connection.ID), spvsourcedocument.ExternalMessageIDEQ(message.ID)).Only(ctx)
		if err == nil {
			return spvDocumentDomain(row), false, nil
		}
	}
	if err != nil {
		return spv.SourceDocument{}, false, err
	}
	return spvDocumentDomain(row), true, nil
}

func (s *Store) ClaimSourceDocument(ctx context.Context, id, owner string, now time.Time, ttl time.Duration) (spv.SourceDocument, error) {
	cutoff := now.Add(-ttl)
	count, err := s.Client.SPVSourceDocument.Update().Where(spvsourcedocument.IDEQ(id), spvsourcedocument.Or(spvsourcedocument.ProcessingStatusEQ(spvsourcedocument.ProcessingStatusDISCOVERED), spvsourcedocument.And(spvsourcedocument.ProcessingStatusEQ(spvsourcedocument.ProcessingStatusDOWNLOADED), spvsourcedocument.ClaimedAtLTE(cutoff)), spvsourcedocument.And(spvsourcedocument.ProcessingStatusEQ(spvsourcedocument.ProcessingStatusFAILED), spvsourcedocument.FailureKindEQ(spvsourcedocument.FailureKindTRANSIENT), spvsourcedocument.AvailableAtLTE(now)), spvsourcedocument.And(spvsourcedocument.ProcessingStatusEQ(spvsourcedocument.ProcessingStatusPROCESSING), spvsourcedocument.ClaimedAtLTE(cutoff)))).SetProcessingStatus(spvsourcedocument.ProcessingStatusPROCESSING).SetClaimOwner(owner).SetClaimedAt(now).AddAttempts(1).SetUpdatedAt(now).Save(ctx)
	if err != nil {
		return spv.SourceDocument{}, err
	}
	if count != 1 {
		return spv.SourceDocument{}, spv.ErrDocumentClaimed
	}
	row, err := s.Client.SPVSourceDocument.Get(ctx, id)
	if err != nil {
		return spv.SourceDocument{}, err
	}
	return spvDocumentDomain(row), nil
}

func (s *Store) StoreRaw(ctx context.Context, id, owner string, raw []byte, contentType, hash string, now time.Time) error {
	count, err := s.Client.SPVSourceDocument.Update().Where(spvsourcedocument.IDEQ(id), spvsourcedocument.ProcessingStatusEQ(spvsourcedocument.ProcessingStatusPROCESSING), spvsourcedocument.ClaimOwnerEQ(owner), spvsourcedocument.RawDocumentIsNil()).SetRawDocument(raw).SetContentType(contentType).SetContentSha256(hash).SetDownloadedAt(now).SetProcessingStatus(spvsourcedocument.ProcessingStatusDOWNLOADED).SetUpdatedAt(now).Save(ctx)
	if err != nil {
		return err
	}
	if count != 1 {
		return apperrors.ErrConflict
	}
	return nil
}

func (s *Store) MarkSourceFailed(ctx context.Context, id, owner, kind, message string, availableAt, now time.Time) error {
	count, err := s.Client.SPVSourceDocument.Update().Where(spvsourcedocument.IDEQ(id), spvsourcedocument.ClaimOwnerEQ(owner), spvsourcedocument.ProcessingStatusIn(spvsourcedocument.ProcessingStatusPROCESSING, spvsourcedocument.ProcessingStatusDOWNLOADED)).SetProcessingStatus(spvsourcedocument.ProcessingStatusFAILED).SetFailureKind(spvsourcedocument.FailureKind(kind)).SetLastError(message).SetAvailableAt(availableAt).SetUpdatedAt(now).ClearClaimOwner().ClearClaimedAt().Save(ctx)
	if err != nil {
		return err
	}
	if count != 1 {
		return apperrors.ErrConflict
	}
	return nil
}

func (s *Store) MarkProcessed(ctx context.Context, id, owner, invoiceID, parserType, parserVersion string, now time.Time) error {
	count, err := s.Client.SPVSourceDocument.Update().Where(spvsourcedocument.IDEQ(id), spvsourcedocument.ProcessingStatusIn(spvsourcedocument.ProcessingStatusPROCESSING, spvsourcedocument.ProcessingStatusDOWNLOADED), spvsourcedocument.ClaimOwnerEQ(owner)).SetProcessingStatus(spvsourcedocument.ProcessingStatusPROCESSED).SetInvoiceID(invoiceID).SetParserType(parserType).SetParserVersion(parserVersion).SetProcessedAt(now).SetUpdatedAt(now).ClearFailureKind().ClearLastError().ClearClaimOwner().ClearClaimedAt().Save(ctx)
	if err != nil {
		return err
	}
	if count != 1 {
		return apperrors.ErrConflict
	}
	return nil
}

func spvDocumentDomain(row *ent.SPVSourceDocument) spv.SourceDocument {
	failure := ""
	if row.FailureKind != nil {
		failure = string(*row.FailureKind)
	}
	return spv.SourceDocument{ID: row.ID, ConnectionID: row.ConnectionID, ClientID: row.ClientID, ExternalMessageID: row.ExternalMessageID, Status: string(row.ProcessingStatus), FailureKind: failure, RawDocument: row.RawDocument, ContentSHA256: valueOrEmpty(row.ContentSha256), ContentType: valueOrEmpty(row.ContentType), Attempts: row.Attempts}
}
func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
func safeStoreError(err error) string {
	if err == nil {
		return ""
	}
	value := err.Error()
	if len(value) > 500 {
		value = value[:500]
	}
	return value
}

func (s *Store) ClientOperational(ctx context.Context, id string) (bool, error) {
	var lifecycle string
	err := s.DB.QueryRowContext(ctx, `SELECT lifecycle FROM clients WHERE id=$1`, id).Scan(&lifecycle)
	if errors.Is(err, sql.ErrNoRows) {
		return false, apperrors.ErrNotFound
	}
	return lifecycle != "INACTIVE", err
}
