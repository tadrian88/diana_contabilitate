package saga

import (
	"encoding/json"
	"fmt"

	"diana-contabilitate/backend/internal/accounting"
	"diana-contabilitate/backend/internal/accountingdate"
	"diana-contabilitate/backend/internal/classification"
	"diana-contabilitate/backend/internal/invoicing"
)

// ShadowResult is acceptance evidence only. It contains no artifact or command.
// Family attribution is supplied by the input manifest's exact RuleVersion IDs.
type ShadowResult struct {
	Proposals      []classification.Proposal
	Readiness      Readiness
	MappingVersion string
	// Compatibility is withheld until all source/decision/readiness guards pass.
	MappingCompatibility string
}

// EvaluateShadow evaluates an accountant-reviewed candidate without installing
// it. Missing approval remains a blocker; this function never fabricates it.
// versions must come from reviewed immutable RuleVersions, not golden outcomes.
// There is no store, observer, task resolver, exporter or pipeline dependency.
func EvaluateShadow(item *invoicing.Invoice, client ClientIdentity, candidate *accounting.Snapshot, versions []classification.RuleCandidate) (ShadowResult, error) {
	return evaluateShadow(item, client, candidate, versions, false)
}

func evaluateShadow(item *invoicing.Invoice, client ClientIdentity, candidate *accounting.Snapshot, versions []classification.RuleCandidate, allowTest bool) (ShadowResult, error) {
	if item == nil || item.ModelVersion != accounting.ModelVersion {
		return ShadowResult{}, fmt.Errorf("shadow evaluation requires an accounting-domain-v2 invoice")
	}
	// Isolate every nested fact/value/evidence pointer from both caller and result.
	raw, err := json.Marshal(struct {
		Invoice  *invoicing.Invoice
		Snapshot *accounting.Snapshot
		Versions []classification.RuleCandidate
	}{item, candidate, versions})
	if err != nil {
		return ShadowResult{}, err
	}
	var copied struct {
		Invoice  *invoicing.Invoice
		Snapshot *accounting.Snapshot
		Versions []classification.RuleCandidate
	}
	if err := json.Unmarshal(raw, &copied); err != nil {
		return ShadowResult{}, err
	}
	preview := copied.Invoice
	preview.AccountingSnapshot = copied.Snapshot
	in := classification.InvoiceContext{ModelVersion: preview.ModelVersion, ClientID: preview.ClientID, IssueDate: accountingdate.FromTime(preview.IssueDate), DocumentType: string(preview.DocumentType), Currency: preview.Total.Currency, SourceFacts: preview.SourceFacts, Snapshot: copied.Snapshot}
	if preview.SupplierCUI != nil {
		in.SupplierID = *preview.SupplierCUI
	}
	for _, line := range preview.Lines {
		in.Lines = append(in.Lines, classification.LineContext{ID: line.ID, Position: line.Position, Description: line.Description, VATRate: line.VATRate, VATValue: line.VATValue, SourceFacts: line.SourceFacts})
	}
	result, err := (classification.DomainPolicy{AllowTestOnly: allowTest}).Evaluate(in)
	if err != nil {
		return ShadowResult{}, err
	}
	preview.AccountingSnapshot = result.Snapshot
	for n := range preview.Lines {
		preview.Lines[n].Classifications = nil
	}
	for n := range result.Proposals {
		p := &result.Proposals[n]
		if p.Rule != nil {
			// A unique exact reference is required; duplicate input cannot win.
			var matches []classification.RuleCandidate
			for _, v := range copied.Versions {
				if v.RuleVersionID == p.Rule.RuleVersionID && v.RuleID == p.Rule.RuleID && v.Version == p.Rule.Version && string(v.Category) == string(p.Dimension) && v.Scope == p.Rule.Origin && v.RulePackVersion == p.Rule.RulePackVersion && v.EffectiveFrom == p.Rule.EffectiveFrom && sameDateEnd(v.EffectiveTo, p.Rule.EffectiveTo) {
					matches = append(matches, v)
				}
			}
			if len(matches) == 1 {
				p.Rule.Provenance = matches[0].Provenance
				p.Rule.ProductionEligible = matches[0].ProductionEligible
			} else {
				p.Rule.ProductionEligible = false
			}
		}
		status := classification.ReviewPending
		var effective *string
		if !p.RequiresReview {
			status = classification.ReviewAccepted
			value := p.ProposedValue
			effective = &value
		}
		for n := range preview.Lines {
			if preview.Lines[n].ID == p.InvoiceLineID {
				preview.Lines[n].Classifications = append(preview.Lines[n].Classifications, classification.Decision{ModelVersion: accounting.ModelVersion, InvoiceLineID: p.InvoiceLineID, Dimension: p.Dimension, TypedValue: p.TypedValue, Evidence: p.Evidence, EffectiveValue: effective, Status: status, Source: p.Source, Rule: p.Rule, PolicyVersion: result.PolicyVersion})
			}
		}
	}
	ready := EvaluateReadiness(preview, client, allowTest, true)
	out := ShadowResult{Proposals: result.Proposals, Readiness: ready, MappingCompatibility: "BLOCKED_OR_UNRESOLVED"}
	if result.Snapshot != nil && result.Snapshot.Pack != nil {
		out.MappingVersion = result.Snapshot.Pack.Mapping.Version
	}
	if ready.Ready {
		out.MappingCompatibility = "COMPATIBLE"
	}
	return out, nil
}

func sameDateEnd(a, b *accountingdate.Date) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}
