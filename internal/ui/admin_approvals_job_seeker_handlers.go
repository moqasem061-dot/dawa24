package ui

import (
	"context"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
)

// AdminApproveJobSeekerSubmit approves a job seeker account and activates it.
func (h *UIHandler) AdminApproveJobSeekerSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, _ := authctx.From(ctx)

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/admin/approvals?tab=job_seekers", "error", i18n.T(lang, "admin.approvals.invalid_user_id"))
		return
	}

	if h.idSvc == nil {
		h.redirectWithNotice(w, r, "/admin/approvals?tab=job_seekers", "error", i18n.T(lang, "admin.approvals.id_service_unavailable"))
		return
	}

	sysCtx := database.AsSystem(ctx)
	if err := h.idSvc.AdminReactivateUser(sysCtx, id, actor.UserID); err != nil {
		h.redirectWithNotice(w, r, "/admin/approvals?tab=job_seekers", "error", h.safeMessage(err, lang))
		return
	}

	h.safeGo("notify-job-seeker-approved", func() {
		bgCtx := database.AsSystem(context.Background())
		title := i18n.T(lang, "admin.approvals.job_seeker_notif_title")
		body := i18n.T(lang, "admin.approvals.job_seeker_notif_body")
		h.dispatchInAppNotification(bgCtx, id, nil, "", title, body)
		h.dispatchApprovalEmail(bgCtx, id, "", title, body, "https://dawa24.com/jobs")
	})

	msg := i18n.T(lang, "admin.approvals.job_seeker_approved_success")
	h.redirectWithNotice(w, r, "/admin/approvals?tab=job_seekers", "success", msg)
}

// AdminRejectJobSeekerSubmit rejects or suspends a job seeker account.
func (h *UIHandler) AdminRejectJobSeekerSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, _ := authctx.From(ctx)

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/admin/approvals?tab=job_seekers", "error", i18n.T(lang, "admin.approvals.invalid_user_id"))
		return
	}

	if h.idSvc == nil {
		h.redirectWithNotice(w, r, "/admin/approvals?tab=job_seekers", "error", i18n.T(lang, "admin.approvals.id_service_unavailable"))
		return
	}

	sysCtx := database.AsSystem(ctx)
	if err := h.idSvc.AdminSuspendUser(sysCtx, id, actor.UserID); err != nil {
		h.redirectWithNotice(w, r, "/admin/approvals?tab=job_seekers", "error", h.safeMessage(err, lang))
		return
	}

	msg := i18n.T(lang, "admin.approvals.job_seeker_rejected_success")
	h.redirectWithNotice(w, r, "/admin/approvals?tab=job_seekers", "success", msg)
}
