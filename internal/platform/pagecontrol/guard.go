package pagecontrol

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
)

type blockedContextKey struct{}

var blockedKey = blockedContextKey{}

// BlockedInfoFrom retrieves BlockedInfo from request context if placed by Guard.
func BlockedInfoFrom(ctx context.Context) (BlockedInfo, bool) {
	if ctx == nil {
		return BlockedInfo{}, false
	}
	info, ok := ctx.Value(blockedKey).(BlockedInfo)
	return info, ok
}

// WithBlockedInfo returns a context containing the given BlockedInfo.
func WithBlockedInfo(ctx context.Context, info BlockedInfo) context.Context {
	return context.WithValue(ctx, blockedKey, info)
}

var (
	customDisabledHandler http.HandlerFunc
	customDisabledMu      sync.RWMutex
)

// SetDisabledHandler registers a custom handler (such as a styled maintenance page)
// to be invoked when a route is disabled by page control.
func SetDisabledHandler(fn http.HandlerFunc) {
	customDisabledMu.Lock()
	defer customDisabledMu.Unlock()
	customDisabledHandler = fn
}

func getDisabledHandler() http.HandlerFunc {
	customDisabledMu.RLock()
	defer customDisabledMu.RUnlock()
	return customDisabledHandler
}

// Guard wraps the application router. It intercepts disabled routes and invokes
// either the registered custom disabled handler (e.g. styled maintenance page)
// or the fallback notFound handler.
//
// It runs before every middleware the router carries, authentication included:
// a disabled page is handled consistently from an anonymous visitor exactly as it is
// from a signed-in one, on both HTML and JSON surfaces.
func Guard(next http.Handler, notFound http.HandlerFunc, log *slog.Logger) http.Handler {
	if log == nil {
		log = slog.Default()
	}
	if notFound == nil {
		notFound = http.NotFound
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := NormalizePath(r.URL.Path)
		if IsProtected(p) {
			next.ServeHTTP(w, r)
			return
		}
		e := Global()
		if e == nil {
			next.ServeHTTP(w, r)
			return
		}
		if blocked, info := e.DecisionInfo(p); blocked {
			log.WarnContext(r.Context(), "pagecontrol: route disabled",
				"path", p, "method", r.Method, "rule_id", info.RuleID)
			ctx := context.WithValue(r.Context(), blockedKey, info)
			reqWithInfo := r.WithContext(ctx)
			if handler := getDisabledHandler(); handler != nil {
				handler(w, reqWithInfo)
				return
			}
			notFound(w, reqWithInfo)
			return
		}
		next.ServeHTTP(w, r)
	})
}

