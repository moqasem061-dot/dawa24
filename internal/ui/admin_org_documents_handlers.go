package ui

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/moqasem061-dot/dawa24/internal/modules/org"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
)

type documentTargetKey struct{}

func (h *UIHandler) adminDocumentOrganization(r *http.Request, permission string) (*org.Organization, error) {
	actor, ok := authctx.From(r.Context())
	if !ok || !actor.IsStaff || !actor.Can(permission) {
		return nil, apperr.Forbidden("document.admin_required", i18n.T(langOf(r), "docs.org_auth_required"))
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		return nil, apperr.NotFound("organization")
	}
	if h.orgSvc == nil {
		return nil, http.ErrNotSupported
	}
	organization, err := h.orgSvc.GetOrganization(database.AsSystem(r.Context()), id)
	if err == nil && organization == nil {
		return nil, apperr.NotFound("organization")
	}
	return organization, err
}

func (h *UIHandler) AdminOrganizationDocumentsPage(w http.ResponseWriter, r *http.Request) {
	organization, err := h.adminDocumentOrganization(r, "hr.document.view")
	if err != nil {
		h.renderError(w, r, err)
		return
	}
	if h.attSvc == nil {
		h.renderError(w, r, http.ErrNotSupported)
		return
	}
	ctx := database.AsSystem(r.Context())
	docs, err := h.attSvc.ListByOrganization(ctx, organization.ID)
	if err != nil {
		h.renderError(w, r, err)
		return
	}
	actor, _ := authctx.From(ctx)
	requests, err := h.attSvc.ListDocumentRequests(ctx, actor, &organization.ID)
	if err != nil {
		h.renderError(w, r, err)
		return
	}
	data := pages.BuildOrganizationDocumentsData(docs, requests, organization.Type == org.TypeVendor)
	data.ReadOnly = !actor.Can("hr.document.update")
	data.UploadPath = fmt.Sprintf("/admin/organizations/%d/documents/upload", organization.ID)
	lang, dir := h.localeAndDir(r)
	h.renderPage(r.Context(), w, "render admin organization documents", pages.AdminOrganizationDocuments(organization.LegalName, lang, dir, data))
}

func (h *UIHandler) AdminOrganizationDocumentsUploadSubmit(w http.ResponseWriter, r *http.Request) {
	organization, err := h.adminDocumentOrganization(r, "hr.document.update")
	if err != nil {
		h.renderError(w, r, err)
		return
	}
	ctx := context.WithValue(r.Context(), documentTargetKey{}, organization.ID)
	h.OrganizationDocumentsUploadSubmit(w, r.WithContext(ctx))
}
