package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// maxImportUploadBytes caps an uploaded catalogue file. The largest real
// distributor export seen is a few megabytes; 32 MB leaves ample headroom while
// keeping a single request from holding that much memory.
const maxImportUploadBytes int64 = 32 << 20

// maxImportRequestBytes caps the whole request body: the file plus generous
// room for the multipart envelope and form fields. Without it a client can
// stream an arbitrarily large "upload" that the server spools to disk in full
// before anything checks the size.
const maxImportRequestBytes int64 = maxImportUploadBytes + (1 << 20)

// importReviewPageSize is the staged-rows review table's page size for this
// request: the reader's choice off ?limit= (pagination.RowsPerPageOptions), or
// the platform default. A browser handed nine thousand table rows at once
// becomes unusable, so there is no "show all".
func importReviewPageSize(r *http.Request) int {
	return pagination.RowsPerPage(r)
}

// requirePlatformAdmin refuses any catalogue-import action from an actor that
// is not platform staff. The routes carry middleware that should already have
// let nothing else through; this re-checks at the feature because these
// endpoints write to the shared master catalogue and every one of them runs
// under AsSystem, which bypasses tenant scoping entirely.
func (h *UIHandler) requirePlatformAdmin(w http.ResponseWriter, r *http.Request) bool {
	actor, ok := authctx.From(r.Context())
	if !ok || !actor.IsPlatformAdmin() {
		http.Error(w, i18n.T(langOf(r), "admin.import.forbidden"), http.StatusForbidden)
		return false
	}
	return true
}

// importReady reports whether the catalogue service can serve this request,
// answering the admin rather than a blank page when it cannot.
func (h *UIHandler) importReady(w http.ResponseWriter, r *http.Request) bool {
	if !h.requirePlatformAdmin(w, r) {
		return false
	}
	if h.catSvc == nil {
		h.renderImportConfigure(w, r, pages.ImportConfigureView{
			Fatal: i18n.T(langOf(r), "admin.import.service_unavailable"),
		}, http.StatusServiceUnavailable)
		return false
	}
	return true
}

// AdminProductsImportPage is step one: choose a file.
func (h *UIHandler) AdminProductsImportPage(w http.ResponseWriter, r *http.Request) {
	h.renderImportConfigure(w, r, pages.ImportConfigureView{}, http.StatusOK)
}

// renderImportConfigure draws the upload screen, optionally carrying a rejection
// from a previous attempt.
func (h *UIHandler) renderImportConfigure(
	w http.ResponseWriter, r *http.Request, seed pages.ImportConfigureView, status int,
) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	view := pages.NewImportConfigureView(
		h.importCategories(ctx),
		h.recentImportSessions(ctx),
		h.aiAvailable(ctx),
	)
	view.Fatal, view.FatalDetail = seed.Fatal, seed.FatalDetail

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := pages.AdminProductsImportPage(lang, dir, view).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render import wizard", "error", err)
	}
}

// AdminProductsImportSubmit is step one's action: read the file's shape and
// send the admin to the mapping screen.
//
// It deliberately does not start processing. Reading a nine-thousand-row file
// into products and matching every one of them against the catalogue is fifteen
// seconds of work that is worthless if the price column was misread — and the
// admin had no chance to notice until it was over.
func (h *UIHandler) AdminProductsImportSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.importReady(w, r) {
		return
	}

	content, filename, uploadErr := readUploadedFile(r)
	if uploadErr != nil {
		h.log.WarnContext(ctx, "import upload rejected", "error", uploadErr)
		h.renderImportConfigure(w, r, pages.ImportConfigureView{
			Fatal: uploadErr.message, FatalDetail: uploadErr.detail,
		}, http.StatusUnprocessableEntity)
		return
	}

	session, _, err := h.catSvc.AnalyzeImport(database.AsSystem(ctx), content, filename, actorUserID(ctx))
	if err != nil {
		h.log.WarnContext(ctx, "import file unreadable", "file", filename, "error", err)
		h.renderImportConfigure(w, r, pages.ImportConfigureView{
			Fatal: h.importMessage(err, r),
		}, http.StatusUnprocessableEntity)
		return
	}

	http.Redirect(w, r, importPath(session.PublicID, "mapping"), http.StatusSeeOther)
}

// AdminProductsImportReviewPage is step three: the staged result.
func (h *UIHandler) AdminProductsImportReviewPage(w http.ResponseWriter, r *http.Request) {
	if !h.requirePlatformAdmin(w, r) {
		return
	}
	h.renderImportReview(w, r, "", http.StatusOK)
}

