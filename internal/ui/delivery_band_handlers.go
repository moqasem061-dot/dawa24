package ui

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// VendorDeliveryBandCreateSubmit creates a new distance delivery fee tier for the vendor.
func (h *UIHandler) VendorDeliveryBandCreateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/coverage", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, "/vendor/coverage", "error", i18n.T(lang, "delivery.band.read_error"))
		return
	}

	fromMeters := 0
	if val := r.PostFormValue("from_meters"); val != "" {
		fromMeters, _ = strconv.Atoi(val)
	} else if val := r.PostFormValue("min_distance_meters"); val != "" {
		fromMeters, _ = strconv.Atoi(val)
	} else if val := r.PostFormValue("min_distance_km"); val != "" {
		km, _ := strconv.Atoi(val)
		fromMeters = km * 1000
	}

	toMeters := 0
	if val := r.PostFormValue("to_meters"); val != "" {
		toMeters, _ = strconv.Atoi(val)
	} else if val := r.PostFormValue("max_distance_meters"); val != "" {
		toMeters, _ = strconv.Atoi(val)
	} else if val := r.PostFormValue("max_distance_km"); val != "" {
		km, _ := strconv.Atoi(val)
		toMeters = km * 1000
	}

	feeAmount, _ := strconv.ParseFloat(r.PostFormValue("delivery_fee"), 64)

	if toMeters <= fromMeters || fromMeters < 0 || feeAmount < 0 {
		h.redirectWithNotice(w, r, "/vendor/coverage", "error", i18n.T(lang, "delivery.band.invalid_params"))
		return
	}

	if h.orgSvc != nil {
		bands, err := h.orgSvc.GetDeliveryBands(ctx, actor.OrganizationID)
		if err != nil {
			bands = []*org.DeliveryBand{}
		}
		feeAmt, _ := money.Parse(fmt.Sprintf("%.2f", feeAmount))
		newBand := &org.DeliveryBand{
			OrganizationID: actor.OrganizationID,
			FromMeters:     fromMeters,
			ToMeters:       toMeters,
			Fee:            feeAmt,
			IsActive:       true,
		}
		bands = append(bands, newBand)
		if err := h.orgSvc.SaveDeliveryBands(ctx, actor.OrganizationID, bands); err != nil {
			h.log.ErrorContext(ctx, "save delivery bands", "error", err, "org", actor.OrganizationID)
			h.redirectWithNotice(w, r, "/vendor/coverage", "error", fmt.Sprintf(i18n.T(lang, "delivery.band.save_failed_format"), err.Error()))
			return
		}
	}

	h.redirectWithNotice(w, r, "/vendor/coverage", "success", i18n.T(lang, "delivery.band.added_success"))
}

// VendorDeliveryBandDeleteSubmit removes a distance delivery fee tier.
func (h *UIHandler) VendorDeliveryBandDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/coverage", http.StatusSeeOther)
		return
	}

	bandID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || bandID <= 0 {
		h.redirectWithNotice(w, r, "/vendor/coverage", "error", i18n.T(lang, "delivery.band.invalid_id"))
		return
	}

	if h.orgSvc != nil {
		bands, err := h.orgSvc.GetDeliveryBands(ctx, actor.OrganizationID)
		if err == nil {
			var updated []*org.DeliveryBand
			for _, b := range bands {
				if b.ID != bandID {
					updated = append(updated, b)
				}
			}
			_ = h.orgSvc.SaveDeliveryBands(ctx, actor.OrganizationID, updated)
		}
	}

	h.redirectWithNotice(w, r, "/vendor/coverage", "success", i18n.T(lang, "delivery.band.deleted_success"))
}

// QuoteVendorDelivery is the same calculation with its reasoning attached, for
// the cart, which shows the pharmacy what it is being charged and why.
func (h *UIHandler) QuoteVendorDelivery(
	ctx context.Context, vendorOrgID int64, customerBranchID *int64,
) org.DeliveryQuote {
	if h.orgSvc == nil || vendorOrgID <= 0 {
		return org.DeliveryQuote{
			Fee: money.Zero, DistanceMeters: org.UnknownDistance, Basis: org.BasisNoBands,
		}
	}
	q, err := h.orgSvc.QuoteDeliveryFor(ctx, vendorOrgID, customerBranchID)
	if err != nil {
		h.log.WarnContext(ctx, "delivery quote failed", "error", err, "vendor", vendorOrgID)
		return org.DeliveryQuote{
			Fee: money.Zero, DistanceMeters: org.UnknownDistance, Basis: org.BasisNoBands,
		}
	}
	return q
}
