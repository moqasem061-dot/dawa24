package ui

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/assistant/actions"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AddOfferToCartSubmit adds an entire special offer bundle to the cart for a pharmacy.
func (h *UIHandler) AddOfferToCartSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		if h.isHTMX(r) {
			w.Header().Set("HX-Redirect", "/auth/login?redirect=/offers")
			w.Header().Set("HX-Trigger", fmt.Sprintf(`{"showToast":{"message":%+q,"type":"error"}}`, i18n.T(langOf(r), "customer.offer.login_required")))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		http.Redirect(w, r, "/auth/login?redirect=/offers", http.StatusSeeOther)
		return
	}

	if !actor.IsBuyer() {
		if h.isHTMX(r) {
			w.Header().Set("HX-Trigger", fmt.Sprintf(`{"showToast":{"message":%+q,"type":"error"}}`, i18n.T(langOf(r), "customer.offer.buy_pharmacy_only")))
			w.WriteHeader(http.StatusForbidden)
			return
		}
		h.redirectWithNotice(w, r, "/offers", "error", i18n.T(langOf(r), "customer.offer.buy_pharmacy_only_notice"))
		return
	}

	userID := actor.UserID
	if h.commSvc == nil || h.promoSvc == nil {
		if h.isHTMX(r) {
			w.Header().Set("HX-Trigger", fmt.Sprintf(`{"showToast":{"message":%+q,"type":"error"}}`, i18n.T(langOf(r), "customer.cart.service_unavailable")))
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		http.Redirect(w, r, "/cart", http.StatusSeeOther)
		return
	}

	offerID, _ := strconv.ParseInt(r.PostFormValue("offer_id"), 10, 64)
	if offerID <= 0 {
		h.redirectWithNotice(w, r, "/offers", "error", i18n.T(langOf(r), "customer.offer.invalid_id"))
		return
	}

	bundleMultiplier, _ := strconv.Atoi(r.PostFormValue("quantity"))
	if bundleMultiplier <= 0 {
		bundleMultiplier, _ = strconv.Atoi(r.PostFormValue("qty"))
	}
	if bundleMultiplier <= 0 {
		bundleMultiplier = 1
	}

	// The bundle's own state first, for its specific reason (expired, paused,
	// unpriced); then the offer rule for this buyer's branch.
	item, _, err := h.offerCartItem(ctx, offerID, bundleMultiplier)
	if err != nil {
		msg := i18n.T(langOf(r), "customer.offer.not_found")
		if refusal, ok := actions.AsRefusal(err); ok {
			msg = refusal.Message
		}
		h.redirectWithNotice(w, r, fmt.Sprintf("/offers/%d", offerID), "error", msg)
		return
	}
	if ok, reason := h.offerPurchasable(ctx, actor, offerID, 0); !ok {
		h.rejectOfferAvailability(w, r, offerID, commerce.AvailabilityResult{MessageAr: reason}, nil)
		return
	}

	if _, aErr := h.commSvc.AddToCart(ctx, userID, buyerOrgID(ctx), item); aErr != nil {
		h.log.ErrorContext(ctx, "add offer to cart",
			"error", aErr, "offer_id", offerID, "user_id", userID)
		h.offerAddFailed(w, r, offerID, "customer.offer.add_failed")
		return
	}

	// Record offer conversion / click
	_ = h.promoSvc.RecordOfferClick(ctx, offerID)

	if h.isHTMX(r) {
		cart, _ := h.commSvc.GetCart(ctx, userID, buyerOrgID(ctx))
		totalCount := 0
		if cart != nil {
			for _, ci := range cart.Items {
				totalCount += ci.Quantity
			}
		}
		w.Header().Set("HX-Trigger", fmt.Sprintf(`{"showToast":{"message":%+q,"type":"success"},"cartUpdated":{"count":%d}}`, i18n.T(langOf(r), "customer.offer.add_success"), totalCount))
		w.WriteHeader(http.StatusOK)
		return
	}

	h.redirectWithNotice(w, r, "/cart", "success", i18n.T(langOf(r), "customer.offer.add_success"))
}

// assertCartLineAvailable runs the availability rule and, when it refuses,
// redirects with the reason. It returns false when the caller must stop.
//
// Every buying surface goes through here so the rules cannot drift: the cart,
// the quantity controls and checkout all ask the same question.
func (h *UIHandler) assertCartLineAvailable(
	w http.ResponseWriter, r *http.Request, actor authctx.Actor,
	variantID, vendorOrgID int64, qty int, back string,
) bool {
	ctx := r.Context()

	branchID := h.buyingBranchID(ctx, &actor)

	res, err := h.cartLineAvailability(ctx, actor, branchID, variantID, vendorOrgID, qty)
	if err != nil {
		// A failed check is not permission to buy.
		h.log.ErrorContext(ctx, "availability check failed", "error", err,
			"variant", variantID, "vendor", vendorOrgID, "branch", branchID)
		if h.isHTMX(r) {
			w.Header().Set("HX-Trigger", fmt.Sprintf(`{"showToast":{"message":%+q,"type":"error"}}`, i18n.T(langOf(r), "customer.cart.availability_check_failed")))
			w.WriteHeader(http.StatusBadRequest)
			return false
		}
		h.redirectWithNotice(w, r, back, "error",
			i18n.T(langOf(r), "customer.cart.availability_check_failed"))
		return false
	}
	if !res.Allowed {
		h.log.InfoContext(ctx, "cart line refused", "reason", res.Reason,
			"variant", variantID, "vendor", vendorOrgID, "branch", branchID, "qty", qty)
		resMsg := res.Message(langOf(r))
		if h.isHTMX(r) {
			if back == "/cart" {
				cart, _ := h.commSvc.GetCart(ctx, actor.UserID, buyerOrgID(ctx))
				h.enrichCartItemsCoverage(ctx, &actor, cart, langOf(r))
				lang, _ := h.localeAndDir(r)
				w.Header().Set("HX-Trigger", fmt.Sprintf(`{"showToast":{"message":%+q,"type":"error"},"cartUpdated":{"count":%d}}`, resMsg, cartTotalItemCount(cart)))
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				_ = pages.CustomerCartContent(cart, h.cartGroupsFor(ctx, cart), lang).Render(ctx, w)
				return false
			}
			w.Header().Set("HX-Trigger", fmt.Sprintf(`{"showToast":{"message":%+q,"type":"error"}}`, resMsg))
			w.WriteHeader(http.StatusBadRequest)
			return false
		}
		h.redirectWithNotice(w, r, back, "error", resMsg)
		return false
	}
	return true
}

