package saga

import (
	"crypto/sha256"
	"diana-contabilitate/backend/internal/accounting"
	"diana-contabilitate/backend/internal/invoicing"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
)

func canonicalValue(v accounting.Value) string { b, _ := json.Marshal(v); return string(b) }
func Generate(item *invoicing.Invoice, client ClientIdentity) (Artifact, error) {
	return generate(item, client, false)
}

// GenerateTestOnly explicitly permits synthetic mappings. Production callers do
// not use this function and cannot enable it with an environment flag.
func GenerateTestOnly(item *invoicing.Invoice, client ClientIdentity) (Artifact, error) {
	return generate(item, client, true)
}
func generate(item *invoicing.Invoice, client ClientIdentity, allowTest bool) (Artifact, error) {
	return generateWith(item, client, allowTest, accounting.DefaultSAGAOutgoingMapping)
}

func generateWith(item *invoicing.Invoice, client ClientIdentity, allowTest bool, outgoingMapping accounting.MappingPolicy) (Artifact, error) {
	if item == nil || item.ModelVersion != accounting.ModelVersion {
		return generateLegacy(item, client)
	}
	if item.PipelineStatus != invoicing.StatusReadyForSAGA && item.PipelineStatus != invoicing.StatusExporting {
		return Artifact{}, invalid("Factura nu este într-o stare exportabilă.", nil)
	}
	ready := evaluateReadiness(item, client, allowTest, false, outgoingMapping)
	if !ready.Ready {
		return Artifact{}, invalid(ready.Reason, nil)
	}
	tag := invoiceTag{ID: item.ID, Header: headerTag{SupplierName: item.SupplierName, SupplierCIF: *item.SupplierCUI, ClientName: client.Name, ClientCIF: client.CUI, Number: item.DocumentNumber, Date: item.IssueDate.Format("02.01.2006")}}
	if item.Outgoing() {
		// SAGA routes the file to Ieșiri because FurnizorCIF is the company.
		// The customer identifier is written unmasked (it may be a CNP).
		tag.Header = headerTag{SupplierName: client.Name, SupplierCIF: client.CUI, ClientName: *item.CustomerName, ClientCIF: *item.CustomerIdentifier, Number: item.DocumentNumber, Date: item.IssueDate.Format("02.01.2006"), CashAccounting: "Nu"}
		if item.AccountingSnapshot.Profile.CashAccounting == "YES" {
			tag.Header.CashAccounting = "Da"
		}
	}
	if item.DueDate != nil {
		tag.Header.DueDate = item.DueDate.Format("02.01.2006")
	}
	snapshot := map[string]string{"model_version": accounting.ModelVersion, "mapping_version": ready.MappingVersion}
	context, err := json.Marshal(item.AccountingSnapshot)
	if err != nil {
		return Artifact{}, err
	}
	snapshot["accounting_snapshot"] = string(context)
	for _, l := range item.Lines {
		tag.Details.Content.Lines = append(tag.Details.Content.Lines, ready.Lines[l.ID])
		for _, d := range l.Classifications {
			if d.ModelVersion == accounting.ModelVersion {
				b, _ := json.Marshal(struct {
					Value    *accounting.Value
					Evidence *accounting.Evidence
					Reason   string
					Human    bool
				}{d.TypedValue, d.Evidence, d.ReviewReason, d.HumanReviewed})
				snapshot[d.ID] = string(b)
			}
		}
	}
	b, err := xml.MarshalIndent(invoiceXML{Invoices: []invoiceTag{tag}}, "", "  ")
	if err != nil {
		return Artifact{}, err
	}
	payload := append([]byte(xml.Header), b...)
	payload = append(payload, '\n')
	if err = validateStructure(payload); err != nil {
		return Artifact{}, err
	}
	sum := sha256.Sum256(payload)
	return Artifact{Filename: fmt.Sprintf("F_%s_%s_%s.xml", filenamePart(tag.Header.SupplierCIF), filenamePart(item.DocumentNumber), item.IssueDate.Format("02.01.2006")), ContentType: ContentType, Payload: payload, SHA256: hex.EncodeToString(sum[:]), ExporterVersion: exportVersion(item), ClassificationSnapshot: snapshot}, nil
}
func exportVersion(item *invoicing.Invoice) string {
	if item != nil && item.ModelVersion == accounting.ModelVersion {
		v := DomainExporterVersion
		if item.Outgoing() {
			v += "/" + accounting.DefaultSAGAOutgoingMapping.Version
		} else if item.AccountingSnapshot != nil && item.AccountingSnapshot.Pack != nil {
			v += "/" + item.AccountingSnapshot.Pack.Mapping.Version
		} else {
			v += "/" + accounting.DefaultSAGAMapping.Version
		}
		return v
	}
	return ExporterVersion
}
