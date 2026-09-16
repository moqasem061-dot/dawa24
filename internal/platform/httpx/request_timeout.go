package httpx

import (
	"context"
	"net/http"
	"strings"
	"time"
)

// RequestTimeout gives every request a deadline on its context.
//
// Without one, a handler blocked on a slow query or an exhausted connection
// pool waits indefinitely. Two things then happen, in this order: the server's
// WriteTimeout fires and closes the connection, and the reverse proxy in front
// of it — which saw a connection close before any response — reports 502 Bad
// Gateway. The user sees a gateway error for what was really one slow query.
//
// Worse, the abandoned handler keeps its pool connection. Under load the slow
// requests pile up, the pool empties, and every other request starts waiting
// too: one slow endpoint takes the whole site down with 502s that name no
// cause.
//
// A context deadline fixes the cause rather than the symptom. pgx honours it,
// so the query is cancelled server-side and the pool connection comes straight
// back; the handler returns an error the app renders as a normal page. The
// deadline is set shorter than the server's WriteTimeout on purpose, so the
// application always wins the race and gets to write a real response.
//
// Long-lived responses are exempt: a streamed download or an event stream is
// meant to stay open, and a deadline would truncate it.
func RequestTimeout(d time.Duration, exempt func(*http.Request) bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if d <= 0 || (exempt != nil && exempt(r)) {
				extendSocketDeadlines(w)
				next.ServeHTTP(w, r)
				return
			}
			// An upload is bounded by how fast the client can send, which is
			// not something the server may put a fifteen-second limit on. It
			// still gets a deadline — an upload that has not arrived in ten
			// minutes is not arriving — but the socket is given room for the
			// transfer first. See LongRequestBudget.
			if IsUpload(r) {
				extendSocketDeadlines(w)
				d = UploadRequestBudget
			}
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// LongRequestBudget is how long a long-running request may hold its socket.
//
// It exists because exempting a route from the CONTEXT deadline did nothing
// about the two deadlines that actually cut these requests off, and those are
// on the connection rather than the context:
//
//   - http.Server.ReadTimeout (15s) bounds reading the WHOLE request, body
//     included. A pharmacy uploading a 30 MB price list over a 2 Mbps Egyptian
//     upstream needs two minutes to send it, and the server closed the socket
//     after fifteen seconds — every time, for everyone on a slow link. That is
//     the "uploads take forever and then fail" report: the transfer never
//     completed, so the browser either hung on a dead connection or retried the
//     whole file from the start.
//   - http.Server.WriteTimeout (30s) starts when the request headers are read,
//     so it bounds the handler as well as the response. A compare batch that
//     legitimately parses for a minute had its connection closed underneath it
//     at thirty seconds, and the proxy reported 502 with no cause — while the
//     handler carried on working and writing rows nobody would ever see.
//
// Fifteen minutes. Long enough for a large file on a poor connection plus the
// work that follows it, short enough that a wedged handler still lets go of its
// socket. Background processing shortens the second half of that considerably;
// this is the ceiling, not the expectation.
const LongRequestBudget = 15 * time.Minute

// UploadRequestBudget bounds an upload that is not otherwise long-running.
//
// It is a real deadline, and it has to be: without one a stalled upload holds
// its handler, and with it its pool connection, which is how one slow endpoint
// used to take the whole site down. Ten minutes is generous for a spreadsheet
// and short enough to be a ceiling rather than a licence.
const UploadRequestBudget = 10 * time.Minute

// IsUpload reports whether a request carries a file body.
//
// Every import screen on the platform posts multipart/form-data, and the ones
// outside longRunningPrefixes — the saving lists, the team lists, the temp
// warehouses, an organisation's catalogue — were subject to the ordinary
// fifteen-second read deadline like any other form post. A form post is a
// kilobyte and a price list is thirty megabytes, and the deadline could not
// tell them apart because it never looked at the body.
func IsUpload(r *http.Request) bool {
	switch r.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch:
	default:
		return false
	}
	ct := r.Header.Get("Content-Type")
	return strings.HasPrefix(strings.TrimSpace(strings.ToLower(ct)), "multipart/form-data")
}

// extendSocketDeadlines lifts the server's read and write deadlines for a
// request that is meant to outlast them.
//
// http.ResponseController is the supported way to reach the connection from a
// handler; when the ResponseWriter does not support it (a test recorder, a
// wrapper that does not Unwrap) the calls return ErrNotSupported and the
// request simply keeps the server's defaults, which is the behaviour it had
// before this existed.
func extendSocketDeadlines(w http.ResponseWriter) {
	rc := http.NewResponseController(w)
	deadline := time.Now().Add(LongRequestBudget)
	_ = rc.SetReadDeadline(deadline)
	_ = rc.SetWriteDeadline(deadline)
}

// longRunningPrefixes are the paths whose responses are meant to stay open
// longer than an ordinary page: file exports, import runs the browser polls,
// and streamed assistant replies.
var longRunningPrefixes = []string{
	// A batch of supplier spreadsheets is read, parsed and matched inside the
	// request; a dozen large files legitimately outlast an ordinary deadline,
	// and cutting one short abandons the batch half-processed.
	"/compare/upload",
	"/compare/run",
	"/compare/files/",
	"/compare/file/",
	"/vendor/ingest/",
	"/admin/products/import/",
	"/admin/products/images/import/",
	"/smart-order/",
	"/assistant/",
	"/documents/",
	// A Telegram or WhatsApp question is answered inside the n8n request, and an answer
	// can take the assistant's full turn budget. The service bounds it.
	"/api/v1/integrations/telegram/",
	"/api/v1/integrations/whatsapp/",
}

// IsLongRunning reports whether a request should be exempt from the standard
// request deadline.
func IsLongRunning(r *http.Request) bool {
	p := r.URL.Path
	if strings.HasSuffix(p, "/export") || strings.HasSuffix(p, ".xlsx") || strings.HasSuffix(p, ".csv") {
		return true
	}
	for _, prefix := range longRunningPrefixes {
		if strings.HasPrefix(p, prefix) {
			return true
		}
	}
	// Server-sent events and other streams.
	return strings.Contains(r.Header.Get("Accept"), "text/event-stream")
}
