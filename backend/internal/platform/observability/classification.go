package observability

import (
	"diana-contabilitate/backend/internal/classification"
	"fmt"
	"strings"
)

func (m *Metrics) ClassificationEvaluated(d classification.Dimension, evaluated, matches int, review bool) {
	index := classificationDimensionIndex(d)
	if index < 0 {
		return
	}
	m.classificationCounts[index][0].Add(uint64(max(evaluated, 0)))
	m.classificationCounts[index][1].Add(uint64(max(matches, 0)))
	if review {
		m.classificationCounts[index][2].Add(1)
	} else {
		m.classificationCounts[index][3].Add(1)
	}
}
func classificationDimensionIndex(d classification.Dimension) int {
	for i, value := range metricDimensions {
		if value == d {
			return i
		}
	}
	return -1
}
func (m *Metrics) writeClassificationMetrics(builder *strings.Builder) {
	fmt.Fprintf(builder, "accounting_readiness_evaluations_total{result=%q} %d\naccounting_readiness_evaluations_total{result=%q} %d\n", "ready", m.accountingReadiness[0].Load(), "blocked", m.accountingReadiness[1].Load())
	names := []string{"classification_rule_evaluations_total", "classification_rule_matches_total", "classification_review_required_total", "classification_production_rule_matches_total"}
	for i, d := range metricDimensions {
		for j, name := range names {
			fmt.Fprintf(builder, "%s{dimension=%q} %d\n", name, d, m.classificationCounts[i][j].Load())
		}
	}
}

var metricDimensions = []classification.Dimension{classification.DimensionAccount, classification.DimensionVAT, classification.DimensionDeductibility, classification.DimensionVATreatment, classification.DimensionVATDeductibility, classification.DimensionExpenseTax}

func (m *Metrics) AccountingReadinessEvaluated(ready bool) {
	i := 1
	if ready {
		i = 0
	}
	m.accountingReadiness[i].Add(1)
}
