package ui

import (
	"context"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/ingest"
	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/importrun"
	"github.com/muhiya/dawa24-store/internal/platform/progress"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/importprogress"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// ImportProgressStream is the live half of /imports/{id}/progress.
//
// Route: GET /imports/{id}/stream
func (h *UIHandler) ImportProgressStream(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Error(w, i18n.T(lang, "common.unauthorized"), http.StatusUnauthorized)
		return
	}
	publicID := chi.URLParam(r, "id")
	if publicID == "" {
		http.Error(w, "missing import id", http.StatusBadRequest)
		return
	}
	if h.importRunRepo == nil || h.progressHub == nil {
		http.Error(w, "streaming unavailable", http.StatusServiceUnavailable)
		return
	}

	isAdmin := actor.IsPlatformAdmin()
	orgID := actor.OrganizationID

	fetch := func(ctx context.Context) (progress.Snapshot, bool) {
		var (
			run *importrun.Run
			err error
		)
		if isAdmin {
			run, err = h.importRunRepo.GetRunByPublicIDSystem(ctx, publicID)
		} else {
			run, err = h.importRunRepo.GetRunByPublicID(ctx, publicID, orgID)
		}
		if err == nil && run != nil {
			return snapshotOfRun(run, lang), true
		}
		if sess, ok := globalSavingImportSessionStore.GetSession(publicID, orgID); ok {
			return snapshotOfSavingSession(sess), true
		}
		if isAdmin {
			if sess, ok := globalSavingImportSessionStore.GetSessionForAdmin(publicID); ok {
				return snapshotOfSavingSession(sess), true
			}
		}
		return progress.Snapshot{}, false
	}

	progress.Stream(w, r, h.progressHub, publicID, fetch)
}

// snapshotOfRun converts a durable import run into a progress snapshot.
func snapshotOfRun(run *importrun.Run, lang string) progress.Snapshot {
	s := progress.Snapshot{
		ID:      run.PublicID,
		Percent: run.Percent,
		Message: importPhaseLabel(run, lang),
		Current: run.ProcessedRows,
		Total:   run.TotalRows,
		State:   run.State,
		Done:    run.IsDone(),
		Error:   run.ErrorMessage,
		At:      run.UpdatedAt,
	}
	if run.State == importrun.StateReady {
		s.Done = true
	}
	if s.Done && !s.IsFailure() {
		s.Percent = 100
	}
	return s
}

// snapshotOfSavingSession converts an in-memory saving session into a progress snapshot.
func snapshotOfSavingSession(sess *pages.SavingImportSession) progress.Snapshot {
	isReady := sess.Status == pages.SessionStateReady
	isDone := isReady || sess.Status == pages.SessionStateCommitted || sess.Status == pages.SessionStateFailed
	state := string(sess.Status)
	if isReady {
		state = "ready"
	}
	pct := sess.Progress
	if isDone && sess.Status != pages.SessionStateFailed {
		pct = 100
	}
	return progress.Snapshot{
		ID:      sess.ID,
		Percent: pct,
		Message: sess.ProgressPhase,
		Current: sess.ProcessedRows,
		Total:   sess.TotalRows,
		State:   state,
		Done:    isDone,
		Error:   sess.ErrorMessage,
		At:      sess.CreatedAt,
	}
}

// importPhaseLabel resolves a user-facing label for a run's state and phase.
func importPhaseLabel(run *importrun.Run, lang string) string {
	if run.State == importrun.StateFailed && run.ErrorMessage != "" {
		return run.ErrorMessage
	}
	if run.Phase != "" {
		return run.Phase
	}
	switch run.State {
	case importrun.StateQueued:
		return i18n.T(lang, "import.state.queued")
	case importrun.StateProcessing:
		return i18n.T(lang, "import.state.processing")
	case importrun.StateReady:
		return i18n.T(lang, "import.state.ready")
	case importrun.StateCommitting:
		return i18n.T(lang, "import.state.committing")
	case importrun.StateCommitted:
		return i18n.T(lang, "import.state.committed")
	case importrun.StateFailed:
		return i18n.T(lang, "import.state.failed")
	case importrun.StateCancelled:
		return i18n.T(lang, "import.state.cancelled")
	}
	return run.State
}

// VendorIngestProgressStream streams one vendor import's progress.
// Route: GET /vendor/ingest/{id}/stream
func (h *UIHandler) VendorIngestProgressStream(w http.ResponseWriter, r *http.Request) {
	publicID := chi.URLParam(r, "id")
	if publicID == "" || h.ingSvc == nil || h.progressHub == nil {
		http.Error(w, "streaming unavailable", http.StatusServiceUnavailable)
		return
	}

	session, err := h.ingSvc.LoadImport(r.Context(), publicID)
	if err != nil || session == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	fetch := func(ctx context.Context) (progress.Snapshot, bool) {
		s, err := h.ingSvc.LoadImport(ctx, publicID)
		if err != nil || s == nil {
			return progress.Snapshot{}, false
		}
		return ingestSnapshot(s), true
	}

	progress.Stream(w, r, h.progressHub, ingest.ProgressKey(session.ID), fetch)
}

