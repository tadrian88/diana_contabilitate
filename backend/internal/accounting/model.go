// Package accounting contains vendor-independent facts and decisions. It does
// not interpret descriptions using AI and does not contain SAGA import codes.
package accounting

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"diana-contabilitate/backend/internal/accountingdate"
	"diana-contabilitate/backend/internal/money"
)

const ModelVersion = "ACCOUNTING_DOMAIN_V2"
const LegacyVersion = "LEGACY_V1"
const IssueDateBasis = "INVOICE_ISSUE_DATE"

var Dimensions = []string{"ACCOUNT", "VAT_TREATMENT", "VAT_DEDUCTIBILITY", "EXPENSE_TAX_TREATMENT"}

// ClassificationResolution is the single domain vocabulary used by the
// classifier, AI workflow and completion checks. A reviewable proposal is not
// a final accounting decision, but it must not be sent to the provider again.
type ClassificationResolution string

const (
	ResolutionNeedsAI     ClassificationResolution = "NEEDS_AI"
	ResolutionNeedsReview ClassificationResolution = "NEEDS_REVIEW"
	ResolutionFinal       ClassificationResolution = "FINAL"
)

func ResolveClassification(dimension string, effective, proposed *Value, source, reviewStatus string) ClassificationResolution {
	if effective != nil && effective.Validate(dimension) == nil && (reviewStatus == "ACCEPTED" || reviewStatus == "CORRECTED") {
		return ResolutionFinal
	}
	if source == "AI_PROPOSAL" || source == "LEARNED_MAPPING" || reviewStatus == "REJECTED" {
		return ResolutionNeedsReview
	}
	if proposed != nil && proposed.Validate(dimension) == nil && source != "NO_MATCH" && source != "AMBIGUOUS" {
		return ResolutionNeedsReview
	}
	return ResolutionNeedsAI
}

type ClassificationResolutionItem struct {
	Dimension    string
	Effective    *Value
	Proposed     *Value
	Source       string
	ReviewStatus string
}

func IsAccountingClassificationComplete(items []ClassificationResolutionItem, expected int) bool {
	if expected <= 0 || len(items) != expected {
		return false
	}
	for _, item := range items {
		if ResolveClassification(item.Dimension, item.Effective, item.Proposed, item.Source, item.ReviewStatus) != ResolutionFinal {
			return false
		}
	}
	return true
}

type Origin string

const (
	Declared   Origin = "DECLARED"
	Calculated Origin = "CALCULATED"
	Unknown    Origin = "UNKNOWN"
)

type AmountFact struct {
	Amount   money.Amount `json:"amount"`
	Currency string       `json:"currency"`
	Origin   Origin       `json:"origin"`
}
type TaxCategory struct {
	Code            string        `json:"code"`
	Rate            *money.Amount `json:"rate,omitempty"`
	Scheme          string        `json:"scheme,omitempty"`
	ExemptionCode   string        `json:"exemptionCode,omitempty"`
	ExemptionReason string        `json:"exemptionReason,omitempty"`
}
type TaxSubtotal struct {
	TaxCategory
	Base AmountFact `json:"base"`
	VAT  AmountFact `json:"vat"`
}
type Adjustment struct {
	Charge   bool        `json:"charge"`
	Amount   AmountFact  `json:"amount"`
	Category TaxCategory `json:"category"`
	Reason   string      `json:"reason,omitempty"`
}
type SourceFacts struct {
	ParserVersion      string        `json:"parserVersion"`
	SourceDocumentID   string        `json:"sourceDocumentId,omitempty"`
	SourceHash         string        `json:"sourceHash,omitempty"`
	TypeCode           string        `json:"typeCode,omitempty"`
	SupplierVATID      string        `json:"supplierVatId,omitempty"`
	SupplierLegalID    string        `json:"supplierLegalId,omitempty"`
	BuyerVATID         string        `json:"buyerVatId,omitempty"`
	BuyerLegalID       string        `json:"buyerLegalId,omitempty"`
	SupplierCountry    string        `json:"supplierCountry,omitempty"`
	BuyerCountry       string        `json:"buyerCountry,omitempty"`
	TaxCurrency        string        `json:"taxCurrency,omitempty"`
	TaxPointDate       string        `json:"taxPointDate,omitempty"`
	PeriodStart        string        `json:"periodStart,omitempty"`
	PeriodEnd          string        `json:"periodEnd,omitempty"`
	TaxPointCode       string        `json:"taxPointCode,omitempty"`
	CashAccounting     string        `json:"cashAccounting"`
	PrecedingInvoice   string        `json:"precedingInvoice,omitempty"`
	BuyerReference     string        `json:"buyerReference,omitempty"`
	ContractReferences []string      `json:"contractReferences,omitempty"`
	Notes              []string      `json:"notes,omitempty"`
	VATTotals          []AmountFact  `json:"vatTotals,omitempty"`
	Subtotals          []TaxSubtotal `json:"subtotals,omitempty"`
	Adjustments        []Adjustment  `json:"adjustments,omitempty"`
	LineExtension      *AmountFact   `json:"lineExtension,omitempty"`
	TaxExclusive       *AmountFact   `json:"taxExclusive,omitempty"`
	TaxInclusive       *AmountFact   `json:"taxInclusive,omitempty"`
	Payable            *AmountFact   `json:"payable,omitempty"`
	Prepaid            *AmountFact   `json:"prepaid,omitempty"`
	Rounding           *AmountFact   `json:"rounding,omitempty"`
}
type LineFacts struct {
	NetAmount   *AmountFact `json:"netAmount,omitempty"`
	VATAmount   *AmountFact `json:"vatAmount,omitempty"`
	PriceAmount *AmountFact `json:"priceAmount,omitempty"`
	SourceID    string      `json:"sourceId"`
	Path        string      `json:"path"`
	TaxCategory
	VATOrigin       Origin        `json:"vatOrigin"`
	SellerItemID    string        `json:"sellerItemId,omitempty"`
	StandardItemID  string        `json:"standardItemId,omitempty"`
	ItemName        string        `json:"itemName,omitempty"`
	ItemDescription string        `json:"itemDescription,omitempty"`
	Note            string        `json:"note,omitempty"`
	PriceBase       *money.Amount `json:"priceBase,omitempty"`
	Adjustments     []Adjustment  `json:"adjustments,omitempty"`
}

