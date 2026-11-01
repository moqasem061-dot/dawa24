package ui

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	platformadmin "github.com/moqasem061-dot/dawa24/internal/modules/platform_admin"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/shared/pagination"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
)

// ContactPage renders the public contact form.
func (h *UIHandler) ContactPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	h.renderPage(ctx, w, "render contact page", pages.ContactPage(lang, dir, false))
}

// ContactSubmit records a public contact inquiry and re-renders the form with a
// success state that keeps the visitor on the page.
func (h *UIHandler) ContactSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	_ = r.ParseForm()

	submitted := false
	if h.adminSvc != nil {
		var userID *int64
		if actor, ok := authctx.From(ctx); ok && actor.UserID > 0 {
			userID = &actor.UserID
		}

		err := h.adminSvc.SubmitContactMessage(database.AsSystem(ctx), &platformadmin.ContactMessage{
			Name:      strings.TrimSpace(r.FormValue("name")),
			Email:     strings.TrimSpace(r.FormValue("email")),
			Phone:     strings.TrimSpace(r.FormValue("phone")),
			Subject:   strings.TrimSpace(r.FormValue("subject")),
			Message:   strings.TrimSpace(r.FormValue("message")),
			Status:    "unread",
			IP:        h.clientIP(r),
			UserAgent: r.UserAgent(),
			UserID:    userID,
		})
		if err != nil {
			h.log.WarnContext(ctx, "contact submit: failed to record inquiry", "error", err)
		} else {
			submitted = true
		}
	}

	h.renderPage(ctx, w, "render contact page after submit", pages.ContactPage(lang, dir, submitted))
}

// AdminMessagesPage renders the platform contact-message inbox.
func (h *UIHandler) AdminMessagesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	limit := pagination.RowsPerPage(r)
	page := pagination.PageNumber(r)
	offset := (page - 1) * limit

	var messages []*platformadmin.ContactMessage
	var total int
	if h.adminSvc != nil {
		if loaded, loaded2, err := h.adminSvc.ListContactMessagesWithTotal(database.AsSystem(ctx), "", limit, offset); err != nil {
			h.logLoadErr(ctx, "adminSvc.ListContactMessagesWithTotal", err)
		} else {
			messages, total = loaded, loaded2
		}
	}

	h.renderPage(ctx, w, "render admin messages", pages.AdminMessages(lang, dir, messages, page, limit, total))
}

// AdminMessageToggleSubmit flips a contact message between read and unread.
//
// The status to write arrives with the form, which is the whole point: it used
// to be discovered by reading the first five hundred messages and searching them
// for this id. That scanned the inbox to flip one row, and every message past
// the five-hundredth could not be marked as read at all — the row simply did
// nothing when an operator clicked it.
func (h *UIHandler) AdminMessageToggleSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 || h.adminSvc == nil {
		h.redirectWithNotice(w, r, "/admin/messages", "error", i18n.T(lang, "admin.messages.status_invalid"))
		return
	}
	// Only the two states this inbox knows how to show. Anything else in the
	// form is a request to store a status no screen renders.
	status := strings.TrimSpace(r.PostFormValue("status"))
	if status != "read" && status != "unread" {
		h.redirectWithNotice(w, r, "/admin/messages", "error", i18n.T(lang, "admin.messages.status_invalid"))
		return
	}
	if err := h.adminSvc.UpdateContactMessageStatus(database.AsSystem(ctx), id, status); err != nil {
		h.logLoadErr(ctx, "adminSvc.UpdateContactMessageStatus", err)
		h.redirectWithNotice(w, r, "/admin/messages", "error", i18n.T(lang, "admin.messages.status_update_failed"))
		return
	}
	h.redirectWithNotice(w, r, "/admin/messages", "success", i18n.T(lang, "admin.messages.status_updated_success"))
}

// AdminMessageDeleteSubmit deletes a contact message from the inbox.
func (h *UIHandler) AdminMessageDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 || h.adminSvc == nil {
		h.redirectWithNotice(w, r, "/admin/messages", "error", i18n.T(lang, "admin.messages.delete_failed"))
		return
	}
	if err := h.adminSvc.DeleteContactMessage(database.AsSystem(ctx), id); err != nil {
		h.logLoadErr(ctx, "adminSvc.DeleteContactMessage", err)
		h.redirectWithNotice(w, r, "/admin/messages", "error", i18n.T(lang, "admin.messages.delete_failed"))
		return
	}
	h.redirectWithNotice(w, r, "/admin/messages", "success", i18n.T(lang, "admin.messages.deleted_success"))
}
