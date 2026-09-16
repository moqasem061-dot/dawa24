package promo

import (
	"context"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// The offers board as one buyer sees it.
//
// Before this there were three answers to "which offers can this branch buy":
// the offers page took the newest hundred running offers and filtered them in
// Go (so an offer past the hundredth never appeared, and the page cost three
// queries per offer); the dashboard strip joined the offer's own branch to
// that branch's weekly coverage (so a supplier-wide offer never appeared, and
// an offer's own location rules were ignored); and checkout re-ran the Go
// filter, which ignored the weekday the supplier had chosen, the supplier's
// approval and the institutional works the catalogue enforces. An offer whose
// branch the supplier had since deleted was invisible everywhere with no
// explanation to anyone.
//
// Now one set of SQL predicates decides, in the listing and in the single-offer
// verdict alike, from the same facts the catalogue and commerce.CheckAvailability
// use: the supplier coverage set resolved by workflow for today, and the
// institutional works connected to the buyer's branch.

// BuyerOfferSort orders the offers board.
type BuyerOfferSort string

const (
	SortOffersNewest       BuyerOfferSort = "newest"
	SortOffersDiscountDesc BuyerOfferSort = "discount_desc"
	SortOffersDiscountAsc  BuyerOfferSort = "discount_asc"
	SortOffersPriceAsc     BuyerOfferSort = "price_asc"
	SortOffersPriceDesc    BuyerOfferSort = "price_desc"
)

// BuyerOfferQuery is one page of offers, or one offer, from a buyer's side.
type BuyerOfferQuery struct {
	// BuyerOrgID is excluded as a seller: a supplier's own promotion is not an
	// offer to it. Zero for a visitor.
	BuyerOrgID int64
	// Buying is set when a buyer branch was resolved. Without it the query
	// browses (live offers of approved suppliers, no coverage); with it every
	// purchase precondition applies.
	Buying    bool
	Branch    BuyerBranch
	Coverage  SupplierCoverage
	OfferID   int64
	Search    string
	Discounts bool // only offers carrying a discount
	Sort      BuyerOfferSort
	Limit     int
	Offset    int
}

// BuyerBranch is the receiving branch's location and institutional reach.
type BuyerBranch struct {
	ID        int64
	CityID    int64
	Lat, Lon  float64
	HasCoords bool
	Weekday   time.Weekday
	// AllowedWorkIDs are the institutional works connected to the branch's
	// own. Empty means the branch may buy from nobody.
	AllowedWorkIDs []int64
}

// SupplierCoverage is workflow's weekly coverage for the branch today, as sets.
type SupplierCoverage struct {
	// OrgIDs reach the branch from at least one coverage row.
	OrgIDs []int64
	// BranchIDs are the supplier branches whose own rows reach it.
	BranchIDs []int64
	// OrgWideIDs reach it from a row bound to no branch, which covers every
	// branch of the supplier — as workflow.ServesPoint treats it.
	OrgWideIDs []int64
}

// BuyerOffer is one offer card.
type BuyerOffer struct {
	ID               int64
	OrganizationID   int64
	OrganizationName i18n.Text
	LegalName        string
	BranchID         *int64
	Title            i18n.Text
	Description      i18n.Text
	DiscountType     DiscountType
	DiscountValue    money.Amount
	MinOrderAmount   money.Amount
	TotalPrice       money.Amount
	StartsAt         *time.Time
	ExpiresAt        *time.Time
	ProductCount     int
	Sponsored        bool
}

// DiscountPercentage is the percentage an offer grants, or zero for a fixed one.
func (o *BuyerOffer) DiscountPercentage() float64 {
	if o.DiscountType != DiscountPercentage {
		return 0
	}
	return float64(o.DiscountValue.Minor()) / 100
}

// SupplierName is the supplier's display name in a language.
func (o *BuyerOffer) SupplierName(lang i18n.Lang) string {
	if name := o.OrganizationName.Get(lang); name != "" {
		return name
	}
	return o.LegalName
}

// OfferVerdict is each purchase precondition of one offer, evaluated for one
// buyer. Coverage and Institutional are meaningful only when the query was
// Buying.
type OfferVerdict struct {
	Found         bool
	Live          bool
	SupplierOK    bool
	BranchOK      bool
	OwnOffer      bool
	Institutional bool
	Covered       bool
}

// Reason is the first precondition that fails, or OfferOK.
func (v OfferVerdict) Reason(buying bool) OfferReason {
	switch {
	case !v.Found:
		return OfferNotFound
	case !v.Live:
		return OfferNotLive
	case !v.SupplierOK:
		return OfferSupplierUnavailable
	case !v.BranchOK:
		return OfferBranchUnavailable
	case v.OwnOffer:
		return OfferOwn
	case buying && !v.Institutional:
		return OfferInstitutionalMismatch
	case buying && !v.Covered:
		return OfferNotCovered
	}
	return OfferOK
}

// OfferReason is a machine-readable refusal; the UI maps it to a message.
type OfferReason string

const (
	OfferOK                    OfferReason = ""
	OfferNotFound              OfferReason = "not_found"
	OfferNotLive               OfferReason = "not_live"
	OfferSupplierUnavailable   OfferReason = "supplier_unavailable"
	OfferBranchUnavailable     OfferReason = "branch_unavailable"
	OfferOwn                   OfferReason = "own_offer"
	OfferInstitutionalMismatch OfferReason = "institutional_mismatch"
	OfferNotCovered            OfferReason = "not_covered"
)

// ListBuyerOffers returns one page of the offers a buyer may see and its total.
func (s *Service) ListBuyerOffers(ctx context.Context, q BuyerOfferQuery) ([]*BuyerOffer, int, error) {
	return s.repo.ListBuyerOffers(ctx, q)
}

// ListRunningOffersByOrg is a supplier's own live offers, for its dashboard.
func (s *Service) ListRunningOffersByOrg(ctx context.Context, orgID int64, limit int) ([]*Offer, error) {
	return s.repo.ListRunningOffersByOrg(ctx, orgID, limit)
}

// OfferVerdict evaluates one offer for a buyer with the listing's own rule.
func (s *Service) OfferVerdict(ctx context.Context, q BuyerOfferQuery) (OfferVerdict, error) {
	return s.repo.OfferVerdict(ctx, q)
}
