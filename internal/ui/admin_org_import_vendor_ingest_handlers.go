package ui

import (
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/ingest"
	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/filesecurity"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminOrgImportVendorIngestPage renders the canonical vendor catalogue ingest page on behalf of target vendor org.
func (h *UIHandler) AdminOrgImportVendorIngestPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)

	orgID, _ := strconv.ParseInt(chi.URLParam(r, "orgID"), 10, 64)
	if orgID <= 0 {
		h.redirectWithNotice(w, r, "/admin/organizations/import?tab=vendor", "error", i18n.T(lang, "validation.invalid_id"))
		return
	}

	sysCtx := database.WithTenant(database.AsSystem(ctx), orgID)
	var targetOrgName string
	if h.orgSvc != nil {
		targetOrg, err := h.orgSvc.GetOrganization(sysCtx, orgID)
		if err != nil || targetOrg == nil || targetOrg.Type != org.TypeVendor {
			h.redirectWithNotice(w, r, "/admin/organizations/import?tab=vendor", "error", "المنشأة المحددة ليست مورداً معتمداً")
			return
		}
		targetOrgName, _ = h.resolveTargetOrgInfo(sysCtx, orgID)
	}

	view := pages.VendorImportView{
		Lang:          lang,
		NoticeType:    r.URL.Query().Get("notice"),
		NoticeMessage: r.URL.Query().Get("msg"),
		Audience:      "admin",
		TargetOrgID:   orgID,
		TargetOrgName: targetOrgName,
	}

	if h.invSvc != nil {
		whs, _ := h.invSvc.ListWarehouses(sysCtx)
		for _, wh := range whs {
			if wh.OrganizationID == orgID {
				view.Warehouses = append(view.Warehouses, wh)
			}
		}
	}

	if h.ingSvc != nil {
		recent, err := h.ingSvc.RecentImports(sysCtx, orgID, 12)
		if err != nil {
			h.log.WarnContext(sysCtx, "admin failed to load recent vendor imports", "error", err, "target_org_id", orgID)
		}
		view.Recent = recent
	}

	h.renderImport(w, r, view)
}

// AdminOrgImportVendorIngestUploadSubmit handles spreadsheet upload on behalf of target vendor org.
func (h *UIHandler) AdminOrgImportVendorIngestUploadSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/admin/organizations/import", http.StatusSeeOther)
		return
	}

	orgID, _ := strconv.ParseInt(chi.URLParam(r, "orgID"), 10, 64)
	if orgID <= 0 {
		h.redirectWithNotice(w, r, "/admin/organizations/import?tab=vendor", "error", i18n.T(lang, "validation.invalid_id"))
		return
	}

	if h.ingSvc == nil {
		h.redirectWithNotice(w, r, fmt.Sprintf("/admin/organizations/import/%d/ingest", orgID), "error", i18n.T(lang, "common.import_service_unavailable"))
		return
	}

	if err := parseImportUpload(w, r); err != nil {
		h.redirectWithNotice(w, r, fmt.Sprintf("/admin/organizations/import/%d/ingest", orgID), "error", i18n.T(lang, "vendor.import.file_too_large"))
		return
	}

	file, fileHeader, err := r.FormFile("file")
	if err != nil {
		h.redirectWithNotice(w, r, fmt.Sprintf("/admin/organizations/import/%d/ingest", orgID), "error", i18n.T(lang, "vendor.import.select_file"))
		return
	}
	defer file.Close()

	if !SupportedUploadName(fileHeader.Filename) {
		h.redirectWithNotice(w, r, fmt.Sprintf("/admin/organizations/import/%d/ingest", orgID), "error", unsupportedUploadMsg(lang))
		return
	}

	fileBytes, err := io.ReadAll(file)
	if err != nil || len(fileBytes) == 0 {
		h.redirectWithNotice(w, r, fmt.Sprintf("/admin/organizations/import/%d/ingest", orgID), "error", i18n.T(lang, "vendor.import.empty_file"))
		return
	}

	if err := filesecurity.ValidateSpreadsheetSecurity(fileBytes, fileHeader.Filename); err != nil {
		h.redirectWithNotice(w, r, fmt.Sprintf("/admin/organizations/import/%d/ingest", orgID), "error", filesecurity.SecurityErrorMessage)
		return
	}

	sysCtx := database.WithTenant(database.AsSystem(ctx), orgID)
	session, _, err := h.ingSvc.StartImport(sysCtx, actor.UserID, fileHeader.Filename, fileBytes)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to start vendor ingest for org", "error", err, "target_org_id", orgID)
		h.redirectWithNotice(w, r, fmt.Sprintf("/admin/organizations/import/%d/ingest", orgID), "error", h.safeMessage(err, lang))
		return
	}

	h.log.InfoContext(ctx, "admin started vendor ingest on behalf of org", "actor_id", actor.UserID, "target_org_id", orgID, "session_id", session.PublicID)
	http.Redirect(w, r, fmt.Sprintf("/admin/organizations/import/%d/ingest/%s", orgID, session.PublicID), http.StatusSeeOther)
}

