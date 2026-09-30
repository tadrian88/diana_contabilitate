package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"diana-contabilitate/backend/internal/accounting"
	"diana-contabilitate/backend/internal/accountingdate"
	"diana-contabilitate/backend/internal/invoicing"
)

func (s *Store) attachClassificationContext(ctx context.Context, item *invoicing.Invoice) error {
	if item.CurrentClassificationRunID == nil {
		return nil
	}
	var snapshotRaw []byte
	var profileID sql.NullString
	var profileVersion sql.NullInt64
	var createdAt sql.NullTime
	err := s.DB.QueryRowContext(ctx, `SELECT snapshot,profile_id,profile_version,created_at FROM classification_runs WHERE id=$1 AND client_id=$2 AND invoice_id=$3`, *item.CurrentClassificationRunID, item.ClientID, item.ID).Scan(&snapshotRaw, &profileID, &profileVersion, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	contextView := &invoicing.ClassificationContext{RunID: *item.CurrentClassificationRunID, ProfileID: profileID.String, ProfileVersion: int(profileVersion.Int64), StaleReasons: []string{}, CreatedAt: createdAt.Time}
	var snapshot accounting.Snapshot
	if json.Unmarshal(snapshotRaw, &snapshot) != nil {
		contextView.ContextStale = true
		contextView.StaleReasons = append(contextView.StaleReasons, "LEGACY_CONTEXT_INCOMPLETE")
		item.ClassificationContext = contextView
		return nil
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT payload FROM client_accounting_profiles WHERE client_id=$1 ORDER BY version`, item.ClientID)
	if err != nil {
		return err
	}
	profiles := []*accounting.Profile{}
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			rows.Close()
			return err
		}
		var profile accounting.Profile
		if json.Unmarshal(raw, &profile) == nil {
			profiles = append(profiles, &profile)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	current := accounting.ApplicableProfiles(profiles, item.ClientID, accountingdate.FromTime(item.IssueDay))
	if len(current) == 1 {
		if snapshot.Profile == nil {
			contextView.StaleReasons = append(contextView.StaleReasons, "FISCAL_PROFILE_AVAILABLE")
		} else if current[0].ID != snapshot.Profile.ID || current[0].Version != snapshot.Profile.Version {
			contextView.StaleReasons = append(contextView.StaleReasons, "FISCAL_PROFILE_CHANGED")
		} else if changed, compareErr := s.accountCatalogChangedForProfile(ctx, snapshot, current[0].AccountCodes); compareErr != nil {
			return compareErr
		} else if changed {
			contextView.StaleReasons = append(contextView.StaleReasons, "ACCOUNT_CATALOG_CHANGED")
		}
		packChanged, packErr := s.accountingPolicyChanged(ctx, snapshot.Pack, current[0], item.ClientID, accountingdate.FromTime(item.IssueDay))
		if packErr != nil {
			return packErr
		}
		if packChanged {
			contextView.StaleReasons = append(contextView.StaleReasons, "ACCOUNTING_POLICY_CHANGED")
		}
	} else if snapshot.Profile != nil {
		contextView.StaleReasons = append(contextView.StaleReasons, "FISCAL_PROFILE_MISSING_OR_AMBIGUOUS")
	}
	var currentContractID string
	var currentContractRevision uint64
	if err = s.DB.QueryRowContext(ctx, `SELECT COALESCE(a.contract_id,''),COALESCE(c.revision,0) FROM invoices i LEFT JOIN invoice_contract_associations a ON a.invoice_id=i.id LEFT JOIN contracts c ON c.id=a.contract_id WHERE i.id=$1 AND i.client_id=$2`, item.ID, item.ClientID).Scan(&currentContractID, &currentContractRevision); err != nil {
		return err
	}
	if currentContractID != snapshot.ContractID || currentContractRevision != snapshot.ContractRevision {
		contextView.StaleReasons = append(contextView.StaleReasons, "CONTRACT_CONTEXT_CHANGED")
	}
	contextView.ContextStale = len(contextView.StaleReasons) > 0
	item.ClassificationContext = contextView
	return nil
}

func (s *Store) accountingPolicyChanged(ctx context.Context, previous *accounting.Pack, profile *accounting.Profile, clientID string, date accountingdate.Date) (bool, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT payload FROM accounting_rule_packs WHERE client_id=$1`, clientID)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	applicable := []*accounting.Pack{}
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			return false, err
		}
		var pack accounting.Pack
		if json.Unmarshal(raw, &pack) == nil && pack.Valid(profile, clientID, date, true) {
			applicable = append(applicable, &pack)
		}
	}
	if err = rows.Err(); err != nil {
		return false, err
	}
	if previous == nil {
		return len(applicable) != 0, nil
	}
	return len(applicable) != 1 || applicable[0].ID != previous.ID || applicable[0].Version != previous.Version, nil
}

// accountCatalogChangedForProfile compares the explicit profile vocabulary
// when present; otherwise it compares the fingerprint of the global postable
// OMFP catalog. Runs recorded before fingerprints existed are not flagged.
func (s *Store) accountCatalogChangedForProfile(ctx context.Context, snapshot accounting.Snapshot, codes []string) (bool, error) {
	if len(codes) > 0 {
		return s.accountCatalogChanged(ctx, snapshot.AccountCatalog, codes)
	}
	if snapshot.AccountCatalogFingerprint == "" {
		return false, nil
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT code FROM accounts WHERE is_active AND postable ORDER BY code`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	current := []string{}
	for rows.Next() {
		var code string
		if err = rows.Scan(&code); err != nil {
			return false, err
		}
		current = append(current, code)
	}
	if err = rows.Err(); err != nil {
		return false, err
	}
	return accountCodesFingerprint(current) != snapshot.AccountCatalogFingerprint, nil
}

// accountCodesFingerprint hashes the sorted set of selectable account codes.
func accountCodesFingerprint(codes []string) string {
	sorted := append([]string(nil), codes...)
	sort.Strings(sorted)
	digest := sha256.Sum256([]byte(strings.Join(sorted, "\n")))
	return hex.EncodeToString(digest[:])
}

func (s *Store) accountCatalogChanged(ctx context.Context, previous []accounting.AccountSnapshot, codes []string) (bool, error) {
	if len(previous) == 0 {
		return true, nil
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT code,COALESCE(parent_code,''),is_active,postable FROM accounts WHERE code=ANY($1::text[]) ORDER BY code`, codes)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	current := []accounting.AccountSnapshot{}
	for rows.Next() {
		var value accounting.AccountSnapshot
		if err = rows.Scan(&value.Code, &value.ParentCode, &value.Active, &value.Postable); err != nil {
			return false, err
		}
		current = append(current, value)
	}
	if err = rows.Err(); err != nil {
		return false, err
	}
	if len(current) != len(codes) {
		return true, nil
	}
	for _, value := range current {
		if !value.Active || !value.Postable {
			return true, nil
		}
	}
	sort.Slice(previous, func(i, j int) bool { return previous[i].Code < previous[j].Code })
	left, _ := json.Marshal(previous)
	right, _ := json.Marshal(current)
	return string(left) != string(right), nil
}
