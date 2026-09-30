// Package smartorder implements Smart Ordering (نظام الطلب الذكي): a pharmacy
// uploads a spreadsheet of what it needs, and the system resolves every row to a
// catalogue product, finds the vendors who can actually deliver it, picks one
// per line under the buyer's criteria, and hands back an order to review.
//
// See specs/001-smart-ordering-system for the specification. Three constraints
// from that work shape everything here:
//
//   - Supplier availability is read from catalog.product_variants and
//     inventory.stocks, never from catalog.product_index's stock columns.
//   - The deterministic matcher is the primary engine. AI resolves only what it
//     leaves below the cutoff, in capped batches, and never overrides it.
//   - Money is money.Amount throughout. A float in a price path is a defect.
package smartorder

import (
	"encoding/json"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// RunStatus is where a run sits in its lifecycle.
type RunStatus string

const (
	StatusDraft      RunStatus = "draft"      // file uploaded, being configured
	StatusMapping    RunStatus = "mapping"    // columns detected, awaiting confirmation
	StatusQueued     RunStatus = "queued"     // handed to the worker
	StatusProcessing RunStatus = "processing" // pipeline running
	StatusCompleted  RunStatus = "completed"  // results ready to review
	StatusStale      RunStatus = "stale"      // configuration changed after results
	StatusFinalizing RunStatus = "finalizing" // re-verifying before placing
	StatusPlaced     RunStatus = "placed"     // terminal: an order exists
	StatusFailed     RunStatus = "failed"     // terminal
)

// MatchMethod records which tier resolved a line. It is shown to the buyer, so
// they know which decisions deserve a second look — an `ai` match earns more
// scrutiny than a `barcode` one.
type MatchMethod string

const (
	MethodSavingProduct  MatchMethod = "saving_product"
	MethodLearnedMapping MatchMethod = "learned_mapping"
	MethodBarcode        MatchMethod = "barcode"
	MethodSKU            MatchMethod = "sku"
	MethodExactName      MatchMethod = "exact_name"
	MethodIdentityKey    MatchMethod = "identity_key"
	MethodFuzzy          MatchMethod = "fuzzy"
	MethodAlias          MatchMethod = "alias"
	MethodAI             MatchMethod = "ai"
	MethodManual         MatchMethod = "manual"
	MethodNone           MatchMethod = "none"
)

// Outcome is why a line will or will not be ordered.
//
// The distinctions matter to the buyer and are not cosmetic: "nobody sells this"
// sends them to a different supplier, "your coverage window closed" means try
// again tomorrow, and "restricted by Corporate Operations" is a permissions
// question. Collapsing these into "unavailable" is what makes an import report
// useless.
type Outcome string

const (
	OutcomeOrdered              Outcome = "ordered"
	OutcomeNoSupplier           Outcome = "no_supplier"
	OutcomeCoverageBlocked      Outcome = "coverage_blocked"
	OutcomeInstitutionalBlocked Outcome = "institutional_blocked"
	OutcomeOutOfStock           Outcome = "out_of_stock"
	OutcomeBelowMinQty          Outcome = "below_min_qty"
	OutcomeQuotaBlocked         Outcome = "quota_blocked"
	OutcomeUnmatched            Outcome = "unmatched"
	OutcomeZeroQty              Outcome = "zero_qty"
	OutcomeRemoved              Outcome = "removed"
	OutcomeBlockedPack          Outcome = "blocked_pack"
)

// Orderable reports whether a line contributes to the order and its total.
func (o Outcome) Orderable() bool { return o == OutcomeOrdered }

// IneligibleReason is why one vendor's offer was excluded.
type IneligibleReason string

const (
	ReasonOwnOrg        IneligibleReason = "own_org"
	ReasonInactive      IneligibleReason = "inactive"
	ReasonInstitutional IneligibleReason = "institutional"
	ReasonCoverage      IneligibleReason = "coverage"
	ReasonNoLocation    IneligibleReason = "branch_no_location"
	ReasonStock         IneligibleReason = "stock"
	ReasonMinQty        IneligibleReason = "min_qty"
	ReasonQuota         IneligibleReason = "quota"
	ReasonBlockedPack   IneligibleReason = "blocked_pack"
)

// OfferState records the offer resolution state for an identified line.
type OfferState string

const (
	OfferStateOffered           OfferState = "offered"
	OfferStateIdentifiedNoOffer OfferState = "identified_no_offer"
	OfferStateBlockedPack       OfferState = "blocked_pack"
)

// Run is one end-to-end execution.
type Run struct {
	ID             int64     `json:"id"`
	PublicID       string    `json:"public_id"`
	RunNumber      string    `json:"run_number"`
	OrganizationID int64     `json:"organization_id"`
	UserID         int64     `json:"user_id"`
	BranchID       int64     `json:"branch_id"`
	UploadID       *int64    `json:"upload_id,omitempty"`
	OriginalFile   string    `json:"original_filename"`
	Status         RunStatus `json:"status"`
	CurrentStep    int       `json:"current_step"`

	Stats Stats `json:"stats"`

	EstimatedTotal money.Amount  `json:"estimated_total"`
	BudgetExceeded *bool         `json:"budget_exceeded,omitempty"`
	BudgetOverage  *money.Amount `json:"budget_overage,omitempty"`

	OrderID     *int64     `json:"order_id,omitempty"`
	FinalizedAt *time.Time `json:"finalized_at,omitempty"`

	AI AIUsage `json:"ai"`

	DeterministicMS *int   `json:"deterministic_ms,omitempty"`
	TotalMS         *int   `json:"total_ms,omitempty"`
	FailureReason   string `json:"failure_reason,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Stats are the per-outcome counters shown on the results screen.
type Stats struct {
	TotalRows                int `json:"total_rows"`
	MatchedRows              int `json:"matched_rows"`
	UnmatchedRows            int `json:"unmatched_rows"`
	NoSupplierRows           int `json:"no_supplier_rows"`
	CoverageBlockedRows      int `json:"coverage_blocked_rows"`
	InstitutionalBlockedRows int `json:"institutional_blocked_rows"`
	BelowMinQtyRows          int `json:"below_min_qty_rows"`
	QuotaBlockedRows         int `json:"quota_blocked_rows"`
}

// FilterCounts represents exact counts for each independent smart order status.
type FilterCounts struct {
	Total           int `json:"total"`
	Unmatched       int `json:"unmatched"`
	MatchedProduct  int `json:"matched_product"`
	MatchedSupplier int `json:"matched_supplier"`
	AvailableStock  int `json:"available_stock"`
	CoveredBranch   int `json:"covered_branch"`
	PriceAvailable  int `json:"price_available"`
	ReadyToOrder    int `json:"ready_to_order"`
}

// BlockedCounts is the run's lines that will NOT be ordered, split by why.
//
// It exists because "not ready" is not one problem. A line with no supplier is
// a sourcing question, a line out of stock is a timing question, and a line
// nothing matched is a data question — and they are answered by different
// people at different moments. Lumping them into one number told a buyer only
// that something was wrong.
type BlockedCounts struct {
	Unmatched            int `json:"unmatched"`
	NoSupplier           int `json:"no_supplier"`
	CoverageBlocked      int `json:"coverage_blocked"`
	InstitutionalBlocked int `json:"institutional_blocked"`
	OutOfStock           int `json:"out_of_stock"`
	BelowMinQty          int `json:"below_min_qty"`
	ZeroQty              int `json:"zero_qty"`
	Removed              int `json:"removed"`
	QuotaBlocked         int `json:"quota_blocked"`
	BlockedPack          int `json:"blocked_pack"`
	IdentifiedNoOffer    int `json:"identified_no_offer"`
}

// Total is every line that will not be ordered, excluding the ones the buyer
// removed on purpose — those are not a problem to be solved.
func (b BlockedCounts) Total() int {
	return b.Unmatched + b.NoSupplier + b.CoverageBlocked + b.InstitutionalBlocked +
		b.OutOfStock + b.BelowMinQty + b.ZeroQty + b.QuotaBlocked + b.BlockedPack
}


// AIUsage is telemetry, not commerce.
//
// CostEstimate is USD with six decimals and deliberately not money.Amount:
// that type is EGP minor units for orders, and conflating the two is how a
// gateway bill ends up on an invoice.
//
// LinesImproved is the number that answers the buyer's actual question — "did
// this do anything for me?" — and it counts lines whose matched product changed,
// not lines the model replied about. A run that reviewed four hundred lines and
// improved three should read as exactly that.
type AIUsage struct {
	Enabled bool `json:"enabled"`
	Calls   int  `json:"calls"`
	// LinesReviewed is how many lines were sent, after the decision cache had
	// answered everything it could.
	LinesReviewed int `json:"lines_reviewed"`
	// LinesAdjudicated is how many lines the model actually answered.
	LinesAdjudicated int `json:"lines_adjudicated"`
	// LinesImproved is how many ended the stage matched to a different product
	// than the deterministic engine left them on.
	LinesImproved int `json:"lines_improved"`
	// LinesVerified is how many lines the deterministic engine had already
	// settled and the model was asked to check anyway.
	//
	// It is reported because it is what makes LinesDisputed a rate rather than
	// a number, and because a buyer looking at a run wants to know that the
	// confident matches were looked at too.
	LinesVerified int `json:"lines_verified"`
	// LinesDisputed is how many of those the model would not confirm.
	//
	// Each one is a line where two independent methods disagreed, and each one
	// is now waiting for the buyer rather than sitting in the order. It is the
	// most useful number this stage produces: everything else describes what
	// was improved, and this describes what was caught.
	LinesDisputed int     `json:"lines_disputed"`
	CacheHits     int     `json:"cache_hits"`
	CostEstimate  float64 `json:"cost_estimate"`
	CeilingHit    bool    `json:"ceiling_hit"`
}

// Improved reports whether the AI stage changed anything the buyer can see.
func (a AIUsage) Improved() bool { return a.LinesImproved > 0 }

// Line is one imported spreadsheet row.
type Line struct {
	ID             int64 `json:"id"`
	RunID          int64 `json:"run_id"`
	OrganizationID int64 `json:"organization_id"`
	RowNumber      int   `json:"row_number"`

	Raw        map[string]string `json:"raw"`
	RawName    string            `json:"raw_name"`
	RawSKU     string            `json:"raw_sku"`
	RawBarcode string            `json:"raw_barcode"`

	// ImportedQty is nil when the cell was blank or unreadable. QtyParseNote
	// says which, because silently turning "2-3" into 2 is how a pharmacy ends
	// up with the wrong order and no way to see why.
	ImportedQty  *float64 `json:"imported_qty,omitempty"`
	QtyParseNote string   `json:"qty_parse_note,omitempty"`
	EditedQty    *float64 `json:"edited_qty,omitempty"`
	EffectiveQty float64  `json:"effective_qty"`

	NormName    string `json:"norm_name"`
	IdentityKey string `json:"identity_key"`

	MatchedProductID *int64      `json:"matched_product_id,omitempty"`
	MatchMethod      MatchMethod `json:"match_method"`
	MatchConfidence  float64     `json:"match_confidence"`
	CorrectedByUser  bool        `json:"match_corrected_by_user"`

	// MatchedProductName is the catalogue's own name for the matched product.
	// It is display-only and is NOT stored on the line: names are edited in the
	// catalogue, and a copy taken at match time would go stale and quietly
	// disagree with the product page the buyer opens next. Service.Results
	// resolves it for whatever page is being rendered.
	MatchedProductName string `json:"matched_product_name,omitempty"`

	Outcome       Outcome    `json:"outcome"`
	OutcomeReason string     `json:"outcome_reason,omitempty"`
	OfferState    OfferState `json:"offer_state,omitempty"`

	// Matching consumer fields (migration 236)
	MatchState       string          `json:"match_state,omitempty"`
	MatchBasis       string          `json:"match_basis,omitempty"`
	MatchGranularity string          `json:"match_granularity,omitempty"`
	MatchRevision    int             `json:"match_revision,omitempty"`
	HumanLocked      bool            `json:"human_locked,omitempty"`
	AIState          string          `json:"ai_state,omitempty"`
	MatchRunID       *int64          `json:"match_run_id,omitempty"`
	MatchDecision    json.RawMessage `json:"match_decision,omitempty"`
	BaselineDecision json.RawMessage `json:"baseline_decision,omitempty"`

	ConsolidatedInto *int64 `json:"consolidated_into_line_id,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Matched reports whether the line resolved to a catalogue product.
func (l *Line) Matched() bool { return l.MatchedProductID != nil && *l.MatchedProductID > 0 }

