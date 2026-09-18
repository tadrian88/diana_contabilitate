package httpserver

import (
	"encoding/json"
	"fmt"
	"time"

	"diana-contabilitate/backend/internal/accounting"
	"diana-contabilitate/backend/internal/accountingdate"
	"diana-contabilitate/backend/internal/classification"
	"diana-contabilitate/backend/internal/contractingestion"
	"diana-contabilitate/backend/internal/contracts"
	"diana-contabilitate/backend/internal/invoicing"
	"diana-contabilitate/backend/internal/money"
	"diana-contabilitate/backend/internal/rules"
	"diana-contabilitate/backend/internal/saga"
	"diana-contabilitate/backend/internal/validationtasks"
)

type clientDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	CUI  string `json:"cui"`
}

type moneyDTO struct {
	Amount   json.RawMessage `json:"amount"`
	Currency string          `json:"currency"`
}

type activityDTO struct {
	ID        string  `json:"id"`
	Label     string  `json:"label"`
	Actor     string  `json:"actor"`
	Timestamp string  `json:"timestamp"`
	Detail    string  `json:"detail"`
	Before    *string `json:"before,omitempty"`
	After     *string `json:"after,omitempty"`
}

type invoiceLineDTO struct {
	SourceFacts     *accounting.LineFacts   `json:"sourceFacts,omitempty"`
	ID              string                  `json:"id"`
	Position        int                     `json:"position"`
	Description     string                  `json:"description"`
	Unit            string                  `json:"unit"`
	VATRate         json.RawMessage         `json:"vatRate"`
	VATValue        moneyDTO                `json:"vatValue"`
	Quantity        json.RawMessage         `json:"quantity"`
	UnitPrice       moneyDTO                `json:"unitPrice"`
	NetValue        moneyDTO                `json:"netValue"`
	TotalValue      moneyDTO                `json:"totalValue"`
	AdditionalInfo  *string                 `json:"additionalInfo,omitempty"`
	Classifications []lineClassificationDTO `json:"classifications"`
}

type ruleReferenceDTO struct {
	RuleVersionID      string               `json:"ruleVersionId"`
	ProductionEligible bool                 `json:"productionEligible"`
	RulePackVersion    string               `json:"rulePackVersion,omitempty"`
	Provenance         *rules.Provenance    `json:"provenance,omitempty"`
	EffectiveFrom      accountingdate.Date  `json:"effectiveFrom,omitempty"`
	EffectiveTo        *accountingdate.Date `json:"effectiveTo,omitempty"`
	RuleID             string               `json:"ruleId"`
	Reference          string               `json:"reference"`
	Version            int                  `json:"version"`
	Origin             string               `json:"origin"`
}

type lineClassificationDTO struct {
	ModelVersion       string               `json:"modelVersion,omitempty"`
	TypedValue         *accounting.Value    `json:"typedValue,omitempty"`
	ProposedTypedValue *accounting.Value    `json:"proposedTypedValue,omitempty"`
	Evidence           *accounting.Evidence `json:"evidence,omitempty"`
	ReviewReason       string               `json:"reviewReason,omitempty"`
	InvoiceDateUsed    accountingdate.Date  `json:"invoiceDateUsed,omitempty"`
	HumanReviewed      bool                 `json:"humanReviewed"`
	ID                 string               `json:"id"`
	Dimension          string               `json:"dimension"`
	Value              string               `json:"value"`
	Confidence         string               `json:"confidence"`
	Explanation        string               `json:"explanation"`
	LegalBasis         string               `json:"legalBasis"`
	Status             string               `json:"status"`
	Revision           uint64               `json:"revision"`
	Rule               *ruleReferenceDTO    `json:"rule,omitempty"`
}

