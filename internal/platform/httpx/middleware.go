// Package httpx holds HTTP middleware and response helpers.
//
// Everything here is transport concern only. No business rule lives in this
// package, and no module imports net/http outside its own http/ subdirectory.
package httpx

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/errtrack"
	"github.com/muhiya/dawa24-store/internal/platform/observability"
	"github.com/muhiya/dawa24-store/internal/platform/reqcache"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// RequestID assigns or adopts a correlation id and echoes it back.
//
// An inbound id is trusted only for correlation, never for authorisation, and is
// length-capped so a hostile client cannot inflate every log line downstream.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" || len(id) > 64 {
			id = newRequestID()
		}
		ctx := observability.WithRequestID(r.Context(), id)
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func newRequestID() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand failing is close to unrecoverable, but a correlation id is
		// not worth killing a request over; fall back to a timestamp.
		return "ts-" + time.Now().UTC().Format("20060102T150405.000000000")
	}
	return hex.EncodeToString(b)
}

// Recover converts a panic into a 500 and keeps the process alive.
//
// It re-panics on http.ErrAbortHandler, which is the standard library's signal
// that the handler intentionally gave up on a connection.
func Recover(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				rec := recover()
				if rec == nil {
					return
				}
				if rec == http.ErrAbortHandler {
					panic(rec)
				}
				stack := string(debug.Stack())
				log.ErrorContext(r.Context(), "panic recovered",
					"panic", rec,
					"path", r.URL.Path,
					"method", r.Method,
					"stack", stack,
				)

				// A panic is the most serious thing that can happen to a
				// request and the least likely to be noticed in a log, so it
				// is recorded with its stack and at CRITICAL.
				event := errtrack.FromRequest(r, errtrack.Event{
					Level:          errtrack.LevelCritical,
					Message:        fmt.Sprintf("panic: %v", rec),
					ExceptionClass: errtrack.PanicClass(rec),
					StackTrace:     stack,
					StatusCode:     http.StatusInternalServerError,
				})
				event.FilePath, event.LineNumber = errtrack.PanicOrigin(stack)
				errtrack.ReportWithActor(r.Context(), event)
				if !headersSent(w) {
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// Logger records one structured line per request.
type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
	wrote  bool
}

func (w *statusWriter) WriteHeader(code int) {
	if w.wrote {
		return
	}
	w.status = code
	w.wrote = true
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// Flush forwards to the underlying writer.
//
// Unwrap alone is not enough: it only helps callers that go through
// http.ResponseController. A handler doing the conventional
// `w.(http.Flusher)` assertion sees this wrapper, not the real writer, and
// without this method the assertion fails — which silently disabled every SSE
// endpoint behind this middleware, including import progress.
func (w *statusWriter) Flush() {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func headersSent(w http.ResponseWriter) bool {
	sw, ok := w.(*statusWriter)
	return ok && sw.wrote
}

// isAssetPath reports whether this request is for a static asset or an
// uploaded file rather than for an application endpoint.
//
// Those requests are logged only when they fail. A catalogue page pulls
// nineteen stylesheets and scripts and up to twenty-four product images, so
// every page view wrote forty-odd lines that said "200" about a file that has
// not changed since the last deploy. With Docker's log rotation set to three
// files of ten megabytes, that is a retention window measured in minutes:
// the noise was not merely wasteful, it was evicting the lines somebody would
// actually want during an incident.
//
// A non-2xx asset request is still logged. A missing image or a 500 from the
// upload handler is a real fault and is exactly what this exists to catch.
func isAssetPath(p string) bool {
	return strings.HasPrefix(p, "/static/") || strings.HasPrefix(p, "/uploads/")
}

// slowRoundTrips is the round-trip count at which a request's repeated
// statements are logged.
const slowRoundTrips = 25

func Logger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			ctx, stats := database.WithQueryStats(r.Context())
			ctx = reqcache.With(ctx)

			next.ServeHTTP(sw, r.WithContext(ctx))

			if sw.status < 400 && isAssetPath(r.URL.Path) {
				return
			}

			level := slog.LevelInfo
			switch {
			case sw.status >= 500:
				level = slog.LevelError
			case sw.status >= 400:
				level = slog.LevelWarn
			}

			attrs := []any{}
			// A page past this many round trips is almost always running a
			// query per row; name the repeated statements so the log says which.
			if stats.RoundTrips() >= slowRoundTrips {
				attrs = append(attrs, "db_repeated", stats.Repeated(6))
			}
			log.Log(r.Context(), level, "http request", append([]any{
				"method", r.Method,
				"path", r.URL.Path,
				"status", sw.status,
				"bytes", sw.bytes,
				"duration_ms", time.Since(start).Milliseconds(),
				// Round trips and pool wait separate a page that asks the
				// database too often from one that waits for a connection.
				"db_round_trips", stats.RoundTrips(),
				"db_ms", stats.QueryTime().Milliseconds(),
				"db_pool_wait_ms", stats.PoolWait().Milliseconds(),
				"ip", clientIP(r),
			}, attrs...)...)
		})
	}
}

