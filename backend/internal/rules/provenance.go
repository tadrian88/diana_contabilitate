package rules

import (
	"diana-contabilitate/backend/internal/accountingdate"
	"net/url"
	"regexp"
	"strings"
)

// Provenance is immutable reviewed evidence, not a live legal lookup or a permission.
type Provenance struct {
	SourceType      string               `json:"sourceType"`
	SourceTitle     string               `json:"sourceTitle"`
	Issuer          string               `json:"issuer"`
	LegalInstrument string               `json:"legalInstrument"`
	Reference       string               `json:"reference"`
	SourceURL       string               `json:"sourceURL"`
	EffectiveFrom   accountingdate.Date  `json:"effectiveFrom"`
	EffectiveTo     *accountingdate.Date `json:"effectiveTo,omitempty"`
	VerifiedAt      accountingdate.Date  `json:"verifiedAt"`
	VerifiedBy      string               `json:"verifiedBy"`
	Notes           string               `json:"notes"`
	// For ACCOUNT, adoption of this limited chart and exact client policy must be explicit.
	AccountingRegime      string `json:"accountingRegime,omitempty"`
	ClientPolicyReference string `json:"clientPolicyReference,omitempty"`
}

func (p *Provenance) Valid() bool {
	if p == nil || !p.EffectiveFrom.Valid() || !p.VerifiedAt.Valid() || (p.EffectiveTo != nil && (!p.EffectiveTo.Valid() || *p.EffectiveTo < p.EffectiveFrom)) {
		return false
	}
	switch p.SourceType {
	case "LEGISLATION", "ACCOUNTING_REGULATION", "CLIENT_ACCOUNTING_POLICY":
	default:
		return false
	}
	for _, value := range []string{p.SourceTitle, p.Issuer, p.LegalInstrument, p.Reference, p.VerifiedBy, p.Notes} {
		if strings.TrimSpace(value) == "" || strings.Contains(value, LegalBasisPlaceholder) {
			return false
		}
	}
	u, err := url.Parse(p.SourceURL)
	return err == nil && u.Scheme == "https" && u.User == nil && (u.Host == "legislatie.just.ro" || u.Host == "static.anaf.ro" || u.Host == "www.anaf.ro" || u.Host == "mfinante.gov.ro" || u.Host == "www.mfinante.gov.ro")
}

const ProductionRulePackVersion = "RO_INCOMING_ACCOUNTING_V1_REVIEW_ONLY"

// No production mappings are seeded: missing client policy, VAT evidence and
// unresolved deductibility semantics are deliberate accounting gates.
func ProductionPack() []Rule { return []Rule{} }

// AccountVocabularyVersion identifies the catalogue-backed vocabulary. This
// lexical guard is deliberately not the catalogue or a client authorization;
// approved profiles and the active account catalogue provide those controls.
const AccountVocabularyVersion = "OMFP_1802_2014_CATALOG_V2"

var productionAccountCode = regexp.MustCompile(`^[0-9]{3,4}(?:\.[0-9A-Za-z]+)*$`)

func ValidProductionAccount(value string) bool {
	return productionAccountCode.MatchString(value)
}
