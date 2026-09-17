package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"diana-contabilitate/backend/internal/accounting"
	"diana-contabilitate/backend/internal/accountingdate"
	"diana-contabilitate/backend/internal/apperrors"
	"diana-contabilitate/backend/internal/clients"
	"diana-contabilitate/backend/internal/spv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5/pgconn"
	"time"
)

const clientColumns = `id,name,cui,display_name,registration_number,country,registered_address,city,region,postal_code,email,phone,default_currency,normalized_identifier,lifecycle,revision,created_at,updated_at`

type clientQuery interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}
type scanner interface{ Scan(...any) error }

func scanClient(row scanner) (clients.Client, error) {
	var c clients.Client
	err := row.Scan(&c.ID, &c.Name, &c.CUI, &c.Company.DisplayName, &c.Company.RegistrationNumber, &c.Company.Country, &c.Company.Address, &c.Company.City, &c.Company.Region, &c.Company.PostalCode, &c.Company.Email, &c.Company.Phone, &c.Company.DefaultCurrency, &c.NormalizedIdentifier, &c.Status, &c.Revision, &c.CreatedAt, &c.UpdatedAt)
	c.Company.Name = c.Name
	c.Company.CUI = c.CUI
	if errors.Is(err, sql.ErrNoRows) {
		err = apperrors.ErrNotFound
	}
	return c, err
}
func loadClientDetail(ctx context.Context, q clientQuery, id string) (clients.Detail, error) {
	d := clients.Detail{Profiles: []*accounting.Profile{}, History: []clients.History{}, SagaEnabled: true}
	c, err := scanClient(q.QueryRowContext(ctx, `SELECT `+clientColumns+` FROM clients WHERE id=$1`, id))
	if err != nil {
		return d, err
	}
	d.Client = c
	rows, err := q.QueryContext(ctx, `SELECT payload FROM client_accounting_profiles WHERE client_id=$1 ORDER BY version DESC`, id)
	if err != nil {
		return d, err
	}
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			rows.Close()
			return d, err
		}
		var p accounting.Profile
		if err = json.Unmarshal(raw, &p); err != nil {
			rows.Close()
			return d, err
		}
		d.Profiles = append(d.Profiles, &p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return d, err
	}
	err = q.QueryRowContext(ctx, `SELECT enabled FROM client_saga_configurations WHERE client_id=$1`, id).Scan(&d.SagaEnabled)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return d, err
	}
	rows, err = q.QueryContext(ctx, `SELECT id,event_type,COALESCE(actor_display,'Sistem'),occurred_at FROM activity_events WHERE client_id=$1 AND aggregate_type IN ('CLIENT','ACCOUNTING_PROFILE','SPV_CONNECTION','SAGA_CONFIGURATION','ACCOUNTING_RULE_PACK') ORDER BY occurred_at DESC,id DESC LIMIT 100`, id)
	if err != nil {
		return d, err
	}
	for rows.Next() {
		var h clients.History
		var at time.Time
		var event string
		if err = rows.Scan(&h.ID, &event, &h.Actor, &at); err != nil {
			rows.Close()
			return d, err
		}
		h.Timestamp = at.Format(time.RFC3339)
		h.Label = clientHistoryLabel(event)
		h.Detail = h.Label
		d.History = append(d.History, h)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return d, err
	}
	packs := []*accounting.Pack{}
	rows, err = q.QueryContext(ctx, `SELECT payload FROM accounting_rule_packs WHERE client_id=$1`, id)
	if err != nil {
		return d, err
	}
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			rows.Close()
			return d, err
		}
		var p accounting.Pack
		if err = json.Unmarshal(raw, &p); err != nil {
			rows.Close()
			return d, err
		}
		packs = append(packs, &p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return d, err
	}
	// Environment/application availability is supplied by the existing ConnectionManager at HTTP boundary.
	d.Onboarding = clients.Derive(c, d.Profiles, packs, d.SagaEnabled, spv.ConnectionView{}, accountingdate.FromTime(time.Now().UTC()))
	return d, nil
}
func clientHistoryLabel(event string) string {
	labels := map[string]string{"CLIENT_CREATED": "Client creat", "CLIENT_UPDATED": "Date companie actualizate", "CLIENT_DEACTIVATED": "Client dezactivat", "CLIENT_REACTIVATED": "Client reactivat", "CLIENT_ACTIVATED": "Client activat", "ACCOUNTING_PROFILE_CREATED": "Versiune profil configurată", "ACCOUNTING_PROFILE_APPROVED": "Versiune profil aprobată", "ANAF_CONFIGURATION_STARTED": "Autorizare ANAF inițiată", "SPV_CONNECTION_CONNECTED": "ANAF autorizat", "SPV_CONNECTION_RECONNECTED": "ANAF reautorizat", "SPV_CONNECTION_DISCONNECTED": "ANAF deconectat", "SPV_MANUAL_SYNC_REQUESTED": "Sincronizare ANAF solicitată", "SAGA_CONFIGURATION_UPDATED": "Configurație export SAGA actualizată", "ACCOUNTING_PACK_RELEASED": "Pack contabil aprobat"}
	if v, ok := labels[event]; ok {
		return v
	}
	return "Configurație operațională actualizată"
}
func (s *Store) GetClientDetail(ctx context.Context, id string) (clients.Detail, error) {
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return clients.Detail{}, err
	}
	defer tx.Rollback()
	d, err := loadClientDetail(ctx, tx, id)
	if err != nil {
		return d, err
	}
	return d, tx.Commit()
}
func (s *Store) ExecuteClientCommand(ctx context.Context, kind string, c clients.Command) (clients.Detail, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return clients.Detail{}, err
	}
	defer tx.Rollback()
	key := "client-management:" + c.Actor.ID + ":" + kind + ":" + c.ClientID + ":" + c.CommandID
	payload, _ := json.Marshal(c)
	sum := sha256.Sum256(payload)
	hash := hex.EncodeToString(sum[:])
	// Serialize identical submissions before checking ledger; different payload reuse conflicts.
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, key); err != nil {
		return clients.Detail{}, err
	}
	var priorHash string
	var priorID string
	err = tx.QueryRowContext(ctx, `SELECT request_hash,client_id FROM client_management_commands WHERE command_key=$1`, key).Scan(&priorHash, &priorID)
	if err == nil {
		if priorHash != hash {
			return clients.Detail{}, apperrors.ErrConflict
		}
		return loadClientDetail(ctx, tx, priorID)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return clients.Detail{}, err
	}
	now := time.Now().UTC()
	id := c.ClientID
	var before clients.Client
	event := "CLIENT_UPDATED"
	if kind == "create" {
		id = stableID("client", key)
	} else {
		before, err = scanClient(tx.QueryRowContext(ctx, `SELECT `+clientColumns+` FROM clients WHERE id=$1 FOR UPDATE`, id))
		if err != nil {
			return clients.Detail{}, err
		}
		if before.Revision != c.ExpectedRevision {
			return clients.Detail{}, fmt.Errorf("%w: revizie învechită; reîncarcă pagina", apperrors.ErrConflict)
		}
	}
	switch kind {
	case "create", "update":
		normalized, e := c.Company.Normalize()
		if kind == "update" {
			normalized, e = c.Company.NormalizeExisting(before)
		}
		if e != nil {
			return clients.Detail{}, e
		}
		v := c.Company
		if kind == "update" {
			// Legacy identifiers can be retained verbatim while changing ordinary metadata.
			var dependent bool
			err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM invoices WHERE client_id=$1) OR EXISTS(SELECT 1 FROM contracts WHERE client_id=$1) OR EXISTS(SELECT 1 FROM spv_connections WHERE client_id=$1) OR EXISTS(SELECT 1 FROM spv_oauth_states WHERE client_id=$1) OR EXISTS(SELECT 1 FROM spv_source_documents WHERE client_id=$1) OR EXISTS(SELECT 1 FROM contract_source_documents WHERE client_id=$1) OR EXISTS(SELECT 1 FROM client_accounting_profiles WHERE client_id=$1) OR EXISTS(SELECT 1 FROM accounting_rule_packs WHERE client_id=$1)`, id).Scan(&dependent)
			if err != nil {
				return clients.Detail{}, err
			}
			if e = clients.CanChangeIdentity(before, v, normalized, dependent); e != nil {
				return clients.Detail{}, e
			}
			_, err = tx.ExecContext(ctx, `UPDATE clients SET name=$2,cui=$3,display_name=$4,registration_number=$5,country=$6,registered_address=$7,city=$8,region=$9,postal_code=$10,email=$11,phone=$12,default_currency=$13,normalized_identifier=$14,revision=revision+1,updated_at=$15 WHERE id=$1`, id, v.Name, v.CUI, v.DisplayName, v.RegistrationNumber, v.Country, v.Address, v.City, v.Region, v.PostalCode, v.Email, v.Phone, v.DefaultCurrency, normalized, now)
		} else {
			event = "CLIENT_CREATED"
			_, err = tx.ExecContext(ctx, `INSERT INTO clients (`+clientColumns+`) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,'ONBOARDING',1,$15,$15)`, id, v.Name, v.CUI, v.DisplayName, v.RegistrationNumber, v.Country, v.Address, v.City, v.Region, v.PostalCode, v.Email, v.Phone, v.DefaultCurrency, normalized, now)
		}
	case "lifecycle":
		event = "CLIENT_ACTIVATED"
		if c.Status == clients.Inactive {
			event = "CLIENT_DEACTIVATED"
		} else if before.Status == clients.Inactive {
			event = "CLIENT_REACTIVATED"
		}
		_, err = tx.ExecContext(ctx, `UPDATE clients SET lifecycle=$2,revision=revision+1,updated_at=$3 WHERE id=$1`, id, c.Status, now)
	case "saga":
		event = "SAGA_CONFIGURATION_UPDATED"
		_, err = tx.ExecContext(ctx, `INSERT INTO client_saga_configurations(client_id,enabled,updated_at) VALUES($1,$2,$3) ON CONFLICT(client_id) DO UPDATE SET enabled=EXCLUDED.enabled,updated_at=EXCLUDED.updated_at`, id, c.SagaEnabled, now)
	case "profile":
		var latest int
		err = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version),0) FROM client_accounting_profiles WHERE client_id=$1`, id).Scan(&latest)
		if err != nil {
			return clients.Detail{}, err
		}
		if latest != c.ExpectedProfileVersion {
			return clients.Detail{}, apperrors.ErrConflict
		}
		p := *c.Profile
		p.ID = stableID("profile", key)
		p.ClientID = id
		p.Version = latest + 1
		p.Approval = accounting.Approval{}
		event = "ACCOUNTING_PROFILE_CREATED"
		if c.Approve {
			p.Approval = accounting.Approval{Actor: c.Actor.Display, At: now, Evidence: c.Evidence}
			if !p.Valid(id, p.EffectiveFrom) {
				return clients.Detail{}, apperrors.ErrValidation
			}
			existing, e := loadClientDetail(ctx, tx, id)
			if e != nil {
				return clients.Detail{}, e
			}
			for _, v := range existing.Profiles {
				if v != nil && v.Approval.Valid() && !v.TestOnly && (p.EffectiveTo == nil || v.EffectiveFrom <= *p.EffectiveTo) && (v.EffectiveTo == nil || p.EffectiveFrom <= *v.EffectiveTo) {
					return clients.Detail{}, fmt.Errorf("%w: perioada se suprapune unui profil aprobat imuabil", apperrors.ErrConflict)
				}
			}
			event = "ACCOUNTING_PROFILE_APPROVED"
		}
		raw, e := json.Marshal(p)
		if e != nil {
			return clients.Detail{}, e
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO client_accounting_profiles(id,client_id,version,payload,created_at) VALUES($1,$2,$3,$4,$5)`, p.ID, id, p.Version, raw, now)
	}
	if err == nil && kind == "create" {
		_, err = tx.ExecContext(ctx, `INSERT INTO client_saga_configurations(client_id,enabled,updated_at) VALUES($1,false,$2)`, id, now)
	}
	if err != nil {
		var pg *pgconn.PgError
		if errors.As(err, &pg) && pg.Code == "23505" {
			return clients.Detail{}, fmt.Errorf("%w: compania există deja", apperrors.ErrConflict)
		}
		return clients.Detail{}, err
	}
	if kind == "profile" || kind == "saga" {
		if _, err = tx.ExecContext(ctx, `UPDATE clients SET revision=revision+1,updated_at=$2 WHERE id=$1`, id, now); err != nil {
			return clients.Detail{}, err
		}
	}
	after, err := loadClientDetail(ctx, tx, id)
	if err != nil {
		return clients.Detail{}, err
	}
	// Safe bounded summaries. No provider errors, credentials or raw event payloads.
	beforeJSON, _ := json.Marshal(map[string]any{"company": before.Company, "name": before.Name, "cui": before.CUI, "status": before.Status, "revision": before.Revision})
	afterJSON, _ := json.Marshal(map[string]any{"company": after.Client.Company, "name": after.Client.Name, "cui": after.Client.CUI, "status": after.Client.Status, "revision": after.Client.Revision, "profileVersion": len(after.Profiles), "sagaEnabled": after.SagaEnabled})
	_, err = tx.ExecContext(ctx, `INSERT INTO activity_events(id,client_id,aggregate_type,aggregate_id,event_type,occurred_at,actor_kind,actor_id,actor_display,automatic,detail,before_snapshot,after_snapshot,correlation_id,idempotency_key) VALUES($1,$2,'CLIENT',$2,$3,$4,'USER',$5,$6,false,$7,$8,$9,$10,$11)`, stableID("evt", key), id, event, now, c.Actor.ID, c.Actor.Display, clientHistoryLabel(event), beforeJSON, afterJSON, c.CorrelationID, key)
	if err != nil {
		return clients.Detail{}, err
	}
	after, err = loadClientDetail(ctx, tx, id)
	if err != nil {
		return clients.Detail{}, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO client_management_commands(command_key,request_hash,client_id,created_at) VALUES($1,$2,$3,$4)`, key, hash, id, now)
	if err != nil {
		return clients.Detail{}, err
	}
	if err = tx.Commit(); err != nil {
		return clients.Detail{}, err
	}
	return after, nil
}

