package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/ingest"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// ListSessions lists import sessions for the current tenant.
func (h *Handler) ListSessions(w http.ResponseWriter, r *http.Request) {
	orgID, ok := database.TenantFrom(r.Context())
	if !ok {
		httpx.Error(w, r, h.log, database.ErrNoTenant)
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	sessions, err := h.service.ListSessions(r.Context(), orgID, limit, offset)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"sessions": sessions, "count": len(sessions)})
}

// ListRows lists staged rows for review.
func (h *Handler) ListRows(w http.ResponseWriter, r *http.Request) {
	id, ok := h.ownedSession(w, r)
	if !ok {
		return
	}

	status := r.URL.Query().Get("status")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	rows, err := h.service.ListImportRows(r.Context(), id, status, limit, offset)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"rows": rows, "count": len(rows)})
}

// UpdateMapping updates column mapping for a session.
func (h *Handler) UpdateMapping(w http.ResponseWriter, r *http.Request) {
	id, ok := h.ownedSession(w, r)
	if !ok {
		return
	}

	var body struct {
		Mapping map[string]string `json:"mapping"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	if err := h.service.UpdateColumnMapping(r.Context(), id, body.Mapping); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// CommitSession marks import session committed.
func (h *Handler) CommitSession(w http.ResponseWriter, r *http.Request) {
	id, ok := h.ownedSession(w, r)
	if !ok {
		return
	}

	if err := h.service.CommitSession(r.Context(), id); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "completed"})
}

// CancelSession marks import session cancelled.
func (h *Handler) CancelSession(w http.ResponseWriter, r *http.Request) {
	id, ok := h.ownedSession(w, r)
	if !ok {
		return
	}

	if err := h.service.CancelSession(r.Context(), id); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

// OverrideRowMatch overrides product match for a staged row.
func (h *Handler) OverrideRowMatch(w http.ResponseWriter, r *http.Request) {
	sessionID, ok := h.ownedSession(w, r)
	if !ok {
		return
	}
	rid, err := strconv.ParseInt(chi.URLParam(r, "rid"), 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("rid.invalid", "Invalid row ID", nil))
		return
	}
	// The row must belong to the session the caller owns; a row id alone
	// reached any organisation's staged rows.
	if row, err := h.service.GetImportRow(r.Context(), rid); err != nil || row == nil || row.SessionID != sessionID {
		httpx.Error(w, r, h.log, apperr.NotFound("import_row"))
		return
	}

	var body struct {
		ProductID int64 `json:"product_id"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	if err := h.service.OverrideRowMatch(r.Context(), rid, body.ProductID); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// StreamEvents handles Server-Sent Events (SSE) for real-time import progress.
func (h *Handler) StreamEvents(w http.ResponseWriter, r *http.Request) {
	id, ok := h.ownedSession(w, r)
	if !ok {
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		httpx.Error(w, r, h.log, apperr.New(apperr.KindUnavailable, "sse.unsupported", "Streaming unsupported"))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// Reverse proxies buffer responses by default, which holds every event until
	// the stream closes — the client would see one burst at the end instead of
	// live progress. This app runs behind such a proxy in every deployed
	// environment, so without this header SSE appears broken in production while
	// working locally.
	w.Header().Set("X-Accel-Buffering", "no")

	// Emit immediately rather than making the client wait a full tick to learn
	// anything. An import that is already finished then closes the stream at
	// once instead of hanging for a second.
	emit := func() bool {
		session, err := h.service.GetSessionProgress(r.Context(), id)
		if err != nil {
			return false
		}
		data, _ := json.Marshal(session)
		fmt.Fprintf(w, "event: progress\ndata: %s\n\n", data)
		flusher.Flush()
		return session.Status != ingest.StatusCompleted && session.Status != ingest.StatusFailed
	}

	if !emit() {
		return
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	// A session wedged in `processing` — a worker that died mid-import — would
	// otherwise hold this goroutine and one query per second for as long as the
	// browser tab stays open. The cap bounds that; the client reconnects if it
	// still cares.
	const maxStreamDuration = 30 * time.Minute
	deadline := time.After(maxStreamDuration)

	for {
		select {
		case <-r.Context().Done():
			return
		case <-deadline:
			fmt.Fprint(w, "event: timeout\ndata: {\"reason\":\"stream_duration_exceeded\"}\n\n")
			flusher.Flush()
			return
		case <-ticker.C:
			if !emit() {
				return
			}
		}
	}
}

// ownedSession parses the session id and admits the caller only to a session of
// their own organisation, or any session for platform staff holding
// ingest.admin. The permission gate in front of these routes says who may run
// imports at all; it said nothing about whose, so a supplier could read,
// re-map, commit or cancel another supplier's staged price list by id.
// Another organisation's session answers 404, like one that does not exist.
func (h *Handler) ownedSession(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid session ID", nil))
		return 0, false
	}
	actor, ok := authctx.From(r.Context())
	if !ok {
		httpx.Error(w, r, h.log, apperr.NotFound("import_session"))
		return 0, false
	}
	session, err := h.service.GetSessionProgress(r.Context(), id)
	if err != nil || session == nil {
		httpx.Error(w, r, h.log, apperr.NotFound("import_session"))
		return 0, false
	}
	staff := actor.IsStaff && actor.Can("ingest.admin")
	if !staff && (actor.OrganizationID <= 0 || session.OrganizationID != actor.OrganizationID) {
		h.log.WarnContext(r.Context(), "ingest: session of another organisation refused",
			"session_id", id, "actor_org", actor.OrganizationID, "user_id", actor.UserID)
		httpx.Error(w, r, h.log, apperr.NotFound("import_session"))
		return 0, false
	}
	return id, true
}
