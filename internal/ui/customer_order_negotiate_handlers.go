package ui

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

func (h *UIHandler) CustomerNegotiateOrderSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/catalog", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, "/catalog", "error", i18n.T(langOf(r), "common.form_invalid"))
		return
	}

	variantID, _ := strconv.ParseInt(r.PostFormValue("variant_id"), 10, 64)
	vendorOrgID, _ := strconv.ParseInt(r.PostFormValue("vendor_org_id"), 10, 64)
	qty, _ := strconv.Atoi(r.PostFormValue("qty"))
	if qty <= 0 {
		qty = 1
	}

	proposedPriceStr := r.PostFormValue("proposed_price")
	proposedPrice, err := money.Parse(proposedPriceStr)
	if err != nil || proposedPrice.IsZero() || proposedPrice.IsNegative() {
		h.redirectWithNotice(w, r, "/catalog", "error", i18n.T(langOf(r), "customer.order.negotiate_invalid_price"))
		return
	}

	notes := strings.TrimSpace(r.PostFormValue("notes"))
	if notes == "" {
		notes = fmt.Sprintf(i18n.T(langOf(r), "customer.order.negotiate_notes_default"), proposedPrice.String(), qty)
	}

	var branchID *int64
	if bStr := r.PostFormValue("branch_id"); bStr != "" {
		if bID, err := strconv.ParseInt(bStr, 10, 64); err == nil && bID > 0 {
			branchID = &bID
		}
	}
	if branchID == nil {
		if buying, ok := authctx.BuyingBranchFrom(ctx); ok && buying.Active != nil && *buying.Active > 0 {
			branchID = buying.Active
		} else if actor.BranchID != nil && *actor.BranchID > 0 {
			branchID = actor.BranchID
		}
	}

	var productID int64
	prodName := i18n.Text{"ar": i18n.TDefault("w4_ui.s_68_68"), "en": "Negotiated Medicine"}
	if h.catSvc != nil && variantID > 0 {
		if v, err := h.catSvc.GetVariant(database.AsSystem(ctx), variantID); err == nil && v != nil {
			productID = v.ProductID
			if vendorOrgID <= 0 {
				vendorOrgID = v.OrganizationID
			}
			if len(v.Name) > 0 {
				prodName = v.Name
			}
		}
	}

	paymentMethod := r.PostFormValue("payment_method")
	if paymentMethod == "" {
		paymentMethod = "cod"
	}

	var pIDPtr *int64
	if productID > 0 {
		pIDPtr = &productID
	}
	var vIDPtr *int64
	if variantID > 0 {
		vIDPtr = &variantID
	}

	input := commerce.CheckoutInput{
		CustomerID:       actor.UserID,
		CustomerOrgID:    actor.OrganizationID,
		CustomerOrgType:  actor.OrgType,
		BranchID:         branchID,
		PaymentMethod:    paymentMethod,
		Notes:            notes,
		IsNegotiation:    true,
		NegotiationNotes: notes,
		Items: []commerce.CheckoutLineItem{
			{
				ProductVariantID:  vIDPtr,
				ProductID:         pIDPtr,
				VendorOrgID:       vendorOrgID,
				ProductName:       prodName,
				Quantity:          qty,
				UnitPrice:         proposedPrice,
				ProposedUnitPrice: proposedPrice,
				IsNegotiated:      true,
			},
		},
	}

	if h.commSvc == nil {
		h.redirectWithNotice(w, r, "/catalog", "error", i18n.T(langOf(r), "customer.order.negotiate_service_unavailable"))
		return
	}

	order, err := h.commSvc.Checkout(ctx, input)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to submit negotiation order", "error", err, "variant_id", variantID)
		h.redirectWithNotice(w, r, "/catalog", "error", fmt.Sprintf(i18n.T(langOf(r), "customer.order.negotiate_error"), h.safeMessage(err, langOf(r))))
		return
	}

	if order != nil {
		custOrgName := ""
		if actor.OrganizationID > 0 {
			custOrgName = h.resolveOrgName(ctx, actor.OrganizationID)
		}
		h.safeGo("notify-negotiation-offer", func() {
			h.notifyNegotiationOffer(context.Background(), vendorOrgID, custOrgName, order.OrderNumber, proposedPrice)
		})
	}

	h.redirectWithNotice(w, r, fmt.Sprintf("/orders/%d", order.ID), "success", i18n.T(langOf(r), "customer.order.negotiate_success"))
}

// enrichOrderPricing populates retail list prices, discount percentages, and offer links
// for display on order detail views when historical snapshots were omitted.
