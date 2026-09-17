package contractingestion

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"diana-contabilitate/backend/internal/apperrors"
	"diana-contabilitate/backend/internal/contracts"
	"diana-contabilitate/backend/internal/invoicing"
	"diana-contabilitate/backend/internal/money"
)

const DefaultMaxPDFBytes int64 = 20 << 20

func (s *Service) MaxPDFBytes() int64 { return s.maxBytes }

var cuiPattern = regexp.MustCompile(`^(RO)?[0-9]{2,10}$`)
var currencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)

type Service struct {
	store        Store
	extractor    ContractExtractor
	availability ContractAvailability
	maxBytes     int64
	clock        func() time.Time
}

func NewService(store Store, extractor ContractExtractor, availability ContractAvailability, maxBytes int64, clock func() time.Time) *Service {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxPDFBytes
	}
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	return &Service{store: store, extractor: extractor, availability: availability, maxBytes: maxBytes, clock: clock}
}

func (s *Service) Upload(ctx context.Context, input Upload) (Document, bool, error) {
	if !authorized(input.Actor, input.ClientID) {
		return Document{}, false, apperrors.ErrNotFound
	}
	if input.ClientID == "" || len(input.Bytes) == 0 {
		return Document{}, false, apperrors.ErrValidation
	}
	if int64(len(input.Bytes)) > s.maxBytes {
		return Document{}, false, ErrDocumentTooLarge
	}
	if len(input.Bytes) < 8 || string(input.Bytes[:5]) != "%PDF-" || !bytes.Contains(input.Bytes[max(0, len(input.Bytes)-1024):], []byte("%%EOF")) {
		return Document{}, false, ErrInvalidPDF
	}
	if ct := strings.ToLower(strings.TrimSpace(input.ContentType)); ct != "" && ct != "application/pdf" && ct != "application/octet-stream" {
		return Document{}, false, ErrInvalidPDF
	}
	name := filepath.Base(strings.ReplaceAll(input.Filename, "\\", "/"))
	name = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, name)
	if name == "" || name == "." {
		name = "contract.pdf"
	}
	if !strings.EqualFold(filepath.Ext(name), ".pdf") {
		return Document{}, false, ErrInvalidPDF
	}
	if len([]rune(name)) > 180 {
		name = string([]rune(name)[:176]) + ".pdf"
	}
	sum := sha256.Sum256(input.Bytes)
	input.Filename, input.ContentType = name, "application/pdf"
	doc, duplicate, err := s.store.CreateDocument(ctx, input, newID("contractdoc"), hex.EncodeToString(sum[:]), s.clock())
	return doc, duplicate, err
}

func (s *Service) List(ctx context.Context, clientID string, actor Actor) ([]Document, error) {
	if !authorized(actor, clientID) {
		return nil, apperrors.ErrNotFound
	}
	return s.store.ListDocuments(ctx, clientID)
}

func (s *Service) Get(ctx context.Context, clientID, id string, actor Actor) (Document, error) {
	if !authorized(actor, clientID) {
		return Document{}, apperrors.ErrNotFound
	}
	return s.store.GetDocument(ctx, clientID, id)
}

func (s *Service) File(ctx context.Context, clientID, id string, actor Actor) (Source, error) {
	if !authorized(actor, clientID) {
		return Source{}, apperrors.ErrNotFound
	}
	if _, err := s.store.GetDocument(ctx, clientID, id); err != nil {
		return Source{}, err
	}
	source, err := s.store.GetSource(ctx, id)
	if err != nil || source.Document.ClientID != clientID {
		if err == nil {
			err = apperrors.ErrNotFound
		}
		return Source{}, err
	}
	return source, nil
}

func (s *Service) Extract(ctx context.Context, documentID string) error {
	if s.extractor == nil {
		return fmt.Errorf("%w: extractor unavailable", ErrExtractionPermanent)
	}
	attemptID := newID("contractextract")
	_, run, err := s.store.BeginExtraction(ctx, documentID, attemptID, s.extractor.Provider(), s.extractor.Model(), s.clock())
	if err != nil || !run {
		return err
	}
	source, err := s.store.GetSource(ctx, documentID)
	if err == nil {
		sum := sha256.Sum256(source.Bytes)
		if int64(len(source.Bytes)) != source.Document.SizeBytes || hex.EncodeToString(sum[:]) != source.Document.SHA256 {
			err = ErrExtractionPermanent
		}
	}
	if err == nil {
		var result ExtractionResult
		result, err = s.extractor.Extract(ctx, source.Bytes, source.Document.MIMEType)
		if err == nil {
			err = ValidateProposal(result.Proposal)
			if err == nil {
				return s.store.CompleteExtraction(ctx, documentID, attemptID, result, s.clock())
			}
		}
	}
	category := "INVALID_OUTPUT"
	if errors.Is(err, ErrExtractionTransient) {
		category = "PROVIDER_TRANSIENT"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		category = "TIMEOUT"
	}
	if errors.Is(err, context.Canceled) {
		category = "INTERRUPTED"
	}
	failureContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if failErr := s.store.FailExtraction(failureContext, documentID, attemptID, category, s.clock()); failErr != nil {
		return errors.Join(err, failErr)
	}
	if errors.Is(err, ErrExtractionTransient) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return err
	}
	return fmt.Errorf("%w: %s", ErrExtractionPermanent, category)
}

