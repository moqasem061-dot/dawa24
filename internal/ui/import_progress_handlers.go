package ui

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/importrun"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// ImportProgressJSON serves the unified progress snapshot for any import run.
//
// Route: GET /imports/{id}/progress
//
// It answers the shared ImportProgress bar (import-progress.js) with:
//   - percent (0..100)
//   - message (human-readable phase)
//   - current (processed rows)
//   - total (total rows)
//   - done (boolean)
//   - state / status
//
// For saving-product imports in the 'ready' state, it also attaches the
// staged items so modal review tables can render immediately without a
// second request.
func (h *UIHandler) ImportProgressJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	ctx := r.Context()
	lang := langOf(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"error":   i18n.T(lang, "common.unauthorized"),
		})
		return
	}

	publicID := chi.URLParam(r, "id")
	if publicID == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"error":   "missing import id",
		})
		return
	}

	// 1. Try durable platform.import_runs first.
	if h.importRunRepo != nil {
		var run *importrun.Run
		var err error
		if actor.IsPlatformAdmin() {
			run, err = h.importRunRepo.GetRunByPublicIDSystem(ctx, publicID)
		} else {
			run, err = h.importRunRepo.GetRunByPublicID(ctx, publicID, actor.OrganizationID)
		}

		if err == nil && run != nil {
			h.respondWithRunProgress(w, r, run)
			return
		}
	}

	// 2. Transitional fallback: in-memory saving products session store.
	if sess, ok := globalSavingImportSessionStore.GetSession(publicID, actor.OrganizationID); ok {
		respondWithSavingSessionProgress(w, sess)
		return
	}
	if actor.IsPlatformAdmin() {
		if sess, ok := globalSavingImportSessionStore.GetSessionForAdmin(publicID); ok {
			respondWithSavingSessionProgress(w, sess)
			return
		}
	}

	w.WriteHeader(http.StatusNotFound)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": false,
		"error":   i18n.T(lang, "customer.saving.import.session_not_found"),
	})
}

func (h *UIHandler) respondWithRunProgress(w http.ResponseWriter, r *http.Request, run *importrun.Run) {
	ctx := r.Context()
	isReady := run.State == importrun.StateReady
	isDone := run.IsDone()

	resp := map[string]any{
		"success":        run.State != importrun.StateFailed,
		"state":          run.State,
		"status":         run.State,
		"phase":          run.Phase,
		"progress_phase": run.Phase,
		"percent":        run.Percent,
		"progress":       run.Percent,
		"processed_rows": run.ProcessedRows,
		"current":        run.ProcessedRows,
		"total_rows":     run.TotalRows,
		"total":          run.TotalRows,
		"error_message":  run.ErrorMessage,
		"error":          run.ErrorMessage,
		"done":           isDone,
		"is_ready":       isReady,
	}

	// Unpack result summary counters if present.
	if len(run.Result) > 0 && string(run.Result) != "{}" {
		var res map[string]any
		if err := json.Unmarshal(run.Result, &res); err == nil {
			for k, v := range res {
				resp[k] = v
			}
		}
	}

	// When ready, attach staged items for saving-products so modal review can render.
	if isReady && run.Kind == importrun.KindSavingProducts && h.importRunRepo != nil {
		rows, _, err := h.importRunRepo.ListRows(ctx, run.ID, false, 5000, 0)
		if err == nil && len(rows) > 0 {
			items := make([]map[string]any, 0, len(rows))
			for _, row := range rows {
				var item map[string]any
				if jsonErr := json.Unmarshal(row.Data, &item); jsonErr == nil {
					item["included"] = row.Included
					items = append(items, item)
				}
			}
			resp["items"] = items
		}
	}

	_ = json.NewEncoder(w).Encode(resp)
}

func respondWithSavingSessionProgress(w http.ResponseWriter, sess *pages.SavingImportSession) {
	if sess == nil {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "session not found"})
		return
	}

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

	resp := map[string]any{
		"id":             sess.ID,
		"public_id":      sess.ID,
		"session_id":     sess.ID,
		"success":        sess.Status != pages.SessionStateFailed,
		"state":          state,
		"status":         string(sess.Status),
		"phase":          sess.ProgressPhase,
		"progress_phase": sess.ProgressPhase,
		"percent":        pct,
		"progress":       pct,
		"processed_rows": sess.ProcessedRows,
		"current":        sess.ProcessedRows,
		"total_rows":     sess.TotalRows,
		"total":          sess.TotalRows,
		"message":        sess.ProgressPhase,
		"error_message":  sess.ErrorMessage,
		"error":          sess.ErrorMessage,
		"done":           isDone,
		"is_ready":       isReady,
		"matched_rows":   sess.MatchedRows,
		"unlinked_rows":  sess.UnlinkedRows,
		"total_quantity": sess.TotalQuantity,
		"total_value":    sess.TotalValue.String(),
	}
	if isReady || sess.Status == pages.SessionStateCommitted {
		resp["items"] = sess.Items
	}
	_ = json.NewEncoder(w).Encode(resp)
}