type classificationReviewItemDTO struct {
	ModelVersion       string               `json:"modelVersion,omitempty"`
	TypedValue         *accounting.Value    `json:"typedValue,omitempty"`
	ProposedTypedValue *accounting.Value    `json:"proposedTypedValue,omitempty"`
	Evidence           *accounting.Evidence `json:"evidence,omitempty"`
	ReviewReason       string               `json:"reviewReason,omitempty"`
	InvoiceDateUsed    accountingdate.Date  `json:"invoiceDateUsed,omitempty"`
	HumanReviewed      bool                 `json:"humanReviewed"`
	ID                 string               `json:"id"`
	LineID             string               `json:"lineId"`
	LineLabel          string               `json:"lineLabel"`
	Dimension          string               `json:"dimension"`
	ProposedValue      string               `json:"proposedValue"`
	Confidence         string               `json:"confidence"`
	Explanation        string               `json:"explanation"`
	LegalBasis         string               `json:"legalBasis"`
	Status             string               `json:"status"`
	ResolvedValue      *string              `json:"resolvedValue,omitempty"`
	Revision           uint64               `json:"revision"`
	Rule               *ruleReferenceDTO    `json:"rule,omitempty"`
}

type invoiceDTO struct {
	ModelVersion       string                  `json:"modelVersion,omitempty"`
	SourceFacts        *accounting.SourceFacts `json:"sourceFacts,omitempty"`
	AccountingSnapshot *accounting.Snapshot    `json:"accountingSnapshot,omitempty"`
	ReadinessReason    string                  `json:"readinessReason,omitempty"`
	ID                 string                  `json:"id"`
	ClientID           string                  `json:"clientId"`
	SupplierName       string                  `json:"supplierName"`
	SupplierCUI        *string                 `json:"supplierCui,omitempty"`
	DocumentNumber     string                  `json:"documentNumber"`
	IssueDate          string                  `json:"issueDate"`
	DueDate            *string                 `json:"dueDate,omitempty"`
	Total              moneyDTO                `json:"total"`
	SPVReference       string                  `json:"spvReference"`
	PipelineStatus     string                  `json:"pipelineStatus"`
	SagaStatus         string                  `json:"sagaStatus"`
	Revision           uint64                  `json:"revision"`
	CreatedAt          string                  `json:"createdAt"`
	UpdatedAt          string                  `json:"updatedAt"`
	Activity           []activityDTO           `json:"activity"`
	Lines              []invoiceLineDTO        `json:"lines"`
	Task               *validationTaskDTO      `json:"task,omitempty"`
	SelectedContractID *string                 `json:"selectedContractId,omitempty"`
	Contract           *contractSummaryDTO     `json:"contract,omitempty"`
	SagaExport         *sagaExportDTO          `json:"sagaExport,omitempty"`
}

type sagaExportDTO struct {
	AttemptID        string  `json:"attemptId"`
	ArtifactStatus   string  `json:"artifactStatus"`
	Filename         string  `json:"filename,omitempty"`
	GeneratedAt      string  `json:"generatedAt"`
	DownloadedAt     *string `json:"downloadedAt,omitempty"`
	ConfirmedAt      *string `json:"confirmedAt,omitempty"`
	ConfirmedBy      *string `json:"confirmedBy,omitempty"`
	ConfirmationType *string `json:"confirmationType,omitempty"`
	InvoiceRevision  uint64  `json:"invoiceRevision"`
}

func sagaExportResponse(view *saga.ExportView) *sagaExportDTO {
	if view == nil {
		return nil
	}
	result := &sagaExportDTO{AttemptID: view.AttemptID, ArtifactStatus: string(view.ArtifactStatus), Filename: view.Filename, GeneratedAt: view.GeneratedAt.Format(time.RFC3339), InvoiceRevision: view.InvoiceRevision}
	if view.DownloadedAt != nil {
		value := view.DownloadedAt.Format(time.RFC3339)
		result.DownloadedAt = &value
	}
	if view.ConfirmedAt != nil {
		value := view.ConfirmedAt.Format(time.RFC3339)
		result.ConfirmedAt = &value
	}
	result.ConfirmedBy = view.ConfirmedBy
	if view.ConfirmationType != nil {
		value := string(*view.ConfirmationType)
		result.ConfirmationType = &value
	}
	return result
}

