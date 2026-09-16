package commerce

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// AddToWishlist adds a product to customer's wishlist.
func (s *Service) AddToWishlist(ctx context.Context, userID int64, productID int64) error {
	return s.repo.AddToWishlist(ctx, userID, productID)
}

// RemoveFromWishlist removes a product from customer's wishlist.
func (s *Service) RemoveFromWishlist(ctx context.Context, userID int64, productID int64) error {
	return s.repo.RemoveFromWishlist(ctx, userID, productID)
}

// GetWishlist returns all wishlist items for a customer.
func (s *Service) GetWishlist(ctx context.Context, userID int64) ([]*WishlistItem, error) {
	return s.repo.ListWishlist(ctx, userID)
}

// GetCart retrieves or initializes the caller's cart, without any line their
// own company supplies.
//
// buyerOrgID is the company the caller is buying for. Self-supplied lines are
// hidden rather than deleted, because a cart belongs to a user and a user may
// be a member of two companies: a line added while acting for a pharmacy is a
// perfectly good line, and it must come back when they switch back to that
// pharmacy — it is only unbuyable while they are acting for the supplier that
// sells it.
//
// Hiding here rather than in the templates is what makes checkout agree with
// the screen. CheckAvailability refuses the same pairing, so a line that
// reached the basket before the caller switched companies cannot be ordered;
// leaving it visible-to-checkout but hidden-on-screen would fail the whole
// order over a line nobody could see.
//
// Pass 0 for a caller with no company — nothing is theirs, so nothing is
// hidden.
//
// GetCart, GetCartLine and AddToCart all take (userID, buyerOrgID) in that
// order, and never in the other. Both are int64, so a transposed call compiles
// and silently reads the wrong cart; keeping one order across the three is the
// only defence the language offers.
func (s *Service) GetCart(ctx context.Context, userID, buyerOrgID int64) (*Cart, error) {
	cart, err := s.repo.GetOrCreateCart(ctx, userID)
	if err != nil {
		return nil, err
	}
	full, err := s.repo.GetCartWithItems(ctx, cart.ID)
	if err != nil {
		return nil, err
	}
	return withoutSelfSuppliedLines(full, buyerOrgID), nil
}

// withoutSelfSuppliedLines returns the cart without lines supplied by buyerOrgID.
func withoutSelfSuppliedLines(cart *Cart, buyerOrgID int64) *Cart {
	if cart == nil || buyerOrgID <= 0 || len(cart.Items) == 0 {
		return cart
	}
	kept := make([]*CartItem, 0, len(cart.Items))
	for _, item := range cart.Items {
		if item != nil && item.OrganizationID == buyerOrgID {
			continue
		}
		kept = append(kept, item)
	}
	cart.Items = kept
	return cart
}

// GetCartLine returns one line of the user's cart, or nil when the variant is
// not in it. The quantity controls need the line's supplier so a re-check can
// run even when the form does not resend it.
//
// It reads through GetCart, so a line the caller's own company supplies is not
// in it: a line you cannot see is a line you cannot re-price.
func (s *Service) GetCartLine(ctx context.Context, userID, buyerOrgID, variantID int64) (*CartItem, error) {
	cart, err := s.GetCart(ctx, userID, buyerOrgID)
	if err != nil {
		return nil, err
	}
	if cart == nil {
		return nil, nil
	}
	for _, it := range cart.Items {
		if it.ProductVariantID == variantID {
			return it, nil
		}
	}
	return nil, nil
}

