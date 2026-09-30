package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"diana-contabilitate/backend/internal/commercialvalidation"
	"diana-contabilitate/backend/internal/platform/postgres"
)

// demoCommercialServices are the services each demo contract prices, worded
// as the Module 6 and Module 7 journey invoices bill them, at the 100.00 RON
// those invoices charge per unit. Vest has two plausible contracts; whichever
// the reviewer confirms prices the same services.
var demoCommercialServices = []struct {
	contractID   string
	descriptions []string
}{
	{"contract-demo-100", []string{"Serviciu demonstrativ automat Module 7", "Element necunoscut pentru decizie umană"}},
	{"contract-demo-201", []string{"Serviciu demonstrativ după confirmare", "Serviciu demonstrativ după reload", "Element necunoscut pentru validare completă"}},
	{"contract-demo-202", []string{"Serviciu demonstrativ după confirmare", "Serviciu demonstrativ după reload", "Element necunoscut pentru validare completă"}},
}

// demoCommercialRules are the confirmed tariffs of one demo contract: a
// fixed price per service, matched to invoice lines by the service wording.
func demoCommercialRules(contractID string, descriptions []string) []commercialvalidation.Rule {
	rules := make([]commercialvalidation.Rule, 0, len(descriptions))
	for index, description := range descriptions {
		serviceID := fmt.Sprintf("%sseed-%s-%d", commercialvalidation.ServiceTariffPrefix, contractID, index+1)
		rules = append(rules, commercialvalidation.Rule{
			ID: serviceID, Kind: commercialvalidation.RuleFixedPrice, Narrative: description + " · 100.00 RON",
			Applicability: commercialvalidation.Applicability{ServiceID: serviceID, Aliases: []string{description}},
			DateBasis:     commercialvalidation.DateInvoiceIssue, Currency: "RON",
			Expression: &commercialvalidation.Expression{Op: "literal", Value: "100.00", Scale: 4},
			Evidence:   []commercialvalidation.Evidence{{Snippet: description + " — 100,00 lei (tarif demonstrativ)"}},
			Blocking:   true,
		})
	}
	return rules
}

// seedDemoCommercialDossiers gives the demo contracts a confirmed, complete
// commercial snapshot, as a contract confirmed from its PDF has, so the
// journey invoices pass commercial validation on their own and the worker
// continues without a human exception. A contract that already has a dossier
// is left untouched, so seeding twice changes nothing.
func seedDemoCommercialDossiers(ctx context.Context, store *postgres.Store, now time.Time) error {
	for _, item := range demoCommercialServices {
		if err := seedCommercialDossier(ctx, store, item.contractID, demoCommercialRules(item.contractID, item.descriptions), now); err != nil {
			return fmt.Errorf("seed commercial dossier %s: %w", item.contractID, err)
		}
	}
	return nil
}

func seedCommercialDossier(ctx context.Context, store *postgres.Store, contractID string, rules []commercialvalidation.Rule, now time.Time) error {
	for _, rule := range rules {
		if err := commercialvalidation.ValidateRule(rule); err != nil {
			return fmt.Errorf("rule %s: %w", rule.ID, err)
		}
	}
	tx, err := store.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var clientID, supplierCUI, reference, buyerCUI string
	var effectiveFrom time.Time
	var effectiveTo sql.NullTime
	if err = tx.QueryRowContext(ctx, `SELECT c.client_id,c.supplier_cui,c.reference,c.effective_from,c.effective_to,cl.cui FROM contracts c JOIN clients cl ON cl.id=c.client_id WHERE c.id=$1`, contractID).
		Scan(&clientID, &supplierCUI, &reference, &effectiveFrom, &effectiveTo, &buyerCUI); err != nil {
		return err
	}
	dossierID := "dossier-seed-" + contractID
	result, err := tx.ExecContext(ctx, `INSERT INTO contract_dossiers(id,client_id,contract_id,supplier_cui,buyer_cui,primary_reference,status,revision,created_at,updated_at)
		VALUES($1,$2,$3,$4,$5,$6,'DRAFT',1,$7,$7) ON CONFLICT DO NOTHING`, dossierID, clientID, contractID, supplierCUI, buyerCUI, reference, now)
	if err != nil {
		return err
	}
	if inserted, _ := result.RowsAffected(); inserted == 0 {
		return nil
	}
	rulesJSON, err := json.Marshal(rules)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(rulesJSON)
	snapshotID := "commercial-snapshot-seed-" + contractID
	var to any
	if effectiveTo.Valid {
		to = effectiveTo.Time
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO contract_commercial_snapshots(id,dossier_id,contract_id,version,schema_version,coverage,effective_from,effective_to,rules,rules_hash,confirmed_by_id,confirmed_by_display,confirmed_at,created_at)
		VALUES($1,$2,$3,1,$4,'COMPLETE',$5,$6,$7,$8,'devseed','Date demonstrative',$9,$9)`, snapshotID, dossierID, contractID, commercialvalidation.RuleSchemaVersion, effectiveFrom, to, rulesJSON, hex.EncodeToString(sum[:]), now); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE contract_dossiers SET active_snapshot_id=$2,status='ACTIVE_COMPLETE' WHERE id=$1`, dossierID, snapshotID); err != nil {
		return err
	}
	return tx.Commit()
}