type validationTaskDTO struct {
	ID                  string                        `json:"id"`
	Type                string                        `json:"type"`
	Status              string                        `json:"status"`
	CreatedAt           string                        `json:"createdAt"`
	UpdatedAt           string                        `json:"updatedAt"`
	WaitingSince        *string                       `json:"waitingSince,omitempty"`
	ResolvedAt          *string                       `json:"resolvedAt,omitempty"`
	Title               string                        `json:"title"`
	Reason              string                        `json:"reason"`
	BlockerCode         *string                       `json:"blockerCode,omitempty"`
	Revision            uint64                        `json:"revision"`
	ContractRequested   bool                          `json:"contractRequested"`
	ContractCandidates  []contractCandidateDTO        `json:"contractCandidates,omitempty"`
	ClassificationItems []classificationReviewItemDTO `json:"classificationItems,omitempty"`
}

type contractCandidateDTO struct {
	ID           string   `json:"id"`
	Reference    string   `json:"reference"`
	SupplierName string   `json:"supplierName"`
	Period       string   `json:"period"`
	Value        moneyDTO `json:"value"`
	Confidence   string   `json:"confidence"`
	Reasons      []string `json:"reasons"`
	UnitType     string   `json:"unitType"`
	PaymentTerms string   `json:"paymentTerms"`
	Recommended  bool     `json:"recommended,omitempty"`
}

type contractSummaryDTO struct {
	ID                  string   `json:"id"`
	Reference           string   `json:"reference"`
	SupplierName        string   `json:"supplierName"`
	Period              string   `json:"period"`
	Value               moneyDTO `json:"value"`
	Currency            string   `json:"currency"`
	UnitType            string   `json:"unitType"`
	PaymentTerms        string   `json:"paymentTerms"`
	HasLegacyTotalValue bool     `json:"hasLegacyTotalValue"`
}

type contractDTO struct {
	contractSummaryDTO
	ClientID            string                   `json:"clientId"`
	SupplierCUI         string                   `json:"supplierCui"`
	SourceReference     *string                  `json:"sourceReference,omitempty"`
	SourceMetadata      *string                  `json:"sourceMetadata,omitempty"`
	SourceDocumentID    *string                  `json:"sourceDocumentId,omitempty"`
	ExtractionAttemptID *string                  `json:"extractionAttemptId,omitempty"`
	Revision            uint64                   `json:"revision"`
	PeriodType          string                   `json:"periodType"`
	ServiceTerms        []contractServiceTermDTO `json:"serviceTerms"`
}
type contractServiceTermDTO struct {
	ServiceDescription string          `json:"serviceDescription"`
	PricingModel       string          `json:"pricingModel"`
	UnitPrice          *string         `json:"unitPrice,omitempty"`
	Currency           string          `json:"currency"`
	Unit               string          `json:"unit,omitempty"`
	QuantitySource     string          `json:"quantitySource"`
	QuantityValue      *string         `json:"quantityValue,omitempty"`
	QuantityDriver     string          `json:"quantityDriver,omitempty"`
	BillingFrequency   string          `json:"billingFrequency"`
	Evidence           json.RawMessage `json:"evidence,omitempty"`
}

type contractDocumentDTO struct {
	ID                  string                              `json:"id"`
	ClientID            string                              `json:"clientId"`
	OriginalFilename    string                              `json:"originalFilename"`
	MIMEType            string                              `json:"mimeType"`
	SizeBytes           int64                               `json:"sizeBytes"`
	SHA256              string                              `json:"sha256"`
	Status              string                              `json:"status"`
	LifecycleState      string                              `json:"lifecycleState"`
	Revision            uint64                              `json:"revision"`
	UploadedAt          string                              `json:"uploadedAt"`
	UploadedBy          *string                             `json:"uploadedBy,omitempty"`
	ConfirmedAt         *string                             `json:"confirmedAt,omitempty"`
	ConfirmedBy         *string                             `json:"confirmedBy,omitempty"`
	ConfirmedContractID *string                             `json:"confirmedContractId,omitempty"`
	BuyerMismatch       bool                                `json:"buyerMismatch"`
	ClientCUI           string                              `json:"clientCui"`
	Duplicate           bool                                `json:"duplicate,omitempty"`
	Extraction          *contractExtractionDTO              `json:"extraction,omitempty"`
	Attempts            []contractExtractionDTO             `json:"attempts"`
	ConfirmedValues     *contractingestion.ReviewedContract `json:"confirmedValues,omitempty"`
}
type contractExtractionDTO struct {
	ID                string                      `json:"id"`
	Provider          string                      `json:"provider"`
	Model             string                      `json:"model"`
	SchemaVersion     string                      `json:"schemaVersion"`
	PromptVersion     string                      `json:"promptVersion"`
	Status            string                      `json:"status"`
	Proposal          *contractingestion.Proposal `json:"proposal,omitempty"`
	SafeErrorCategory string                      `json:"safeErrorCategory,omitempty"`
	StartedAt         string                      `json:"startedAt"`
	CompletedAt       *string                     `json:"completedAt,omitempty"`
}

