package main

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"runtime/debug"
	"testing"

	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// Building the router is not a formality. chi panics at mount time for a whole
// class of mistakes — a Use() after the first route on a mux, a duplicate
// pattern, a malformed path — and none of them are visible to the compiler, to
// go vet, or to a test that exercises a handler in isolation.
//
// RegisterPublicRoutes called r.Use(h.visitorMiddleware) on the root mux, which
// by then already carried the identity routes. Build, vet and the whole suite
// stayed green; the binary panicked on boot in production:
//
//	panic: chi: all middlewares must be defined before routes on a mux
//
// The dependencies are deliberately unconnected, because that is the state the
// real server mounts in — routes are registered before the database and cache
// finish connecting, which is why a nil-safe database.New() is the right
// stand-in and a nil *DB is not.
func TestNewRouterMountsWithoutPanicking(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("newRouter panicked while mounting routes: %v\n%s", r, debug.Stack())
		}
	}()

	h := newRouter(&config.Config{}, log, &dependencies{db: database.New()}, nil, nil, nil, []database.Migration{})
	if h == nil {
		t.Fatal("newRouter returned nil")
	}

	// Serving one request proves the mux routes, rather than merely having been
	// constructed without complaint.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code == http.StatusNotFound {
		t.Errorf("public route / is not mounted: got %d", rec.Code)
	}
}
