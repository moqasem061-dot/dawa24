package smartorder

import (
	"time"

	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// Candidate is one vendor's offer of a matched product.
//
// Ineligible candidates are kept, not discarded. The buyer's first question
// about any line is "why not that supplier", and recomputing the answer means
// re-running coverage against a clock that has since moved.
type Candidate struct {
	ID             int64  `json:"id"`
	LineID         int64  `json:"line_id"`
	OrganizationID int64  `json:"organization_id"` // the buyer's org, not the vendor's
	VendorOrgID    int64  `json:"vendor_org_id"`
	VariantID      int64  `json:"variant_id"`
	BranchID       *int64 `json:"branch_id,omitempty"`

	Price        money.Amount `json:"price"`
	DiscountBps  int64        `json:"discount_bps"` // basis points: 1250 == 12.50%
	NetUnitPrice money.Amount `json:"net_unit_price"`
	Unit         string       `json:"unit,omitempty"`
	MinOrderQty  int          `json:"min_order_qty"`
	StockQty     int          `json:"stock_qty"`
	IsFollowed   bool         `json:"is_followed"`

	Eligible          bool             `json:"eligible"`
	IneligibleReason  IneligibleReason `json:"ineligible_reason,omitempty"`
	CoverageDistanceM *int             `json:"coverage_distance_m,omitempty"`

	// ConvertedQty is the required quantity converted from pack relationship (T076).
	// If 0, effective quantity is used directly.
	ConvertedQty int `json:"converted_qty,omitempty"`
}

// Criterion is one supplier-selection rule the buyer can enable and prioritise.
type Criterion string

const (
	CriterionLowestPrice       Criterion = "lowest_price"
	CriterionHighestDiscount   Criterion = "highest_discount"
	CriterionFollowedSuppliers Criterion = "followed_suppliers"
)

// DecidedBy records what settled a line's supplier, so the choice is explainable.
type DecidedBy string

const (
	DecidedLowestPrice       DecidedBy = "lowest_price"
	DecidedHighestDiscount   DecidedBy = "highest_discount"
	DecidedFollowedSuppliers DecidedBy = "followed_suppliers"
	DecidedDefault           DecidedBy = "default"
	DecidedOnlyCandidate     DecidedBy = "only_candidate"
	DecidedUser              DecidedBy = "user"
)

// Selection is the chosen candidate for a line and the reasoning behind it.
type Selection struct {
	LineID         int64     `json:"line_id"`
	OrganizationID int64     `json:"organization_id"`
	CandidateID    int64     `json:"candidate_id"`
	DecidedBy      DecidedBy `json:"decided_by"`

	// ToleranceApplied means the top criterion's winner was passed over for
	// being too expensive. SkippedCandidateID and SkippedExcessPct name it and
	// say by how much, because a silent override reads as a bug to the buyer.
	ToleranceApplied   bool     `json:"tolerance_applied"`
	SkippedCandidateID *int64   `json:"skipped_candidate_id,omitempty"`
	SkippedExcessPct   *float64 `json:"skipped_excess_pct,omitempty"`

	UserOverridden bool         `json:"user_overridden"`
	LineNet        money.Amount `json:"line_net"`
	UpdatedAt      time.Time    `json:"updated_at"`
}

// Stage names a pipeline phase, for progress reporting.
type Stage string

const (
	StageParse      Stage = "parse"
	StageNormalize  Stage = "normalize"
	StageResolve    Stage = "resolve"
	StageCandidates Stage = "candidates"
	// StageInitialDone marks the deterministic engine finishing. It is a
	// milestone rather than work, and it exists because a buyer must be able to
	// see that ordinary matching is complete while the AI stage is still
	// running — otherwise a run that is 90% done looks identical to one that has
	// hung.
	StageInitialDone Stage = "initial_done"
	// StageAIEnhance is the AI stage. StageAdjudicate is its former name, kept
	// so runs recorded before the rework still render.
	StageAIEnhance  Stage = "ai_enhance"
	StageAdjudicate Stage = "adjudicate"
	StageSelect     Stage = "select"
	StageFinalize   Stage = "finalize"
)

// Event is an append-only progress record. It backs both the live progress
// stream and the audit trail, which is why it is stored rather than logged.
type Event struct {
	ID             int64     `json:"id"`
	RunID          int64     `json:"run_id"`
	OrganizationID int64     `json:"organization_id"`
	Stage          Stage     `json:"stage"`
	Processed      *int      `json:"processed,omitempty"`
	Total          *int      `json:"total,omitempty"`
	Message        i18n.Text `json:"message"`
	Level          string    `json:"level"`
	CreatedAt      time.Time `json:"created_at"`
}

// PackRelationship models an approved conversion relationship between units.
type PackRelationship struct {
	ID        int64      `json:"id"`
	ProductID int64      `json:"product_id"`
	FromUnit  string     `json:"from_unit"`
	ToUnit    string     `json:"to_unit"`
	Factor    int        `json:"factor"`
	CreatedAt time.Time  `json:"created_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}
