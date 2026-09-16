package ui

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func (h *UIHandler) CustomerCartPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/cart", http.StatusSeeOther)
		return
	}

	if !actor.IsBuyer() {
		h.redirectWithNotice(w, r, "/catalog", "error", i18n.T(langOf(r), "customer.cart.pharmacy_only"))
		return
	}

	if h.commSvc == nil {
		h.renderPage(ctx, w, "render cart page", pages.CustomerCart(nil, nil, lang, dir, h.isHTMX(r)))
		return
	}

	cart, err := h.commSvc.GetCart(ctx, actor.UserID, buyerOrgID(ctx))
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	h.enrichCartItemsCoverage(ctx, &actor, cart, langOf(r))

	h.renderPage(ctx, w, "render cart page",
		pages.CustomerCart(cart, h.cartGroups(ctx, &actor, cart), lang, dir, h.isHTMX(r)))
}

func (h *UIHandler) AddToCartSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		if h.isHTMX(r) {
			w.Header().Set("HX-Redirect", "/auth/login?redirect=/cart")
			w.Header().Set("HX-Trigger", fmt.Sprintf(`{"showToast":{"message":%+q,"type":"error"}}`, i18n.T(langOf(r), "customer.cart.login_required")))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		http.Redirect(w, r, "/auth/login?redirect=/cart", http.StatusSeeOther)
		return
	}

	if !actor.IsBuyer() {
		if h.isHTMX(r) {
			w.Header().Set("HX-Trigger", fmt.Sprintf(`{"showToast":{"message":%+q,"type":"error"}}`, i18n.T(langOf(r), "customer.cart.add_pharmacy_only")))
			w.WriteHeader(http.StatusForbidden)
			return
		}
		h.redirectWithNotice(w, r, "/catalog", "error", i18n.T(langOf(r), "customer.cart.order_pharmacy_only"))
		return
	}

	userID := actor.UserID
	if h.commSvc == nil {
		if h.isHTMX(r) {
			w.Header().Set("HX-Trigger", fmt.Sprintf(`{"showToast":{"message":%+q,"type":"error"}}`, i18n.T(langOf(r), "customer.cart.service_unavailable")))
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		http.Redirect(w, r, "/cart", http.StatusSeeOther)
		return
	}

	variantID, _ := strconv.ParseInt(r.PostFormValue("variant_id"), 10, 64)
	productID, _ := strconv.ParseInt(r.PostFormValue("product_id"), 10, 64)
	vendorOrgID, _ := strconv.ParseInt(r.PostFormValue("vendor_org_id"), 10, 64)
	if vendorOrgID <= 0 {
		vendorOrgID, _ = strconv.ParseInt(r.PostFormValue("organization_id"), 10, 64)
	}

	qty, _ := strconv.Atoi(r.PostFormValue("quantity"))
	if qty <= 0 {
		qty, _ = strconv.Atoi(r.PostFormValue("qty"))
	}
	if qty <= 0 {
		qty = 1
	}

	back := strings.TrimSpace(r.PostFormValue("return_to"))
	if back == "" {
		back = r.Header.Get("Referer")
	}
	if back == "" {
		back = "/cart"
	}

	variantID, productID, vendorOrgID = h.resolveCartLine(ctx, variantID, productID, vendorOrgID)

	// Stock, supplier approval, branch ownership and weekly coverage are all
	// decided by commerce.CheckAvailability. Nothing here defaults a missing
	// supplier or quietly reduces the quantity the pharmacy asked for.
	if !h.assertCartLineAvailable(w, r, actor, variantID, vendorOrgID, qty, back) {
		return
	}

	item := &commerce.CartItem{
		ProductID:        productID,
		ProductVariantID: variantID,
		OrganizationID:   vendorOrgID,
		Quantity:         qty,
	}

	// Keep the offer identity if an offer was selected
	if offerID, err := strconv.ParseInt(r.PostFormValue("offer_id"), 10, 64); err == nil && offerID > 0 {
		item.OfferID = &offerID
	}

	h.priceCartItem(ctx, item)

	if _, err := h.commSvc.AddToCart(ctx, userID, buyerOrgID(ctx), item); err != nil {
		h.log.ErrorContext(ctx, "add to cart", "error", err,
			"user", userID, "variant", variantID, "vendor", vendorOrgID)
		if h.isHTMX(r) {
			w.Header().Set("HX-Trigger", fmt.Sprintf(`{"showToast":{"message":%+q,"type":"error"}}`, h.safeMessage(err, langOf(r))))
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		h.redirectWithNotice(w, r, back, "error", h.safeMessage(err, langOf(r)))
		return
	}

	if h.isHTMX(r) {
		cart, _ := h.commSvc.GetCart(ctx, userID, buyerOrgID(ctx))
		itemCount := 0
		if cart != nil {
			for _, ci := range cart.Items {
				itemCount += ci.Quantity
			}
		}
		w.Header().Set("HX-Trigger", fmt.Sprintf(`{"showToast":{"message":%+q,"type":"success"},"cartUpdated":{"count":%d}}`, i18n.T(langOf(r), "customer.cart.add_success"), itemCount))
		w.WriteHeader(http.StatusOK)
		return
	}

	if returnTo := strings.TrimSpace(r.PostFormValue("return_to")); returnTo != "" {
		h.redirectWithNotice(w, r, returnTo, "success", i18n.T(langOf(r), "customer.cart.add_success"))
		return
	}

	http.Redirect(w, r, "/cart", http.StatusSeeOther)
}

func (h *UIHandler) RemoveFromCartSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, err := authctx.UserID(ctx)
	if err != nil {
		http.Redirect(w, r, "/auth/login?redirect=/cart", http.StatusSeeOther)
		return
	}

	if h.commSvc == nil {
		http.Redirect(w, r, "/cart", http.StatusSeeOther)
		return
	}

	// item_id addresses any line, including an offer line, which has no
	// variant to key off. variant_id remains for markup not yet updated.
	if itemID, _ := strconv.ParseInt(r.PostFormValue("item_id"), 10, 64); itemID > 0 {
		if _, rErr := h.commSvc.RemoveCartLine(ctx, userID, itemID); rErr != nil {
			h.log.ErrorContext(ctx, "remove cart line", "error", rErr, "item_id", itemID)
		}
	} else {
		variantID, _ := strconv.ParseInt(r.PostFormValue("variant_id"), 10, 64)
		if _, rErr := h.commSvc.RemoveFromCart(ctx, userID, variantID); rErr != nil {
			h.log.ErrorContext(ctx, "remove cart item", "error", rErr, "variant_id", variantID)
		}
	}

	if h.isHTMX(r) {
		cart, _ := h.commSvc.GetCart(ctx, userID, buyerOrgID(ctx))
		actor, _ := authctx.From(ctx)
		h.enrichCartItemsCoverage(ctx, &actor, cart, langOf(r))
		w.Header().Set("HX-Trigger", fmt.Sprintf(`{"cartUpdated":{"count":%d}}`, cartTotalItemCount(cart)))
		lang, _ := h.localeAndDir(r)
		h.renderPage(ctx, w, "render customer cart content",
			pages.CustomerCartContent(cart, h.cartGroupsFor(ctx, cart), lang))
		return
	}

	http.Redirect(w, r, "/cart", http.StatusSeeOther)
}

