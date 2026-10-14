package ui

import (
	"encoding/json"
	"github.com/go-chi/chi/v5"
	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"net/http"
)

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
		"failed":  progress.Phase == catalog.ImportPhaseFailed || session.Status == catalog.SessionFailed,
		"status":  string(session.Status),
		"elapsed": int(progress.Elapsed().Seconds()),
	}
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		h.log.WarnContext(r.Context(), "write import progress", "session", publicID, "error", err)
	}
}
