package ui

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// Soft-delete recovery. These screens previously rendered a hardcoded model
// list with invented row counts, and restore/purge logged a line and told the
// administrator it had worked. Everything here now goes through
// platform_admin's trash service, which discovers soft-deletable tables from
// information_schema and counts rows with real queries.

// AdminTrashListPage lists every soft-deletable table with its live counts.
func (h *UIHandler) AdminTrashListPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	if h.adminSvc == nil {
		h.redirectWithNotice(w, r, "/admin/dashboard", "error", i18n.T(lang, "admin.dev.admin_service_unavailable"))
		return
	}
	models, err := h.adminSvc.ListTrashModels(ctx)
	if err != nil {
		h.log.ErrorContext(ctx, "list trash models", "error", err)
		h.renderError(w, r, err)
		return
	}

	entries := make([]pages.ModelMetaEntry, 0, len(models))
	for _, m := range models {
		entries = append(entries, pages.ModelMetaEntry{
			Key:          m.Key,
			NameAr:       m.NameAr,
			NameEn:       m.NameEn,
			SchemaTable:  m.Key,
			TotalCount:   int(m.TotalCount),
			TrashedCount: int(m.TrashedRows),
		})
	}

	h.renderPage(ctx, w, "render admin trash list", pages.AdminTrashListPage(entries, lang, dir))
}

// AdminTrashListModelPage lists the deleted rows of one table.
func (h *UIHandler) AdminTrashListModelPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	modelKey := chi.URLParam(r, "model")

	if h.adminSvc == nil {
		h.redirectWithNotice(w, r, "/admin/dashboard", "error", i18n.T(lang, "admin.dev.admin_service_unavailable"))
		return
	}

	search := strings.TrimSpace(r.URL.Query().Get("q"))
	limit := pagination.RowsPerPage(r)
	page := pagination.PageNumber(r)
	offset := (page - 1) * limit

	rows, total, err := h.adminSvc.ListTrashedRowsFiltered(ctx, modelKey, search, limit, offset)
	if err != nil {
		h.log.ErrorContext(ctx, "list trashed rows", "error", err, "model", modelKey)
		h.renderError(w, r, err)
		return
	}

	items := make([]pages.TrashRowView, 0, len(rows))
	for _, row := range rows {
		items = append(items, pages.TrashRowView{
			ID:               row.ID,
			Label:            row.Label,
			Code:             row.Code,
			OrgName:          row.OrganizationName,
			DeletedByName:    row.DeletedByName,
			DeletedAt:        row.DeletedAt,
			CanRestore:       row.CanRestore,
			CannotRestoreWhy: row.CannotRestoreWhy,
		})
	}

	view := pages.AdminTrashModelView{
		ModelKey:   modelKey,
		Rows:       items,
		Search:     search,
		Page:       page,
		PerPage:    limit,
		TotalCount: total,
		NoticeKind: r.URL.Query().Get("notice_type"),
		Notice:     r.URL.Query().Get("notice_msg"),
	}

	h.renderPage(ctx, w, "render admin trash list model", pages.AdminTrashListModelPage(view, lang, dir))
}

// AdminTrashRestoreSubmit clears deleted_at on one row.
func (h *UIHandler) AdminTrashRestoreSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	modelKey := chi.URLParam(r, "model")
	back := "/admin/trash-list/" + url.PathEscape(modelKey)
	if q := strings.TrimSpace(r.FormValue("q")); q != "" {
		back += "?q=" + url.QueryEscape(q)
	}

	rowID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || rowID <= 0 {
		h.redirectWithNotice(w, r, back, "error", i18n.T(lang, "admin.dev.invalid_log_id"))
		return
	}
	if h.adminSvc == nil {
		h.redirectWithNotice(w, r, back, "error", i18n.T(lang, "admin.dev.admin_service_unavailable"))
		return
	}

	actor, _ := authctx.From(ctx)
	if err := h.adminSvc.RestoreTrashedRow(ctx, modelKey, rowID, actor.UserID); err != nil {
		h.log.ErrorContext(ctx, "restore trashed row", "error", err, "model", modelKey, "id", rowID)
		h.redirectWithNotice(w, r, back, "error", h.safeMessage(err, lang))
		return
	}
	h.redirectWithNotice(w, r, back, "success", i18n.T(lang, "admin.trash.restored_success"))
}

// AdminTrashPurgeSubmit permanently removes one row. Irreversible, so the
// service records the row's contents in the audit log before deleting it.
func (h *UIHandler) AdminTrashPurgeSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	modelKey := chi.URLParam(r, "model")
	back := "/admin/trash-list/" + url.PathEscape(modelKey)
	if q := strings.TrimSpace(r.FormValue("q")); q != "" {
		back += "?q=" + url.QueryEscape(q)
	}

	rowID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || rowID <= 0 {
		h.redirectWithNotice(w, r, back, "error", i18n.T(lang, "admin.dev.invalid_log_id"))
		return
	}
	if h.adminSvc == nil {
		h.redirectWithNotice(w, r, back, "error", i18n.T(lang, "admin.dev.admin_service_unavailable"))
		return
	}

	actor, _ := authctx.From(ctx)
	if err := h.adminSvc.PurgeTrashedRow(ctx, modelKey, rowID, actor.UserID); err != nil {
		h.log.ErrorContext(ctx, "purge trashed row", "error", err, "model", modelKey, "id", rowID)
		h.redirectWithNotice(w, r, back, "error", h.safeMessage(err, lang))
		return
	}
	h.redirectWithNotice(w, r, back, "success", i18n.T(lang, "admin.trash.purged_success"))
}