// AddToCart adds or updates an item in the caller's cart.
//
// buyerOrgID is the company the caller is buying for. A line that company
// supplies is refused here rather than only at the screen: the JSON API reaches
// this function too, and a rule enforced in one of two callers is not enforced.
func (s *Service) AddToCart(ctx context.Context, userID, buyerOrgID int64, item *CartItem) (*Cart, error) {
	if buyerOrgID > 0 && item.OrganizationID == buyerOrgID {
		return nil, apperr.Validation(
			"cart.line_unavailable."+string(ReasonOwnOrganization),
			i18n.T("ar", "err.own_organization_supply"), nil)
	}

	// Always resolve and enforce authentic catalog price for variant cart lines
	if s.availability != nil && item.ProductVariantID > 0 {
		if v, err := s.availability.Variant(ctx, item.ProductVariantID); err == nil && v.ID > 0 {
			if item.OrganizationID <= 0 {
				item.OrganizationID = v.OrganizationID
			}
			if v.EffectivePrice.IsPositive() {
				item.UnitPrice = v.EffectivePrice
			} else if v.Price.IsPositive() {
				item.UnitPrice = v.Price
			}
		}
	}

	cart, err := s.repo.GetOrCreateCart(ctx, userID)
	if err != nil {
		return nil, err
	}
	if err := s.repo.AddToCartItem(ctx, cart.ID, item); err != nil {
		return nil, err
	}
	full, err := s.repo.GetCartWithItems(ctx, cart.ID)
	if err != nil {
		return nil, err
	}
	return withoutSelfSuppliedLines(full, buyerOrgID), nil
}

// RemoveFromCart removes an item from cart.
func (s *Service) RemoveFromCart(ctx context.Context, userID int64, variantID int64) (*Cart, error) {
	cart, err := s.repo.GetOrCreateCart(ctx, userID)
	if err != nil {
		return nil, err
	}
	if err := s.repo.RemoveCartItem(ctx, cart.ID, variantID); err != nil {
		return nil, err
	}
	return s.repo.GetCartWithItems(ctx, cart.ID)
}

// RemoveCartLine removes one cart line by its own id.
//
// Offer lines have no variant to key off, so RemoveFromCart cannot reach them.
func (s *Service) RemoveCartLine(ctx context.Context, userID, itemID int64) (*Cart, error) {
	cart, err := s.repo.GetOrCreateCart(ctx, userID)
	if err != nil {
		return nil, err
	}
	if err := s.repo.RemoveCartItemByID(ctx, cart.ID, itemID); err != nil {
		return nil, err
	}
	return s.repo.GetCartWithItems(ctx, cart.ID)
}

// SetCartLineQuantity sets an absolute quantity on one cart line, removing the
// line when the quantity reaches zero.
func (s *Service) SetCartLineQuantity(ctx context.Context, userID, itemID int64, qty int) (*Cart, error) {
	cart, err := s.repo.GetOrCreateCart(ctx, userID)
	if err != nil {
		return nil, err
	}

	// An offer's quantity belongs to the offer, not to the buyer.
	//
	// A bundle is priced, stocked and approved as one thing. Multiplying it in
	// the cart produced a total the vendor never quoted, and the control that
	// did it sent no variant, so nothing checked whether the extra bundles
	// could be filled — the refusal arrived at checkout as "بيانات الطلب غير
	// صالحة", after the pharmacy had committed to the order.
	//
	// The rule lives here rather than in the handler because the cart page, the
	// htmx stepper and order editing all reach this method, and only one of the
	// three would have remembered to ask.
	if qty > 0 {
		current, err := s.repo.GetCartWithItems(ctx, cart.ID)
		if err != nil {
			return nil, err
		}
		for _, line := range current.Items {
			if line.ID == itemID && line.IsOfferLine() && line.Quantity != qty {
				return nil, apperr.Validation("cart.offer_quantity_locked",
					i18n.TDefault("customer.offer.quantity_locked"), nil)
			}
		}
	}

	if err := s.repo.SetCartItemQuantityByID(ctx, cart.ID, itemID, qty); err != nil {
		return nil, err
	}
	return s.repo.GetCartWithItems(ctx, cart.ID)
}

// ClearCart empties the cart.
func (s *Service) ClearCart(ctx context.Context, userID int64) error {
	cart, err := s.repo.GetOrCreateCart(ctx, userID)
	if err != nil {
		return err
	}
	return s.repo.ClearCart(ctx, cart.ID)
}