func contractDocumentResponse(item contractingestion.Document, duplicate bool) contractDocumentDTO {
	result := contractDocumentDTO{ID: item.ID, ClientID: item.ClientID, ClientCUI: item.ClientCUI, OriginalFilename: item.OriginalFilename, MIMEType: item.MIMEType, SizeBytes: item.SizeBytes, SHA256: item.SHA256, Status: string(item.Status), LifecycleState: item.LifecycleState, Revision: item.Revision, UploadedAt: item.UploadedAt.Format(time.RFC3339), UploadedBy: item.UploadedByDisplay, ConfirmedContractID: item.ConfirmedContractID, BuyerMismatch: item.BuyerMismatch, Duplicate: duplicate, ConfirmedBy: item.ConfirmedByDisplay}
	if item.ConfirmedAt != nil {
		value := item.ConfirmedAt.Format(time.RFC3339)
		result.ConfirmedAt = &value
	}
	result.ConfirmedValues = item.ConfirmedValues
	result.Attempts = make([]contractExtractionDTO, 0, len(item.Attempts))
	for _, a := range item.Attempts {
		dto := contractExtractionDTO{ID: a.ID, Provider: a.Provider, Model: a.Model, SchemaVersion: a.SchemaVersion, PromptVersion: a.PromptVersion, Status: a.Status, Proposal: a.Proposal, SafeErrorCategory: a.SafeErrorCategory, StartedAt: a.StartedAt.Format(time.RFC3339)}
		if a.CompletedAt != nil {
			value := a.CompletedAt.Format(time.RFC3339)
			dto.CompletedAt = &value
		}
		result.Attempts = append(result.Attempts, dto)
	}
	if item.LatestAttempt != nil {
		a := item.LatestAttempt
		dto := &contractExtractionDTO{ID: a.ID, Provider: a.Provider, Model: a.Model, SchemaVersion: a.SchemaVersion, PromptVersion: a.PromptVersion, Status: a.Status, Proposal: a.Proposal, SafeErrorCategory: a.SafeErrorCategory, StartedAt: a.StartedAt.Format(time.RFC3339)}
		if a.CompletedAt != nil {
			value := a.CompletedAt.Format(time.RFC3339)
			dto.CompletedAt = &value
		}
		result.Extraction = dto
	}
	return result
}

type contractInvoiceDTO struct {
	ID             string   `json:"id"`
	ClientID       string   `json:"clientId"`
	SupplierName   string   `json:"supplierName"`
	DocumentNumber string   `json:"documentNumber"`
	IssueDate      string   `json:"issueDate"`
	Total          moneyDTO `json:"total"`
	SPVReference   string   `json:"spvReference"`
	PipelineStatus string   `json:"pipelineStatus"`
	SagaStatus     string   `json:"sagaStatus"`
}

type ruleVersionDTO struct {
	ID                 string            `json:"id"`
	ProductionEligible bool              `json:"productionEligible"`
	RulePackVersion    string            `json:"rulePackVersion,omitempty"`
	Provenance         *rules.Provenance `json:"provenance,omitempty"`
	Version            int               `json:"version"`
	EffectiveFrom      string            `json:"effectiveFrom"`
	EffectiveTo        *string           `json:"effectiveTo,omitempty"`
	Criteria           string            `json:"criteria"`
	Result             string            `json:"result"`
	Explanation        string            `json:"explanation"`
	LegalBasis         string            `json:"legalBasis"`
	CreatedAt          string            `json:"createdAt"`
	Actor              string            `json:"actor"`
}

