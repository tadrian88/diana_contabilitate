package clients

import (
	"diana-contabilitate/backend/internal/apperrors"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"diana-contabilitate/backend/internal/fiscalidentity"
)

type Lifecycle string

const (
	Onboarding Lifecycle = "ONBOARDING"
	Active     Lifecycle = "ACTIVE"
	Inactive   Lifecycle = "INACTIVE"
)

// Company contains mutable master data, never integration credentials or policy.
type Company struct {
	Name               string `json:"name"`
	CUI                string `json:"cui"`
	DisplayName        string `json:"displayName"`
	RegistrationNumber string `json:"registrationNumber"`
	Country            string `json:"country"`
	Address            string `json:"address"`
	City               string `json:"city"`
	Region             string `json:"region"`
	PostalCode         string `json:"postalCode"`
	Email              string `json:"email"`
	Phone              string `json:"phone"`
	DefaultCurrency    string `json:"defaultCurrency"`
}
type Client struct {
	// Keep existing direct fields for protected callers.
	ID                   string    `json:"id"`
	Name                 string    `json:"name"`
	CUI                  string    `json:"cui"`
	Company              Company   `json:"company"`
	NormalizedIdentifier string    `json:"normalizedIdentifier"`
	Status               Lifecycle `json:"status"`
	Revision             uint64    `json:"revision"`
	CreatedAt            time.Time `json:"createdAt"`
	UpdatedAt            time.Time `json:"updatedAt"`
}

var romanianID = regexp.MustCompile(`^[1-9][0-9]{1,9}$`)
var countryCode = regexp.MustCompile(`^[A-Z]{2}$`)
var currencyCode = regexp.MustCompile(`^[A-Z]{3}$`)
var foreignID = regexp.MustCompile(`^[A-Z0-9][A-Z0-9.-]{1,31}$`)

func NormalizeIdentifier(raw, country string) (string, error) {
	value := strings.ToUpper(strings.TrimSpace(raw))
	if country == "RO" {
		var ok bool
		value, ok = fiscalidentity.Romanian(value)
		if !ok || !romanianID.MatchString(value) {
			return "", fmt.Errorf("%w: CUI românesc invalid (2–10 cifre)", apperrors.ErrValidation)
		}
	} else if !foreignID.MatchString(value) {
		return "", fmt.Errorf("%w: identificator fiscal invalid", apperrors.ErrValidation)
	}
	return value, nil
}
func (c *Company) Normalize() (string, error) {
	c.Name = strings.TrimSpace(c.Name)
	c.CUI = strings.TrimSpace(c.CUI)
	c.Country = strings.ToUpper(strings.TrimSpace(c.Country))
	c.DefaultCurrency = strings.ToUpper(strings.TrimSpace(c.DefaultCurrency))
	if c.Name == "" || len(c.Name) > 255 || !countryCode.MatchString(c.Country) || !currencyCode.MatchString(c.DefaultCurrency) {
		return "", fmt.Errorf("%w: denumire, țară și monedă obligatorii", apperrors.ErrValidation)
	}
	fields := []*string{&c.DisplayName, &c.RegistrationNumber, &c.Address, &c.City, &c.Region, &c.PostalCode, &c.Email, &c.Phone}
	for _, f := range fields {
		*f = strings.TrimSpace(*f)
		if len(*f) > 500 {
			return "", apperrors.ErrValidation
		}
	}
	if c.Email != "" {
		a, e := mail.ParseAddress(c.Email)
		if e != nil || a.Address != c.Email {
			return "", fmt.Errorf("%w: email invalid", apperrors.ErrValidation)
		}
	}
	return NormalizeIdentifier(c.CUI, c.Country)
}
func CanChangeIdentity(before Client, after Company, normalized string, dependent bool) error {
	if dependent && (before.NormalizedIdentifier != normalized || before.Company.Country != after.Country) {
		return fmt.Errorf("%w: CUI/țara nu pot fi schimbate după apariția documentelor, profilurilor sau conexiunilor", apperrors.ErrConflict)
	}
	return nil
}

// Existing synthetic identities remain editable only when retained verbatim.
func (c *Company) NormalizeExisting(before Client) (string, error) {
	normalized, err := c.Normalize()
	if err == nil {
		return normalized, nil
	}
	if c.CUI != before.CUI || c.Country != before.Company.Country {
		return "", err
	}
	raw := c.CUI
	c.CUI = "12"
	_, err = c.Normalize()
	c.CUI = raw
	return before.NormalizedIdentifier, err
}
