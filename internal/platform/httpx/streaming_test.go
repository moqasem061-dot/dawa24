package httpx_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/muhiya/dawa24-store/internal/platform/httpx"
)

// SSE endpoints assert http.Flusher directly. When Logger wraps the writer
// without forwarding Flush, that assertion fails and every stream refuses to
// start — which is how import progress was silently dead.
func TestFlusherSurvivesLoggerMiddleware(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	var gotFlusher bool
	h := httpx.Logger(log)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, gotFlusher = w.(http.Flusher)
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	if !gotFlusher {
		t.Fatal("ResponseWriter is not an http.Flusher after Logger middleware; SSE handlers will refuse to stream")
	}
}
