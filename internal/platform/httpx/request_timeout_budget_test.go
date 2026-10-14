package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// An upload's long budget belongs to that upload. It used to be written into
// the middleware's shared deadline, so every request after the first upload on
// the server ran under ten minutes instead of its own budget.
func TestUploadBudgetDoesNotLeakIntoLaterRequests(t *testing.T) {
	var got time.Duration
	h := RequestTimeout(25*time.Second, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if dl, ok := r.Context().Deadline(); ok {
			got = time.Until(dl)
		}
	}))

	up := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/vendor/ingest/upload", nil)
	up.Header.Set("Content-Type", "multipart/form-data; boundary=x")
	h.ServeHTTP(httptest.NewRecorder(), up)
	if got < 5*time.Minute {
		t.Fatalf("upload budget = %s, want the long upload budget", got)
	}

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/vendor/ingest/x", nil))
	if got > 26*time.Second {
		t.Fatalf("a plain request after an upload got %s; the upload budget leaked", got)
	}
}
