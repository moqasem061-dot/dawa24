package ui

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
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

	msg := "تم اعتماد وتفعيل حساب الباحث عن عمل بنجاح"
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

	msg := "تم إيقاف / رفض حساب الباحث عن عمل"
	h.redirectWithNotice(w, r, "/admin/approvals?tab=job_seekers", "success", msg)
}
