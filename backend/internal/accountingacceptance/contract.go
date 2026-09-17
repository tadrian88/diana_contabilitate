// Package accountingacceptance defines offline accountant input and golden
// acceptance evidence. It is deliberately not imported by the runtime engine.
package accountingacceptance

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"diana-contabilitate/backend/internal/accounting"
	"diana-contabilitate/backend/internal/accountingdate"
)

const FormatVersion = "PRODUCTION_RULE_PACK_INPUT_V1"

// Input is not an executable Pack. Existing domain types are reused, and exact
// approved rule versions are referenced only after an operator prepares them.
type Input struct {
	FormatVersion string                    `json:"formatVersion"`
	Profile       *accounting.Profile       `json:"profile"`
	Mapping       *accounting.MappingPolicy `json:"mapping"`
	Families      []Family                  `json:"families"`
	Examples      []Example                 `json:"examples"`
}

type Family struct {
	ID            string               `json:"familyId"`
	ClientID      string               `json:"clientId"`
	Name          string               `json:"name"`
	EffectiveFrom accountingdate.Date  `json:"effectiveFrom"`
	EffectiveTo   *accountingdate.Date `json:"effectiveTo,omitempty"`
	Scope         string               `json:"scope"`
	Predicate     accounting.Predicate `json:"predicate"`
	// Every predicate field needs an accounting relevance or safety guard reason.
	PredicateReasons  map[string]string   `json:"predicateReasons"`
	RequiredFacts     []string            `json:"requiredFacts"`
	ClientPolicy      string              `json:"clientPolicy"`
	AcquisitionPolicy string              `json:"acquisitionPolicy"`
	Decisions         map[string]Decision `json:"decisions"`
	Exceptions        []string            `json:"exceptions"`
	RuleVersionIDs    []string            `json:"ruleVersionIds"`
	Approval          accounting.Approval `json:"approval"`
}

type Decision struct {
	Value               *accounting.Value `json:"value"`
	PolicyReference     string            `json:"policyReference"`
	Basis               string            `json:"basis"`
	RequiredClientFacts []string          `json:"requiredClientFacts"`
}

// Example outcomes are acceptance assertions; they are never runtime inputs.
type Example struct {
	ID                           string              `json:"exampleId"`
	Kind                         string              `json:"kind"` // POSITIVE or NEGATIVE_BOUNDARY
	Fixture                      string              `json:"sourceFixture"`
	FixtureSHA256                string              `json:"fixtureSha256"` // exact sanitized file bytes
	OriginalSourceReference      string              `json:"originalSourceReference"`
	ClientID                     string              `json:"clientId"`
	ProfileID                    string              `json:"profileId"`
	ProfileVersion               int                 `json:"profileVersion"`
	Lines                        []ExpectedLine      `json:"lines"`
	ExpectedReadiness            string              `json:"expectedReadiness"` // READY_FOR_SAGA or AWAITING_REVIEW
	ExpectedMappingCompatibility bool                `json:"expectedSagaMappingCompatibility"`
	MappingVersion               string              `json:"mappingVersion"`
	BoundaryReason               string              `json:"boundaryReason,omitempty"`
	Approval                     accounting.Approval `json:"approval"`
}

type ExpectedLine struct {
	SourceLineID string `json:"sourceLineId"`
	FamilyID     string `json:"familyId,omitempty"` // absent for unsupported lines
	// A null value asserts that this dimension must remain unresolved.
	Decisions map[string]*accounting.Value `json:"decisions"`
}