func (h *UIHandler) UpdateCartQuantitySubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, err := authctx.UserID(ctx)
	if err != nil {
		http.Redirect(w, r, "/auth/login?redirect=/cart", http.StatusSeeOther)
		return
	}

	if h.commSvc == nil {
		http.Redirect(w, r, "/cart", http.StatusSeeOther)
		return
	}

	variantID, _ := strconv.ParseInt(r.PostFormValue("variant_id"), 10, 64)
	itemID, _ := strconv.ParseInt(r.PostFormValue("item_id"), 10, 64)
	qty, _ := strconv.Atoi(r.PostFormValue("quantity"))
	if qty < 0 {
		qty = 0
	}

	// An offer line has no variant, so it is addressed by its own id and skips
	// the stock check below: an offer is sold as a unit by its supplier, and
	// there is no variant whose stock could answer the question.
	if itemID > 0 && variantID <= 0 {
		if _, err := h.commSvc.SetCartLineQuantity(ctx, userID, itemID, qty); err != nil {
			h.log.ErrorContext(ctx, "set cart line quantity", "error", err,
				"user", userID, "item", itemID, "qty", qty)
			h.redirectWithNotice(w, r, "/cart", "error", h.safeMessage(err, langOf(r)))
			return
		}
		if h.isHTMX(r) {
			cart, _ := h.commSvc.GetCart(ctx, userID, buyerOrgID(ctx))
			actor, _ := authctx.From(ctx)
			h.enrichCartItemsCoverage(ctx, &actor, cart, langOf(r))
			w.Header().Set("HX-Trigger", fmt.Sprintf(`{"cartUpdated":{"count":%d}}`, cartTotalItemCount(cart)))
			lang, _ := h.localeAndDir(r)
			h.renderPage(ctx, w, "render customer cart content", pages.CustomerCartContent(cart, h.cartGroupsFor(ctx, cart), lang))
			return
		}
		http.Redirect(w, r, "/cart", http.StatusSeeOther)
		return
	}

	// Raising a quantity is a purchase decision and gets the same check as
	// adding the line. The client's "+" button is a hint; this is the rule.
	// Quantity 0 means "remove", which needs no availability check.
	if qty > 0 {
		actor, ok := authctx.From(ctx)
		if !ok {
			http.Redirect(w, r, "/auth/login?redirect=/cart", http.StatusSeeOther)
			return
		}
		vendorOrgID, _ := strconv.ParseInt(r.PostFormValue("vendor_org_id"), 10, 64)
		if vendorOrgID <= 0 {
			// The cart row knows its supplier even when the form omits it.
			if line, err := h.commSvc.GetCartLine(ctx, userID, buyerOrgID(ctx), variantID); err == nil && line != nil {
				vendorOrgID = line.OrganizationID
			}
		}
		if !h.assertCartLineAvailable(w, r, actor, variantID, vendorOrgID, qty, "/cart") {
			return
		}
	}

	if _, err := h.commSvc.SetCartQuantity(ctx, userID, variantID, qty); err != nil {
		h.log.ErrorContext(ctx, "set cart quantity", "error", err,
			"user", userID, "variant", variantID, "qty", qty)
		h.redirectWithNotice(w, r, "/cart", "error", h.safeMessage(err, langOf(r)))
		return
	}

	if h.isHTMX(r) {
		cart, _ := h.commSvc.GetCart(ctx, userID, buyerOrgID(ctx))
		actor, _ := authctx.From(ctx)
		h.enrichCartItemsCoverage(ctx, &actor, cart, langOf(r))
		w.Header().Set("HX-Trigger", fmt.Sprintf(`{"cartUpdated":{"count":%d}}`, cartTotalItemCount(cart)))
		lang, _ := h.localeAndDir(r)
		h.renderPage(ctx, w, "render customer cart content",
			pages.CustomerCartContent(cart, h.cartGroupsFor(ctx, cart), lang))
		return
	}

	http.Redirect(w, r, "/cart", http.StatusSeeOther)
}

