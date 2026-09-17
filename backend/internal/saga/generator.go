package saga

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"

	"diana-contabilitate/backend/internal/accountingdate"
	"diana-contabilitate/backend/internal/classification"
	"diana-contabilitate/backend/internal/invoicing"
	"diana-contabilitate/backend/internal/money"
	"diana-contabilitate/backend/internal/rules"
)

const (
	// ExporterVersion versions Diana's implementation of the SAGA C invoice XML
	// import contract documented by SAGA Software. SAGA publishes no XSD or
	// independent schema version for this format.
	ExporterVersion = "SAGA_C_INVOICE_XML_2026_V1"
	ContentType     = "application/xml; charset=utf-8"
)

type FailureCategory string

const (
	FailureDataInvalid             FailureCategory = "DATA_INVALID"
	FailureSerialization           FailureCategory = "SERIALIZATION_FAILED"
	FailureUnsupportedDocumentType FailureCategory = "UNSUPPORTED_DOCUMENT_TYPE"
)

type Failure struct {
	Category FailureCategory
	Safe     string
	Cause    error
}

func (e *Failure) Error() string   { return e.Safe }
func (e *Failure) Unwrap() error   { return e.Cause }
func (e *Failure) Permanent() bool { return true }

func Category(err error) FailureCategory {
	var failure *Failure
	if errors.As(err, &failure) {
		return failure.Category
	}
	return FailureSerialization
}

type ClientIdentity struct {
	ID   string
	Name string
	CUI  string
}

type Artifact struct {
	Filename               string
	ContentType            string
	Payload                []byte
	SHA256                 string
	ExporterVersion        string
	ClassificationSnapshot map[string]string
}

type invoiceXML struct {
	XMLName  xml.Name     `xml:"Facturi"`
	Invoices []invoiceTag `xml:"Factura"`
}

type invoiceTag struct {
	Header  headerTag  `xml:"Antet"`
	Details detailsTag `xml:"Detalii"`
	ID      string     `xml:"FacturaID,omitempty"`
}

type headerTag struct {
	SupplierName string `xml:"FurnizorNume"`
	SupplierCIF  string `xml:"FurnizorCIF"`
	ClientName   string `xml:"ClientNume"`
	ClientCIF    string `xml:"ClientCIF"`
	Number       string `xml:"FacturaNumar"`
	Date         string `xml:"FacturaData"`
	DueDate      string `xml:"FacturaScadenta,omitempty"`
	Currency     string `xml:"FacturaMoneda,omitempty"`
}

type detailsTag struct {
	Content contentTag `xml:"Continut"`
}

type contentTag struct {
	Lines []lineTag `xml:"Linie"`
}

type lineTag struct {
	Position       int    `xml:"LinieNrCrt"`
	Description    string `xml:"Descriere"`
	AdditionalInfo string `xml:"InformatiiSuplimentare,omitempty"`
	Unit           string `xml:"UM"`
	Quantity       string `xml:"Cantitate"`
	UnitPrice      string `xml:"Pret"`
	NetValue       string `xml:"Valoare"`
	VATRate        string `xml:"ProcTVA"`
	VATValue       string `xml:"TVA"`
	Account        string `xml:"Cont"`
	Deductibility  string `xml:"TipDeducere,omitempty"`
}

var (
	accountPattern = regexp.MustCompile(`^[0-9]{3,}(?:\.[0-9A-Za-z]+)*$`)
	unsafeFilename = regexp.MustCompile(`[^0-9A-Za-z._-]+`)
)

