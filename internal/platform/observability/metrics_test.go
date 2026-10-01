package observability

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestMetrics_CountersAndGauges(t *testing.T) {
	m := &MatchingMetrics{}

	m.HardIdentityViolations.Inc()
	m.HardIdentityViolations.Add(2)
	assert.Equal(t, uint64(3), m.HardIdentityViolations.Value())

	m.TotalReviews.Inc()
	m.TotalCorrections.Inc()
	assert.Equal(t, uint64(1), m.TotalReviews.Value())
	assert.Equal(t, uint64(1), m.TotalCorrections.Value())

	m.TotalSpendEGP.Set(45.50)
	m.SpendCeilingEGP.Set(100.00)
	assert.Equal(t, 45.50, m.TotalSpendEGP.Value())
	assert.Equal(t, 100.00, m.SpendCeilingEGP.Value())
}

func TestMetrics_HistogramSnapshot(t *testing.T) {
	h := &Histogram{}
	h.Observe(1.0)
	h.Observe(2.0)
	h.Observe(3.0)
	h.Observe(4.0)
	h.Observe(5.0)

	count, sum, p50, p95, p99 := h.Snapshot()
	assert.Equal(t, uint64(5), count)
	assert.Equal(t, 15.0, sum)
	assert.Equal(t, 3.0, p50)
	assert.Equal(t, 5.0, p95)
	assert.Equal(t, 5.0, p99)
}

func TestMetrics_ExpositionAndHandler(t *testing.T) {
	IncHardIdentityViolations()
	RecordReviewAction(true)
	RecordReviewAction(false)
	ObserveReviewableDuration(1200 * time.Millisecond)
	ObserveAIDuration(3500 * time.Millisecond)
	IncStaleKnowledge()
	SetSpendTracking(12.5, 50.0)

	var buf bytes.Buffer
	GlobalMatchingMetrics.WriteMetrics(&buf)
	out := buf.String()

	assert.Contains(t, out, "matching_hard_identity_violations_total")
	assert.Contains(t, out, "matching_reviews_total")
	assert.Contains(t, out, "matching_corrections_total")
	assert.Contains(t, out, "matching_reviewable_duration_seconds")
	assert.Contains(t, out, "matching_ai_duration_seconds")
	assert.Contains(t, out, "matching_stale_knowledge_total")
	assert.Contains(t, out, "matching_spend_egp_total")
	assert.Contains(t, out, "matching_spend_ceiling_egp")

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	MetricsHandler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "matching_hard_identity_violations_total")
}