// cartGroups partitions the cart by supplier and prices each supplier's
// delivery to the pharmacy's active branch.
//
// The quote is computed here rather than in the template because it is a
// database read per supplier — the vendor's own warehouse coordinates and their
// شرائح ورسوم التوصيل — and a template is the wrong place to do I/O. Two or
// three suppliers is the usual basket, so this is a handful of cached reads.
func (h *UIHandler) cartGroups(
	ctx context.Context, actor *authctx.Actor, cart *commerce.Cart,
) []pages.CartGroup {
	groups := pages.GroupCartBySupplier(cart)
	if h.orgSvc == nil || len(groups) == 0 {
		return groups
	}
	var branchID *int64
	if id := h.buyingBranchID(ctx, actor); id > 0 {
		branchID = &id
	}
	for i := range groups {
		groups[i].Delivery = h.QuoteVendorDelivery(ctx, groups[i].OrganizationID, branchID)
	}
	return groups
}

// cartGroupsFor is the same thing for the HTMX partials, which have the request
// context but not an already-resolved actor.
func (h *UIHandler) cartGroupsFor(ctx context.Context, cart *commerce.Cart) []pages.CartGroup {
	actor, ok := authctx.From(ctx)
	if !ok {
		return pages.GroupCartBySupplier(cart)
	}
	return h.cartGroups(ctx, &actor, cart)
}

// resolveCartLine fills a missing product or supplier from the variant, or a
// missing variant from the product and supplier.
func (h *UIHandler) resolveCartLine(ctx context.Context, variantID, productID, vendorOrgID int64) (int64, int64, int64) {
	if h.catSvc == nil {
		return variantID, productID, vendorOrgID
	}
	if variantID > 0 && (productID <= 0 || vendorOrgID <= 0) {
		if v, err := h.catSvc.GetVariant(database.AsSystem(ctx), variantID); err == nil && v != nil {
			if productID <= 0 {
				productID = v.ProductID
			}
			if vendorOrgID <= 0 {
				vendorOrgID = v.OrganizationID
			}
		}
	} else if variantID <= 0 && productID > 0 && vendorOrgID > 0 {
		if vars, err := h.catSvc.ListVariantsByProduct(database.AsSystem(ctx), productID); err == nil {
			for _, v := range vars {
				if v != nil && v.OrganizationID == vendorOrgID && v.DeletedAt == nil && v.Status == catalog.StatusActive {
					variantID = v.ID
					break
				}
			}
		}
	}
	return variantID, productID, vendorOrgID
}

// priceCartItem sets the line's price from the catalogue. Prices are never
// client-controlled.
func (h *UIHandler) priceCartItem(ctx context.Context, item *commerce.CartItem) {
	if h.catSvc == nil {
		return
	}
	if item.ProductVariantID > 0 {
		if v, err := h.catSvc.GetVariant(database.AsSystem(ctx), item.ProductVariantID); err == nil && v != nil && !v.Price.IsZero() {
			item.UnitPrice = v.EffectiveSellingPrice()
		}
	}
	if item.UnitPrice.IsZero() && item.ProductID > 0 {
		if prod, _, err := h.catSvc.GetProduct(database.AsSystem(ctx), item.ProductID); err == nil && prod != nil {
			item.ProductName = prod.Name
			item.UnitPrice = prod.EffectivePrice()
		}
	}
}