// Value is a tagged union. Validate rejects unrelated fields and arbitrary text.
type Value struct {
	Kind           string        `json:"kind"`
	Account        string        `json:"account,omitempty"`
	Percentage     *money.Amount `json:"percentage,omitempty"`
	Basis          string        `json:"basis,omitempty"`
	Category       string        `json:"category,omitempty"`
	Reason         string        `json:"reason,omitempty"`
	Timing         string        `json:"timing,omitempty"`
	SourceCategory string        `json:"sourceCategory,omitempty"`
	SourceRate     *money.Amount `json:"sourceRate,omitempty"`
}

var accountCode = regexp.MustCompile(`^[0-9]{3,}(?:\.[0-9A-Za-z]+)*$`)

func (v Value) Validate(dimension string) error {
	invalid := func() error { return fmt.Errorf("valoare contabilă invalidă pentru %s", dimension) }
	if v.Percentage != nil {
		p, ok := newRat(*v.Percentage)
		if !ok || p.Sign() <= 0 || p.Cmp(rat("100")) >= 0 {
			return invalid()
		}
	}
	if dimension != "ACCOUNT" && v.Account != "" {
		return invalid()
	}
	if dimension != "VAT_TREATMENT" && (v.Timing != "" || v.SourceCategory != "" || v.SourceRate != nil) {
		return invalid()
	}
	if v.Kind != "LIMITED" && v.Percentage != nil {
		return invalid()
	}
	if dimension != "EXPENSE_TAX_TREATMENT" && v.Category != "" {
		return invalid()
	}
	switch dimension {
	case "ACCOUNT":
		if v.Kind != "ACCOUNT" || !accountCode.MatchString(v.Account) || v.Percentage != nil || v.Basis != "" || v.Category != "" || v.Reason != "" {
			return invalid()
		}
	case "VAT_TREATMENT":
		if (v.Kind != "ORDINARY" && v.Kind != "SPECIAL_UNSUPPORTED") || (v.Timing != "IMMEDIATE" && v.Timing != "DEFERRED" && v.Timing != "UNSUPPORTED") || v.Percentage != nil || v.Category != "" || strings.TrimSpace(v.SourceCategory) == "" || v.SourceRate == nil || !v.SourceRate.Valid() {
			return invalid()
		}
	case "VAT_DEDUCTIBILITY":
		if v.Kind != "FULL" && v.Kind != "NONE" && v.Kind != "LIMITED" && v.Kind != "NOT_APPLICABLE" {
			return invalid()
		}
	case "EXPENSE_TAX_TREATMENT":
		if v.Kind != "FULLY_DEDUCTIBLE" && v.Kind != "NONDEDUCTIBLE" && v.Kind != "LIMITED" && v.Kind != "PERIOD_LIMIT_CATEGORY" && v.Kind != "NOT_APPLICABLE" {
			return invalid()
		}
		if v.Kind == "PERIOD_LIMIT_CATEGORY" && (strings.TrimSpace(v.Category) == "" || strings.TrimSpace(v.Basis) == "") {
			return invalid()
		}
		if v.Kind != "PERIOD_LIMIT_CATEGORY" && v.Category != "" {
			return invalid()
		}
	default:
		return invalid()
	}
	if v.Kind == "LIMITED" && (v.Percentage == nil || strings.TrimSpace(v.Basis) == "") {
		return invalid()
	}
	if (v.Kind == "NOT_APPLICABLE" || v.Kind == "NONE" || v.Kind == "NONDEDUCTIBLE" || v.Kind == "SPECIAL_UNSUPPORTED") && strings.TrimSpace(v.Reason) == "" {
		return invalid()
	}
	return nil
}
func (v Value) Text() string {
	if v.Kind == "ACCOUNT" {
		return v.Account
	}
	return v.Kind
}
func DecodeValue(raw []byte, dimension string) (*Value, error) {
	var v Value
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&v); err != nil {
		return nil, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("trailing JSON value")
	}
	if err := v.Validate(dimension); err != nil {
		return nil, err
	}
	return &v, nil
}