type ruleDTO struct {
	ID           string           `json:"id"`
	Reference    string           `json:"reference"`
	Name         string           `json:"name"`
	Category     string           `json:"category"`
	Scope        string           `json:"scope"`
	ClientID     *string          `json:"clientId,omitempty"`
	ParentRuleID *string          `json:"parentRuleId,omitempty"`
	Revision     uint64           `json:"revision"`
	Versions     []ruleVersionDTO `json:"versions"`
}

type validationTaskInboxDTO struct {
	Task    validationTaskDTO `json:"task"`
	Invoice taskInvoiceDTO    `json:"invoice"`
	Client  clientDTO         `json:"client"`
}

type taskInvoiceDTO struct {
	ID             string   `json:"id"`
	ClientID       string   `json:"clientId"`
	SupplierName   string   `json:"supplierName"`
	SupplierCUI    *string  `json:"supplierCui,omitempty"`
	DocumentNumber string   `json:"documentNumber"`
	IssueDate      string   `json:"issueDate"`
	Total          moneyDTO `json:"total"`
	SPVReference   string   `json:"spvReference"`
	PipelineStatus string   `json:"pipelineStatus"`
	SagaStatus     string   `json:"sagaStatus"`
}

func invoiceResponse(item *invoicing.Invoice) (invoiceDTO, error) {
	amount := item.Total.Amount.String()
	if !item.Total.Amount.Valid() {
		return invoiceDTO{}, fmt.Errorf("invalid stored amount")
	}
	result := invoiceDTO{ModelVersion: item.ModelVersion, SourceFacts: item.SourceFacts, AccountingSnapshot: item.AccountingSnapshot, ReadinessReason: item.ReadinessReason,
		ID: item.ID, ClientID: item.ClientID, SupplierName: item.SupplierName,
		SupplierCUI: item.SupplierCUI, DocumentNumber: item.DocumentNumber,
		IssueDate: item.IssueDate.Format(time.RFC3339), Total: moneyDTO{Amount: json.RawMessage(amount), Currency: item.Total.Currency},
		SPVReference: item.SPVReference, PipelineStatus: string(item.PipelineStatus), SagaStatus: string(item.SagaStatus),
		Revision: item.Revision, CreatedAt: item.CreatedAt.Format(time.RFC3339), UpdatedAt: item.UpdatedAt.Format(time.RFC3339),
		Activity: make([]activityDTO, 0, len(item.Activity)),
		Lines:    make([]invoiceLineDTO, 0, len(item.Lines)),
	}
	if item.DueDate != nil {
		value := item.DueDate.Format(time.RFC3339)
		result.DueDate = &value
	}
	if item.ActiveTask != nil {
		task := validationTaskResponse(item.ActiveTask)
		result.Task = &task
	}
	if item.ContractAssociation != nil {
		id := item.ContractAssociation.ContractID
		result.SelectedContractID = &id
		result.Contract = associationContractResponse(item.ContractAssociation)
	}
	for _, event := range item.Activity {
		actor := string(event.ActorKind)
		if event.ActorDisplay != nil {
			actor = *event.ActorDisplay
		}
		result.Activity = append(result.Activity, activityDTO{
			ID: event.ID, Label: activityLabel(event.EventType), Actor: actor,
			Timestamp: event.OccurredAt.Format(time.RFC3339), Detail: event.Detail,
			Before: snapshotString(event.BeforeSnapshot), After: snapshotString(event.AfterSnapshot),
		})
	}
	for _, line := range item.Lines {
		for _, amount := range []string{line.VATRate.String(), line.VATValue.String(), line.Quantity.String(), line.UnitPrice.String(), line.NetValue.String(), line.TotalValue.String()} {
			if amount == "" {
				return invoiceDTO{}, fmt.Errorf("invalid stored line amount")
			}
		}
		result.Lines = append(result.Lines, invoiceLineDTO{SourceFacts: line.SourceFacts,
			ID: line.ID, Position: line.Position, Description: line.Description, Unit: line.Unit,
			VATRate: json.RawMessage(line.VATRate.String()), VATValue: moneyDTO{Amount: json.RawMessage(line.VATValue.String()), Currency: item.Total.Currency},
			Quantity: json.RawMessage(line.Quantity.String()), UnitPrice: moneyDTO{Amount: json.RawMessage(line.UnitPrice.String()), Currency: item.Total.Currency},
			NetValue: moneyDTO{Amount: json.RawMessage(line.NetValue.String()), Currency: item.Total.Currency}, TotalValue: moneyDTO{Amount: json.RawMessage(line.TotalValue.String()), Currency: item.Total.Currency}, AdditionalInfo: line.AdditionalInfo,
			Classifications: classificationResponses(line.Classifications),
		})
	}
	return result, nil
}