func (s *Store) CreateClientOAuthAttempt(ctx context.Context, state spv.OAuthState, ciphertext, key string, actor spv.Actor) (string, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, key); err != nil {
		return "", err
	}
	var replay string
	var expiry time.Time
	var consumed sql.NullTime
	err = tx.QueryRowContext(ctx, `SELECT a.state_ciphertext,s.expires_at,s.consumed_at FROM client_oauth_attempts a JOIN spv_oauth_states s ON s.id=a.state_id WHERE a.command_key=$1`, key).Scan(&replay, &expiry, &consumed)
	if err == nil {
		if consumed.Valid || !expiry.After(state.CreatedAt) {
			return "", spv.ErrInvalidOAuthState
		}
		return replay, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	var lifecycle string
	if err = tx.QueryRowContext(ctx, `SELECT lifecycle FROM clients WHERE id=$1 FOR UPDATE`, state.ClientID).Scan(&lifecycle); err != nil {
		return "", err
	}
	if lifecycle == "INACTIVE" {
		return "", spv.ErrConnectionInactive
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO spv_oauth_states(id,state_hash,client_id,environment,return_path,expires_at,created_at) VALUES($1,$2,$3,$4,$5,$6,$7)`, state.ID, state.StateHash, state.ClientID, state.Environment, state.ReturnPath, state.ExpiresAt, state.CreatedAt)
	if err != nil {
		return "", err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO client_oauth_attempts(command_key,state_id,state_ciphertext) VALUES($1,$2,$3)`, key, state.ID, ciphertext)
	if err != nil {
		return "", err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO activity_events(id,client_id,aggregate_type,aggregate_id,event_type,occurred_at,actor_kind,actor_id,actor_display,automatic,detail,correlation_id,idempotency_key) VALUES($1,$2,'CLIENT',$2,'ANAF_CONFIGURATION_STARTED',$3,'USER',$4,$5,false,'Autorizare ANAF inițiată; certificatul rămâne pe dispozitiv.',$6,$7)`, stableID("evt", key), state.ClientID, state.CreatedAt, actor.ID, actor.Display, actor.CorrelationID, key)
	if err != nil {
		return "", err
	}
	if err = tx.Commit(); err != nil {
		return "", err
	}
	return ciphertext, nil
}