type Approval struct {
	Actor    string    `json:"actor"`
	At       time.Time `json:"at"`
	Evidence []string  `json:"evidence"`
}

func (a Approval) Valid() bool {
	return strings.TrimSpace(a.Actor) != "" && !a.At.IsZero() && len(a.Evidence) > 0 && allNonempty(a.Evidence)
}
func allNonempty(values []string) bool {
	for _, v := range values {
		if strings.TrimSpace(v) == "" {
			return false
		}
	}
	return true
}

type Profile struct {
	TestOnly            bool                 `json:"testOnly"`
	ID                  string               `json:"id"`
	ClientID            string               `json:"clientId"`
	Version             int                  `json:"version"`
	SupersedesProfileID string               `json:"supersedesProfileId,omitempty"`
	EffectiveFrom       accountingdate.Date  `json:"effectiveFrom"`
	EffectiveTo         *accountingdate.Date `json:"effectiveTo,omitempty"`
	Framework           string               `json:"framework"`
	AccountCodes        []string             `json:"accountCodes,omitempty"`
	ChartPolicy         string               `json:"chartPolicy"`
	TaxRegime           string               `json:"taxRegime"`
	VATRegistration     string               `json:"vatRegistration"`
	DeductionActivity   string               `json:"deductionActivity"`
	CashAccounting      string               `json:"cashAccounting"`
	ProRata             string               `json:"proRata"`
	Approval            Approval             `json:"approval"`
	ConfigurationIssues []ProfileIssue       `json:"configurationIssues,omitempty"`
}

type ProfileIssue struct {
	Code        string `json:"code"`
	AccountCode string `json:"accountCode"`
	Message     string `json:"message"`
}

// ApplicableProfiles applies immutable profile succession before selecting by
// date. A successor never mutates its predecessor; it only removes it from the
// set used by future runs.
func ApplicableProfiles(profiles []*Profile, client string, date accountingdate.Date) []*Profile {
	superseded := map[string]bool{}
	for _, profile := range profiles {
		if profile != nil && profile.SupersedesProfileID != "" && profile.Valid(client, date) {
			superseded[profile.SupersedesProfileID] = true
		}
	}
	result := []*Profile{}
	for _, profile := range profiles {
		if profile != nil && !superseded[profile.ID] && profile.Valid(client, date) {
			result = append(result, profile)
		}
	}
	return result
}