func activityLabel(eventType string) string {
	labels := map[string]string{
		"INVOICE_PIPELINE_TRANSITION":        "Pipeline actualizat",
		"SAGA_EXPORT_FAILED":                 "Export SAGA eșuat",
		"SAGA_EXPORT_ARTIFACT_GENERATED":     "Fișier SAGA generat",
		"SAGA_EXPORT_ARTIFACT_FAILED":        "Generare fișier SAGA eșuată",
		"SAGA_EXPORT_DOWNLOADED":             "Fișier SAGA descărcat",
		"SAGA_IMPORT_CONFIRMED":              "Import SAGA confirmat manual",
		"VALIDATION_TASK_CREATED":            "Revizuire solicitată",
		"MISSING_CONTRACT_REQUESTED":         "Contract solicitat",
		"MISSING_CONTRACT_REEVALUATED":       "Contract reevaluat automat",
		"MISSING_CONTRACT_RESOLVED":          "Contract lipsă rezolvat",
		"MISSING_CONTRACT_STILL_WAITING":     "Contract încă indisponibil",
		"CONTRACT_MATCHING_EXECUTED":         "Potrivire contract evaluată",
		"CONTRACT_AUTO_ASSOCIATED":           "Contract asociat automat",
		"CONTRACT_MATCH_TASK_RESOLVED":       "Task de contract rezolvat",
		"CONTRACT_CONFIRMED":                 "Contract confirmat",
		"AUTOMATED_CLASSIFICATION_COMPLETED": "Clasificare automată finalizată",
		"CLASSIFICATION_ROUTED":              "Clasificare evaluată",
		"CLASSIFICATION_TASK_RESOLVED":       "Task de clasificare rezolvat",
		"INVOICE_REVIEW_COMPLETED":           "Revizuire clasificare finalizată",
		"RULE_VERSION_CREATED":               "Versiune de regulă creată",
		"CLIENT_OVERRIDE_CREATED":            "Override client creat",
	}
	if label, ok := labels[eventType]; ok {
		return label
	}
	return eventType
}

func validationTaskResponse(item *validationtasks.Task) validationTaskDTO {
	result := validationTaskDTO{
		ID: item.ID, Type: string(item.Type), Status: string(item.Status),
		CreatedAt: item.CreatedAt.Format(time.RFC3339), UpdatedAt: item.UpdatedAt.Format(time.RFC3339),
		Title: item.Title, Reason: item.Reason, BlockerCode: item.BlockerCode, Revision: item.Revision,
		ContractRequested: item.Type == validationtasks.TypeMissingContract && item.Status == validationtasks.StatusWaiting,
	}
	if item.WaitingSince != nil {
		value := item.WaitingSince.Format(time.RFC3339)
		result.WaitingSince = &value
	}
	if item.ResolvedAt != nil {
		value := item.ResolvedAt.Format(time.RFC3339)
		result.ResolvedAt = &value
	}
	for _, candidate := range item.ContractCandidates {
		result.ContractCandidates = append(result.ContractCandidates, contractCandidateDTO{
			ID: candidate.ID, Reference: candidate.Reference, SupplierName: candidate.SupplierName,
			Period:     formatPeriod(candidate.EffectiveFrom, candidate.EffectiveTo),
			Value:      moneyDTO{Amount: json.RawMessage(candidate.ValueAmount), Currency: candidate.Currency},
			Confidence: candidate.Confidence, Reasons: candidate.Reasons, UnitType: candidate.UnitType,
			PaymentTerms: candidate.PaymentTerms, Recommended: candidate.Recommended,
		})
	}
	for _, classification := range item.ClassificationItems {
		result.ClassificationItems = append(result.ClassificationItems, classificationReviewResponse(classification))
	}
	return result
}

