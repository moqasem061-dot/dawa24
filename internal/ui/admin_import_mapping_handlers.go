package ui

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// Step two of the catalogue import: the column review.
//
// The step the wizard did not have. An upload used to start processing in the
// same request, so the first thing an admin saw was the result of a mapping
// nobody had looked at — and a wrong mapping produced a review screen reporting
// nothing found, with nothing on it to say why.
//
// Everything here is free of consequence. Preview re-reads the file under the
// admin's corrections and redraws; prepare is the one deliberate act that hands
// the settings to a background run. Neither touches the catalogue.

// AdminProductsImportMappingPage is step two: how the file will be read.
func (h *UIHandler) AdminProductsImportMappingPage(w http.ResponseWriter, r *http.Request) {
	if !h.importReady(w, r) {
		return
	}
	h.renderImportMapping(w, r, catalog.ImportSettings{}, "", "", http.StatusOK)
}

// AdminProductsImportPreview re-reads the file under the admin's corrections
// and redraws the mapping screen. It stages nothing.
func (h *UIHandler) AdminProductsImportPreview(w http.ResponseWriter, r *http.Request) {
	if !h.importReady(w, r) {
		return
	}
	lang := langOf(r)
	if err := r.ParseForm(); err != nil {
		h.renderImportMapping(w, r, catalog.ImportSettings{},
			i18n.T(lang, "admin.import.parse_settings_failed"), "error", http.StatusBadRequest)
		return
	}
	h.renderImportMapping(w, r, readImportSettings(r),
		i18n.T(lang, "admin.import.preview_updated_success"), "ok", http.StatusOK)
}

// renderImportMapping draws step two, applying settings when the admin has just
// submitted some and falling back to whatever the session already carries.
func (h *UIHandler) renderImportMapping(
	w http.ResponseWriter, r *http.Request,
	settings catalog.ImportSettings, notice, kind string, status int,
) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	publicID := chi.URLParam(r, "id")
	sysCtx := database.AsSystem(ctx)

	if settings.Mode == "" {
		// No submission: preview the session as it stands, so landing on this
		// screen already shows what the detected mapping yields.
		session, err := h.catSvc.GetImportSession(sysCtx, publicID)
		if err != nil {
			h.importSessionGone(w, r, publicID, err)
			return
		}
		settings = catalog.ImportSettings{
			Mode: session.Mode, Options: session.Options, Overrides: session.Overrides,
		}
	}

	session, preview, err := h.catSvc.PreviewImport(sysCtx, publicID, settings)
	if err != nil {
		if session == nil {
			h.importSessionGone(w, r, publicID, err)
			return
		}
		// A run already in flight, or a file that has expired: the session is
		// still real, so say what happened on the screen the admin is on.
		notice, kind, status = h.importMessage(err, r), "error", http.StatusUnprocessableEntity
	}

	view := pages.NewImportMappingView(session, preview, h.importCategories(ctx), h.aiAvailable(ctx))
	view.Notice, view.NoticeKind = notice, kind

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := pages.AdminProductsImportMapping(lang, dir, view).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render import mapping", "error", err)
	}
}

// AdminProductsImportPrepare starts the background run under the settings the
// admin confirmed on the mapping screen.
func (h *UIHandler) AdminProductsImportPrepare(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	publicID := chi.URLParam(r, "id")

	if !h.importReady(w, r) {
		return
	}
	lang := langOf(r)
	if err := r.ParseForm(); err != nil {
		h.renderImportMapping(w, r, catalog.ImportSettings{},
			i18n.T(lang, "admin.import.parse_settings_failed"), "error", http.StatusBadRequest)
		return
	}

	settings := readImportSettings(r)
	if err := h.catSvc.PrepareImportAsync(database.AsSystem(ctx), publicID, settings); err != nil {
		h.log.ErrorContext(ctx, "import preparation could not start",
			"session", publicID, "error", err)
		h.renderImportMapping(w, r, settings, h.importMessage(err, r), "error", http.StatusUnprocessableEntity)
		return
	}
	http.Redirect(w, r, importPath(publicID, ""), http.StatusSeeOther)
}
