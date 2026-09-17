package observability

import (
	"diana-contabilitate/backend/internal/classification"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClassificationMetricsBoundedDimensions(t *testing.T) {
	m := NewMetrics()
	m.ClassificationEvaluated(classification.DimensionVAT, 2, 1, false)
	m.ClassificationEvaluated(classification.DimensionAccount, 0, 0, true)
	m.ClassificationEvaluated("client-secret", 99, 99, false)
	response := httptest.NewRecorder()
	m.Handler(nil).ServeHTTP(response, httptest.NewRequest("GET", "/metrics", nil))
	body := response.Body.String()
	for _, expected := range []string{`classification_rule_evaluations_total{dimension="VAT"} 2`, `classification_production_rule_matches_total{dimension="VAT"} 1`, `classification_review_required_total{dimension="ACCOUNT"} 1`} {
		if !strings.Contains(body, expected) {
			t.Fatalf("missing %s", expected)
		}
	}
	if strings.Contains(body, "client-secret") {
		t.Fatal("unbounded dimension label")
	}
}

func TestAccountingReadinessMetricsAndFourDimensions(t *testing.T) {
	m := NewMetrics()
	m.AccountingReadinessEvaluated(true)
	m.AccountingReadinessEvaluated(false)
	for _, d := range []classification.Dimension{classification.DimensionVATreatment, classification.DimensionVATDeductibility, classification.DimensionExpenseTax} {
		m.ClassificationEvaluated(d, 1, 0, true)
	}
	response := httptest.NewRecorder()
	m.Handler(nil).ServeHTTP(response, httptest.NewRequest("GET", "/metrics", nil))
	body := response.Body.String()
	for _, expected := range []string{`accounting_readiness_evaluations_total{result="ready"} 1`, `accounting_readiness_evaluations_total{result="blocked"} 1`, `classification_review_required_total{dimension="VAT_DEDUCTIBILITY"} 1`} {
		if !strings.Contains(body, expected) {
			t.Fatal(expected, body)
		}
	}
}