func classificationResponses(items []classification.Decision) []lineClassificationDTO {
	result := make([]lineClassificationDTO, 0, len(items))
	for _, item := range items {
		value := item.ProposedValue
		if item.EffectiveValue != nil {
			value = *item.EffectiveValue
		}
		result = append(result, lineClassificationDTO{ModelVersion: item.ModelVersion, TypedValue: item.TypedValue, ProposedTypedValue: item.ProposedTypedValue, Evidence: item.Evidence, ReviewReason: item.ReviewReason, InvoiceDateUsed: item.InvoiceDateUsed, HumanReviewed: item.HumanReviewed, ID: item.ID, Dimension: string(item.Dimension), Value: value, Confidence: item.Confidence, Explanation: item.Explanation, LegalBasis: item.LegalBasis, Status: string(item.Status), Revision: item.Revision, Rule: ruleReferenceResponse(item.Rule)})
	}
	return result
}

func classificationReviewResponse(item classification.Decision) classificationReviewItemDTO {
	return classificationReviewItemDTO{ModelVersion: item.ModelVersion, TypedValue: item.TypedValue, ProposedTypedValue: item.ProposedTypedValue, Evidence: item.Evidence, ReviewReason: item.ReviewReason, InvoiceDateUsed: item.InvoiceDateUsed, HumanReviewed: item.HumanReviewed, ID: item.ID, LineID: item.InvoiceLineID, LineLabel: item.LineLabel, Dimension: string(item.Dimension), ProposedValue: item.ProposedValue, Confidence: item.Confidence, Explanation: item.Explanation, LegalBasis: item.LegalBasis, Status: string(item.Status), ResolvedValue: item.EffectiveValue, Revision: item.Revision, Rule: ruleReferenceResponse(item.Rule)}
}

func ruleReferenceResponse(item *classification.RuleReference) *ruleReferenceDTO {
	if item == nil {
		return nil
	}
	return &ruleReferenceDTO{RuleVersionID: item.RuleVersionID, ProductionEligible: item.ProductionEligible, RulePackVersion: item.RulePackVersion, Provenance: item.Provenance, EffectiveFrom: item.EffectiveFrom, EffectiveTo: item.EffectiveTo, RuleID: item.RuleID, Reference: item.Reference, Version: item.Version, Origin: string(item.Origin)}
}

func ruleResponse(item *rules.Rule) ruleDTO {
	result := ruleDTO{ID: item.ID, Reference: item.Reference, Name: item.Name, Category: string(item.Category), Scope: string(item.Scope), ClientID: item.ClientID, ParentRuleID: item.ParentRuleID, Revision: item.Revision, Versions: make([]ruleVersionDTO, 0, len(item.Versions))}
	for _, version := range item.Versions {
		var to *string
		if version.EffectiveTo != nil {
			value := version.EffectiveTo.Format("2006-01-02")
			to = &value
		}
		result.Versions = append(result.Versions, ruleVersionDTO{ID: version.ID, ProductionEligible: version.ProductionEligible, RulePackVersion: version.RulePackVersion, Provenance: version.Provenance, Version: version.Version, EffectiveFrom: version.EffectiveFrom.Format("2006-01-02"), EffectiveTo: to, Criteria: version.Criteria, Result: version.Result, Explanation: version.Explanation, LegalBasis: version.LegalBasis, CreatedAt: version.CreatedAt.Format(time.RFC3339), Actor: version.CreatedByDisplay})
	}
	return result
}