func (p *Profile) Valid(client string, date accountingdate.Date) bool {
	return p != nil && p.ID != "" && p.ClientID == client && p.Version > 0 && p.EffectiveFrom.Valid() && date.Within(p.EffectiveFrom, p.EffectiveTo) && p.Approval.Valid() && p.validAccounts() && oneOf(p.Framework, "OMFP_1802_2014", "OTHER", "UNKNOWN") && oneOf(p.TaxRegime, "PROFIT_TAX", "MICROENTERPRISE", "OTHER", "UNKNOWN") && oneOf(p.VATRegistration, "ORDINARY_REGISTERED", "NOT_REGISTERED", "SPECIAL_REGISTERED", "UNKNOWN") && oneOf(p.DeductionActivity, "WITH_DEDUCTION_RIGHT", "MIXED", "WITHOUT_DEDUCTION_RIGHT", "UNKNOWN") && oneOf(p.CashAccounting, "YES", "NO", "UNKNOWN") && oneOf(p.ProRata, "YES", "NO", "UNKNOWN")
}
func oneOf(v string, allowed ...string) bool {
	for _, a := range allowed {
		if a == v {
			return true
		}
	}
	return false
}
func (p *Profile) Ordinary() bool {
	return p != nil && p.Framework == "OMFP_1802_2014" && p.ChartPolicy != "" && len(p.AccountCodes) > 0 && p.validAccounts() && p.TaxRegime == "PROFIT_TAX" && p.VATRegistration == "ORDINARY_REGISTERED" && p.DeductionActivity == "WITH_DEDUCTION_RIGHT" && p.CashAccounting == "NO" && p.ProRata == "NO"
}

// ProfileDerivedExpenseTaxValue is the only decision Diana derives from the
// approved profile alone: microenterprises are outside profit tax, so the
// expense profit-tax treatment is not applicable.
func (p *Profile) ProfileDerivedExpenseTaxValue() (Value, bool) {
	if p == nil || p.TaxRegime != "MICROENTERPRISE" {
		return Value{}, false
	}
	return Value{Kind: "NOT_APPLICABLE", Reason: "Profilul aprobat indică regimul microîntreprinderii pentru această dată."}, true
}

// SAGAExportSupported reports whether the code-owned SAGA mapping can represent
// this profile faithfully. Classification works for every valid profile; only
// the export is limited, with an explicit reason for unsupported variants.
func (p *Profile) SAGAExportSupported() (bool, string) {
	switch {
	case p == nil:
		return false, "Lipsește profilul fiscal aprobat."
	case p.Framework != "OMFP_1802_2014":
		return false, "Exportul SAGA este disponibil doar pentru clienții care aplică OMFP 1802/2014."
	case p.TaxRegime != "PROFIT_TAX" && p.TaxRegime != "MICROENTERPRISE":
		return false, "Exportul SAGA este disponibil doar pentru clienții plătitori de impozit pe profit sau microîntreprinderi."
	case p.VATRegistration != "ORDINARY_REGISTERED":
		return false, "Exportul SAGA pentru clienții neînregistrați în scopuri de TVA sau cu regim special de TVA nu este încă suportat; clasificarea rămâne disponibilă."
	case p.DeductionActivity != "WITH_DEDUCTION_RIGHT":
		return false, "Exportul SAGA pentru activități fără drept de deducere sau mixte nu este încă suportat; clasificarea rămâne disponibilă."
	case p.CashAccounting != "NO":
		return false, "Exportul SAGA pentru clienții cu TVA la încasare nu este încă suportat; clasificarea rămâne disponibilă."
	case p.ProRata != "NO":
		return false, "Exportul SAGA pentru clienții cu pro-rata nu este încă suportat; clasificarea rămâne disponibilă."
	case !p.validAccounts():
		return false, "Lista de conturi a profilului este invalidă."
	}
	return true, ""
}

// DefaultSAGAMapping is the code-owned SAGA mapping used when no per-client
// release pack exists: ordinary/immediate VAT with full deduction omits
// TipDeducere. Its approval is the reviewed exporter code itself.
var DefaultSAGAMapping = MappingPolicy{
	Version:              "SAGA_C_DOMAIN_V2_ORDINARY_V1",
	Approved:             true,
	OrdinaryFullOmission: true,
	Approval:             Approval{Actor: "Diana SAGA exporter", At: time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC), Evidence: []string{"Docs/DECISIONS.md D-108"}},
}

