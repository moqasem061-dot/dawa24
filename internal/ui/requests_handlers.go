package ui

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// RequestsPage renders the document/action request inbox.
func (h *UIHandler) RequestsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/requests", http.StatusSeeOther)
		return
	}

	limit := pagination.RowsPerPage(r)
	page := pagination.PageNumber(r)
	offset := (page - 1) * limit

	data := pages.RequestsData{
		CurrentOrgID: actor.OrganizationID,
		Page:         page,
		PerPage:      limit,
	}
	if h.wfSvc != nil && actor.OrganizationID > 0 {
		data.Requests, data.TotalCount, _ = h.wfSvc.ListInboxWithTotal(ctx, actor.OrganizationID, r.URL.Query().Get("status"), limit, offset)
	}
	if h.orgSvc != nil {
		typ := org.TypeVendor
		status := org.StatusApproved
		data.Suppliers, _ = h.orgSvc.ListOrganizations(ctx, &typ, &status, 50, 0)
	}

	h.renderPage(ctx, w, "render requests page", pages.RequestsPage(lang, dir, r.URL.Query().Get("status"), data))
}

// RequestCreateSubmit sends a document/action request to another organization.
func (h *UIHandler) RequestCreateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/requests", http.StatusSeeOther)
		return
	}

	toOrgID, _ := strconv.ParseInt(r.PostFormValue("to_org_id"), 10, 64)
	title := i18n.New(r.PostFormValue("title"), r.PostFormValue("title"))
	if h.wfSvc == nil {
		h.redirectWithNotice(w, r, "/requests", "error", i18n.T(lang, "admin.import.service_unavailable"))
		return
	}

	_, err := h.wfSvc.CreateRequest(ctx, actor.UserID, actor.OrganizationID, toOrgID,
		workflow.RequestType(r.PostFormValue("type")), title, r.PostFormValue("description"))
	if err != nil {
		h.redirectWithNotice(w, r, "/requests", "error", h.safeMessage(err, lang))
		return
	}
	h.redirectWithNotice(w, r, "/requests", "success", i18n.T(lang, "requests.sent_success"))
}

// RequestRespondSubmit accepts or declines an incoming request.
func (h *UIHandler) RequestRespondSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/requests", http.StatusSeeOther)
		return
	}

	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	status := workflow.RequestStatus(r.PostFormValue("status"))
	if h.wfSvc != nil && id > 0 {
		_ = h.wfSvc.RespondRequest(ctx, id, status)
	}
	http.Redirect(w, r, "/requests", http.StatusSeeOther)
}
