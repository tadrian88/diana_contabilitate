package postgres

import (
	"context"
	"diana-contabilitate/backend/ent"
	"diana-contabilitate/backend/ent/invoice"
	"diana-contabilitate/backend/ent/invoiceline"
	"diana-contabilitate/backend/internal/saga"
)

func evaluateAccountingReadiness(ctx context.Context, tx *ent.Tx, id string, allowTest bool) (saga.Readiness, error) {
	row, err := tx.Invoice.Query().Where(invoice.IDEQ(id)).WithLines(func(q *ent.InvoiceLineQuery) {
		q.WithClassifications(func(c *ent.LineClassificationQuery) {
			c.WithRuleVersion(func(v *ent.RuleVersionQuery) { v.WithRule() })
		}).Order(ent.Asc(invoiceline.FieldPosition))
	}).Only(ctx)
	if err != nil {
		return saga.Readiness{}, err
	}
	item, err := invoiceDomain(row)
	if err != nil {
		return saga.Readiness{}, err
	}
	client, err := tx.AccountingClient.Get(ctx, row.ClientID)
	if err != nil {
		return saga.Readiness{}, err
	}
	return saga.EvaluateReadiness(item, saga.ClientIdentity{ID: client.ID, Name: client.Name, CUI: client.Cui}, allowTest, true), nil
}
