package ui

import (
	"context"
	"fmt"
	"net/http"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	"github.com/moqasem061-dot/dawa24/internal/modules/promo"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

func (h *UIHandler) priceCartItem(ctx context.Context, cartLine *commerce.CartItem) error {
	if cartLine == nil {
		return fmt.Errorf("catalogue price is unavailable")
	}
	if h.catSvc == nil {
		if cartLine.OfferID != nil {
			return fmt.Errorf("selected supply cannot be verified")
		}
		return nil
	}
	if cartLine.ProductVariantID <= 0 {
		return h.priceUnvariantedCartLine(ctx, cartLine)
	}
	variant, err := h.catSvc.GetVariant(database.AsSystem(ctx), cartLine.ProductVariantID)
	if err != nil {
		h.logLoadErr(ctx, "catSvc.GetVariant for cart price", err)
		return err
	}
	if !validCartVariant(cartLine, variant) {
		return fmt.Errorf("selected supply is unavailable")
	}
	return h.setCartItemPrice(ctx, cartLine, variant)
}

func (h *UIHandler) priceUnvariantedCartLine(ctx context.Context, cartLine *commerce.CartItem) error {
	if cartLine.OfferID != nil {
		return fmt.Errorf("selected supply has no variant")
	}
	product, _, err := h.catSvc.GetProduct(database.AsSystem(ctx), cartLine.ProductID)
	if err != nil {
		return err
	}
	if product != nil {
		cartLine.ProductName = product.Name
		cartLine.UnitPrice = product.EffectivePrice()
	}
	return nil
}

func (h *UIHandler) setCartItemPrice(ctx context.Context, cartLine *commerce.CartItem, variant *catalog.ProductVariant) error {
	if cartLine.OfferID != nil {
		price, err := h.promotionalCartPrice(ctx, cartLine, variant.Price)
		if err != nil {
			return err
		}
		cartLine.UnitPrice = price
	} else {
		cartLine.UnitPrice = variant.EffectiveSellingPrice()
	}
	if !cartLine.UnitPrice.IsPositive() {
		return fmt.Errorf("selected supply has no purchasable price")
	}
	return nil
}

func validCartVariant(cartLine *commerce.CartItem, variant *catalog.ProductVariant) bool {
	return cartLine != nil && variant != nil && variant.ID == cartLine.ProductVariantID &&
		variant.ProductID == cartLine.ProductID && variant.OrganizationID == cartLine.OrganizationID &&
		variant.Status == catalog.StatusActive && variant.DeletedAt == nil
}

func (h *UIHandler) promotionalCartPrice(ctx context.Context, cartLine *commerce.CartItem, listPrice money.Amount) (money.Amount, error) {
	if h.promoSvc == nil || cartLine.OfferID == nil || *cartLine.OfferID <= 0 {
		return money.Zero, fmt.Errorf("selected promotion is unavailable")
	}
	actor, ok := authctx.From(ctx)
	if !ok {
		return money.Zero, fmt.Errorf("buyer session is unavailable")
	}
	if allowed, _ := h.offerPurchasable(ctx, actor, *cartLine.OfferID, 0); !allowed {
		return money.Zero, fmt.Errorf("selected promotion is no longer available")
	}
	rows, err := h.promoSvc.ListOffersForProduct(ctx, cartLine.ProductID)
	if err != nil {
		h.logLoadErr(ctx, "promoSvc.ListOffersForProduct for cart price", err)
		return money.Zero, err
	}
	return selectedPromotionalPrice(rows, cartLine, listPrice)
}

func selectedPromotionalPrice(rows []*promo.OfferProductWithOffer, cartLine *commerce.CartItem, listPrice money.Amount) (money.Amount, error) {
	for _, offerLine := range rows {
		if !matchesPromotionalCartLine(offerLine, cartLine) {
			continue
		}
		quotedPrice, _ := promo.EffectivePrice(listPrice, offerLine.Product, offerLine.Offer)
		if quotedPrice.IsPositive() {
			return quotedPrice, nil
		}
	}
	return money.Zero, fmt.Errorf("selected promotion does not include this supply")
}

func matchesPromotionalCartLine(offerLine *promo.OfferProductWithOffer, cartLine *commerce.CartItem) bool {
	if offerLine == nil || offerLine.Offer == nil || offerLine.Product == nil || cartLine == nil || cartLine.OfferID == nil {
		return false
	}
	if offerLine.Offer.ID != *cartLine.OfferID || offerLine.Offer.OrganizationID != cartLine.OrganizationID || offerLine.Product.ProductID != cartLine.ProductID {
		return false
	}
	return offerLine.Product.VariantID == nil || *offerLine.Product.VariantID == cartLine.ProductVariantID
}

func (h *UIHandler) rejectCartPrice(w http.ResponseWriter, r *http.Request, back string) {
	message := i18n.T(langOf(r), "customer.cart.price_unavailable")
	if h.isHTMX(r) {
		w.Header().Set("HX-Trigger", fmt.Sprintf(`{"showToast":{"message":%+q,"type":"error"}}`, message))
		w.WriteHeader(http.StatusConflict)
		return
	}
	h.redirectWithNotice(w, r, back, "error", message)
}