// MissingInput assists discovery only. An empty result does NOT certify approval
// authenticity, legal applicability, target-SAGA import or release eligibility.
func (in Input) MissingInput() []string {
	var missing []string
	if in.FormatVersion != FormatVersion {
		missing = append(missing, "formatVersion")
	}
	if in.Profile == nil {
		missing = append(missing, "approved dated pilot profile")
	} else {
		if !in.Profile.Valid(in.Profile.ClientID, in.Profile.EffectiveFrom) || in.Profile.TestOnly {
			missing = append(missing, "non-test approved pilot profile with valid period")
		}
		for field, value := range map[string]string{"framework": in.Profile.Framework, "taxRegime": in.Profile.TaxRegime, "vatRegistration": in.Profile.VATRegistration, "deductionActivity": in.Profile.DeductionActivity, "cashAccounting": in.Profile.CashAccounting, "proRata": in.Profile.ProRata, "chartPolicy": in.Profile.ChartPolicy} {
			if value == "UNKNOWN" || strings.TrimSpace(value) == "" {
				missing = append(missing, "profile."+field)
			}
		}
		if len(in.Profile.AccountCodes) == 0 {
			missing = append(missing, "profile.accountCodes")
		}
	}
	if in.Mapping == nil || in.Mapping.TestOnly || !in.Mapping.Approved || !in.Mapping.Approval.Valid() || in.Mapping.Version == "" {
		missing = append(missing, "approved mapping version and real target-SAGA evidence")
	}
	if len(in.Families) == 0 {
		missing = append(missing, "at least one explicitly approved real family")
	}
	seen := map[string]bool{}
	for n, family := range in.Families {
		prefix := fmt.Sprintf("families[%d]", n)
		if family.ID == "" || seen[family.ID] {
			missing = append(missing, prefix+".uniqueFamilyId")
		}
		seen[family.ID] = true
		if family.ClientID == "" || in.Profile == nil || family.ClientID != in.Profile.ClientID {
			missing = append(missing, prefix+".pilotClientId")
		}
		if family.Name == "" || family.Scope == "" || len(family.RequiredFacts) == 0 {
			missing = append(missing, prefix+".scope/name/requiredFacts")
		}
		if !family.EffectiveFrom.Within(family.EffectiveFrom, family.EffectiveTo) {
			missing = append(missing, prefix+".effectivePeriod")
		}
		if family.ClientPolicy == "" || family.AcquisitionPolicy == "" {
			missing = append(missing, prefix+".client/acquisitionPolicy")
		}
		if !family.Approval.Valid() {
			missing = append(missing, prefix+".explicitAccountantApproval")
		}
		for _, dimension := range accounting.Dimensions {
			d := family.Decisions[dimension]
			if d.Value == nil || d.Value.Validate(dimension) != nil || d.PolicyReference == "" || d.Basis == "" {
				missing = append(missing, prefix+"."+dimension+" explicit result and basis")
			}
		}
		positive, negative := 0, 0
		for _, example := range in.Examples {
			for _, line := range example.Lines {
				if line.FamilyID == family.ID && example.Approval.Valid() {
					if example.Kind == "POSITIVE" {
						positive++
					}
					if example.Kind == "NEGATIVE_BOUNDARY" {
						negative++
					}
					break
				}
			}
		}
		if positive == 0 {
			missing = append(missing, prefix+".approvedPositiveRealInvoice")
		}
		if negative == 0 {
			missing = append(missing, prefix+".approvedNegativeBoundary")
		}
	}

	for n, example := range in.Examples {
		prefix := fmt.Sprintf("examples[%d]", n)
		if example.ID == "" || example.Fixture == "" || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(example.FixtureSHA256) || example.OriginalSourceReference == "" {
			missing = append(missing, prefix+".fixture/hash/realSourceLineage")
		}
		if example.ClientID == "" || example.ProfileID == "" || example.ProfileVersion <= 0 || example.MappingVersion == "" || !example.Approval.Valid() {
			missing = append(missing, prefix+".profile/mapping/approval")
		}
		if example.Kind != "POSITIVE" && example.Kind != "NEGATIVE_BOUNDARY" || example.Kind == "NEGATIVE_BOUNDARY" && example.BoundaryReason == "" {
			missing = append(missing, prefix+".kind/boundaryReason")
		}
		if example.ExpectedReadiness != "READY_FOR_SAGA" && example.ExpectedReadiness != "AWAITING_REVIEW" || len(example.Lines) == 0 {
			missing = append(missing, prefix+".expectedReadiness/lines")
		}
		for _, line := range example.Lines {
			if line.SourceLineID == "" {
				missing = append(missing, prefix+".sourceLineId")
			}
			for _, dimension := range accounting.Dimensions {
				value, exists := line.Decisions[dimension]
				if !exists || value != nil && value.Validate(dimension) != nil {
					missing = append(missing, prefix+"."+dimension+" expectation")
				}
			}
		}
	}
	sort.Strings(missing)
	return missing
}