func generateLegacy(item *invoicing.Invoice, client ClientIdentity) (Artifact, error) {
	if item == nil || item.ID == "" || client.ID == "" || client.ID != item.ClientID || strings.TrimSpace(client.Name) == "" || strings.TrimSpace(client.CUI) == "" {
		return Artifact{}, invalid("Identitatea clientului SAGA este incompletă sau nu aparține facturii.", nil)
	}
	documentType := item.DocumentType
	if documentType == "" {
		documentType = invoicing.DocumentTypeInvoice
	}
	if documentType != invoicing.DocumentTypeInvoice {
		return Artifact{}, &Failure{Category: FailureUnsupportedDocumentType, Safe: "Documentele CreditNote nu pot fi exportate până la verificarea semanticii de storno SAGA."}
	}
	if item.PipelineStatus != invoicing.StatusReadyForSAGA && item.PipelineStatus != invoicing.StatusExporting {
		return Artifact{}, invalid("Factura nu este într-o stare exportabilă către SAGA.", nil)
	}
	if item.ActiveTask != nil || strings.TrimSpace(item.SupplierName) == "" || item.SupplierCUI == nil || strings.TrimSpace(*item.SupplierCUI) == "" || strings.TrimSpace(item.DocumentNumber) == "" || item.IssueDate.IsZero() || len(item.Lines) == 0 {
		return Artifact{}, invalid("Factura nu conține toate datele obligatorii pentru exportul SAGA.", nil)
	}
	if len(item.Total.Currency) != 3 || item.Total.Currency != strings.ToUpper(item.Total.Currency) || !item.Total.Amount.Valid() {
		return Artifact{}, invalid("Moneda sau totalul facturii nu este valid pentru exportul SAGA.", nil)
	}

	result := invoiceTag{ID: item.ID}
	result.Header = headerTag{
		SupplierName: strings.TrimSpace(item.SupplierName), SupplierCIF: strings.TrimSpace(*item.SupplierCUI),
		ClientName: strings.TrimSpace(client.Name), ClientCIF: strings.TrimSpace(client.CUI),
		Number: strings.TrimSpace(item.DocumentNumber), Date: item.IssueDate.Format("02.01.2006"),
	}
	if item.DueDate != nil {
		result.Header.DueDate = item.DueDate.Format("02.01.2006")
	}
	if item.Total.Currency != "RON" {
		result.Header.Currency = item.Total.Currency
	}

	snapshot := make(map[string]string, len(item.Lines)*3)
	lineTotal := new(big.Rat)
	for _, line := range item.Lines {
		for _, decision := range line.Classifications {
			if !decision.HumanReviewed && decision.InvoiceDateUsed != accountingdate.FromTime(item.IssueDate) {
				return Artifact{}, invalid("Data de clasificare automată nu coincide cu data facturii.", nil)
			}
		}
		external, err := exportLine(line, snapshot)
		if err != nil {
			return Artifact{}, err
		}
		lineAmount, _ := new(big.Rat).SetString(line.TotalValue.String())
		lineTotal.Add(lineTotal, lineAmount)
		result.Details.Content.Lines = append(result.Details.Content.Lines, external)
	}
	invoiceTotal, _ := new(big.Rat).SetString(item.Total.Amount.String())
	if lineTotal.Cmp(invoiceTotal) != 0 {
		return Artifact{}, invalid("Totalul facturii nu este egal cu suma exactă a liniilor.", nil)
	}

	document := invoiceXML{Invoices: []invoiceTag{result}}
	body, err := xml.MarshalIndent(document, "", "  ")
	if err != nil {
		return Artifact{}, &Failure{Category: FailureSerialization, Safe: "Fișierul SAGA nu a putut fi serializat.", Cause: err}
	}
	payload := append([]byte(xml.Header), body...)
	payload = append(payload, '\n')
	if err = validateStructure(payload); err != nil {
		return Artifact{}, &Failure{Category: FailureSerialization, Safe: "Fișierul SAGA generat nu a trecut validarea structurală.", Cause: err}
	}
	digest := sha256.Sum256(payload)
	filename := fmt.Sprintf("F_%s_%s_%s.xml", filenamePart(*item.SupplierCUI), filenamePart(item.DocumentNumber), item.IssueDate.Format("02.01.2006"))
	return Artifact{Filename: filename, ContentType: ContentType, Payload: payload, SHA256: hex.EncodeToString(digest[:]), ExporterVersion: ExporterVersion, ClassificationSnapshot: snapshot}, nil
}