type Predicate struct {
	SupplierVATRegistration string       `json:"supplierVatRegistration"`
	SupplierCashAccounting  string       `json:"supplierCashAccounting"`
	Version                 string       `json:"version"`
	SupplierID              string       `json:"supplierId"`
	DescriptionContains     string       `json:"descriptionContains,omitempty"`
	SellerItemID            string       `json:"sellerItemId,omitempty"`
	ContractReference       string       `json:"contractReference,omitempty"`
	Category                string       `json:"category"`
	Rate                    money.Amount `json:"rate"`
	Currency                string       `json:"currency"`
	TypeCode                string       `json:"typeCode"`
}
type Rule struct {
	ID                string               `json:"id"`
	VersionID         string               `json:"versionId"`
	Version           int                  `json:"version"`
	Dimension         string               `json:"dimension"`
	Scope             string               `json:"scope"`
	ParentID          string               `json:"parentId,omitempty"`
	ClientPolicy      string               `json:"clientPolicy"`
	AcquisitionPolicy string               `json:"acquisitionPolicy"`
	DateBasis         string               `json:"dateBasis"`
	EffectiveFrom     accountingdate.Date  `json:"effectiveFrom"`
	EffectiveTo       *accountingdate.Date `json:"effectiveTo,omitempty"`
	Predicate         Predicate            `json:"predicate"`
	Result            Value                `json:"result"`
	Explanation       string               `json:"explanation"`
	LegalBasis        string               `json:"legalBasis"`
	Evidence          []string             `json:"evidence"`
}

func (r Rule) Valid() bool {
	return r.ID != "" && r.VersionID != "" && r.Version > 0 && r.Result.Validate(r.Dimension) == nil && oneOf(r.Scope, "GLOBAL", "CLIENT_OVERRIDE") && (r.Scope != "CLIENT_OVERRIDE" || (r.ClientPolicy != "" && r.ParentID != "")) && (r.Scope != "GLOBAL" || r.ParentID == "") && r.AcquisitionPolicy != "" && r.DateBasis == IssueDateBasis && r.EffectiveFrom.Valid() && (r.EffectiveTo == nil || (r.EffectiveTo.Valid() && *r.EffectiveTo >= r.EffectiveFrom)) && r.Predicate.Version == "ORDINARY_INCOMING_V1" && r.Predicate.SupplierID != "" && r.Predicate.SupplierCashAccounting == "NO" && r.Predicate.SupplierVATRegistration == "ORDINARY_REGISTERED" && r.Predicate.Category == "S" && r.Predicate.Rate.Valid() && rat(r.Predicate.Rate.String()).Sign() > 0 && r.Predicate.Currency == "RON" && r.Predicate.TypeCode == "380" && (strings.TrimSpace(r.Predicate.DescriptionContains) != "" || r.Predicate.SellerItemID != "" || r.Predicate.ContractReference != "") && r.Explanation != "" && r.LegalBasis != "" && len(r.Evidence) > 0 && allNonempty(r.Evidence)
}

type MappingPolicy struct {
	Version              string   `json:"version"`
	Approved             bool     `json:"approved"`
	TestOnly             bool     `json:"testOnly"`
	OrdinaryFullOmission bool     `json:"ordinaryFullOmission"`
	Approval             Approval `json:"approval"`
}
type Pack struct {
	ID            string               `json:"id"`
	Version       int                  `json:"version"`
	ClientID      string               `json:"clientId"`
	ProfileID     string               `json:"profileId"`
	TestOnly      bool                 `json:"testOnly"`
	Approval      Approval             `json:"approval"`
	EffectiveFrom accountingdate.Date  `json:"effectiveFrom"`
	EffectiveTo   *accountingdate.Date `json:"effectiveTo,omitempty"`
	Rules         []Rule               `json:"rules"`
	Mapping       MappingPolicy        `json:"mapping"`
}

func (p *Pack) Valid(profile *Profile, client string, date accountingdate.Date, allowTest bool) bool {
	if p == nil || profile == nil || p.ID == "" || p.Version <= 0 || p.ClientID != client || p.ProfileID != profile.ID || !p.Approval.Valid() || !p.EffectiveFrom.Valid() || !date.Within(p.EffectiveFrom, p.EffectiveTo) || p.EffectiveFrom < profile.EffectiveFrom || profile.EffectiveTo != nil && (p.EffectiveTo == nil || *p.EffectiveTo > *profile.EffectiveTo) || p.TestOnly && !allowTest || p.Mapping.TestOnly != p.TestOnly || profile.TestOnly != p.TestOnly {
		return false
	}
	seen := map[string]bool{}
	for _, r := range p.Rules {
		if !r.Valid() || seen[r.VersionID] || r.ClientPolicy != profile.ChartPolicy || r.Dimension == "ACCOUNT" && !profile.RuleAccountAllowed(r.Result.Account) || r.EffectiveFrom < p.EffectiveFrom || (p.EffectiveTo != nil && (r.EffectiveTo == nil || *r.EffectiveTo > *p.EffectiveTo)) {
			return false
		}
		seen[r.VersionID] = true
	}
	return true
}

