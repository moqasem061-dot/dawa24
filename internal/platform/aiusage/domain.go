// Package aiusage stores and reads the Store's own record of AI consumption.
//
// It sits beside the Gateway rather than inside it because the two answer
// different questions. The Gateway knows what a request cost right now; this
// knows what a منشأة has spent over the last month, on which feature, and can
// still say so when the Gateway is unreachable. The screens that report usage
// to a pharmacy read from here; the Gateway is consulted only for the live
// budget window, which is genuinely its to publish.
package aiusage

import (
	"context"
	"time"
)

// Entry is one recorded AI call, as a screen reads it back.
//
// It is a separate type from gateway.UsageEvent on purpose: the write side is
// what a call produced, the read side is what a table row holds, and conflating
// them is how an id and a created_at end up being optional fields on a struct
// that is sometimes neither stored nor loaded.
type Entry struct {
	ID             int64
	OrganizationID int64
	UserID         int64
	Capability     string
	Feature        string
	Model          string
	RequestID      string
	InputTokens    int
	OutputTokens   int
	CostNanoUSD    int64
	CostKnown      bool
	DurationMS     int
	Status         string
	FinishReason   string
	ErrorMessage   string
	FromCache      bool
	Fallback       bool
	CreatedAt      time.Time
}

// TotalTokens is what the tenant was charged tokens for on this call.
func (e Entry) TotalTokens() int { return e.InputTokens + e.OutputTokens }

// CostUSD renders the recorded cost in dollars.
//
// Callers must check CostKnown first. A zero here means the Gateway published
// no price, and presenting that as "$0.00 spent" is the same class of mistake
// as the invented per-token rates this ledger replaced.
func (e Entry) CostUSD() float64 { return float64(e.CostNanoUSD) / 1e9 }

// Filter narrows a listing.
type Filter struct {
	OrganizationID int64
	// Since bounds the start of the window. Zero means the whole retained history.
	Since time.Time
	// Until bounds the end of the window. Zero means unbounded.
	Until time.Time
	// Feature, when set, restricts to one screen or tool.
	Feature string
	// Model, when set, restricts to one model name or alias.
	Model string
	// Status, when set, restricts to one outcome — which is how an operator
	// finds every call that timed out or was refused for quota.
	Status string
	// Search queries gateway_request_id, error_message, feature, or model.
	Search string
	Limit  int
	Offset int
}

// Summary is the aggregate the usage cards draw.
type Summary struct {
	Requests int
	// Succeeded counts calls that reached a model and returned an answer.
	Succeeded int
	Failed    int
	// Cached counts answers the Gateway served without billing.
	Cached int
	// FellBack counts calls that never reached a model at all, so the
	// deterministic path answered. Reported separately because it measures
	// availability, not consumption.
	FellBack int

	InputTokens  int64
	OutputTokens int64

	CostNanoUSD int64
	// PricedRequests is how many of the requests carried a published price.
	// When it is below Requests, the cost below is a floor rather than a total,
	// and a screen that does not say so is overstating its own certainty.
	PricedRequests int

	Since time.Time
	Until time.Time
}

// TotalTokens is the summed token count over the window.
func (s Summary) TotalTokens() int64 { return s.InputTokens + s.OutputTokens }

// CostUSD renders the summed cost in dollars.
func (s Summary) CostUSD() float64 { return float64(s.CostNanoUSD) / 1e9 }

// CostIsComplete reports whether every request in the window carried a price.
func (s Summary) CostIsComplete() bool {
	return s.Requests == 0 || s.PricedRequests >= s.Requests
}

// FeatureUsage is one row of the per-feature breakdown.
type FeatureUsage struct {
	Feature     string
	Requests    int
	TotalTokens int64
	CostNanoUSD int64
}

// CostUSD renders this feature's share of the bill in dollars.
func (f FeatureUsage) CostUSD() float64 { return float64(f.CostNanoUSD) / 1e9 }

// Repository is the storage port.
type Repository interface {
	// Insert writes one recorded call.
	Insert(ctx context.Context, e Entry) error
	// List returns matching entries, newest first.
	List(ctx context.Context, f Filter) ([]Entry, int, error)
	// Summarize aggregates a window.
	Summarize(ctx context.Context, orgID int64, since time.Time) (Summary, error)
	// ByFeature breaks a window down by the screen that spent it.
	ByFeature(ctx context.Context, orgID int64, since time.Time) ([]FeatureUsage, error)
}
