package http

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
)

// The body reader guards everything the sync posts: a 32 MiB ceiling, gzip
// inflation with its own 128 MiB ceiling (a decompression bomb's natural
// home), the JSON content type, and parse errors carrying a position.

func readBodyOf(t *testing.T, body []byte, contentType, encoding string) (int, string, string) {
	t.Helper()
	req := httptest.NewRequest("POST", "/products/sync", bytes.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if encoding != "" {
		req.Header.Set("Content-Encoding", encoding)
	}
	w := httptest.NewRecorder()
	data, err := readBody(w, req)
	return w.Code, string(data), errText(err)
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func TestBodyTooLargeIsRefused(t *testing.T) {
	huge := make([]byte, erp.MaxBodyBytes+16)
	code, _, text := readBodyOf(t, huge, "application/json", "")
	if code != 413 {
		t.Fatalf("status = %d, want 413", code)
	}
	if text == "" {
		t.Error("expected the refusal sentinel")
	}
}

func TestGzipBodyInflates(t *testing.T) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write([]byte(`{"items":[]}`)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	code, data, text := readBodyOf(t, buf.Bytes(), "application/json", "gzip")
	if code != 200 || text != "" {
		t.Fatalf("gzipped body refused: %d (%q)", code, text)
	}
	if !strings.Contains(data, `"items":[]`) {
		t.Errorf("inflated = %q", data)
	}
}

func TestGzipBombRefused(t *testing.T) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := io.Copy(zw, io.LimitReader(spaces{}, erp.MaxInflatedBytes+1024)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	code, _, text := readBodyOf(t, buf.Bytes(), "application/json", "gzip")
	if code != 413 {
		t.Fatalf("status = %d, want 413 (err %q)", code, text)
	}
	if !strings.Contains(text, "ERP_BODY_TOO_LARGE") {
		t.Errorf("err = %q", text)
	}
}

func TestWrongContentType(t *testing.T) {
	code, _, text := readBodyOf(t, []byte(`{"items":[]}`), "text/plain", "")
	if code != 415 {
		t.Fatalf("status = %d, want 415", code)
	}
	if !strings.Contains(text, "ERP_UNSUPPORTED_MEDIA") {
		t.Errorf("err = %q", text)
	}
}

func TestBrokenJSONCarriesPosition(t *testing.T) { // T127 wording; decode position handled by decode
	code, _, text := readBodyOf(t, []byte(`{"items": [`), "application/json", "")
	// readBody does not parse JSON: a broken body passes through untouched
	// (no envelope written, recorder keeps its 200 default) and the decoder
	// reports the position.
	if code != 200 || text != "" {
		t.Fatalf("readBody should pass short bodies through, status %d (%s)", code, text)
	}
}

// spaces is an endless stream of ASCII spaces, the cheapest gzip bomb.
type spaces struct{}

func (spaces) Read(b []byte) (int, error) {
	for i := range b {
		b[i] = ' '
	}
	return len(b), nil
}
