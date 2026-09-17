package classification

import (
	"diana-contabilitate/backend/internal/accounting"
	"diana-contabilitate/backend/internal/rules"
	"fmt"
	"strings"
)

const DomainPolicyVersion = "DETERMINISTIC_CLASSIFICATION_V1_DOMAIN_V2"

// DomainPolicy's TEST_ONLY opt-in is a constructor-level capability, never an
// environment fallback and never enabled by the API/production worker.
type DomainPolicy struct {
	AllowTestOnly bool
	Observer      ProductionObserver
}

func (DomainPolicy) Version() string { return DomainPolicyVersion }
func (p DomainPolicy) Evaluate(in InvoiceContext) (Result, error) {
	out := Result{PolicyVersion: p.Version(), ModelVersion: accounting.ModelVersion, Snapshot: in.Snapshot}
	if out.Snapshot == nil {
		out.Snapshot = &accounting.Snapshot{}
	}
	out.Snapshot.TestOnly = p.AllowTestOnly && out.Snapshot.Pack != nil && out.Snapshot.Pack.TestOnly
	profile, pack := out.Snapshot.Profile, out.Snapshot.Pack
	for _, line := range in.Lines {
		for _, dimension := range accounting.Dimensions {
			evaluated, matchCount := 0, 0
			proposal := Proposal{ModelVersion: accounting.ModelVersion, Dimension: Dimension(dimension), InvoiceLineID: line.ID, InvoiceDateUsed: in.IssueDate, ProposedValue: "Necesită decizie", Confidence: "Necesită revizuire", Explanation: "Lipsesc profilul/politica aprobate sau o regulă deterministă aplicabilă.", LegalBasis: "ACCOUNTING RULE SOURCE REQUIRED", RequiresReview: true, Source: SourceNoMatch}
			if profile.Valid(in.ClientID, in.IssueDate) && (!profile.TestOnly || p.AllowTestOnly) {
				proposal.Evidence = &accounting.Evidence{ModelVersion: accounting.ModelVersion, ProfileID: profile.ID, ProfileVersion: profile.Version, PolicyID: profile.ChartPolicy, DateBasis: accounting.IssueDateBasis, Date: in.IssueDate}
				if in.SourceFacts != nil {
					proposal.Evidence.SourceDocumentID = in.SourceFacts.SourceDocumentID
					proposal.Evidence.ParserVersion = in.SourceFacts.ParserVersion
					proposal.Evidence.SourceHash = in.SourceFacts.SourceHash
				}
				if line.SourceFacts != nil {
					proposal.Evidence.SourcePath = line.SourceFacts.Path
				}
				if dimension == "EXPENSE_TAX_TREATMENT" && profile.TaxRegime == "MICROENTERPRISE" {
					v := accounting.Value{Kind: "NOT_APPLICABLE", Reason: "Profilul aprobat indică regimul microîntreprinderii pentru această dată."}
					proposal.TypedValue = &v
					proposal.ProposedValue = v.Text()
					proposal.Explanation = v.Reason
					proposal.LegalBasis = "Profil fiscal aprobat; necesită confirmare contabilă în domeniul inițial."
				}
			}
			if profile.Valid(in.ClientID, in.IssueDate) && (!profile.TestOnly || p.AllowTestOnly) && profile.Ordinary() && pack.Valid(profile, in.ClientID, in.IssueDate, p.AllowTestOnly) && ordinarySource(in, line) {
				applicable := []accounting.Rule{}
				overridden := map[string]bool{}
				for _, r := range pack.Rules {
					if r.Dimension == dimension && in.IssueDate.Within(r.EffectiveFrom, r.EffectiveTo) {
						applicable = append(applicable, r)
						if r.Scope == "CLIENT_OVERRIDE" && r.ParentID != "" {
							overridden[r.ParentID] = true
						}
					}
				}
				matches := []accounting.Rule{}
				logical := map[string]int{}
				overlap := false
				for _, r := range applicable {
					if r.Scope == "GLOBAL" && overridden[r.ID] {
						continue
					}
					evaluated++
					logical[r.ID]++
					overlap = overlap || logical[r.ID] > 1
					if matchesDomain(r, in, line) {
						matches = append(matches, r)
						matchCount++
					}
				}
				if len(matches) > 1 || overlap {
					proposal.Source = SourceAmbiguous
					proposal.Explanation = "Reguli/versiuni aplicabile multiple; nu s-a ales arbitrar."
				}
				if len(matches) == 1 && !overlap {
					r := matches[0]
					v := r.Result
					proposal.TypedValue = &v
					proposal.ProposedValue = v.Text()
					proposal.RequiresReview = false
					proposal.Source = SourceRule
					proposal.Confidence = "Potrivire deterministă unică"
					proposal.Explanation = r.Explanation
					proposal.LegalBasis = r.LegalBasis
					proposal.Evidence = &accounting.Evidence{ModelVersion: accounting.ModelVersion, PackID: pack.ID, PackVersion: pack.Version, ProfileID: profile.ID, ProfileVersion: profile.Version, PolicyID: r.ClientPolicy, RuleVersionID: r.VersionID, SourceDocumentID: in.SourceFacts.SourceDocumentID, ParserVersion: in.SourceFacts.ParserVersion, SourceHash: in.SourceFacts.SourceHash, SourcePath: line.SourceFacts.Path, DateBasis: r.DateBasis, Date: in.IssueDate}
					// Actual immutable RuleVersion FK remains the execution reference.
					proposal.Rule = &RuleReference{RuleID: r.ID, RuleVersionID: r.VersionID, Reference: r.ID, Version: r.Version, Origin: rules.Scope(r.Scope), ProductionEligible: !pack.TestOnly, RulePackVersion: fmt.Sprintf("%s/%d", pack.ID, pack.Version), EffectiveFrom: r.EffectiveFrom, EffectiveTo: r.EffectiveTo}
				}
			}
			if p.Observer != nil {
				p.Observer.ClassificationEvaluated(proposal.Dimension, evaluated, matchCount, proposal.RequiresReview)
			}
			out.Proposals = append(out.Proposals, proposal)
		}
	}
	return out, nil
}
func ordinarySource(in InvoiceContext, l LineContext) bool {
	f := in.SourceFacts
	lf := l.SourceFacts
	return in.DocumentType == "INVOICE" && in.Currency == "RON" && f != nil && f.TypeCode == "380" && f.PrecedingInvoice == "" && f.SupplierVATID == in.SupplierID && f.SupplierCountry == "RO" && f.BuyerCountry == "RO" && f.CashAccounting != "YES" && (f.CashAccounting == "NO" || f.CashAccounting == "UNKNOWN") && f.TaxPointCode == "" && (f.TaxPointDate == "" || f.TaxPointDate == string(in.IssueDate)) && lf != nil && lf.Rate != nil && lf.Rate.Valid() && lf.Code == "S" && lf.Scheme == "VAT" && lf.ExemptionCode == "" && lf.ExemptionReason == "" && lf.Rate.Equal(l.VATRate)
}
func matchesDomain(r accounting.Rule, in InvoiceContext, l LineContext) bool {
	pred := r.Predicate
	if pred.SupplierID != in.SupplierID || pred.Currency != in.Currency || pred.TypeCode != in.SourceFacts.TypeCode || pred.Category != l.SourceFacts.Code || !pred.Rate.Equal(*l.SourceFacts.Rate) {
		return false
	}
	if pred.DescriptionContains != "" && !strings.Contains(normalize(l.Description), normalize(pred.DescriptionContains)) {
		return false
	}
	if pred.SellerItemID != "" && pred.SellerItemID != l.SourceFacts.SellerItemID {
		return false
	}
	if pred.ContractReference != "" && (in.Snapshot == nil || in.Snapshot.ContractID == "" || pred.ContractReference != in.Snapshot.ContractReference) {
		return false
	}
	return true
}

// ValidateRuleMatch rechecks structured guards at the serialization boundary.
func ValidateRuleMatch(r accounting.Rule, in InvoiceContext, l LineContext) bool {
	return r.Valid() && in.IssueDate.Within(r.EffectiveFrom, r.EffectiveTo) && ordinarySource(in, l) && matchesDomain(r, in, l)
}