// AdminOrgImportVendorIngestSessionPage renders the active session stage for target vendor org.
func (h *UIHandler) AdminOrgImportVendorIngestSessionPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)

	orgID, _ := strconv.ParseInt(chi.URLParam(r, "orgID"), 10, 64)
	publicID := chi.URLParam(r, "id")

	if h.ingSvc == nil {
		h.redirectWithNotice(w, r, "/admin/organizations/import?tab=vendor", "error", i18n.T(lang, "common.import_service_unavailable"))
		return
	}

	sysCtx := database.WithTenant(database.AsSystem(ctx), orgID)
	session, err := h.ingSvc.LoadImport(sysCtx, publicID)
	if err != nil {
		h.redirectWithNotice(w, r, fmt.Sprintf("/admin/organizations/import/%d/ingest", orgID), "error", h.safeMessage(err, lang))
		return
	}

	var warehouses []*inventory.Warehouse
	if h.invSvc != nil {
		whs, _ := h.invSvc.ListWarehouses(sysCtx)
		for _, wh := range whs {
			if wh.OrganizationID == orgID {
				warehouses = append(warehouses, wh)
			}
		}
	}

	var targetOrgName string
	if h.orgSvc != nil {
		targetOrgName, _ = h.resolveTargetOrgInfo(sysCtx, orgID)
	}

	view := pages.VendorImportView{
		Lang:          lang,
		Session:       session,
		Warehouses:    warehouses,
		NoticeType:    r.URL.Query().Get("notice"),
		NoticeMessage: r.URL.Query().Get("msg"),
		Audience:      "admin",
		TargetOrgID:   orgID,
		TargetOrgName: targetOrgName,
	}
	view.AIAvailable, view.AIUnavailableReason = h.vendorImportAIState(sysCtx, lang)

	switch {
	case session.Phase.Terminal():
		h.loadImportResults(r, &view)
	case session.Phase == ingest.PhaseReview:
		h.loadImportReview(r, &view)
	case session.Phase == ingest.PhaseProcessing:
		// Background processing poll
	default:
		_, analysis, aErr := h.ingSvc.Analysis(sysCtx, publicID)
		if aErr != nil {
			view.Fatal = h.safeMessage(aErr, lang)
		} else {
			view.Analysis = analysis
		}
	}

	h.renderImport(w, r, view)
}

// AdminOrgImportVendorIngestMapSubmit saves column mapping for vendor ingest.
func (h *UIHandler) AdminOrgImportVendorIngestMapSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID, _ := strconv.ParseInt(chi.URLParam(r, "orgID"), 10, 64)
	publicID := chi.URLParam(r, "id")

	if h.ingSvc == nil {
		h.redirectWithNotice(w, r, "/admin/organizations/import?tab=vendor", "error", i18n.T(langOf(r), "common.import_service_unavailable"))
		return
	}

	overrides, err := vendorMappingOverrides(r)
	if err != nil {
		h.redirectWithNotice(w, r, fmt.Sprintf("/admin/organizations/import/%d/ingest/%s", orgID, publicID), "error", err.Error())
		return
	}

	sysCtx := database.WithTenant(database.AsSystem(ctx), orgID)
	if _, _, err := h.ingSvc.SaveMapping(sysCtx, publicID, overrides); err != nil {
		h.redirectWithNotice(w, r, fmt.Sprintf("/admin/organizations/import/%d/ingest/%s", orgID, publicID), "error", h.safeMessage(err, langOf(r)))
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/admin/organizations/import/%d/ingest/%s", orgID, publicID), http.StatusSeeOther)
}

// AdminOrgImportVendorIngestCommitSubmit commits the vendor catalogue ingest on behalf of org.
func (h *UIHandler) AdminOrgImportVendorIngestCommitSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, _ := authctx.From(ctx)

	orgID, _ := strconv.ParseInt(chi.URLParam(r, "orgID"), 10, 64)
	publicID := chi.URLParam(r, "id")

	if h.ingSvc == nil {
		h.redirectWithNotice(w, r, "/admin/organizations/import?tab=vendor", "error", i18n.T(lang, "common.import_service_unavailable"))
		return
	}

	sysCtx := database.WithTenant(database.AsSystem(ctx), orgID)
	if _, err := h.ingSvc.CommitImport(sysCtx, publicID); err != nil {
		h.log.ErrorContext(ctx, "failed to commit vendor ingest on behalf of org", "error", err, "target_org_id", orgID, "session_id", publicID)
		h.notifyImportRunFailed(ctx, actor.UserID, orgID, 0, err.Error())
		h.redirectWithNotice(w, r, fmt.Sprintf("/admin/organizations/import/%d/ingest/%s", orgID, publicID), "error", h.safeMessage(err, lang))
		return
	}

	h.log.InfoContext(ctx, "admin committed vendor ingest on behalf of org", "actor_id", actor.UserID, "target_org_id", orgID, "session_id", publicID)
	h.notifyImportRunFinished(ctx, actor.UserID, orgID, 0, 0)
	http.Redirect(w, r, fmt.Sprintf("/admin/organizations/import/%d/ingest/%s", orgID, publicID), http.StatusSeeOther)
}

// AdminOrgImportVendorIngestCancelSubmit cancels the vendor ingest session.
func (h *UIHandler) AdminOrgImportVendorIngestCancelSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID, _ := strconv.ParseInt(chi.URLParam(r, "orgID"), 10, 64)
	publicID := chi.URLParam(r, "id")

	if h.ingSvc != nil && publicID != "" {
		_ = h.ingSvc.CancelImport(database.WithTenant(database.AsSystem(ctx), orgID), publicID)
	}
	h.redirectWithNotice(w, r, "/admin/organizations/import?tab=vendor", "info", i18n.T(langOf(r), "vendor.import.cancelled_notice"))
}
