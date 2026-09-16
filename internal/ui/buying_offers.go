package ui

import (
	"context"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// Promotional offers (العروض والخصومات) from the buyer's side.
//
// Every surface that shows or sells an offer builds its question here and asks
// promo.ListBuyerOffers or promo.OfferVerdict, which evaluate one rule in SQL.
// The facts that rule needs about the buyer — the receiving branch, the
// institutional works it may buy from, and who delivers to it today — are the
// catalogue's own, resolved the same way, so an offer and a listing from the
// same supplier cannot disagree about whether the branch can buy.

// offerBuyer is a buyer offer query together with why it may not run.
type offerBuyer struct {
	Query promo.BuyerOfferQuery
	// Notice is set when the caller is buying but no receiving branch can be
	// resolved; nothing is buyable and the reason is shown instead.
	Notice string
}

// buyerOfferQuery resolves the offer question for the caller. branchID zero
// means the caller's current buying branch.
func (h *UIHandler) buyerOfferQuery(ctx context.Context, actor authctx.Actor, branchID int64) offerBuyer {
	if !actor.IsBuyer() {
		// A visitor or staff browses: live offers of approved suppliers.
		return offerBuyer{}
	}
	out := offerBuyer{Query: promo.BuyerOfferQuery{BuyerOrgID: actor.OrganizationID, Buying: true}}
	if branchID <= 0 {
		branchID = h.buyingBranchID(ctx, &actor)
	}
	if branchID <= 0 || h.orgSvc == nil {
		out.Notice = i18n.T("ar", "buying.select_branch_first")
		return out
	}
	branch, err := h.orgSvc.GetBranch(database.AsSystem(ctx), branchID)
	if err != nil || branch == nil || branch.OrganizationID != actor.OrganizationID {
		out.Notice = i18n.T("ar", "buying.select_branch_first")
		return out
	}

	coord, _ := branchCoord(branch)
	b := promo.BuyerBranch{
		ID:        branch.ID,
		Lat:       coord.Lat,
		Lon:       coord.Lon,
		HasCoords: branch.Latitude != nil && branch.Longitude != nil && (coord.Lat != 0 || coord.Lon != 0),
		Weekday:   time.Now().Weekday(),
	}
	if branch.CityID != nil {
		b.CityID = *branch.CityID
	}
	if works, err := h.orgSvc.ConnectedWorkIDsForBranch(database.AsSystem(ctx), branch.ID); err == nil {
		b.AllowedWorkIDs = works
	} else {
		h.log.WarnContext(ctx, "offers: institutional works for branch", "branch_id", branch.ID, "error", err)
	}
	cov := h.coveringVendorBranchesFor(ctx, branch.ID)
	out.Query.Branch = b
	out.Query.Coverage = promo.SupplierCoverage{OrgIDs: cov.OrgIDs, BranchIDs: cov.BranchIDs, OrgWideIDs: cov.OrgWideIDs}
	return out
}

// offerRefusal is the Arabic reason an offer cannot be bought, or "".
func offerRefusal(reason promo.OfferReason) string {
	switch reason {
	case promo.OfferOK:
		return ""
	case promo.OfferNotFound, promo.OfferNotLive:
		return "هذا العرض غير متاح حالياً: انتهت مدته أو أوقفه المورد أو لم يُعتمد بعد."
	case promo.OfferSupplierUnavailable:
		return "المورد صاحب هذا العرض غير متاح حالياً على المنصة."
	case promo.OfferBranchUnavailable:
		return "فرع المورد المرتبط بهذا العرض لم يعد متاحاً، لذلك لا يمكن طلبه الآن."
	case promo.OfferOwn:
		return i18n.T("ar", "err.own_organization_supply")
	case promo.OfferInstitutionalMismatch:
		return i18n.TDefault("commerce.availability.branch_institutional_mismatch")
	case promo.OfferNotCovered:
		return "فرع الاستلام خارج نطاق تغطية هذا العرض اليوم."
	}
	return offerCheckFailed
}

// offerCheckFailed is shown when the offer rule could not be evaluated.
const offerCheckFailed = "تعذر التحقق من إتاحة العرض الآن. حاول مرة أخرى بعد قليل."

// offerPurchasable decides whether the caller may buy one offer now, with the
// reason when not. A failed check is not permission to buy.
func (h *UIHandler) offerPurchasable(ctx context.Context, actor authctx.Actor, offerID, branchID int64) (bool, string) {
	reason := h.offerRefusalFor(ctx, h.buyerOfferQuery(ctx, actor, branchID), offerID)
	return reason == "", reason
}

// offerRefusalFor evaluates one offer for an already-resolved buyer, so a page
// checking several offers resolves the buyer's branch, works and coverage once.
func (h *UIHandler) offerRefusalFor(ctx context.Context, buyer offerBuyer, offerID int64) string {
	if h.promoSvc == nil {
		return offerCheckFailed
	}
	if buyer.Notice != "" {
		return buyer.Notice
	}
	q := buyer.Query
	q.OfferID = offerID
	verdict, err := h.promoSvc.OfferVerdict(ctx, q)
	if err != nil {
		h.log.ErrorContext(ctx, "offers: verdict", "offer_id", offerID, "error", err)
		return offerCheckFailed
	}
	return offerRefusal(verdict.Reason(q.Buying))
}

// offerCards renders buyer offers as board cards.
func offerCards(offers []*promo.BuyerOffer, lang string, buying bool) []*pages.OfferCardData {
	cards := make([]*pages.OfferCardData, 0, len(offers))
	for _, o := range offers {
		name := o.SupplierName(i18n.Lang(lang))
		if name == "" {
			name = i18n.T(lang, "offers.default_supplier_name")
		}
		card := &pages.OfferCardData{
			ID:                 o.ID,
			Title:              o.Title,
			Description:        o.Description,
			OrganizationID:     o.OrganizationID,
			OrganizationName:   name,
			DiscountType:       string(o.DiscountType),
			DiscountValue:      o.DiscountValue,
			DiscountPercentage: o.DiscountPercentage(),
			MinOrderAmount:     o.MinOrderAmount,
			TotalPrice:         o.TotalPrice,
			ProductsCount:      o.ProductCount,
			IsSponsored:        o.Sponsored,
			IsCustomerUser:     buying,
			IsCovered:          buying,
		}
		if o.StartsAt != nil {
			card.StartsAt = *o.StartsAt
		}
		if o.ExpiresAt != nil {
			card.ExpiresAt = *o.ExpiresAt
		}
		cards = append(cards, card)
	}
	return cards
}

// buyingBranch resolves the active branch the buyer is shopping for.
func (h *UIHandler) buyingBranch(ctx context.Context, actor *authctx.Actor) *org.Branch {
	if h.orgSvc == nil || actor == nil || actor.OrganizationID <= 0 {
		return nil
	}
	branchID := h.buyingBranchID(ctx, actor)
	if branchID <= 0 {
		return nil
	}
	branch, _ := h.orgSvc.GetBranch(ctx, branchID)
	return branch
}
