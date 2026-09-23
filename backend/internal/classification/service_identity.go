package classification

import (
	"strings"
	"unicode"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

const NormalizedDescriptionV1 = "NORMALIZED_DESCRIPTION_V1"
const ExactIdentifierV1 = "EXACT_IDENTIFIER_V1"

type ServiceIdentityKind string

const (
	IdentitySellerItemID          ServiceIdentityKind = "SELLER_ITEM_ID"
	IdentityStandardItemID        ServiceIdentityKind = "STANDARD_ITEM_ID"
	IdentityNormalizedDescription ServiceIdentityKind = "NORMALIZED_DESCRIPTION"
)

type ServiceIdentity struct {
	Kind              ServiceIdentityKind
	Value             string
	NormalizerVersion string
}

// NormalizeDescriptionV1 deliberately preserves every letter and number. It
// only normalizes Unicode/case and treats punctuation as harmless separators.
func NormalizeDescriptionV1(value string) string {
	value = cases.Fold().String(norm.NFKC.String(value))
	value = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || unicode.IsPunct(r) {
			return ' '
		}
		return r
	}, value)
	return strings.Join(strings.Fields(value), " ")
}

func serviceIdentities(line LineContext) []ServiceIdentity {
	result := make([]ServiceIdentity, 0, 3)
	if line.SourceFacts != nil && strings.TrimSpace(line.SourceFacts.SellerItemID) != "" {
		result = append(result, ServiceIdentity{Kind: IdentitySellerItemID, Value: strings.TrimSpace(line.SourceFacts.SellerItemID), NormalizerVersion: ExactIdentifierV1})
	}
	if line.SourceFacts != nil && strings.TrimSpace(line.SourceFacts.StandardItemID) != "" {
		result = append(result, ServiceIdentity{Kind: IdentityStandardItemID, Value: strings.TrimSpace(line.SourceFacts.StandardItemID), NormalizerVersion: ExactIdentifierV1})
	}
	if value := NormalizeDescriptionV1(line.Description); value != "" {
		result = append(result, ServiceIdentity{Kind: IdentityNormalizedDescription, Value: value, NormalizerVersion: NormalizedDescriptionV1})
	}
	return result
}

func PreferredServiceIdentity(line LineContext) (ServiceIdentity, bool) {
	identities := serviceIdentities(line)
	if len(identities) == 0 {
		return ServiceIdentity{}, false
	}
	return identities[0], true
}
