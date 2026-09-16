package ui

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
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
		messages, total, _ = h.adminSvc.ListContactMessagesWithTotal(database.AsSystem(ctx), "", limit, offset)
	}

	h.renderPage(ctx, w, "render admin messages", pages.AdminMessages(lang, dir, messages, page, limit, total))
}

// AdminMessageToggleSubmit toggles the status of a contact message between unread and read.
func (h *UIHandler) AdminMessageToggleSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err == nil && id > 0 && h.adminSvc != nil {
		msgs, err := h.adminSvc.ListContactMessages(database.AsSystem(ctx), "", 500, 0)
		if err == nil {
			for _, m := range msgs {
				if m.ID == id {
					newStatus := "read"
					if m.Status == "read" {
						newStatus = "unread"
					}
					_ = h.adminSvc.UpdateContactMessageStatus(database.AsSystem(ctx), id, newStatus)
					break
				}
			}
		}
	}
	h.redirectWithNotice(w, r, "/admin/messages", "success", i18n.T(langOf(r), "admin.messages.status_updated_success"))
}

// AdminMessageDeleteSubmit deletes a contact message from the inbox.
func (h *UIHandler) AdminMessageDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err == nil && id > 0 && h.adminSvc != nil {
		_ = h.adminSvc.DeleteContactMessage(database.AsSystem(ctx), id)
	}
	h.redirectWithNotice(w, r, "/admin/messages", "success", i18n.T(langOf(r), "admin.messages.deleted_success"))
}
