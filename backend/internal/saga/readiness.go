package saga

import (
	"diana-contabilitate/backend/internal/accounting"
	"diana-contabilitate/backend/internal/accountingdate"
	"diana-contabilitate/backend/internal/classification"
	"diana-contabilitate/backend/internal/invoicing"
	"fmt"
)

const DomainExporterVersion = "SAGA_C_DOMAIN_V2_V1"

type Readiness struct {
	Ready          bool
	Reason         string
	MappingVersion string
	Lines          map[string]lineTag
}

// EvaluateReadiness is the single authority for domain-v2 completion, review and
// generation. ignoreTask is used only inside the completion transaction before
// the current classification task is resolved; other task types still block.
func EvaluateReadiness(item *invoicing.Invoice, client ClientIdentity, allowTest, ignoreTask bool) Readiness {
	fail := func(reason string) Readiness { return Readiness{Reason: reason} }
	if item == nil || item.ID == "" || client.ID != item.ClientID || client.Name == "" || client.CUI == "" || item.SupplierCUI == nil || *item.SupplierCUI == "" || item.SupplierName == "" || item.DocumentNumber == "" || item.IssueDate.IsZero() {
		return fail("Identitatea facturii/clientului este incompletă.")
	}
	if item.ActiveTask != nil && (!ignoreTask || item.ActiveTask.Type != "CLASSIFICATION") {
		return fail("Există o sarcină de validare activă.")
	}
	if item.DocumentType != "INVOICE" {
		return fail("CreditNote/storno nu este acceptat.")
	}
	if item.ModelVersion != accounting.ModelVersion {
		// Legacy validation preserves exactly the old export contract/meaning.
		copy := *item
		copy.PipelineStatus = invoicing.StatusReadyForSAGA
		if ignoreTask {
			copy.ActiveTask = nil
		}
		if _, err := generateLegacy(&copy, client); err != nil {
			return fail(err.Error())
		}
		return Readiness{Ready: true, MappingVersion: ExporterVersion}
	}
	if item.ModelVersion == accounting.ModelVersion && (item.SourceFacts == nil || invoicing.NormalizeBusinessIdentifier(item.SourceFacts.BuyerVATID) != invoicing.NormalizeBusinessIdentifier(client.CUI) || invoicing.NormalizeBusinessIdentifier(item.SourceFacts.SupplierVATID) != invoicing.NormalizeBusinessIdentifier(*item.SupplierCUI)) {
		return fail("Identitatea fiscală sursă nu corespunde clientului/furnizorului facturii.")
	}
	date := accountingdate.FromTime(item.IssueDate)
	snapshot := item.AccountingSnapshot
	if snapshot == nil || !snapshot.Profile.Valid(item.ClientID, date) || !snapshot.Pack.Valid(snapshot.Profile, item.ClientID, date, allowTest) {
		return fail("Lipsește un snapshot aprobat și coerent de profil/politică/pack.")
	}
	mapping := snapshot.Pack.Mapping
	if mapping.Version == "" || !mapping.Approved || !mapping.Approval.Valid() || mapping.TestOnly && !allowTest || !mapping.OrdinaryFullOmission {
		return fail("Maparea SAGA pentru tratamentul obișnuit nu este aprobată/validată.")
	}
	if !snapshot.Profile.Ordinary() {
		return fail("Profilul fiscal/utilizarea necesită un tratament neacceptat de maparea inițială.")
	}
	if item.SourceFacts != nil && item.SourceFacts.TaxPointDate != "" && item.SourceFacts.TaxPointDate != string(date) {
		return fail("Data exigibilității necesită revizuirea tratamentului.")
	}
	if item.SourceFacts != nil && item.SourceFacts.CashAccounting == "UNKNOWN" {
		confirmed := false
		for _, r := range snapshot.Pack.Rules {
			if r.Predicate.SupplierID == *item.SupplierCUI && r.Predicate.SupplierCashAccounting == "NO" && r.Predicate.SupplierVATRegistration == "ORDINARY_REGISTERED" && r.AcquisitionPolicy != "" && date.Within(r.EffectiveFrom, r.EffectiveTo) {
				confirmed = true
			}
		}
		if !confirmed {
			return fail("Lipsește confirmarea aprobată a regimului TVA al furnizorului.")
		}
	}
	var sourceLines []accounting.SourceLine
	for _, l := range item.Lines {
		sourceLines = append(sourceLines, accounting.SourceLine{ID: l.ID, Facts: l.SourceFacts, Net: l.NetValue, VAT: l.VATValue, Total: l.TotalValue, Rate: l.VATRate})
	}
	if err := accounting.Reconcile(item.SourceFacts, sourceLines, item.Total.Amount, item.Total.Currency); err != nil {
		return fail(err.Error())
	}
	positions := map[int]bool{}
	lineIDs := map[string]bool{}
	result := Readiness{Ready: true, MappingVersion: mapping.Version, Lines: map[string]lineTag{}}
	for _, l := range item.Lines {
		if l.ID == "" || positions[l.Position] || lineIDs[l.ID] {
			return fail("Identificatori/poziții de linie duplicate sau absente.")
		}
		positions[l.Position] = true
		lineIDs[l.ID] = true
		if !l.Quantity.Valid() || !l.UnitPrice.Valid() || l.Quantity.String()[0] == '-' || l.Quantity.Equal("0") || l.UnitPrice.String()[0] == '-' {
			return fail("Cantitate/preț neacceptate pentru factura obișnuită.")
		}
		if l.SourceFacts.PriceAmount != nil && !l.SourceFacts.PriceAmount.Amount.Equal(l.UnitPrice) {
			return fail("Prețul liniei diferă de sursa e-Factura.")
		}
		if l.Position <= 0 || l.Description == "" || l.Unit == "" || !l.Quantity.Valid() || !l.UnitPrice.Valid() {
			return fail("Datele cantitative ale liniei sunt invalide.")
		}
		if !accounting.MatchProduct(l.Quantity, l.UnitPrice, l.NetValue) {
			return fail("Cantitatea și prețul nu se reconciliază cu valoarea liniei.")
		}
		values := map[string]accounting.Value{}
		for _, d := range l.Classifications {
			if d.ModelVersion != accounting.ModelVersion {
				continue
			}
			if _, exists := values[string(d.Dimension)]; exists {
				return fail("Decizii duplicate pentru aceeași dimensiune.")
			}
			if (d.Status != classification.ReviewAccepted && d.Status != classification.ReviewCorrected) || d.EffectiveValue == nil || d.TypedValue != nil && *d.EffectiveValue != d.TypedValue.Text() || d.Status == classification.ReviewPending || d.TypedValue == nil || d.TypedValue.Validate(string(d.Dimension)) != nil {
				return fail("Decizii contabile nefinalizate sau invalide.")
			}
			e := d.Evidence
			if e == nil || e.ModelVersion != accounting.ModelVersion || e.ProfileID != snapshot.Profile.ID || e.ProfileVersion != snapshot.Profile.Version || e.PolicyID != snapshot.Profile.ChartPolicy || e.DateBasis != accounting.IssueDateBasis || e.Date != date || e.SourceDocumentID != item.SourceFacts.SourceDocumentID || e.ParserVersion != item.SourceFacts.ParserVersion || e.SourceHash != item.SourceFacts.SourceHash || e.SourcePath != l.SourceFacts.Path {
				return fail("Proveniența deciziei nu corespunde snapshot-ului.")
			}
			if !d.HumanReviewed {
				if d.Source != classification.SourceRule || d.Rule == nil || e.PackID != snapshot.Pack.ID || e.PackVersion != snapshot.Pack.Version || e.RuleVersionID != d.Rule.RuleVersionID || d.PolicyVersion != classification.DomainPolicyVersion {
					return fail("Decizie automată fără dovada regulii/pack-ului.")
				}
				if !snapshot.Pack.TestOnly && (!d.Rule.ProductionEligible || !d.Rule.Provenance.Valid() || d.Rule.RulePackVersion != fmt.Sprintf("%s/%d", snapshot.Pack.ID, snapshot.Pack.Version)) {
					return fail("Versiunea regulii nu este eligibilă/verificată pentru release-ul de producție.")
				}
				found := false
				for _, r := range snapshot.Pack.Rules {
					if r.VersionID == e.RuleVersionID && r.Dimension == string(d.Dimension) && date.Within(r.EffectiveFrom, r.EffectiveTo) && r.Result.Validate(r.Dimension) == nil {
						input := classification.InvoiceContext{ModelVersion: item.ModelVersion, IssueDate: date, DocumentType: string(item.DocumentType), Currency: item.Total.Currency, ClientID: item.ClientID, SupplierID: *item.SupplierCUI, SourceFacts: item.SourceFacts, Snapshot: snapshot}
						line := classification.LineContext{ID: l.ID, Description: l.Description, VATRate: l.VATRate, VATValue: l.VATValue, SourceFacts: l.SourceFacts}
						if !classification.ValidateRuleMatch(r, input, line) {
							return fail("Faptele curente nu justifică regula automată.")
						}
						if !sameValue(r.Result, *d.TypedValue) {
							return fail("Rezultatul automat diferă de regula imuabilă.")
						}
						found = true
					}
				}
				if !found {
					return fail("Regula deciziei nu există în release-ul selectat.")
				}
			} else if d.ReviewReason == "" {
				return fail("Lipsește motivul confirmării/corecției contabile.")
			}
			values[string(d.Dimension)] = *d.TypedValue
		}
		for _, dimension := range accounting.Dimensions {
			if _, ok := values[dimension]; !ok {
				return fail("Lipsește decizia " + dimension + ".")
			}
		}
		if !snapshot.Profile.AccountAllowed(values["ACCOUNT"].Account) {
			return fail("Contul/analiticul nu este în vocabularul aprobat al politicii clientului.")
		}
		tag, err := mapDomainLine(l, values, mapping)
		if err != nil {
			return fail(err.Error())
		}
		result.Lines[l.ID] = tag
	}
	return result
}
func sameValue(a, b accounting.Value) bool { return canonicalValue(a) == canonicalValue(b) }
func mapDomainLine(l invoicing.Line, v map[string]accounting.Value, p accounting.MappingPolicy) (lineTag, error) {
	treatment, vat, expense := v["VAT_TREATMENT"], v["VAT_DEDUCTIBILITY"], v["EXPENSE_TAX_TREATMENT"]
	if treatment.Kind != "ORDINARY" || treatment.Timing != "IMMEDIATE" || treatment.SourceCategory != l.SourceFacts.Code || treatment.SourceRate == nil || !treatment.SourceRate.Equal(l.VATRate) {
		return lineTag{}, fmt.Errorf("Tratamentul TVA nu este compatibil cu sursa/maparea SAGA.")
	}
	// No combined import instruction is guessed. Only the explicitly approved
	// full/full omission is implemented; N50/I extensions await real validation.
	if vat.Kind != "FULL" || expense.Kind != "FULLY_DEDUCTIBLE" || !p.OrdinaryFullOmission {
		return lineTag{}, fmt.Errorf("Combinația dreptului de deducere TVA și tratamentului cheltuielii nu poate fi reprezentată fidel de maparea SAGA inițială.")
	}
	additional := ""
	if l.AdditionalInfo != nil {
		additional = *l.AdditionalInfo
	}
	return lineTag{Position: l.Position, Description: l.Description, AdditionalInfo: additional, Unit: l.Unit, Quantity: l.Quantity.String(), UnitPrice: l.UnitPrice.String(), NetValue: l.NetValue.String(), VATRate: l.VATRate.String(), VATValue: l.VATValue.String(), Account: v["ACCOUNT"].Account}, nil
}
