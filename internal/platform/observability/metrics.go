// Package observability provides metrics instrumentation and telemetry for Dawa24.
package observability

import (
	"fmt"
	"io"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// MetricType indicates the kind of metric.
type MetricType string

const (
	MetricTypeCounter   MetricType = "counter"
	MetricTypeGauge     MetricType = "gauge"
	MetricTypeHistogram MetricType = "histogram"
)

// Counter is a monotonically increasing 64-bit integer counter.
type Counter struct {
	val uint64
}

func (c *Counter) Inc() {
	atomic.AddUint64(&c.val, 1)
}

func (c *Counter) Add(delta uint64) {
	atomic.AddUint64(&c.val, delta)
}

func (c *Counter) Value() uint64 {
	return atomic.LoadUint64(&c.val)
}

// Gauge represents a numerical value that can go up and down.
type Gauge struct {
	mu  sync.RWMutex
	val float64
}

func (g *Gauge) Set(val float64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.val = val
}

func (g *Gauge) Value() float64 {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.val
}

// Histogram tracks sample values and computes basic percentiles (p50, p95, p99).
type Histogram struct {
	mu      sync.RWMutex
	samples []float64
	sum     float64
	count   uint64
}

func (h *Histogram) Observe(val float64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.samples = append(h.samples, val)
	h.sum += val
	h.count++
}

func (h *Histogram) Snapshot() (count uint64, sum float64, p50 float64, p95 float64, p99 float64) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	count = h.count
	sum = h.sum
	if len(h.samples) == 0 {
		return count, sum, 0, 0, 0
	}

	copied := make([]float64, len(h.samples))
	copy(copied, h.samples)
	sort.Float64s(copied)

	p50 = percentile(copied, 0.50)
	p95 = percentile(copied, 0.95)
	p99 = percentile(copied, 0.99)
	return count, sum, p50, p95, p99
}

func percentile(sorted []float64, p float64) float64 {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	idx := int(float64(n)*p + 0.999999) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= n {
		idx = n - 1
	}
	return sorted[idx]
}

// MatchingMetrics holds operational counters and histograms for product matching (US13 T153).
type MatchingMetrics struct {
	// HardIdentityViolations counts identity mismatches flagged by reviewers.
	HardIdentityViolations Counter
	// TotalReviews counts total review actions recorded.
	TotalReviews Counter
	// TotalCorrections counts human review corrections.
	TotalCorrections Counter
	// ReviewableDuration tracks latency from start to reviewable state.
	ReviewableDuration Histogram
	// AIDuration tracks AI enhancement runtime.
	AIDuration Histogram
	// StaleKnowledge counts knowledge entries marked stale.
	StaleKnowledge Counter
	// TotalSpendEGP tracks total EGP spent on AI calls.
	TotalSpendEGP Gauge
	// SpendCeilingEGP tracks current spend ceiling in EGP.
	SpendCeilingEGP Gauge
}

// GlobalMatchingMetrics is the package-level metrics registry instance.
var GlobalMatchingMetrics = &MatchingMetrics{}

// IncHardIdentityViolations records a reviewer-flagged identity violation.
func IncHardIdentityViolations() {
	GlobalMatchingMetrics.HardIdentityViolations.Inc()
}

// RecordReviewAction records a review decision and whether it was a correction.
func RecordReviewAction(wasCorrection bool) {
	GlobalMatchingMetrics.TotalReviews.Inc()
	if wasCorrection {
		GlobalMatchingMetrics.TotalCorrections.Inc()
	}
}

// ObserveReviewableDuration records the time taken to reach reviewable state.
func ObserveReviewableDuration(d time.Duration) {
	GlobalMatchingMetrics.ReviewableDuration.Observe(d.Seconds())
}

// ObserveAIDuration records AI enhancement run duration.
func ObserveAIDuration(d time.Duration) {
	GlobalMatchingMetrics.AIDuration.Observe(d.Seconds())
}

// IncStaleKnowledge records an invalidation of matching knowledge.
func IncStaleKnowledge() {
	GlobalMatchingMetrics.StaleKnowledge.Inc()
}

