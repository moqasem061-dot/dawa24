package smartorder

import (
	"context"
	"fmt"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// Finalisation.
//
// The gap between generating an order and placing it is real: a buyer reviews
// for ten minutes, and in that time a coverage window closes, a vendor sells its
// last box, or a product is deactivated. Every line is therefore re-verified at
// the moment of placing.
//
// What happens when a line has gone stale is the important part. The system does
// **not** substitute another supplier, and does **not** quietly drop the line.
// Both are defensible-sounding and both are wrong: a pharmacy that ordered
// twenty boxes from a specific supplier and silently received them from another,
// or did not receive them at all, discovers it at delivery. The buyer is told
// what changed and decides.

// OrderPlacer creates the order once every line has been re-verified.
//
// An interface because order creation belongs to commerce, and modules must not
// import each other. It also means finalisation can be tested without a
// checkout.
type OrderPlacer interface {
	PlaceOrder(ctx context.Context, req PlaceOrderRequest) (orderID int64, err error)
}

// PlaceOrderRequest is a finalised smart order handed to commerce.
type PlaceOrderRequest struct {
	OrganizationID int64
	UserID         int64
	BranchID       int64
	Lines          []PlaceOrderLine
	Total          money.Amount
	SourceRunID    int64
}

// PlaceOrderLine is one line of the finalised order.
type PlaceOrderLine struct {
	LineID      int64
	VariantID   int64
	VendorOrgID int64
	Quantity    float64
	UnitPrice   money.Amount
	DiscountBps int64
	LineNet     money.Amount
	RawName     string
}

// StaleLine is a line that changed between generation and finalisation.
//
// Detail is bilingual because it is read by a pharmacist on an Arabic,
// right-to-left screen. It used to be an English sentence, rendered raw into
// that screen, so the one moment the buyer most needed to understand — their
// order refused at the last click — was the one moment the platform answered
// them in the wrong language.
type StaleLine struct {
	LineID  int64            `json:"line_id"`
	RawName string           `json:"raw_name"`
	Reason  IneligibleReason `json:"reason,omitempty"`
	Detail  i18n.Text        `json:"detail"`
}

// Reverifier re-checks a candidate against the world as it is now.
type Reverifier interface {
	Recheck(ctx context.Context, buyerOrgID, branchID int64, c Candidate, qty float64) (bool, IneligibleReason, error)
}

// Finalizer turns a reviewed run into an order.
type Finalizer struct {
	repo    Repository
	placer  OrderPlacer
	recheck Reverifier
}

// NewFinalizer constructs the finalisation use case.
func NewFinalizer(repo Repository, placer OrderPlacer, recheck Reverifier) *Finalizer {
	return &Finalizer{repo: repo, placer: placer, recheck: recheck}
}

// Finalize re-verifies every orderable line and places the order.
//
// Returns the stale lines instead of an order when anything changed. The caller
// renders them for the buyer to resolve; nothing is written until they do.
func (f *Finalizer) Finalize(ctx context.Context, run *Run) (orderID int64, stale []StaleLine, err error) {
	if err := run.CanFinalize(); err != nil {
		return 0, nil, err
	}

	lines, _, err := f.repo.ListLines(ctx, run.ID, LineFilter{
		Outcome: string(OutcomeOrdered),
		All:     true,
	})
	if err != nil {
		return 0, nil, err
	}
	if len(lines) == 0 {
		return 0, nil, apperr.Validation("smartorder.nothing_to_order",
			"no line in this order has a supplier and a quantity above zero", nil)
	}

	// Every selection of the run in one query rather than one per line. The
	// re-check below is already a per-line decision; adding a round trip in
	// front of each made placing a large order take longer than building it.
	selections, err := f.repo.ListSelectionsByRun(ctx, run.OrganizationID, run.ID)
	if err != nil {
		return 0, nil, err
	}

	orderLines := make([]PlaceOrderLine, 0, len(lines))
	total := money.Amount{}

	for _, l := range lines {
		sel, ok := selections[l.ID]
		if !ok {
			stale = append(stale, StaleLine{
				LineID: l.ID, RawName: l.RawName,
				Detail: i18n.New("لم يعد هناك مورد مختار لهذا الصنف.",
					"no supplier is selected for this line any more"),
			})
			continue
		}
		candidate, err := f.repo.GetCandidate(ctx, run.OrganizationID, sel.CandidateID)
		if err != nil {
			stale = append(stale, StaleLine{
				LineID: l.ID, RawName: l.RawName,
				Detail: i18n.New("لم يعد عرض المورد المختار موجوداً.",
					"the selected supplier's offer no longer exists"),
			})
			continue
		}

		fresh, reason, err := f.recheck.Recheck(ctx, run.OrganizationID, run.BranchID, *candidate, l.EffectiveQty)
		if err != nil {
			return 0, nil, err
		}
		if !fresh {
			stale = append(stale, StaleLine{
				LineID: l.ID, RawName: l.RawName, Reason: reason,
				Detail: staleDetail(reason),
			})
			continue
		}

		net, err := LineNet(candidate.NetUnitPrice, l.EffectiveQty)
		if err != nil {
			return 0, nil, err
		}
		if total, err = total.Add(net); err != nil {
			return 0, nil, err
		}
		orderLines = append(orderLines, PlaceOrderLine{
			LineID:      l.ID,
			VariantID:   candidate.VariantID,
			VendorOrgID: candidate.VendorOrgID,
			Quantity:    l.EffectiveQty,
			UnitPrice:   candidate.Price,
			DiscountBps: candidate.DiscountBps,
			LineNet:     net,
			RawName:     l.RawName,
		})
	}

	// Any stale line blocks the whole order. Placing the good lines and leaving
	// the rest would split one decision into two without asking.
	if len(stale) > 0 {
		return 0, stale, nil
	}

	if err := run.TransitionTo(StatusFinalizing); err != nil {
		return 0, nil, err
	}
	if err := f.repo.UpdateRunStatus(ctx, run.ID, run.Status, run.CurrentStep, ""); err != nil {
		return 0, nil, err
	}

	orderID, err = f.placer.PlaceOrder(ctx, PlaceOrderRequest{
		OrganizationID: run.OrganizationID,
		UserID:         run.UserID,
		BranchID:       run.BranchID,
		Lines:          orderLines,
		Total:          total,
		SourceRunID:    run.ID,
	})
	if err != nil {
		// Put the run back where the buyer can retry rather than stranding it
		// in `finalizing` with no order.
		_ = f.repo.UpdateRunStatus(ctx, run.ID, StatusCompleted, 4,
			fmt.Sprintf("order creation failed: %v", err))
		return 0, nil, err
	}

	// FinalizeRun refuses if finalized_at is already set, so a double submit
	// produces exactly one order.
	if err := f.repo.FinalizeRun(ctx, run.ID, orderID); err != nil {
		return 0, nil, err
	}
	return orderID, nil, nil
}

// staleDetail explains a re-verification failure in the buyer's terms.
func staleDetail(reason IneligibleReason) i18n.Text {
	switch reason {
	case ReasonCoverage:
		return i18n.New("أُغلقت نافذة توصيل هذا المورد لفرعك منذ إنشاء الطلب.",
			"this supplier's delivery window for your branch has closed since the order was generated")
	case ReasonNoLocation:
		return i18n.New("لم يتم تحديد موقع فرع الاستلام على الخريطة.",
			"delivery branch has no map location")
	case ReasonStock:
		return i18n.New("نفدت كمية هذا المورد منذ إنشاء الطلب.",
			"this supplier has sold out since the order was generated")
	case ReasonMinQty:
		return i18n.New("الكمية أصبحت أقل من الحد الأدنى للطلب لدى هذا المورد.",
			"the quantity is now below this supplier's minimum order")
	case ReasonQuota:
		return i18n.New("تم استنفاد الحصة المخصصة لفرعك لهذا الصنف لدى هذا المورد.",
			"this supplier's quota for your branch on this item has been reached")
	case ReasonInstitutional:
		return i18n.New("العمل المؤسسي لفرع الاستلام لم يعد متصلاً بالأعمال المؤسسية لفروع هذا المورد.",
			"the delivery branch's institutional work is no longer connected to this supplier's branches")
	case ReasonInactive:
		return i18n.New("تم إيقاف هذا المورد أو هذا الصنف.",
			"this supplier or product has been deactivated")
	case ReasonOwnOrg:
		return i18n.New("أصبح هذا العرض تابعاً لمنشأتك أنت.",
			"this offer now belongs to your own organisation")
	default:
		return i18n.New("لم يعد هذا الصنف متاحاً بالشكل الذي أُنشئ به الطلب.",
			"this line is no longer available as generated")
	}
}