// renderImportReview draws the review screen for the session in the URL.
//
// The session is read once, through SessionProgress, and every decision below
// is made from that one read. Reading it twice — once for the rows and again
// for the progress — meant a run that finished between the two rendered a
// progress bar for work that was already over, and one that failed between them
// rendered a progress bar carrying the failure as its status line.
func (h *UIHandler) renderImportReview(w http.ResponseWriter, r *http.Request, notice string, status int) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	publicID := chi.URLParam(r, "id")

	if h.catSvc == nil {
		http.Error(w, "catalog service unavailable", http.StatusServiceUnavailable)
		return
	}

	sysCtx := database.AsSystem(ctx)
	progress, session, err := h.catSvc.SessionProgress(sysCtx, publicID)
	if err != nil {
		h.importSessionGone(w, r, publicID, err)
		return
	}

	// A session still on the mapping step has nothing staged. Sending the admin
	// to a review screen full of zeros — which is precisely what the old flow
	// did on every failed run — tells them the import found nothing when in
	// truth it has not run yet.
	if session.Status == catalog.SessionDraft {
		http.Redirect(w, r, importPath(publicID, "mapping"), http.StatusSeeOther)
		return
	}

	view := pages.NewImportReviewView(session, catalog.StagingCounts{}, nil, 0,
		pages.ParseStagingFilter(r.URL.Query(), importReviewPageSize(r)),
		h.importCategories(ctx), h.aiAvailable(ctx))
	view.Notice = notice
	view.SetStructure(session.Structure)

	// While a run is in flight the staged rows belong to the previous pass, so
	// the page shows progress instead — and does not pay for reading rows it
	// will not draw.
	if session.IsProcessing() {
		view.Progress, view.Working = progress, true
	} else {
		h.attachStagedRows(sysCtx, &view, session)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if h.isHTMX(r) {
		if err := pages.ImportRowsCard(view).Render(ctx, w); err != nil {
			h.log.ErrorContext(ctx, "render import review rows partial", "error", err)
		}
		return
	}
	if err := pages.AdminProductsImportReview(lang, dir, view).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render import review", "error", err)
	}
}

// attachStagedRows fills the review table for a session that has finished
// preparing.
func (h *UIHandler) attachStagedRows(
	ctx context.Context, view *pages.ImportReviewView, session *catalog.ImportSession,
) {
	rows, total, err := h.catSvc.ListStagingRowsFor(ctx, session.ID, view.Filter)
	if err != nil {
		h.log.ErrorContext(ctx, "staged rows unavailable", "session", session.PublicID, "error", err)
		return
	}
	counts, err := h.catSvc.StagingCounts(ctx, session.ID)
	if err != nil {
		h.log.ErrorContext(ctx, "staging counts unavailable", "session", session.PublicID, "error", err)
	}
	view.SetRows(rows, total, counts)
}

// importSessionGone sends the admin back to the upload screen with a reason,
// for a session that has expired, been reaped, or never existed.
func (h *UIHandler) importSessionGone(w http.ResponseWriter, r *http.Request, publicID string, cause error) {
	h.log.WarnContext(r.Context(), "import session unavailable", "session", publicID, "error", cause)
	h.renderImportConfigure(w, r, pages.ImportConfigureView{
		Fatal: i18n.T(langOf(r), "admin.import.session_expired"),
	}, http.StatusNotFound)
}

// AdminProductsImportRowToggle includes or excludes one staged row.
func (h *UIHandler) AdminProductsImportRowToggle(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	publicID := chi.URLParam(r, "id")

	if !h.requirePlatformAdmin(w, r) {
		return
	}
	if h.catSvc == nil {
		http.Error(w, "catalog service unavailable", http.StatusServiceUnavailable)
		return
	}
	rowID, err := strconv.ParseInt(chi.URLParam(r, "rowID"), 10, 64)
	if err != nil {
		http.Error(w, "invalid row", http.StatusBadRequest)
		return
	}

	included := r.PostFormValue("included") == "1"
	if err := h.catSvc.SetRowIncluded(database.AsSystem(ctx), publicID, rowID, included); err != nil {
		h.log.WarnContext(ctx, "could not toggle staged row", "session", publicID, "row", rowID, "error", err)
	}

	if h.isHTMX(r) {
		row, err := h.catSvc.GetStagingRow(database.AsSystem(ctx), publicID, rowID)
		if err == nil && row != nil {
			view := pages.ImportReviewView{Session: &catalog.ImportSession{PublicID: publicID}}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			if err := pages.ImportRowLine(view, row).Render(ctx, w); err == nil {
				return
			}
		}
	}

	http.Redirect(w, r, importPath(publicID, "")+querySuffix(r), http.StatusSeeOther)
}