func ValidateProposal(p Proposal) error {
	hasContractData := false
	for _, f := range []Field{p.SupplierName, p.SupplierCUI, p.Reference, p.EffectiveFrom, p.EffectiveTo, p.TotalValue, p.Currency, p.UnitType, p.PaymentTerms} {
		if f.Status == "PRESENT" || f.Status == "AMBIGUOUS" {
			hasContractData = true
		}
	}
	if !hasContractData {
		return apperrors.ErrValidation
	}
	fields := []Field{p.SupplierName, p.SupplierCUI, p.Reference, p.EffectiveFrom, p.EffectiveTo, p.TotalValue, p.Currency, p.UnitType, p.PaymentTerms, p.BuyerCUI}
	for _, f := range fields {
		if f.Value != nil && len(*f.Value) > 4096 {
			return apperrors.ErrValidation
		}
		for _, alternative := range f.Alternatives {
			if len(alternative) > 4096 {
				return apperrors.ErrValidation
			}
		}
		if f.Status != "PRESENT" && f.Status != "MISSING" && f.Status != "AMBIGUOUS" {
			return apperrors.ErrValidation
		}
		if f.Confidence != ConfidenceHigh && f.Confidence != ConfidenceMedium && f.Confidence != ConfidenceLow && f.Confidence != ConfidenceUnknown {
			return apperrors.ErrValidation
		}
		if f.Evidence.Page != nil && *f.Evidence.Page < 1 {
			return apperrors.ErrValidation
		}
		if f.Status == "MISSING" && f.Value != nil {
			return apperrors.ErrValidation
		}
		if f.Status == "PRESENT" && (f.Value == nil || strings.TrimSpace(*f.Value) == "" || f.Evidence.Snippet == "") {
			return apperrors.ErrValidation
		}
		if f.Status == "AMBIGUOUS" && len(f.Alternatives) < 2 {
			return apperrors.ErrValidation
		}
		if len(f.Evidence.Snippet) > 2000 || len(f.Alternatives) > 10 {
			return apperrors.ErrValidation
		}
	}
	for _, f := range []Field{p.EffectiveFrom, p.EffectiveTo} {
		if f.Value != nil {
			if _, err := time.Parse("2006-01-02", *f.Value); err != nil {
				return apperrors.ErrValidation
			}
		}
	}
	if p.SupplierCUI.Value != nil && !validCUI(*p.SupplierCUI.Value) {
		return apperrors.ErrValidation
	}
	if p.Currency.Value != nil && !validCurrency(*p.Currency.Value) {
		return apperrors.ErrValidation
	}
	if p.TotalValue.Value != nil {
		if !validAmount(*p.TotalValue.Value) {
			return apperrors.ErrValidation
		}
	}
	return nil
}

func (s *Service) Retry(ctx context.Context, clientID, documentID string, revision uint64, actor Actor) error {
	if !authorized(actor, clientID) {
		return apperrors.ErrNotFound
	}
	return s.store.RetryExtraction(ctx, clientID, documentID, revision, actor, s.clock())
}

func (s *Service) Confirm(ctx context.Context, command ConfirmCommand) (string, bool, error) {
	if !authorized(command.Actor, command.ClientID) {
		return "", false, apperrors.ErrNotFound
	}
	if command.CommandID == "" || command.ExpectedDocumentRevision == 0 {
		return "", false, apperrors.ErrValidation
	}
	doc, err := s.store.GetDocument(ctx, command.ClientID, command.DocumentID)
	if err != nil {
		return "", false, err
	}
	if doc.BuyerMismatch {
		return "", false, ErrBuyerMismatch
	}
	value, err := validatedContract(command)
	if err != nil {
		return "", false, err
	}
	id, changed, err := s.store.ConfirmDocument(ctx, command, value, s.clock())
	if err != nil {
		return "", false, err
	}
	if s.availability == nil {
		return id, changed, fmt.Errorf("contract availability unavailable")
	}
	_, err = s.availability.ContractAvailable(ctx, contracts.AvailableCommand{ContractID: id, CommandID: "contract-ingestion:" + command.DocumentID + ":available", CorrelationID: command.Actor.CorrelationID})
	return id, changed, err
}