func exportLine(line invoicing.Line, snapshot map[string]string) (lineTag, error) {
	if line.Position <= 0 || strings.TrimSpace(line.Description) == "" || strings.TrimSpace(line.Unit) == "" || !line.Quantity.Valid() || !line.UnitPrice.Valid() || !line.NetValue.Valid() || !line.VATRate.Valid() || !line.VATValue.Valid() || !line.TotalValue.Valid() {
		return lineTag{}, invalid(fmt.Sprintf("Linia %d conține date obligatorii invalide.", line.Position), nil)
	}
	net, _ := new(big.Rat).SetString(line.NetValue.String())
	vat, _ := new(big.Rat).SetString(line.VATValue.String())
	total, _ := new(big.Rat).SetString(line.TotalValue.String())
	if new(big.Rat).Add(net, vat).Cmp(total) != 0 {
		return lineTag{}, invalid(fmt.Sprintf("Linia %d nu respectă egalitatea exactă valoare + TVA = total.", line.Position), nil)
	}

	values := make(map[classification.Dimension]string, 3)
	for _, decision := range line.Classifications {
		if decision.Status == classification.ReviewPending || decision.EffectiveValue == nil || strings.TrimSpace(*decision.EffectiveValue) == "" {
			return lineTag{}, invalid(fmt.Sprintf("Linia %d are clasificări nefinalizate.", line.Position), nil)
		}
		if !decision.HumanReviewed && (decision.PolicyVersion != classification.ProductionPolicyVersion || decision.Dimension == classification.DimensionDeductibility || decision.Status != classification.ReviewAccepted || decision.Source != classification.SourceRule || decision.Rule == nil || !decision.Rule.ProductionEligible || !decision.Rule.Provenance.Valid() || decision.Rule.RulePackVersion == "" || strings.Contains(decision.LegalBasis, rules.LegalBasisPlaceholder) || !decision.InvoiceDateUsed.Within(decision.Rule.EffectiveFrom, decision.Rule.EffectiveTo) || !decision.InvoiceDateUsed.Within(decision.Rule.Provenance.EffectiveFrom, decision.Rule.Provenance.EffectiveTo)) {
			return lineTag{}, invalid(fmt.Sprintf("Linia %d are o clasificare automată demonstrativă sau neverificată; este necesară confirmarea contabilului.", line.Position), nil)
		}
		value := strings.TrimSpace(*decision.EffectiveValue)
		values[decision.Dimension] = value
		ruleVersion := "manual"
		if decision.Rule != nil {
			ruleVersion = decision.Rule.RuleVersionID
		}
		snapshot[decision.ID] = fmt.Sprintf("line=%s;dimension=%s;value=%s;policy=%s;rule_version=%s;revision=%d;invoice_date=%s;production=%t;pack=%s;legal_basis=%s", decision.InvoiceLineID, decision.Dimension, value, decision.PolicyVersion, ruleVersion, decision.Revision, decision.InvoiceDateUsed, decision.Rule != nil && decision.Rule.ProductionEligible, rulePack(decision.Rule), decision.LegalBasis)
	}
	for _, dimension := range classification.Dimensions {
		if values[dimension] == "" {
			return lineTag{}, invalid(fmt.Sprintf("Linia %d nu are clasificarea finală %s.", line.Position, dimension), nil)
		}
	}
	if !accountPattern.MatchString(values[classification.DimensionAccount]) {
		return lineTag{}, invalid(fmt.Sprintf("Linia %d nu are un cod de cont SAGA explicit și valid.", line.Position), nil)
	}
	classifiedVAT, err := money.Parse(values[classification.DimensionVAT])
	if err != nil || !classifiedVAT.Equal(line.VATRate) {
		return lineTag{}, invalid(fmt.Sprintf("Linia %d are o clasificare TVA care nu confirmă procentul din factură.", line.Position), err)
	}
	deductibility := values[classification.DimensionDeductibility]
	switch deductibility {
	case "SAGA_DEFAULT":
		deductibility = ""
	case "N50", "I":
	default:
		return lineTag{}, invalid(fmt.Sprintf("Linia %d nu are un cod de deductibilitate SAGA aprobat (SAGA_DEFAULT, N50 sau I).", line.Position), nil)
	}
	additional := ""
	if line.AdditionalInfo != nil {
		additional = strings.TrimSpace(*line.AdditionalInfo)
	}
	return lineTag{
		Position: line.Position, Description: strings.TrimSpace(line.Description), AdditionalInfo: additional,
		Unit: strings.TrimSpace(line.Unit), Quantity: line.Quantity.String(), UnitPrice: line.UnitPrice.String(),
		NetValue: line.NetValue.String(), VATRate: line.VATRate.String(), VATValue: line.VATValue.String(),
		Account: values[classification.DimensionAccount], Deductibility: deductibility,
	}, nil
}

func validateStructure(payload []byte) error {
	var document invoiceXML
	if err := xml.Unmarshal(payload, &document); err != nil {
		return err
	}
	if document.XMLName.Local != "Facturi" || len(document.Invoices) != 1 || len(document.Invoices[0].Details.Content.Lines) == 0 {
		return errors.New("unexpected SAGA XML structure")
	}
	return nil
}

func invalid(message string, cause error) error {
	return &Failure{Category: FailureDataInvalid, Safe: message, Cause: cause}
}

func filenamePart(value string) string {
	value = strings.Trim(unsafeFilename.ReplaceAllString(strings.TrimSpace(value), "_"), "._-")
	if value == "" {
		return "necunoscut"
	}
	if len(value) > 80 {
		return value[:80]
	}
	return value
}

func rulePack(rule *classification.RuleReference) string {
	if rule == nil {
		return ""
	}
	return rule.RulePackVersion
}
