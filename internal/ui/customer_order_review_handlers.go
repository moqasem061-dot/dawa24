package ui

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

func (h *UIHandler) ReviewSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || !actor.IsBuyer() {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	_ = r.ParseForm()
	targetOrgID, _ := strconv.ParseInt(r.PostFormValue("organization_id"), 10, 64)
	redirectURL := r.PostFormValue("redirect_url")
	if redirectURL == "" {
		redirectURL = fmt.Sprintf("/suppliers/%d", targetOrgID)
	}
	if targetOrgID <= 0 {
		h.redirectWithNotice(w, r, redirectURL, "error", i18n.T(langOf(r), "customer.order.invalid_target_org"))
		return
	}
	// A company rating itself is not a rating. It became reachable when
	// suppliers gained the buying surface and could post this form at their
	// own id.
	if ownedByBuyer(buyerOrgID(ctx), targetOrgID) {
		h.redirectWithNotice(w, r, redirectURL, "error", i18n.T(langOf(r), "err.own_organization_supply"))
		return
	}

	var orderID *int64
	var shipmentID *int64
	if oIDStr := r.PostFormValue("order_id"); oIDStr != "" {
		if oID, err := strconv.ParseInt(oIDStr, 10, 64); err == nil && oID > 0 {
			orderID = &oID
		}
	}
	if sIDStr := r.PostFormValue("shipment_id"); sIDStr != "" {
		if sID, err := strconv.ParseInt(sIDStr, 10, 64); err == nil && sID > 0 {
			shipmentID = &sID
		}
	}

	lang := langOf(r)
	if orderID != nil && h.commSvc != nil {
		ord, err := h.commSvc.GetOrder(ctx, *orderID)
		isOwner := ord != nil && (ord.CustomerID == actor.UserID || (ord.OrganizationID != nil && *ord.OrganizationID == actor.OrganizationID))
		if err != nil || !isOwner {
			h.redirectWithNotice(w, r, redirectURL, "error", i18n.T(lang, "orders.review.invalid_order"))
			return
		}
		var vendorShipment *commerce.OrderShipment
		for _, s := range ord.Shipments {
			if s != nil && s.OrganizationID == targetOrgID {
				vendorShipment = s
				break
			}
		}
		if vendorShipment == nil || (vendorShipment.Status != commerce.StatusDelivered && vendorShipment.Status != commerce.StatusCompleted) {
			h.redirectWithNotice(w, r, redirectURL, "error", i18n.T(lang, "orders.review.not_delivered"))
			return
		}
		if shipmentID == nil && vendorShipment != nil {
			shipmentID = &vendorShipment.ID
		}
	} else if h.orgSvc != nil {
		hasPurchased, _ := h.orgSvc.HasDeliveredOrderFromVendor(ctx, actor.OrganizationID, targetOrgID)
		if !hasPurchased {
			h.redirectWithNotice(w, r, redirectURL, "error", i18n.T(lang, "orders.review.not_completed_purchase"))
			return
		}
	}

	repScore, _ := strconv.Atoi(r.PostFormValue("rating_rep"))
	speedScore, _ := strconv.Atoi(r.PostFormValue("rating_speed"))
	qualityScore, _ := strconv.Atoi(r.PostFormValue("rating_quality"))

	if repScore < 1 || repScore > 5 {
		repScore = 5
	}
	if speedScore < 1 || speedScore > 5 {
		speedScore = 5
	}
	if qualityScore < 1 || qualityScore > 5 {
		qualityScore = 5
	}
	overallScore := int(math.Round(float64(repScore+speedScore+qualityScore) / 3.0))
	if overallScore < 1 {
		overallScore = 1
	}
	if overallScore > 5 {
		overallScore = 5
	}

	reviewerOrgID := actor.OrganizationID

	rev := &org.Review{
		OrganizationID: targetOrgID,
		UserID:         actor.UserID,
		OrderID:        orderID,
		ShipmentID:     shipmentID,
		ReviewerOrgID:  &reviewerOrgID,
		Rating:         overallScore,
		ReviewText:     strings.TrimSpace(r.PostFormValue("review_text")),
		Context:        "supplier",
		IsApproved:     true,
		Ratings: []org.ReviewRating{
			{Criterion: "rep", Score: repScore},
			{Criterion: "speed", Score: speedScore},
			{Criterion: "quality", Score: qualityScore},
		},
	}

	if h.orgSvc != nil {
		if err := h.orgSvc.SubmitReview(ctx, rev); err != nil {
			h.log.ErrorContext(ctx, "failed to submit review", "error", err, "target_org_id", targetOrgID)
			h.redirectWithNotice(w, r, redirectURL, "error", h.safeMessage(err, lang))
			return
		}
		h.notifyNewReview(ctx, targetOrgID, overallScore, rev.ReviewText)
	}

	h.redirectWithNotice(w, r, redirectURL, "success", i18n.T(lang, "orders.review.success_recorded"))
}

// CustomerNegotiateOrderSubmit initiates a price negotiation order with a supplier.
