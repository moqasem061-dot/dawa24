package progress

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Serving one import's progress as server-sent events.
//
// The two stream endpoints this replaces each ran a database query once a
// second for every open connection, for as long as the tab stayed open — and
// one of them kept doing it for a run wedged in `processing`, forever. This one
// reads the database twice in the ordinary case: once when the viewer connects,
// and once more if a whole safetyPoll goes by without the hub saying anything.
//
// Everything else arrives because the work announced it.

const (
	// safetyPoll is how long the stream will sit on hub silence before asking
	// the database whether it missed something.
	//
	// Ten seconds. Long enough that a healthy stream almost never queries after
	// the first read, short enough that a message lost to a Redis restart
	// delays a bar by a moment rather than stranding it. This is the number
	// that used to be one second, per connection.
	safetyPoll = 10 * time.Second

	// heartbeat keeps the connection alive through proxies that close an idle
	// one. It is a comment frame, not an event, so a client that is only
	// listening for progress never sees it.
	heartbeat = 20 * time.Second

	// maxStreamDuration bounds a single connection.
	//
	// A run wedged in `processing` — a worker that died mid-import — would
	// otherwise hold this goroutine for as long as the tab stays open. The
	// client reconnects if it still cares; EventSource does that by itself.
	maxStreamDuration = 30 * time.Minute
)

// Fetch reads the current snapshot of a run from wherever the truth lives.
//
// Returning ok=false means "no such run, or not this viewer's" and ends the
// stream: authorisation is the caller's, not this package's.
//
// A Fetch is NOT required to set At. Most sources have no timestamp to give —
// a vendor import session carries a phase and a percentage and nothing that
// says when — so Stream stamps a zero At with the time of the read. See
// readNow, and the bug it exists to prevent.
type Fetch func(ctx context.Context) (Snapshot, bool)

// readNow performs a fetch and timestamps the result.
//
// The stamp is what makes the safety poll work at all. Stream drops any
// snapshot older than the one it is already showing, which is right for events
// arriving out of order across a process boundary — and catastrophic for a
// fetch that always returns a ZERO timestamp, because a zero time is before
// everything. The moment one snapshot had been published, every subsequent
// safety read looked stale and was discarded, so the one mechanism that exists
// to recover a stream whose publisher has gone quiet was silently switched off
// for the rest of the connection.
//
// What that looked like: a bar that stopped where the last published event left
// it and never finished, on a run that had finished. The page had to be
// reloaded to see the result.
func readNow(ctx context.Context, fetch Fetch) (Snapshot, bool) {
	s, ok := fetch(ctx)
	if !ok {
		return s, false
	}
	if s.At.IsZero() {
		s.At = time.Now()
	}
	return s, true
}

// Stream writes one run's progress to w as server-sent events until the run
// finishes, the client goes away, or maxStreamDuration expires.
//
// It falls back to a single JSON snapshot when the ResponseWriter cannot flush,
// so a client behind a buffering layer gets an answer instead of a connection
// that never delivers anything.
func Stream(w http.ResponseWriter, r *http.Request, hub *Hub, id string, fetch Fetch) {
	flusher, canStream := w.(http.Flusher)
	if !canStream {
		snap, ok := readNow(r.Context(), fetch)
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(snap)
		return
	}

	// Subscribe BEFORE the first read, or a run that finishes in the gap
	// between them publishes its terminal snapshot to nobody and the bar sits
	// at ninety-nine per cent until the safety poll notices.
	events, last, known, cancel := hub.Subscribe(id)
	defer cancel()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// Reverse proxies buffer responses by default, which holds every event
	// until the stream closes — the client would see one burst at the end
	// instead of live progress. This app runs behind such a proxy in every
	// deployed environment, so without this header SSE appears broken in
	// production while working locally.
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	current, ok := readNow(r.Context(), fetch)
	if !ok {
		writeEvent(w, flusher, "gone", Snapshot{ID: id})
		return
	}
	// The hub may already hold something newer than the database does: the
	// worker publishes before its write is visible to another connection.
	if known && last.At.After(current.At) {
		current = last
	}
	if !writeEvent(w, flusher, "progress", current) || current.Terminal() {
		return
	}

	poll := time.NewTicker(safetyPoll)
	defer poll.Stop()
	beat := time.NewTicker(heartbeat)
	defer beat.Stop()
	deadline := time.After(maxStreamDuration)

	for {
		select {
		case <-r.Context().Done():
			return

		case <-deadline:
			writeEvent(w, flusher, "timeout", current)
			return

		case s, open := <-events:
			if !open {
				return
			}
			if s.At.Before(current.At) {
				continue // an older message overtaken in flight
			}
			current = s
			if !writeEvent(w, flusher, "progress", s) || s.Terminal() {
				return
			}
			// A stream that is being fed does not need to ask.
			poll.Reset(safetyPoll)
			beat.Reset(heartbeat)

		case <-poll.C:
			s, ok := readNow(r.Context(), fetch)
			if !ok {
				writeEvent(w, flusher, "gone", current)
				return
			}
			if s.At.Before(current.At) || (s.Percent == current.Percent && s.State == current.State && !s.Terminal()) {
				continue // nothing new to say
			}
			current = s
			if !writeEvent(w, flusher, "progress", s) || s.Terminal() {
				return
			}
			beat.Reset(heartbeat)

		case <-beat.C:
			// A comment frame. Keeps proxies from closing an idle connection
			// without telling the client anything it has to handle.
			if _, err := fmt.Fprint(w, ": keep-alive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// writeEvent emits one SSE frame and reports whether the connection is still
// usable.
func writeEvent(w http.ResponseWriter, f http.Flusher, name string, s Snapshot) bool {
	payload, err := json.Marshal(s)
	if err != nil {
		return false
	}
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, payload); err != nil {
		return false
	}
	f.Flush()
	return true
}