// AdminOrgImportVendorIngestProgressStream streams vendor import progress for admin on behalf of an org.
func (h *UIHandler) AdminOrgImportVendorIngestProgressStream(w http.ResponseWriter, r *http.Request) {
	publicID := chi.URLParam(r, "id")
	orgID, _ := strconv.ParseInt(chi.URLParam(r, "orgID"), 10, 64)
	if publicID == "" || h.ingSvc == nil || h.progressHub == nil {
		http.Error(w, "streaming unavailable", http.StatusServiceUnavailable)
		return
	}

	sysCtx := database.WithTenant(database.AsSystem(r.Context()), orgID)
	session, err := h.ingSvc.LoadImport(sysCtx, publicID)
	if err != nil || session == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	fetch := func(ctx context.Context) (progress.Snapshot, bool) {
		s, err := h.ingSvc.LoadImport(sysCtx, publicID)
		if err != nil || s == nil {
			return progress.Snapshot{}, false
		}
		return ingestSnapshot(s), true
	}

	progress.Stream(w, r, h.progressHub, ingest.ProgressKey(session.ID), fetch)
}

func ingestSnapshot(s *ingest.Session) progress.Snapshot {
	running := s.Phase == ingest.PhaseProcessing
	percent := s.ProgressPercent
	switch {
	case !running && s.Phase != ingest.PhaseFailed:
		percent = 100
	case percent >= 100:
		percent = 99
	}
	return progress.Snapshot{
		ID:      ingest.ProgressKey(s.ID),
		Percent: percent,
		Message: s.ProgressNote,
		State:   string(s.Phase),
		Done:    !running,
		Error:   s.ErrorMessage,
	}
}

// AdminProductsImportProgressStream streams one administrative import's progress.
// Route: GET /admin/products/import/{id}/stream
func (h *UIHandler) AdminProductsImportProgressStream(w http.ResponseWriter, r *http.Request) {
	publicID := chi.URLParam(r, "id")
	if !h.requirePlatformAdmin(w, r) {
		return
	}
	if publicID == "" || h.catSvc == nil || h.progressHub == nil {
		http.Error(w, "streaming unavailable", http.StatusServiceUnavailable)
		return
	}

	fetch := func(ctx context.Context) (progress.Snapshot, bool) {
		p, session, err := h.catSvc.SessionProgress(database.AsSystem(ctx), publicID)
		if err != nil || session == nil {
			return progress.Snapshot{}, false
		}
		return progress.Snapshot{
			ID:      publicID,
			Percent: p.Percent(),
			Message: p.Message,
			Current: p.Current,
			Total:   p.Total,
			State:   string(session.Status),
			Done:    !session.IsProcessing(),
			Error:   adminImportFailure(p),
			At:      p.UpdatedAt,
		}, true
	}

	progress.Stream(w, r, h.progressHub, publicID, fetch)
}

func adminImportFailure(p catalog.ImportProgress) string {
	if p.Phase == catalog.ImportPhaseFailed {
		return p.Message
	}
	return ""
}

// SmartOrderProgressStream streams one smart-order run's progress.
// Route: GET /customer/smart-order/{id}/stream
func (h *UIHandler) SmartOrderProgressStream(w http.ResponseWriter, r *http.Request) {
	if h.progressHub == nil || h.smartOrderSvc == nil {
		http.Error(w, "streaming unavailable", http.StatusServiceUnavailable)
		return
	}
	run, ok := h.smartOrderRun(w, r)
	if !ok {
		return
	}
	lang, _ := h.localeAndDir(r)
	runID, orgID, publicID := run.ID, run.OrganizationID, run.PublicID

	fetch := func(ctx context.Context) (progress.Snapshot, bool) {
		current, err := h.smartOrderSvc.Get(ctx, orgID, publicID)
		if err != nil || current == nil {
			return progress.Snapshot{}, false
		}
		events, _ := h.smartOrderSvc.Events(ctx, runID, 0)
		return smartOrderSnapshot(current, events, lang), true
	}

	progress.Stream(w, r, h.progressHub, smartorder.ProgressKey(runID), fetch)
}

func smartOrderSnapshot(run *smartorder.Run, events []*smartorder.Event, lang string) progress.Snapshot {
	percent := smartorder.RunPercent(events)
	caption := i18n.T(lang, "smartorder.staging_caption")
	if stage := smartorder.CurrentStage(events); stage != "" {
		caption = stage.Label()
	}

	done := false
	switch run.Status {
	case smartorder.StatusCompleted, smartorder.StatusStale,
		smartorder.StatusPlaced, smartorder.StatusFailed:
		done = true
		if run.Status != smartorder.StatusFailed {
			percent = importprogress.Complete
		}
	default:
		if percent >= importprogress.Complete {
			percent = importprogress.Complete - 1
		}
		if percent <= 0 {
			percent = 2
		}
	}

	return progress.Snapshot{
		ID:      smartorder.ProgressKey(run.ID),
		Percent: percent,
		Message: caption,
		State:   string(run.Status),
		Done:    done,
		Error:   run.FailureReason,
	}
}
