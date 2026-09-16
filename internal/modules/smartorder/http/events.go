package http

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/httpx"
)

// Events streams progress as server-sent events.
// GET /api/v1/smart-order/{id}/events
//
// The stream is advisory. A buyer who loses it can reload the progress page and
// see the true state, because progress lives in the database rather than in this
// connection — which is also what makes a run resumable from another device.
func (h *Handler) Events(w http.ResponseWriter, r *http.Request) {
	orgID, _, ok := h.actor(w, r)
	if !ok {
		return
	}
	run, ok := h.run(w, r, orgID)
	if !ok {
		return
	}

	flusher, canStream := w.(http.Flusher)
	if !canStream {
		// No streaming available: return the current backlog as JSON so the
		// client can poll instead of hanging on a connection that will never
		// deliver.
		events, err := h.svc.Events(r.Context(), run.ID, 0)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"events": events})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	after := int64(0)
	if v := r.URL.Query().Get("after"); v != "" {
		after, _ = strconv.ParseInt(v, 10, 64)
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	deadline := time.After(10 * time.Minute)

	for {
		events, err := h.svc.Events(r.Context(), run.ID, after)
		if err == nil {
			for _, e := range events {
				after = e.ID
				fmt.Fprintf(w, "event: progress\ndata: {\"stage\":%q,\"processed\":%s,\"total\":%s}\n\n",
					e.Stage, intOrNull(e.Processed), intOrNull(e.Total))
			}
			if len(events) > 0 {
				flusher.Flush()
			}
		}

		select {
		case <-r.Context().Done():
			return
		case <-deadline:
			// A run that has not finished in ten minutes has a problem the
			// stream cannot solve; the client falls back to polling.
			return
		case <-ticker.C:
		}
	}
}

func intOrNull(v *int) string {
	if v == nil {
		return "null"
	}
	return strconv.Itoa(*v)
}
