package ui

import (
	"fmt"
	"net/http"

	"github.com/moqasem061-dot/dawa24/internal/modules/attachments"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
)

func (h *UIHandler) renderMissingDocError(w http.ResponseWriter, r *http.Request, doc *attachments.Document, reason, lang, dir string) {
	actor, _ := authctx.From(r.Context())

	returnURL := "/customer/documents"
	if actor.IsPlatformAdmin() || actor.IsStaff {
		returnURL = "/admin/approvals?tab=documents"
	} else if actor.IsVendor() {
		returnURL = "/vendor/documents"
	}

	view := pages.DocumentUnavailableView{
		ReturnURL: returnURL,
		IsAdmin:   actor.IsPlatformAdmin() || actor.IsStaff,
	}

	if doc != nil {
		view.DocID = doc.ID
		view.DocTypeLabel = pages.FormatDocTypeLabel(doc.DocumentType)
		view.OriginalName = doc.OriginalName
		if view.OriginalName == "" {
			view.OriginalName = fmt.Sprintf("Document #%d", doc.ID)
		}
		view.UploadDate = doc.CreatedAt.Format("2006-01-02 03:04 PM")
		if doc.OrganizationID != nil && *doc.OrganizationID > 0 {
			view.OrgID = *doc.OrganizationID
			if h.orgSvc != nil {
				if orgObj, err := h.orgSvc.GetOrganization(r.Context(), *doc.OrganizationID); err == nil && orgObj != nil {
					view.OrgName = orgObj.LegalName
				}
			}
			if view.OrgName == "" {
				view.OrgName = fmt.Sprintf(i18n.T(lang, "docs.serve.org_fallback"), *doc.OrganizationID)
			}
		}
		switch doc.Status {
		case attachments.StatusVerified:
			view.StatusLabel = i18n.T(lang, "docs.serve.status_verified")
		case attachments.StatusRejected:
			view.StatusLabel = i18n.T(lang, "docs.serve.status_rejected")
		default:
			view.StatusLabel = i18n.T(lang, "docs.serve.status_pending")
		}
	} else {
		view.DocTypeLabel = i18n.T(lang, "docs.serve.doc_unregistered")
		view.OriginalName = i18n.T(lang, "docs.serve.file_unavailable")
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	if err := pages.DocumentUnavailablePage(view, lang, dir).Render(r.Context(), w); err != nil {
		h.log.ErrorContext(r.Context(), "render document unavailable page", "error", err)
	}
}