// offerAddFailed reports why an offer could not be added, over HTMX or a
// redirect, so the two paths cannot drift apart.
func (h *UIHandler) offerAddFailed(w http.ResponseWriter, r *http.Request, offerID int64, key string) {
	// i18n.T is printf-like to vet, and a non-constant key trips that check.
	// The key is chosen from a fixed set by the caller, never built from input.
	msg := i18n.Translate(langOf(r), key)
	if h.isHTMX(r) {
		w.Header().Set("HX-Trigger", fmt.Sprintf(`{"showToast":{"message":%+q,"type":"error"}}`, msg))
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	h.redirectWithNotice(w, r, fmt.Sprintf("/offers/%d", offerID), "error", msg)
}

// cartLineAvailability is the purchase rule for one prospective cart line at
// the buyer's branch: stock, supplier approval, branch ownership, coverage,
// institutional visibility and quota, decided by commerce.CheckAvailability.
func (h *UIHandler) cartLineAvailability(
	ctx context.Context, actor authctx.Actor, branchID, variantID, vendorOrgID int64, qty int,
) (commerce.AvailabilityResult, error) {
	return h.commSvc.CheckAvailability(ctx, commerce.AvailabilityRequest{
		VariantID:        variantID,
		VendorOrgID:      vendorOrgID,
		CustomerOrgID:    actor.OrganizationID,
		CustomerBranchID: branchID,
		Quantity:         qty,
		When:             time.Now(),
	})
}

// offerCartItem is the cart line for a bundle: its supplier and the price the
// supplier set for it, which the offer authority decides rather than any line
// arithmetic the buyer could influence. It refuses a withdrawn, unapproved or
// expired bundle, and a bundle with no price: charging an invented amount for
// it is not a price the supplier offered.
//
// Coverage and ownership are the offer rule's; callers ask offerPurchasable
// first. The web handler and Capsule's offer_add both build the line here.
func (h *UIHandler) offerCartItem(ctx context.Context, offerID int64, quantity int) (*commerce.CartItem, string, error) {
	if h.promoSvc == nil {
		return nil, "", actions.Refuse("العروض غير متاحة حالياً.")
	}
	sp, spErr := h.promoSvc.GetSpecialOffer(ctx, offerID)
	base, baseErr := h.promoSvc.GetOffer(ctx, offerID)
	if (spErr != nil || sp == nil) && (baseErr != nil || base == nil) {
		return nil, "", actions.Refuse("العرض المطلوب غير موجود.")
	}
	if sp != nil {
		if msg := validateSpecialOfferForCheckout(sp); msg != "" {
			return nil, "", actions.Refuse("%s", msg)
		}
	}

	var (
		orgID     int64
		unitPrice money.Amount
		title     string
	)
	if sp != nil {
		orgID, title = sp.OrganizationID, sp.Title.Get(i18n.AR)
		unitPrice = bundlePrice(sp)
	}
	if base != nil {
		if orgID <= 0 {
			orgID = base.OrganizationID
		}
		if title == "" {
			title = base.Title.Get(i18n.AR)
		}
		// The offer's discount value is a discount, never a price.
		if !unitPrice.IsPositive() && base.MinOrderAmount.IsPositive() {
			unitPrice = base.MinOrderAmount
		}
	}
	if orgID <= 0 {
		return nil, "", actions.Refuse("تعذّر تحديد المورد صاحب هذا العرض.")
	}
	if !unitPrice.IsPositive() {
		return nil, "", actions.Refuse("لم يحدد المورد سعراً لهذا العرض بعد، لذلك لا يمكن طلبه الآن.")
	}
	if quantity <= 0 {
		quantity = 1
	}
	id := offerID
	return &commerce.CartItem{OrganizationID: orgID, Quantity: quantity, UnitPrice: unitPrice, OfferID: &id}, title, nil
}

// bundlePrice is what a supplier charges for one bundle: the bundle price, or
// the sum of its priced lines, or its minimum order amount.
func bundlePrice(sp *promo.SpecialOffer) money.Amount {
	if sp.TotalPrice.IsPositive() {
		return sp.TotalPrice
	}
	var sum money.Amount
	for _, p := range sp.Products {
		if !p.CustomPrice.IsPositive() {
			continue
		}
		q := int64(p.Quantity)
		if q <= 0 {
			q = 1
		}
		sum, _ = sum.Add(money.FromMinor(p.CustomPrice.Minor() * q))
	}
	if sum.IsPositive() {
		return sum
	}
	return sp.MinOrderAmount
}