// Locale resolves the request language and writes it into the context.
//
// Order of precedence: explicit ?lang override, then the user's saved cookie,
// then Accept-Language, then Arabic. Arabic is the default because this platform
// is Arabic-first, not because it is alphabetically convenient.
func Locale(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var lang i18n.Lang

		switch {
		case r.URL.Query().Get("lang") != "":
			lang = i18n.ParseLang(r.URL.Query().Get("lang"))
			http.SetCookie(w, &http.Cookie{
				Name: "lang", Value: string(lang), Path: "/",
				MaxAge: 365 * 24 * 3600, HttpOnly: false, SameSite: http.SameSiteLaxMode,
			})
		case cookieValue(r, "lang") != "":
			lang = i18n.ParseLang(cookieValue(r, "lang"))
		case r.Header.Get("Accept-Language") != "":
			lang = i18n.ParseLang(r.Header.Get("Accept-Language"))
		default:
			lang = i18n.Default
		}

		next.ServeHTTP(w, r.WithContext(WithLang(r.Context(), lang)))
	})
}

func cookieValue(r *http.Request, name string) string {
	c, err := r.Cookie(name)
	if err != nil {
		return ""
	}
	return c.Value
}

// ClientIP resolves the address a request should be attributed to.
//
// trustedHops is how many reverse proxies sit in front of this process. It
// matters more than it looks: X-Forwarded-For is a header the caller writes,
// and every proxy appends to it rather than replacing it. Reading the first
// entry — which is what this function used to do, and what most examples do —
// means a scraper sending "X-Forwarded-For: <random>" is a brand new client on
// every request, and every per-address defence built on it counts to one
// forever.
//
// Counting from the right is what fixes that. The last entry was written by the
// proxy nearest to us and cannot be forged by the caller; the one before it was
// written by the proxy before that. With one proxy in front, the rightmost
// entry is the real client and everything to its left is whatever the caller
// invented.
//
// trustedHops of 0 ignores X-Forwarded-For entirely and uses the peer address,
// which is correct when nothing is in front of this process.
func ClientIP(r *http.Request, trustedHops int) string {
	if trustedHops > 0 {
		if parts := splitForwarded(r.Header.Get("X-Forwarded-For")); len(parts) > 0 {
			// Index from the right; a chain shorter than the configured hop
			// count means the request did not come through the whole chain, so
			// the leftmost entry is the furthest we can trust.
			i := len(parts) - trustedHops
			if i < 0 {
				i = 0
			}
			return parts[i]
		}
		if rip := strings.TrimSpace(r.Header.Get("X-Real-IP")); rip != "" {
			return rip
		}
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func splitForwarded(header string) []string {
	if strings.TrimSpace(header) == "" {
		return nil
	}
	raw := strings.Split(header, ",")
	out := make([]string, 0, len(raw))
	for _, p := range raw {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// clientIP is the log line's view of the caller. One proxy (Elest.io's) sits in
// front of this process in every deployed environment.
func clientIP(r *http.Request) string { return ClientIP(r, 1) }