func validatedContract(c ConfirmCommand) (contracts.Contract, error) {
	v := c.Contract
	for _, text := range []string{v.SupplierName, v.SupplierCUI, v.Reference, v.EffectiveFrom, v.EffectiveTo, v.TotalValue, v.Currency, v.UnitType, v.PaymentTerms} {
		if len(text) > 4096 {
			return contracts.Contract{}, apperrors.ErrValidation
		}
	}
	if strings.TrimSpace(v.SupplierName) == "" || strings.TrimSpace(v.Reference) == "" || strings.TrimSpace(v.UnitType) == "" || strings.TrimSpace(v.PaymentTerms) == "" {
		return contracts.Contract{}, apperrors.ErrValidation
	}
	cui := strings.TrimSpace(v.SupplierCUI)
	if !validCUI(cui) {
		return contracts.Contract{}, apperrors.ErrValidation
	}
	currency := strings.ToUpper(strings.TrimSpace(v.Currency))
	if !validCurrency(currency) {
		return contracts.Contract{}, apperrors.ErrValidation
	}
	from, e1 := time.Parse("2006-01-02", v.EffectiveFrom)
	to, e2 := time.Parse("2006-01-02", v.EffectiveTo)
	amount, e3 := money.Parse(v.TotalValue)
	if e1 != nil || e2 != nil || e3 != nil || !validAmount(v.TotalValue) || to.Before(from) {
		return contracts.Contract{}, apperrors.ErrValidation
	}
	return contracts.Contract{ID: newID("contract"), ClientID: c.ClientID, SupplierName: strings.TrimSpace(v.SupplierName), SupplierCUI: cui, NormalizedSupplierCUI: invoicing.NormalizeBusinessIdentifier(cui), Reference: strings.TrimSpace(v.Reference), EffectiveFrom: from, EffectiveTo: to, Value: money.Money{Amount: amount, Currency: currency}, UnitType: strings.TrimSpace(v.UnitType), PaymentTerms: strings.TrimSpace(v.PaymentTerms), Revision: 1}, nil
}

func validCUI(value string) bool {
	value = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(value), " ", ""))
	return cuiPattern.MatchString(value) && strings.Trim(strings.TrimPrefix(value, "RO"), "0") != ""
}
func validCurrency(value string) bool {
	value = strings.ToUpper(strings.TrimSpace(value))
	return currencyPattern.MatchString(value) && strings.Contains(" "+isoCurrencies+" ", " "+value+" ")
}

const isoCurrencies = "AED AFN ALL AMD ANG AOA ARS AUD AWG AZN BAM BBD BDT BGN BHD BIF BMD BND BOB BOV BRL BSD BTN BWP BYN BZD CAD CDF CHE CHF CHW CLF CLP CNY COP COU CRC CUP CVE CZK DJF DKK DOP DZD EGP ERN ETB EUR FJD FKP GBP GEL GHS GIP GMD GNF GTQ GYD HKD HNL HTG HUF IDR ILS INR IQD IRR ISK JMD JOD JPY KES KGS KHR KMF KPW KRW KWD KYD KZT LAK LBP LKR LRD LSL LYD MAD MDL MGA MKD MMK MNT MOP MRU MUR MVR MWK MXN MXV MYR MZN NAD NGN NIO NOK NPR NZD OMR PAB PEN PGK PHP PKR PLN PYG QAR RON RSD RUB RWF SAR SBD SCR SDG SEK SGD SHP SLE SLL SOS SRD SSP STN SVC SYP SZL THB TJS TMT TND TOP TRY TTD TWD TZS UAH UGX USD USN UYI UYU UYW UZS VED VES VND VUV WST XAF XCD XOF XPF YER ZAR ZMW ZWG"

func validAmount(value string) bool {
	if _, err := money.Parse(value); err != nil {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(value, "-"), ".")
	return len(parts[0]) <= 16 && (len(parts) == 1 || len(parts[1]) <= 4)
}

func authorized(actor Actor, clientID string) bool {
	if actor.AllClients {
		return true
	}
	for _, id := range actor.AuthorizedClientIDs {
		if id == clientID {
			return true
		}
	}
	return false
}
func newID(prefix string) string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return prefix + "-" + hex.EncodeToString(b[:])
}

func stableID(prefix, value string) string {
	sum := sha256.Sum256([]byte(value))
	return prefix + "-" + hex.EncodeToString(sum[:16])
}
