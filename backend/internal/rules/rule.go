package rules

import "time"

const LegalBasisPlaceholder = "Exemplu demonstrativ — bază legală nevalidată"

type Category string

const (
	CategoryAccount       Category = "ACCOUNT"
	CategoryVAT           Category = "VAT"
	CategoryDeductibility Category = "DEDUCTIBILITY"
)

var Categories = []Category{CategoryAccount, CategoryVAT, CategoryDeductibility, "VAT_TREATMENT", "VAT_DEDUCTIBILITY", "EXPENSE_TAX_TREATMENT"}

type Scope string

const (
	ScopeGlobal         Scope = "GLOBAL"
	ScopeClientOverride Scope = "CLIENT_OVERRIDE"
)

type MatchKind string

const (
	MatchDescriptionContains MatchKind = "DESCRIPTION_CONTAINS"
	MatchAlways              MatchKind = "ALWAYS"
	MatchNoAutomation        MatchKind = "NO_AUTOMATION"
	MatchVATSourceRateEquals MatchKind = "VAT_SOURCE_RATE_EQUALS"
)

type Rule struct {
	ID           string
	Reference    string
	Name         string
	Category     Category
	Scope        Scope
	ClientID     *string
	ParentRuleID *string
	Revision     uint64
	Versions     []Version
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type Version struct {
	ProductionEligible bool
	RulePackVersion    string
	Provenance         *Provenance
	ID                 string
	RuleID             string
	Version            int
	Criteria           string
	Result             string
	Explanation        string
	LegalBasis         string
	MatchKind          MatchKind
	MatchValue         *string
	EffectiveFrom      time.Time
	EffectiveTo        *time.Time
	CreatedByID        *string
	CreatedByDisplay   string
	CreatedAt          time.Time
}

type Filter struct{ ClientID string }

func ValidCategory(value Category) bool {
	for _, category := range Categories {
		if value == category {
			return true
		}
	}
	return false
}
