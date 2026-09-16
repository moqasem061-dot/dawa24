package ui

import (
	"context"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// PublicAdClick tracks ad click events and redirects the visitor to the target.
func (h *UIHandler) PublicAdClick(w http.ResponseWriter, r *http.Request) {
	adIDStr := chi.URLParam(r, "ad")
	adID, err := strconv.ParseInt(adIDStr, 10, 64)
	if err != nil || adID <= 0 {
		http.Redirect(w, r, "/catalog", http.StatusSeeOther)
		return
	}

	var userID *int64
	if actor, ok := authctx.From(r.Context()); ok && actor.UserID > 0 {
		userID = &actor.UserID
	}

	ip := h.clientIP(r)
	ua := r.UserAgent()

	// 1. Record real click in promo system
	if h.promoSvc != nil {
		_ = h.promoSvc.RecordAdClick(database.AsSystem(r.Context()), adID, userID, ip, ua)
	}

	// 2. Resolve destination URL
	destURL := "/catalog"
	if h.promoSvc != nil {
		if ad, err := h.promoSvc.GetAd(database.AsSystem(r.Context()), adID); err == nil && ad != nil {
			destURL = ad.ResolveClickURL()
		}
	}

	h.log.InfoContext(r.Context(), "tracked ad click", "ad_id", adID, "destination", destURL)
	http.Redirect(w, r, destURL, http.StatusSeeOther)
}

// PublicAdImpression tracks live advertisement views.
func (h *UIHandler) PublicAdImpression(w http.ResponseWriter, r *http.Request) {
	adIDStr := chi.URLParam(r, "ad")
	adID, err := strconv.ParseInt(adIDStr, 10, 64)
	if err == nil && adID > 0 && h.promoSvc != nil {
		var userID *int64
		if actor, ok := authctx.From(r.Context()); ok && actor.UserID > 0 {
			userID = &actor.UserID
		}
		_ = h.promoSvc.RecordAdImpression(database.AsSystem(r.Context()), adID, userID, h.clientIP(r), r.UserAgent())
	}
	w.WriteHeader(http.StatusNoContent)
}

// AdminSponsorshipRequestApproveSubmit approves a pending sponsorship request from the admin UI.
func (h *UIHandler) AdminSponsorshipRequestApproveSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if h.promoSvc == nil {
		http.Redirect(w, r, "/admin/offers-packages?tab=requests", http.StatusSeeOther)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Redirect(w, r, "/admin/offers-packages?tab=requests", http.StatusSeeOther)
		return
	}
	sysCtx := database.AsSystem(ctx)
	notes := r.PostFormValue("notes")
	req, err := h.promoSvc.AdminApproveSponsorshipRequest(sysCtx, id, notes)
	if err != nil {
		h.redirectWithNotice(w, r, "/admin/offers-packages?tab=requests", "error", h.safeMessage(err, langOf(r)))
		return
	}
	if req != nil && req.OrganizationID > 0 {
		pkgName := i18n.TDefault("w4_ui.s_80_80")
		if req.Package != nil {
			pkgName = req.Package.Name.Get("ar")
		}
		h.safeGo("notify-sponsorship-approved", func() {
			h.notifySponsorshipStatus(context.Background(), req.OrganizationID, pkgName, true, notes)
		})
	}
	h.redirectWithNotice(w, r, "/admin/offers-packages?tab=requests", "success", i18n.T(langOf(r), "admin.promo.sponsorship_approved_success"))
}

// AdminSponsorshipRequestRejectSubmit rejects a pending sponsorship request from the admin UI.
func (h *UIHandler) AdminSponsorshipRequestRejectSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if h.promoSvc == nil {
		http.Redirect(w, r, "/admin/offers-packages?tab=requests", http.StatusSeeOther)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Redirect(w, r, "/admin/offers-packages?tab=requests", http.StatusSeeOther)
		return
	}
	sysCtx := database.AsSystem(ctx)
	req, _ := h.promoSvc.GetSponsorshipRequestByID(sysCtx, id)
	notes := r.PostFormValue("notes")
	if err := h.promoSvc.AdminRejectSponsorshipRequest(sysCtx, id, notes); err != nil {
		h.redirectWithNotice(w, r, "/admin/offers-packages?tab=requests", "error", h.safeMessage(err, langOf(r)))
		return
	}
	if req != nil && req.OrganizationID > 0 {
		pkgName := i18n.TDefault("w4_ui.s_80_80")
		if req.Package != nil {
			pkgName = req.Package.Name.Get("ar")
		}
		h.safeGo("notify-sponsorship-rejected", func() {
			h.notifySponsorshipStatus(context.Background(), req.OrganizationID, pkgName, false, notes)
		})
	}
	h.redirectWithNotice(w, r, "/admin/offers-packages?tab=requests", "success", i18n.T(langOf(r), "admin.promo.sponsorship_rejected_success"))
}