// AdminProductsImportSelect includes or excludes every row sharing an action.
func (h *UIHandler) AdminProductsImportSelect(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	publicID := chi.URLParam(r, "id")
	if !h.requirePlatformAdmin(w, r) {
		return
	}
	if h.catSvc == nil {
		http.Error(w, "catalog service unavailable", http.StatusServiceUnavailable)
		return
	}

	action := catalog.RowAction(r.PostFormValue("action"))
	included := r.PostFormValue("included") == "1"
	affected, err := h.catSvc.SetRowsIncludedByAction(database.AsSystem(ctx), publicID, action, included)
	if err != nil {
		h.renderImportReview(w, r, h.importMessage(err, r), http.StatusUnprocessableEntity)
		return
	}

	h.log.InfoContext(ctx, "bulk staged row selection", "session", publicID, "action", action, "included", included, "rows", affected)
	http.Redirect(w, r, importPath(publicID, "")+querySuffix(r), http.StatusSeeOther)
}

// AdminProductsImportCommit is step four: write the reviewed rows.
func (h *UIHandler) AdminProductsImportCommit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	publicID := chi.URLParam(r, "id")
	if !h.requirePlatformAdmin(w, r) {
		return
	}
	if h.catSvc == nil {
		http.Error(w, "catalog service unavailable", http.StatusServiceUnavailable)
		return
	}

	sysCtx := database.AsSystem(ctx)
	session, err := h.catSvc.GetImportSession(sysCtx, publicID)
	if err != nil {
		h.importSessionGone(w, r, publicID, err)
		return
	}

	if session.Mode.IsDestructive() && r.PostFormValue("confirm_destructive") != "1" {
		h.renderImportReview(w, r, i18n.T(langOf(r), "admin.import.confirm_destructive_required"), http.StatusUnprocessableEntity)
		return
	}

	written, result, err := h.catSvc.CommitImport(sysCtx, publicID)
	if err != nil {
		h.log.ErrorContext(ctx, "import commit failed", "session", publicID, "failures", len(result.Failures), "error", err)
		h.renderImportReview(w, r, h.importMessage(err, r), http.StatusUnprocessableEntity)
		return
	}

	h.refreshProductIndex(ctx)
	h.watermarkImportedImages(written.ID)
	h.log.InfoContext(ctx, "import committed", "session", written.PublicID, "inserted", result.Inserted, "updated", result.Updated)
	h.redirectWithNotice(w, r, "/admin/products", "success", fmt.Sprintf(
		i18n.T(langOf(r), "admin.import.committed_success_format"), result.Total(), result.Inserted, result.Updated))
}

// AdminProductsImportCancel discards a session without touching the catalogue.
func (h *UIHandler) AdminProductsImportCancel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	publicID := chi.URLParam(r, "id")
	if !h.requirePlatformAdmin(w, r) {
		return
	}

	if h.catSvc != nil {
		if err := h.catSvc.CancelImport(database.AsSystem(ctx), publicID); err != nil {
			h.log.WarnContext(ctx, "could not cancel import", "session", publicID, "error", err)
			h.redirectWithNotice(w, r, importPath(publicID, "")+querySuffix(r), "error", h.importMessage(err, r))
			return
		}
	}
	h.redirectWithNotice(w, r, "/admin/products/import", "success",
		i18n.T(langOf(r), "admin.import.cancelled_success"))
}

// AdminProductsImportProgress reports a run's state as JSON.
//
// The review screen polls it while a file is being processed. It reads the
// session rather than a map in this process: progress held only in memory
// reported "failed" for a healthy run after a restart, and reported nothing at
// all for one started by another replica — which is how a completed import came
// to render as an empty screen with no explanation.
func (h *UIHandler) AdminProductsImportProgress(w http.ResponseWriter, r *http.Request) {
	publicID := chi.URLParam(r, "id")

	if !h.requirePlatformAdmin(w, r) {
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")

	if h.catSvc == nil {
		http.Error(w, `{"phase":"failed","message":"catalog service unavailable"}`,
			http.StatusServiceUnavailable)
		return
	}

	progress, session, err := h.catSvc.SessionProgress(database.AsSystem(r.Context()), publicID)
	if err != nil {
		http.Error(w, `{"phase":"failed","message":"import session unavailable"}`, http.StatusNotFound)
		return
	}

	payload := map[string]any{
		"phase":   string(progress.Phase),
		"message": progress.Message,
		"current": progress.Current,
		"total":   progress.Total,
		"percent": progress.Percent(),
		"done":    !session.IsProcessing(),
		"failed":  progress.Phase == catalog.ImportPhaseFailed,
		"status":  string(session.Status),
		"elapsed": int(progress.Elapsed().Seconds()),
	}
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		h.log.WarnContext(r.Context(), "write import progress", "session", publicID, "error", err)
	}
}