func contractResponse(item *contracts.Contract) contractDTO {
	result := contractDTO{
		contractSummaryDTO: contractSummaryDTO{
			ID: item.ID, Reference: item.Reference, SupplierName: item.SupplierName,
			Period:   formatPeriod(item.EffectiveFrom, item.EffectiveTo),
			Value:    moneyDTO{Amount: json.RawMessage(item.Value.Amount.String()), Currency: item.Value.Currency},
			Currency: item.Value.Currency, UnitType: item.UnitType, PaymentTerms: item.PaymentTerms, HasLegacyTotalValue: item.HasLegacyTotalValue,
		},
		ClientID: item.ClientID, SupplierCUI: item.SupplierCUI, SourceReference: item.SourceReference,
		SourceDocumentID: item.SourceDocumentID, ExtractionAttemptID: item.ExtractionAttemptID,
		SourceMetadata: item.SourceMetadata, Revision: item.Revision, PeriodType: item.PeriodType, ServiceTerms: []contractServiceTermDTO{},
	}
	for _, term := range item.ServiceTerms {
		dto := contractServiceTermDTO{ServiceDescription: term.ServiceDescription, PricingModel: term.PricingModel, Currency: term.Currency, Unit: term.Unit, QuantitySource: term.QuantitySource, QuantityDriver: term.QuantityDriver, BillingFrequency: term.BillingFrequency, Evidence: term.EvidenceJSON}
		if term.UnitPrice != nil {
			value := term.UnitPrice.String()
			dto.UnitPrice = &value
		}
		if term.QuantityValue != nil {
			value := term.QuantityValue.String()
			dto.QuantityValue = &value
		}
		result.ServiceTerms = append(result.ServiceTerms, dto)
	}
	return result
}

func associationContractResponse(item *contracts.AssociationSnapshot) *contractSummaryDTO {
	return &contractSummaryDTO{
		ID: item.ContractID, Reference: item.Reference, SupplierName: item.SupplierName,
		Period:   formatPeriod(item.EffectiveFrom, item.EffectiveTo),
		Value:    moneyDTO{Amount: json.RawMessage(item.Value.Amount.String()), Currency: item.Value.Currency},
		Currency: item.Value.Currency, UnitType: item.UnitType, PaymentTerms: item.PaymentTerms, HasLegacyTotalValue: true,
	}
}

func contractInvoiceResponse(item contracts.AssociatedInvoice) contractInvoiceDTO {
	return contractInvoiceDTO{
		ID: item.ID, ClientID: item.ClientID, SupplierName: item.SupplierName, DocumentNumber: item.DocumentNumber,
		IssueDate: item.IssueDate.Format(time.RFC3339), Total: moneyDTO{Amount: json.RawMessage(item.Total.Amount.String()), Currency: item.Total.Currency},
		SPVReference: item.SPVReference, PipelineStatus: item.PipelineStatus, SagaStatus: item.SagaStatus,
	}
}

func formatPeriod(from time.Time, to *time.Time) string {
	if to == nil {
		return from.UTC().Format("02.01.2006") + " – nedeterminat"
	}
	return from.UTC().Format("02.01.2006") + " – " + to.UTC().Format("02.01.2006")
}

func validationTaskInboxResponse(item validationtasks.InboxItem) (validationTaskInboxDTO, error) {
	amount, err := money.Parse(item.Invoice.TotalAmount)
	if err != nil {
		return validationTaskInboxDTO{}, fmt.Errorf("invalid task invoice amount: %w", err)
	}
	return validationTaskInboxDTO{
		Task: validationTaskResponse(&item.Task),
		Invoice: taskInvoiceDTO{
			ID: item.Invoice.ID, ClientID: item.Invoice.ClientID, SupplierName: item.Invoice.SupplierName,
			SupplierCUI: item.Invoice.SupplierCUI, DocumentNumber: item.Invoice.DocumentNumber,
			IssueDate:    item.Invoice.IssueDate.Format(time.RFC3339),
			Total:        moneyDTO{Amount: json.RawMessage(amount.String()), Currency: item.Invoice.Currency},
			SPVReference: item.Invoice.SPVReference, PipelineStatus: item.Invoice.PipelineStatus, SagaStatus: item.Invoice.SagaStatus,
		},
		Client: clientDTO{ID: item.Client.ID, Name: item.Client.Name, CUI: item.Client.CUI},
	}, nil
}

func snapshotString(value json.RawMessage) *string {
	if len(value) == 0 {
		return nil
	}
	var result string
	if err := json.Unmarshal(value, &result); err != nil {
		result = string(value)
	}
	return &result
}
