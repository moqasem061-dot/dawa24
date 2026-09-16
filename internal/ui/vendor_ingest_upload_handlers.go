package ui

import (
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/ingest"
	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/filesecurity"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// maxImportUpload bounds the multipart body. It matches the service's own limit
// so an oversized file is refused by the reader rather than after being read.
const maxImportUpload = ingest.MaxImportBytes

// VendorIngestPage renders the upload screen and the import history.
func (h *UIHandler) VendorIngestPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/ingest", http.StatusSeeOther)
		return
	}

	view := pages.VendorImportView{
		Lang:          langOf(r),
		NoticeType:    r.URL.Query().Get("notice"),
		NoticeMessage: r.URL.Query().Get("msg"),
	}
	view.Warehouses = h.vendorWarehouses(r)
	if h.ingSvc != nil && actor.OrganizationID > 0 {
		recent, err := h.ingSvc.RecentImports(ctx, actor.OrganizationID, 12)
		if err != nil {
			h.log.WarnContext(ctx, "import history unavailable", "error", err)
		}
		view.Recent = recent
	}
	h.renderImport(w, r, view)
}

// VendorIngestSessionPage renders whichever stage the import has reached.
func (h *UIHandler) VendorIngestSessionPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	publicID := chi.URLParam(r, "id")
	if h.ingSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/ingest", "error", i18n.T(langOf(r), "common.import_service_unavailable"))
		return
	}

	session, err := h.ingSvc.LoadImport(ctx, publicID)
	if err != nil {
		h.redirectWithNotice(w, r, "/vendor/ingest", "error", h.safeMessage(err, langOf(r)))
		return
	}

	view := pages.VendorImportView{
		Lang:          langOf(r),
		Session:       session,
		Warehouses:    h.vendorWarehouses(r),
		NoticeType:    r.URL.Query().Get("notice"),
		NoticeMessage: r.URL.Query().Get("msg"),
	}
	view.AIAvailable, view.AIUnavailableReason = h.vendorImportAIState(ctx, langOf(r))

	switch {
	case session.Phase.Terminal():
		h.loadImportResults(r, &view)
	case session.Phase == ingest.PhaseReview:
		h.loadImportReview(r, &view)
	case session.Phase == ingest.PhaseProcessing:
		// Nothing to analyse: the run owns the file and the screen polls.
	default:
		_, analysis, aErr := h.ingSvc.Analysis(ctx, publicID)
		if aErr != nil {
			view.Fatal = h.safeMessage(aErr, langOf(r))
		} else {
			view.Analysis = analysis
		}
	}
	h.renderImport(w, r, view)
}

// VendorIngestUploadSubmit analyses an uploaded file and opens an import.
func (h *UIHandler) VendorIngestUploadSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/ingest", http.StatusSeeOther)
		return
	}
	if h.ingSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/ingest", "error", i18n.T(langOf(r), "common.import_service_unavailable"))
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxImportUpload)
	if err := r.ParseMultipartForm(uploadMemoryBudget); err != nil {
		h.redirectWithNotice(w, r, "/vendor/ingest", "error",
			i18n.T(langOf(r), "vendor.ingest.file_too_large"))
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		h.redirectWithNotice(w, r, "/vendor/ingest", "error",
			i18n.T(langOf(r), "vendor.ingest.invalid_file_format"))
		return
	}
	defer func() { _ = file.Close() }()

	content, err := io.ReadAll(file)
	if err != nil {
		h.redirectWithNotice(w, r, "/vendor/ingest", "error", i18n.T(langOf(r), "vendor.ingest.read_file_error"))
		return
	}

	if err := filesecurity.ValidateSpreadsheetSecurity(content, header.Filename); err != nil {
		h.redirectWithNotice(w, r, "/vendor/ingest", "error", filesecurity.SecurityErrorMessage)
		return
	}

	session, _, err := h.ingSvc.StartImport(ctx, actor.UserID, header.Filename, content)
	if err != nil {
		h.redirectWithNotice(w, r, "/vendor/ingest", "error", h.safeMessage(err, langOf(r)))
		return
	}
	http.Redirect(w, r, "/vendor/ingest/"+session.PublicID, http.StatusSeeOther)
}

// loadImportResults fills the results table for a finished import.
func (h *UIHandler) loadImportResults(r *http.Request, view *pages.VendorImportView) {
	ctx := r.Context()
	filter := ingest.RowFilter{
		Outcome:    r.URL.Query().Get("outcome"),
		MatchLevel: r.URL.Query().Get("match"),
		Search:     r.URL.Query().Get("q"),
		Limit:      50,
	}
	if page, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && page > 1 {
		filter.Offset = (page - 1) * filter.Limit
	}
	view.Filter = filter
	rows, total, err := h.ingSvc.ImportRows(ctx, view.Session.PublicID, filter)
	if err != nil {
		h.log.WarnContext(ctx, "import rows unavailable", "error", err)
		return
	}
	view.Rows, view.RowTotal = rows, total

	counts, err := h.ingSvc.ImportRowCounts(ctx, view.Session.PublicID)
	if err != nil {
		h.log.WarnContext(ctx, "import row counts unavailable", "error", err)
		return
	}
	view.RowCounts = counts
}

// vendorWarehouses lists the vendor's warehouses for the settings stage.
func (h *UIHandler) vendorWarehouses(r *http.Request) []*inventory.Warehouse {
	if h.invSvc == nil {
		return nil
	}
	actor, ok := authctx.From(r.Context())
	if !ok || actor.OrganizationID <= 0 {
		return nil
	}
	list, err := h.invSvc.ListWarehouses(r.Context())
	if err != nil {
		h.log.WarnContext(r.Context(), "warehouses unavailable for import", "error", err)
		return nil
	}
	var filtered []*inventory.Warehouse
	for _, wh := range list {
		if wh.OrganizationID == actor.OrganizationID {
			filtered = append(filtered, wh)
		}
	}
	return filtered
}

// renderImport writes the page.
func (h *UIHandler) renderImport(w http.ResponseWriter, r *http.Request, view pages.VendorImportView) {
	lang, dir := h.localeAndDir(r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorIngestPage(view, lang, dir).Render(r.Context(), w); err != nil {
		h.log.ErrorContext(r.Context(), "render vendor import page", "error", err)
	}
}
