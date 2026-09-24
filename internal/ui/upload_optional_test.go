package ui

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http/httptest"
	"strings"
	"testing"
)

// A form with no file is fine; the field is optional.
func TestOptionalUploadAcceptsNoFile(t *testing.T) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = mw.WriteField("amount", "100")
	_ = mw.Close()
	r := httptest.NewRequestWithContext(context.Background(), "POST", "/", &body)
	r.Header.Set("Content-Type", mw.FormDataContentType())

	path, err := optionalUpload(r, "receipt", "receipts")
	if err != nil || path != "" {
		t.Fatalf("no file: got (%q, %v), want (\"\", nil)", path, err)
	}
}

// A plain url-encoded post cannot carry a file, so it has none to save.
func TestOptionalUploadAcceptsAURLEncodedForm(t *testing.T) {
	r := httptest.NewRequestWithContext(context.Background(), "POST", "/", strings.NewReader("amount=100"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	path, err := optionalUpload(r, "receipt", "receipts")
	if err != nil || path != "" {
		t.Fatalf("url-encoded form: got (%q, %v), want (\"\", nil)", path, err)
	}
}

// A request whose body cannot be read as the form it claims to be is not a
// missing file, and must not be treated as one.
func TestOptionalUploadReportsABrokenUpload(t *testing.T) {
	r := httptest.NewRequestWithContext(context.Background(), "POST", "/", strings.NewReader("--x\r\nnot a valid part"))
	r.Header.Set("Content-Type", "multipart/form-data; boundary=x")

	if _, err := optionalUpload(r, "receipt", "receipts"); err == nil {
		t.Fatal("a malformed upload was treated as no file")
	}
}
