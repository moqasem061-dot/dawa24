package httpx

import (
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// A slow upload must survive the server's read deadline.
//
// http.Server.ReadTimeout bounds reading the WHOLE request, body included. The
// deployed value is fifteen seconds, and a pharmacy sending a thirty-megabyte
// price list over a two-megabit upstream needs two minutes — so the socket was
// closed mid-transfer, every time, for every user on a slow link. Exempting the
// route from the CONTEXT deadline never touched this: the context and the
// connection are two different deadlines, and only one of them was being
// managed.
//
// The server here is configured with a deliberately tiny ReadTimeout and the
// body is sent slower than that. Without the deadline extension this test hangs
// up mid-body; with it the handler reads every byte.
func TestSlowUploadOutlivesTheServerReadTimeout(t *testing.T) {
	const (
		serverReadTimeout = 250 * time.Millisecond
		chunks            = 8
		chunkDelay        = 60 * time.Millisecond // 480ms total, ~2x the timeout
		chunkSize         = 1024
	)

	var got struct {
		n   int
		err error
	}
	done := make(chan struct{})

	handler := RequestTimeout(25*time.Second, IsLongRunning)(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer close(done)
			b, err := io.ReadAll(r.Body)
			got.n, got.err = len(b), err
			w.WriteHeader(http.StatusOK)
		}))

	// Wrapped in Logger because that is what the real chain does, and because
	// its statusWriter is what http.ResponseController has to see through. A
	// wrapper without Unwrap would make the whole fix a silent no-op.
	discard := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := httptest.NewUnstartedServer(Logger(discard)(handler))
	srv.Config.ReadTimeout = serverReadTimeout
	srv.Config.WriteTimeout = serverReadTimeout
	srv.Start()
	defer srv.Close()

	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	go func() {
		part, err := mw.CreateFormFile("file", "prices.xlsx")
		if err != nil {
			_ = pw.CloseWithError(err)
			return
		}
		for i := 0; i < chunks; i++ {
			time.Sleep(chunkDelay)
			if _, err := part.Write(make([]byte, chunkSize)); err != nil {
				_ = pw.CloseWithError(err)
				return
			}
		}
		_ = mw.Close()
		_ = pw.Close()
	}()

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/vendor/saving/import", pr)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("the slow upload was cut off: %v", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("handler never ran")
	}

	if got.err != nil {
		t.Fatalf("body read failed: %v", got.err)
	}
	if got.n < chunks*chunkSize {
		t.Fatalf("read %d bytes of the body, want at least %d — the upload was truncated",
			got.n, chunks*chunkSize)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200", resp.StatusCode)
	}
}

// The extension is scoped to uploads and long-running routes. An ordinary form
// post keeps the server's deadlines, because a request that is slow for any
// other reason is a request that should be cut off.
func TestIsUploadOnlyMatchesFileBodies(t *testing.T) {
	for _, tc := range []struct {
		method, contentType string
		want                bool
	}{
		{http.MethodPost, "multipart/form-data; boundary=x", true},
		{http.MethodPut, "MULTIPART/FORM-DATA; boundary=x", true},
		{http.MethodPost, "application/x-www-form-urlencoded", false},
		{http.MethodPost, "application/json", false},
		{http.MethodGet, "multipart/form-data; boundary=x", false},
		{http.MethodPost, "", false},
	} {
		r := httptest.NewRequest(tc.method, "/anything", nil)
		if tc.contentType != "" {
			r.Header.Set("Content-Type", tc.contentType)
		}
		if got := IsUpload(r); got != tc.want {
			t.Errorf("IsUpload(%s %q) = %v, want %v", tc.method, tc.contentType, got, tc.want)
		}
	}
}
