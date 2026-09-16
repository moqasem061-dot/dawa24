package ui

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/attachments"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// AdminDocumentsPage redirects to the unified documents & approvals audit registry.
func (h *UIHandler) AdminDocumentsPage(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/admin/approvals?tab=documents", http.StatusSeeOther)
}

// AdminCreateDocumentRequestSubmit issues an administrative document request to an organization.
func (h *UIHandler) AdminCreateDocumentRequestSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, _ := authctx.From(ctx)

	orgID, err := strconv.ParseInt(r.PostFormValue("organization_id"), 10, 64)
	if err != nil || orgID <= 0 {
		h.redirectWithNotice(w, r, "/admin/approvals?tab=requests", "error", i18n.T(lang, "admin.docs.select_valid_org"))
		return
	}

	docType := attachments.DocumentType(strings.TrimSpace(r.PostFormValue("document_type")))
	title := strings.TrimSpace(r.PostFormValue("title"))
	description := strings.TrimSpace(r.PostFormValue("description"))
	deadlineDays, _ := strconv.Atoi(r.PostFormValue("deadline_days"))
	if deadlineDays <= 0 {
		deadlineDays = 30
	}

	if title == "" {
		h.redirectWithNotice(w, r, "/admin/approvals?tab=requests", "error", i18n.T(lang, "admin.docs.title_required"))
		return
	}

	if h.attSvc == nil {
		h.redirectWithNotice(w, r, "/admin/approvals?tab=requests", "error", i18n.T(lang, "admin.docs.service_unavailable"))
		return
	}

	sysCtx := database.AsSystem(ctx)
	if _, err := h.attSvc.CreateDocumentRequest(sysCtx, actor, orgID, docType, title, description, deadlineDays); err != nil {
		h.redirectWithNotice(w, r, "/admin/approvals?tab=requests", "error", h.safeMessage(err, lang))
		return
	}

	h.notifyDocumentRequested(ctx, orgID, title, description, deadlineDays)
	h.redirectWithNotice(w, r, "/admin/approvals?tab=requests", "success", i18n.T(lang, "admin.docs.request_created_success"))
}

// AdminCancelDocumentRequestSubmit cancels an active document request.
func (h *UIHandler) AdminCancelDocumentRequestSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, _ := authctx.From(ctx)

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/admin/approvals?tab=requests", "error", i18n.T(lang, "admin.docs.invalid_request_id"))
		return
	}

	if h.attSvc != nil {
		sysCtx := database.AsSystem(ctx)
		_ = h.attSvc.CancelDocumentRequest(sysCtx, actor, id)
	}

	h.redirectWithNotice(w, r, "/admin/approvals?tab=requests", "success", i18n.T(lang, "admin.docs.request_cancelled_success"))
}

// AdminVerifyUploadedDocSubmit audits, categorizes, and approves/rejects an uploaded document.
func (h *UIHandler) AdminVerifyUploadedDocSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, _ := authctx.From(ctx)

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/admin/approvals?tab=documents", "error", i18n.T(lang, "admin.docs.invalid_doc_id"))
		return
	}

	docType := attachments.DocumentType(strings.TrimSpace(r.PostFormValue("document_type")))
	status := attachments.DocumentStatus(strings.TrimSpace(r.PostFormValue("status")))
	notes := strings.TrimSpace(r.PostFormValue("notes"))

	if status != attachments.StatusVerified && status != attachments.StatusRejected {
		status = attachments.StatusVerified
	}

	if h.attSvc != nil {
		sysCtx := database.AsSystem(ctx)
		doc, _ := h.attSvc.GetByIDAdmin(sysCtx, id)
		if err := h.attSvc.VerifyDocumentWithType(sysCtx, actor, id, docType, status, notes); err != nil {
			h.redirectWithNotice(w, r, "/admin/approvals?tab=documents", "error", h.safeMessage(err, lang))
			return
		}
		if doc != nil && doc.OrganizationID != nil && *doc.OrganizationID > 0 {
			docName := string(docType)
			if docName == "" && doc.Title != "" {
				docName = doc.Title
			} else if docName == "" && doc.OriginalName != "" {
				docName = doc.OriginalName
			}
			h.safeGo("notify-document-verified", func() {
				h.notifyDocumentVerified(context.Background(), *doc.OrganizationID, docName, status == attachments.StatusVerified, notes)
			})
		}
	}

	msg := i18n.T(lang, "admin.docs.verified_success")
	if status == attachments.StatusRejected {
		msg = i18n.T(lang, "admin.docs.rejected_success")
	}
	h.redirectWithNotice(w, r, "/admin/approvals?tab=documents", "success", msg)
}