// SetSpendTracking records spend and spend ceiling in EGP.
func SetSpendTracking(spentEGP, ceilingEGP float64) {
	GlobalMatchingMetrics.TotalSpendEGP.Set(spentEGP)
	GlobalMatchingMetrics.SpendCeilingEGP.Set(ceilingEGP)
}

// WriteMetrics writes Prometheus-compatible metric lines to the given writer.
func (m *MatchingMetrics) WriteMetrics(w io.Writer) {
	fmt.Fprintf(w, "# HELP matching_hard_identity_violations_total Total hard identity violations reported\n")
	fmt.Fprintf(w, "# TYPE matching_hard_identity_violations_total counter\n")
	fmt.Fprintf(w, "matching_hard_identity_violations_total %d\n\n", m.HardIdentityViolations.Value())

	fmt.Fprintf(w, "# HELP matching_reviews_total Total review decisions processed\n")
	fmt.Fprintf(w, "# TYPE matching_reviews_total counter\n")
	fmt.Fprintf(w, "matching_reviews_total %d\n\n", m.TotalReviews.Value())

	fmt.Fprintf(w, "# HELP matching_corrections_total Total reviewer corrections applied\n")
	fmt.Fprintf(w, "# TYPE matching_corrections_total counter\n")
	fmt.Fprintf(w, "matching_corrections_total %d\n\n", m.TotalCorrections.Value())

	count, sum, p50, p95, p99 := m.ReviewableDuration.Snapshot()
	fmt.Fprintf(w, "# HELP matching_reviewable_duration_seconds Latency to reviewable state\n")
	fmt.Fprintf(w, "# TYPE matching_reviewable_duration_seconds summary\n")
	fmt.Fprintf(w, "matching_reviewable_duration_seconds{quantile=\"0.50\"} %f\n", p50)
	fmt.Fprintf(w, "matching_reviewable_duration_seconds{quantile=\"0.95\"} %f\n", p95)
	fmt.Fprintf(w, "matching_reviewable_duration_seconds{quantile=\"0.99\"} %f\n", p99)
	fmt.Fprintf(w, "matching_reviewable_duration_seconds_sum %f\n", sum)
	fmt.Fprintf(w, "matching_reviewable_duration_seconds_count %d\n\n", count)

	aiCount, aiSum, aiP50, aiP95, aiP99 := m.AIDuration.Snapshot()
	fmt.Fprintf(w, "# HELP matching_ai_duration_seconds AI enhancement run duration\n")
	fmt.Fprintf(w, "# TYPE matching_ai_duration_seconds summary\n")
	fmt.Fprintf(w, "matching_ai_duration_seconds{quantile=\"0.50\"} %f\n", aiP50)
	fmt.Fprintf(w, "matching_ai_duration_seconds{quantile=\"0.95\"} %f\n", aiP95)
	fmt.Fprintf(w, "matching_ai_duration_seconds{quantile=\"0.99\"} %f\n", aiP99)
	fmt.Fprintf(w, "matching_ai_duration_seconds_sum %f\n", aiSum)
	fmt.Fprintf(w, "matching_ai_duration_seconds_count %d\n\n", aiCount)

	fmt.Fprintf(w, "# HELP matching_stale_knowledge_total Total stale knowledge entries invalidated\n")
	fmt.Fprintf(w, "# TYPE matching_stale_knowledge_total counter\n")
	fmt.Fprintf(w, "matching_stale_knowledge_total %d\n\n", m.StaleKnowledge.Value())

	fmt.Fprintf(w, "# HELP matching_spend_egp_total Total EGP spent on AI enhancement\n")
	fmt.Fprintf(w, "# TYPE matching_spend_egp_total gauge\n")
	fmt.Fprintf(w, "matching_spend_egp_total %f\n\n", m.TotalSpendEGP.Value())

	fmt.Fprintf(w, "# HELP matching_spend_ceiling_egp Current EGP budget ceiling\n")
	fmt.Fprintf(w, "# TYPE matching_spend_ceiling_egp gauge\n")
	fmt.Fprintf(w, "matching_spend_ceiling_egp %f\n", m.SpendCeilingEGP.Value())
}

// MetricsHandler serves an HTTP endpoint exporting the matching metrics.
func MetricsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		GlobalMatchingMetrics.WriteMetrics(w)
	})
}
