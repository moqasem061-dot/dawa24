package commerce

import (
	"context"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Refusals the quota screen can produce. A repository with no quota support is
// a wiring fault, not a user error, so it is reported as one rather than as an
// empty page that silently does nothing when the button is pressed.
var (
	ErrQuotaUnavailable = apperr.Conflict("quota.unavailable",
		"Quota management is not available on this deployment.")
	ErrQuotaTargetInvalid = apperr.Validation("quota.target_invalid",
		"A product variant and a branch are both required.", nil)
)

// The per-branch purchase quota: "this branch may take at most N of this
// variant, in total, ever".
//
// The cap is a catalog fact (catalog.product_variants.quota_limit) and reaches
// commerce through VariantAvailability, the same way stock does. What has been
// consumed is a commerce fact, and it is deliberately *not* stored as a
// counter.
//
// Consumption is summed from the orders themselves — SUM(order_lines.quantity)
// for that variant, over orders whose branch_id is this branch and whose status
// is not one of the four that undo a purchase. A counter would have to be moved
// by checkout, cancellation, a pharmacy editing a pending order, a return, a
// refund, a soft delete and two admin paths; every one of those is a place the
// counter can drift from the orders it claims to describe, and the drift is
// silent until a supplier is asked why a branch that cancelled everything still
// cannot buy. Summing the orders cannot drift, because the orders *are* the
// consumption.
//
// A supplier can reset one branch's consumption. That records an instant, not a
// credit: orders placed at or before it stop counting, and the branch begins
// again from zero against the same cap. It frees that branch and nothing else —
// every other branch has its own allowance and is unaffected either way.

// QuotaReleasingStatuses are the order states that give a branch its quota
// back. They are the terminal states in which no goods end up with the buyer.
//
// StatusDelivered and StatusCompleted are deliberately absent: a completed
// order is the whole point of the quota. StatusRefunded is present because a
// refund unwinds the sale.
var QuotaReleasingStatuses = []OrderStatus{
	StatusCancelled, StatusFailed, StatusReturned, StatusRefunded,
}

// QuotaReleasingStatusStrings is the same list in the form SQL wants. Both the
// gate and the supplier's report read it, so the two cannot disagree about
// which orders still hold quota.
func QuotaReleasingStatusStrings() []string {
	out := make([]string, 0, len(QuotaReleasingStatuses))
	for _, s := range QuotaReleasingStatuses {
		out = append(out, string(s))
	}
	return out
}

// IsQuota reports whether a refusal came from the per-branch quota.
//
// The buying surfaces group refusals into three kinds: "your branch cannot be
// served at all" (hide it), "this item cannot be ordered right now" (show it,
// explain, disable the button) and "adjust the quantity". A quota refusal is
// the second kind — the same as out-of-stock. Classifying it as the first would
// make a restricted item disappear from the catalogue the moment a branch
// finished its allowance, which reads as the supplier having delisted it.
func (r Reason) IsQuota() bool {
	return r == ReasonQuotaExhausted || r == ReasonQuotaExceeded
}

// BranchQuotaUsage is one branch's standing against one variant's cap.
type BranchQuotaUsage struct {
	VariantID  int64      `json:"variant_id"`
	BranchID   int64      `json:"branch_id"`
	Limit      int        `json:"limit"`
	Used       int        `json:"used"`
	ReleasedAt *time.Time `json:"released_at,omitempty"`
}

// Remaining is what the branch may still buy. It never goes below zero: a
// branch that somehow consumed more than its cap (a quota lowered after the
// fact, an order edited by an administrator) is simply finished, not owed
// anything.
func (u BranchQuotaUsage) Remaining() int {
	if u.Limit <= 0 {
		return 0
	}
	if u.Used >= u.Limit {
		return 0
	}
	return u.Limit - u.Used
}

// Exhausted reports whether the branch has taken its whole allowance.
func (u BranchQuotaUsage) Exhausted() bool {
	return u.Limit > 0 && u.Used >= u.Limit
}

// PercentUsed drives the progress bar on the supplier's screen, 0-100.
func (u BranchQuotaUsage) PercentUsed() int {
	if u.Limit <= 0 {
		return 0
	}
	pct := u.Used * 100 / u.Limit
	if pct > 100 {
		return 100
	}
	if pct < 0 {
		return 0
	}
	return pct
}

// BranchQuotaRow is one line of the supplier's quota screen: a buying branch,
// the variant it bought, and how much of that variant's allowance it has taken.
type BranchQuotaRow struct {
	VariantID     int64      `json:"variant_id"`
	VariantName   string     `json:"variant_name"`
	ProductID     int64      `json:"product_id"`
	ProductName   string     `json:"product_name"`
	SKU           string     `json:"sku,omitempty"`
	QuotaLimit    int        `json:"quota_limit"`
	BranchID      int64      `json:"branch_id"`
	BranchName    string     `json:"branch_name"`
	CustomerOrgID int64      `json:"customer_org_id"`
	CustomerName  string     `json:"customer_name"`
	Used          int        `json:"used"`
	OrderCount    int        `json:"order_count"`
	LastOrderAt   *time.Time `json:"last_order_at,omitempty"`
	ReleasedAt    *time.Time `json:"released_at,omitempty"`
	ReleaseCount  int        `json:"release_count"`
	// ReleasedUnits is what the release took off the branch's tally: the
	// quantity it had bought before the supplier reset it. Without it a
	// released row reads as "never bought anything", which is exactly the
	// history the supplier needs when deciding whether to release again.
	ReleasedUnits int `json:"released_units"`
}

// Usage projects the row onto the arithmetic the templates share with the gate.
func (r *BranchQuotaRow) Usage() BranchQuotaUsage {
	if r == nil {
		return BranchQuotaUsage{}
	}
	return BranchQuotaUsage{
		VariantID:  r.VariantID,
		BranchID:   r.BranchID,
		Limit:      r.QuotaLimit,
		Used:       r.Used,
		ReleasedAt: r.ReleasedAt,
	}
}

// Remaining is what this branch may still take of this variant.
func (r *BranchQuotaRow) Remaining() int { return r.Usage().Remaining() }

// Exhausted reports whether this branch has taken its whole allowance.
func (r *BranchQuotaRow) Exhausted() bool { return r.Usage().Exhausted() }

// PercentUsed drives the row's progress bar.
func (r *BranchQuotaRow) PercentUsed() int { return r.Usage().PercentUsed() }

// WasReleased reports whether the supplier has reset this pairing at least once.
func (r *BranchQuotaRow) WasReleased() bool {
	return r != nil && r.ReleasedAt != nil
}

// QuotaVariantRow is the other half of the screen: one restricted variant and
// what has happened to its allowance across every branch.
type QuotaVariantRow struct {
	VariantID         int64  `json:"variant_id"`
	VariantName       string `json:"variant_name"`
	ProductID         int64  `json:"product_id"`
	ProductName       string `json:"product_name"`
	SKU               string `json:"sku,omitempty"`
	QuotaLimit        int    `json:"quota_limit"`
	BranchCount       int    `json:"branch_count"`
	ExhaustedBranches int    `json:"exhausted_branches"`
	TotalUsed         int    `json:"total_used"`
}

// QuotaSummary is the headline row of stat cards.
type QuotaSummary struct {
	VariantsWithQuota int `json:"variants_with_quota"`
	BranchesConsuming int `json:"branches_consuming"`
	BranchesExhausted int `json:"branches_exhausted"`
	TotalUnitsUsed    int `json:"total_units_used"`
	ReleasesMade      int `json:"releases_made"`
}

// QuotaFilter narrows the supplier's quota screen.
type QuotaFilter struct {
	Query         string
	VariantID     int64
	CustomerOrgID int64
	BranchID      int64
	// State is "", QuotaStateExhausted, QuotaStateActive or QuotaStateReleased.
	State  string
	Limit  int
	Offset int
}

// The states a quota row can be filtered to.
const (
	QuotaStateExhausted = "exhausted"
	QuotaStateActive    = "active"
	QuotaStateReleased  = "released"
)

// QuotaOption is one entry of the screen's variant / branch pickers.
type QuotaOption struct {
	ID    int64  `json:"id"`
	Label string `json:"label"`
}

// QuotaBackend is the persistence the quota rule and the quota screen need.
//
// It is an optional interface on Repository rather than a member of it, the
// same shape catalog.VendorCatalogBackend uses: a repository that does not
// implement it simply has no quotas, which is the correct behaviour for the
// in-memory repositories the domain tests are written against.
type QuotaBackend interface {
	// BranchQuotaUsed sums what one branch has already committed to buying of
	// one variant. excludeOrderID lets an order being edited discount its own
	// current lines, so raising a line from 2 to 3 is measured as 3 against the
	// cap and not as 5.
	BranchQuotaUsed(ctx context.Context, variantID, branchID, excludeOrderID int64) (int, error)
	// ReleaseBranchQuota resets one branch's consumption of one variant.
	ReleaseBranchQuota(ctx context.Context, vendorOrgID, variantID, branchID, actorUserID int64, note string) error
	// UndoBranchQuotaRelease removes a reset, restoring the consumption the
	// supplier had freed. It exists because "I released the wrong branch" is a
	// real mistake and re-consuming the allowance by hand is not possible.
	UndoBranchQuotaRelease(ctx context.Context, vendorOrgID, variantID, branchID int64) error
	ListBranchQuotaRows(ctx context.Context, vendorOrgID int64, f QuotaFilter) ([]*BranchQuotaRow, int, error)
	ListQuotaVariantRows(ctx context.Context, vendorOrgID int64, f QuotaFilter) ([]*QuotaVariantRow, int, error)
	QuotaSummaryForVendor(ctx context.Context, vendorOrgID int64) (QuotaSummary, error)
	QuotaVariantOptions(ctx context.Context, vendorOrgID int64) ([]QuotaOption, error)
	QuotaCustomerOptions(ctx context.Context, vendorOrgID int64) ([]QuotaOption, error)
	QuotaBranchOptions(ctx context.Context, vendorOrgID int64) ([]QuotaOption, error)
}

func (s *Service) quotaBackend() (QuotaBackend, bool) {
	backend, ok := s.repo.(QuotaBackend)
	return backend, ok
}

// BranchQuotaFor answers "how much of this variant may this branch still take".
//
// It is the read behind both the gate and the quantity box on every buying
// surface, so the number the pharmacy is offered and the number the server will
// accept come from one place.
func (s *Service) BranchQuotaFor(ctx context.Context, variantID, branchID int64, limit int) (BranchQuotaUsage, error) {
	usage := BranchQuotaUsage{VariantID: variantID, BranchID: branchID, Limit: limit}
	if limit <= 0 || variantID <= 0 || branchID <= 0 {
		return usage, nil
	}
	backend, ok := s.quotaBackend()
	if !ok {
		return usage, nil
	}
	used, err := backend.BranchQuotaUsed(ctx, variantID, branchID, 0)
	if err != nil {
		return usage, err
	}
	usage.Used = used
	return usage, nil
}

// ReleaseBranchQuota frees one branch's consumption of one variant.
func (s *Service) ReleaseBranchQuota(ctx context.Context, vendorOrgID, variantID, branchID, actorUserID int64, note string) error {
	backend, ok := s.quotaBackend()
	if !ok {
		return ErrQuotaUnavailable
	}
	if vendorOrgID <= 0 || variantID <= 0 || branchID <= 0 {
		return ErrQuotaTargetInvalid
	}
	return backend.ReleaseBranchQuota(ctx, vendorOrgID, variantID, branchID, actorUserID, note)
}

// UndoBranchQuotaRelease puts a released branch's consumption back.
func (s *Service) UndoBranchQuotaRelease(ctx context.Context, vendorOrgID, variantID, branchID int64) error {
	backend, ok := s.quotaBackend()
	if !ok {
		return ErrQuotaUnavailable
	}
	if vendorOrgID <= 0 || variantID <= 0 || branchID <= 0 {
		return ErrQuotaTargetInvalid
	}
	return backend.UndoBranchQuotaRelease(ctx, vendorOrgID, variantID, branchID)
}

// ListBranchQuotaRows returns one page of "which branch took how much".
func (s *Service) ListBranchQuotaRows(ctx context.Context, vendorOrgID int64, f QuotaFilter) ([]*BranchQuotaRow, int, error) {
	backend, ok := s.quotaBackend()
	if !ok {
		return nil, 0, nil
	}
	return backend.ListBranchQuotaRows(ctx, vendorOrgID, f)
}

// ListQuotaVariantRows returns one page of "which items are restricted".
func (s *Service) ListQuotaVariantRows(ctx context.Context, vendorOrgID int64, f QuotaFilter) ([]*QuotaVariantRow, int, error) {
	backend, ok := s.quotaBackend()
	if !ok {
		return nil, 0, nil
	}
	return backend.ListQuotaVariantRows(ctx, vendorOrgID, f)
}

// QuotaSummary computes the supplier's headline quota figures.
func (s *Service) QuotaSummary(ctx context.Context, vendorOrgID int64) (QuotaSummary, error) {
	backend, ok := s.quotaBackend()
	if !ok {
		return QuotaSummary{}, nil
	}
	return backend.QuotaSummaryForVendor(ctx, vendorOrgID)
}

// QuotaFilterOptions returns the screen's pickers: variants, customers, and branches.
func (s *Service) QuotaFilterOptions(ctx context.Context, vendorOrgID int64) (variants, customers, branches []QuotaOption, err error) {
	backend, ok := s.quotaBackend()
	if !ok {
		return nil, nil, nil, nil
	}
	variants, err = backend.QuotaVariantOptions(ctx, vendorOrgID)
	if err != nil {
		return nil, nil, nil, err
	}
	customers, err = backend.QuotaCustomerOptions(ctx, vendorOrgID)
	if err != nil {
		return nil, nil, nil, err
	}
	branches, err = backend.QuotaBranchOptions(ctx, vendorOrgID)
	if err != nil {
		return nil, nil, nil, err
	}
	return variants, customers, branches, nil
}

// QuotaBatchBackend answers the quota question for a page rather than a line.
//
// A separate optional interface rather than a method on QuotaBackend, so the
// in-memory repositories the domain tests are written against keep working
// without implementing it: a backend that does not offer the batch simply gets
// asked one variant at a time, which is what every caller did before.
type QuotaBatchBackend interface {
	BranchQuotaUsedBatch(ctx context.Context, variantIDs []int64, branchID int64) (map[int64]int, error)
}

// branchQuotaUsedFor resolves consumption for many variants at once.
//
// The buying catalogue evaluates up to ninety-six offers per page, and asking
// per offer made a page view ninety-six sequential round trips. Where the
// repository can answer in one statement it does; where it cannot, this falls
// back to the per-variant read so the rule is unchanged either way.
func (s *Service) branchQuotaUsedFor(
	ctx context.Context, variantIDs []int64, branchID int64,
) (map[int64]int, error) {
	used := make(map[int64]int, len(variantIDs))
	if len(variantIDs) == 0 || branchID <= 0 {
		return used, nil
	}
	if batch, ok := s.repo.(QuotaBatchBackend); ok {
		return batch.BranchQuotaUsedBatch(ctx, variantIDs, branchID)
	}
	backend, ok := s.quotaBackend()
	if !ok {
		return used, nil
	}
	for _, variantID := range variantIDs {
		n, err := backend.BranchQuotaUsed(ctx, variantID, branchID, 0)
		if err != nil {
			return nil, err
		}
		if n > 0 {
			used[variantID] = n
		}
	}
	return used, nil
}
