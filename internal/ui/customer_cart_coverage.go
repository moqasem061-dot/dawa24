package ui

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

func cartTotalItemCount(cart *commerce.Cart) int {
	if cart == nil {
		return 0
	}
	count := 0
	for _, ci := range cart.Items {
		count += ci.Quantity
	}
	return count
}

func (h *UIHandler) enrichCartItemsCoverage(ctx context.Context, actor *authctx.Actor, cart *commerce.Cart, lang string) {
	if cart == nil || len(cart.Items) == 0 {
		return
	}
	branchID := h.buyingBranchID(ctx, actor)
	for _, it := range cart.Items {
		it.IsCovered = true
		if branchID <= 0 {
			it.IsCovered = false
			it.CoverageReason = i18n.T(lang, "buying.select_branch_first")
		} else if it.ProductVariantID > 0 && it.OrganizationID > 0 {
			orgID := int64(0)
			if actor != nil {
				orgID = actor.OrganizationID
			}
			res, err := h.commSvc.CheckAvailability(ctx, commerce.AvailabilityRequest{
				VariantID:        it.ProductVariantID,
				VendorOrgID:      it.OrganizationID,
				CustomerOrgID:    orgID,
				CustomerBranchID: branchID,
				Quantity:         it.Quantity,
				When:             time.Now(),
			})
			if err == nil && !res.Allowed {
				it.CoverageReason = res.DisplayReasonAr()
				switch res.Disposition() {
				case commerce.DispositionHidden:
					it.IsCovered = false
				case commerce.DispositionBlocked:
					// Remains covered, cart line carries blocked reason
				}
			}
		}
	}
}

// CartCountBadge renders an HTMX badge fragment showing the total number of items in the cart.
func (h *UIHandler) CartCountBadge(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || !actor.IsBuyer() || (!actor.IsOwner && !actor.CanAny("pharmacy.cart.use", "vendor.buying.cart.use")) || h.commSvc == nil {
		w.WriteHeader(http.StatusOK)
		return
	}
	cart, err := h.commSvc.GetCart(ctx, actor.UserID, buyerOrgID(ctx))
	if err != nil || cart == nil {
		w.WriteHeader(http.StatusOK)
		return
	}
	count := cartTotalItemCount(cart)
	if count <= 0 {
		w.WriteHeader(http.StatusOK)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<span class="nav-count nav-count--cart">%d</span>`, count)
}