type Evidence struct {
	ModelVersion     string              `json:"modelVersion"`
	PackID           string              `json:"packId"`
	PackVersion      int                 `json:"packVersion"`
	ProfileID        string              `json:"profileId"`
	ProfileVersion   int                 `json:"profileVersion"`
	PolicyID         string              `json:"policyId"`
	RuleVersionID    string              `json:"ruleVersionId"`
	SourceDocumentID string              `json:"sourceDocumentId"`
	ParserVersion    string              `json:"parserVersion"`
	SourceHash       string              `json:"sourceHash"`
	SourcePath       string              `json:"sourcePath"`
	DateBasis        string              `json:"dateBasis"`
	Date             accountingdate.Date `json:"date"`
}

// LegalCitation is verified application evidence attached to one
// InvoiceLine × accounting dimension proposal. Provider output never controls
// Verified; the deterministic corpus validator sets it before persistence.
type LegalCitation struct {
	FragmentID  string `json:"fragmentId"`
	VersionID   string `json:"versionId"`
	CitationKey string `json:"citationKey"`
	ContentHash string `json:"contentHash"`
	Verified    bool   `json:"verified"`
}

// ValidationResult preserves granular trust-boundary results on the canonical
// classification. Invalid AI output remains auditable and reviewable.
type ValidationResult struct {
	Code              string   `json:"code"`
	Message           string   `json:"message"`
	SuggestedAccounts []string `json:"suggestedAccounts,omitempty"`
}

type ProposalProvenance struct {
	AnalysisRunID string `json:"analysisRunId"`
	Provider      string `json:"provider"`
	Model         string `json:"model"`
	SchemaVersion string `json:"schemaVersion"`
	PromptVersion string `json:"promptVersion"`
	// Knowledge fields are populated only when an exact, explicitly promoted
	// accountant decision produced the proposal.  Keeping them on the immutable
	// proposal provenance preserves the original AI/rule provenance on the
	// source classification while linking every later use to the promoted item.
	KnowledgeID      string `json:"knowledgeId,omitempty"`
	KnowledgeVersion int    `json:"knowledgeVersion,omitempty"`
	SourceInvoiceID  string `json:"sourceInvoiceId,omitempty"`
	SourceLineID     string `json:"sourceLineId,omitempty"`
	SourceDecisionID string `json:"sourceDecisionId,omitempty"`
	PromotedBy       string `json:"promotedBy,omitempty"`
	PromotedAt       string `json:"promotedAt,omitempty"`
}
type Snapshot struct {
	Profile           *Profile          `json:"profile,omitempty"`
	Pack              *Pack             `json:"pack,omitempty"`
	ContractReference string            `json:"contractReference,omitempty"`
	ContractID        string            `json:"contractId,omitempty"`
	ContractRevision  uint64            `json:"contractRevision,omitempty"`
	TestOnly          bool              `json:"testOnly"`
	AccountCatalog    []AccountSnapshot `json:"accountCatalog,omitempty"`
	// AccountCatalogFingerprint identifies the global postable catalog used
	// when the profile has no explicit account list.
	AccountCatalogFingerprint string `json:"accountCatalogFingerprint,omitempty"`
}

type AccountSnapshot struct {
	Code       string `json:"code"`
	ParentCode string `json:"parentCode,omitempty"`
	Active     bool   `json:"active"`
	Postable   bool   `json:"postable"`
}

func (s *Snapshot) Fingerprint() string {
	raw, _ := json.Marshal(s)
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func (p *Profile) validAccounts() bool {
	seen := map[string]bool{}
	for _, code := range p.AccountCodes {
		if !accountCode.MatchString(code) || seen[code] {
			return false
		}
		seen[code] = true
	}
	return true
}

// RuleAccountAllowed is stricter than AccountAllowed: deterministic rule
// automation never infers an account vocabulary, so it requires an explicit
// profile list (D-110 applies only to human/AI-reviewed decisions).
func (p *Profile) RuleAccountAllowed(code string) bool {
	return p != nil && len(p.AccountCodes) > 0 && p.AccountAllowed(code)
}

// AccountAllowed applies the profile's optional account vocabulary. An empty
// list means the whole global OMFP catalog is allowed; callers still require
// the account to be active and postable in that catalog.
func (p *Profile) AccountAllowed(code string) bool {
	if p == nil {
		return false
	}
	if len(p.AccountCodes) == 0 {
		return true
	}
	for _, approved := range p.AccountCodes {
		if approved == code {
			return true
		}
	}
	return false
}
