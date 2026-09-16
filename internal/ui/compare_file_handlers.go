package ui

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

func (h *UIHandler) checkFileOwnership(actor authctx.Actor, file *compare.CompareFile) bool {
	if file == nil {
		return false
	}
	if actor.IsStaff || actor.IsPlatformAdmin() {
		return true
	}
	if file.UserID == actor.UserID {
		return true
	}
	if file.OrganizationID != nil && actor.OrganizationID > 0 && *file.OrganizationID == actor.OrganizationID {
		return true
	}
	return false
}

func compareReturnPath(r *http.Request) string {
	if ret := strings.TrimSpace(r.FormValue("return_url")); ret != "" {
		if safe := safeLocalRedirect(ret, ""); safe != "" {
			return safe
		}
	}
	if ret := strings.TrimSpace(r.URL.Query().Get("return_url")); ret != "" {
		if safe := safeLocalRedirect(ret, ""); safe != "" {
			return safe
		}
	}
	ref := r.Header.Get("Referer")
	if ref != "" && strings.Contains(ref, "/admin/organizations/import/") && strings.Contains(ref, "/compare") {
		return safeLocalRedirect(ref, "/compare/tool")
	}
	return "/compare/tool"
}

// CompareFileRenameSubmit handles renaming a supplier file label.
func (h *UIHandler) CompareFileRenameSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/compare/tool", http.StatusSeeOther)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, compareReturnPath(r), "error", i18n.T(lang, "compare.file.invalid_id"))
		return
	}

	if h.compareSvc != nil {
		file, err := h.compareSvc.GetFile(ctx, id)
		if err != nil || !h.checkFileOwnership(actor, file) {
			h.redirectWithNotice(w, r, compareReturnPath(r), "error", i18n.T(lang, "compare.file.edit_forbidden"))
			return
		}

		newName := strings.TrimSpace(r.FormValue("supplier_name"))
		if newName == "" {
			h.redirectWithNotice(w, r, compareReturnPath(r), "error", i18n.T(lang, "compare.file.supplier_name_required"))
			return
		}
		if err := h.compareSvc.RenameFile(ctx, id, newName); err != nil {
			h.redirectWithNotice(w, r, compareReturnPath(r), "error", h.safeMessage(err, lang))
			return
		}
	}
	h.redirectWithNotice(w, r, compareReturnPath(r), "success", i18n.T(lang, "compare.file.renamed_success"))
}

// CompareFileArchiveSubmit handles manually archiving a file.
func (h *UIHandler) CompareFileArchiveSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/compare/tool", http.StatusSeeOther)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, compareReturnPath(r), "error", i18n.T(lang, "compare.file.invalid_id"))
		return
	}
	if h.compareSvc != nil {
		file, err := h.compareSvc.GetFile(ctx, id)
		if err != nil || !h.checkFileOwnership(actor, file) {
			h.redirectWithNotice(w, r, compareReturnPath(r), "error", i18n.T(lang, "compare.file.edit_forbidden"))
			return
		}
		if err := h.compareSvc.ArchiveFile(ctx, id, i18n.T(lang, "compare.file.manual_archive_reason")); err != nil {
			h.redirectWithNotice(w, r, compareReturnPath(r), "error", h.safeMessage(err, lang))
			return
		}
	}
	h.redirectWithNotice(w, r, compareReturnPath(r), "success", i18n.T(lang, "compare.file.archived_success"))
}

// CompareFileUnarchiveSubmit handles restoring an archived file.
func (h *UIHandler) CompareFileUnarchiveSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/compare/tool", http.StatusSeeOther)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, compareReturnPath(r), "error", i18n.T(lang, "compare.file.invalid_id"))
		return
	}
	if h.compareSvc != nil {
		file, err := h.compareSvc.GetFile(ctx, id)
		if err != nil || !h.checkFileOwnership(actor, file) {
			h.redirectWithNotice(w, r, compareReturnPath(r), "error", i18n.T(lang, "compare.file.edit_forbidden"))
			return
		}
		if err := h.compareSvc.UnarchiveFile(ctx, id); err != nil {
			h.redirectWithNotice(w, r, compareReturnPath(r), "error", h.safeMessage(err, lang))
			return
		}
	}
	h.redirectWithNotice(w, r, compareReturnPath(r), "success", i18n.T(lang, "compare.file.unarchived_success"))
}

// CompareFileDeleteSubmit handles soft-deleting a file.
func (h *UIHandler) CompareFileDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/compare/tool", http.StatusSeeOther)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, compareReturnPath(r), "error", i18n.T(lang, "compare.file.invalid_id"))
		return
	}
	if h.compareSvc != nil {
		file, err := h.compareSvc.GetFile(ctx, id)
		if err != nil || !h.checkFileOwnership(actor, file) {
			h.redirectWithNotice(w, r, compareReturnPath(r), "error", i18n.T(lang, "compare.file.delete_forbidden"))
			return
		}
		if err := h.compareSvc.DeleteFile(ctx, id); err != nil {
			h.redirectWithNotice(w, r, compareReturnPath(r), "error", h.safeMessage(err, lang))
			return
		}
	}
	h.redirectWithNotice(w, r, compareReturnPath(r), "success", i18n.T(lang, "compare.file.deleted_success"))
}
