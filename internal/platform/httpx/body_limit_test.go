package httpx

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMaxRequestBody(t *testing.T) {
	var readErr error
	h := MaxRequestBody(10)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, readErr = io.ReadAll(r.Body)
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", strings.NewReader(strings.Repeat("x", 11))))
	if rec.Code == http.StatusOK {
		t.Error("declared oversize body was accepted")
	}

	// A body without a declared length is cut off while reading.
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", io.NopCloser(strings.NewReader(strings.Repeat("x", 11))))
	req.ContentLength = -1
	h.ServeHTTP(httptest.NewRecorder(), req)
	if readErr == nil {
		t.Error("undeclared oversize body was read in full")
	}
}
