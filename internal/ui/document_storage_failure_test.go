package ui

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/platform/config"
	"github.com/moqasem061-dot/dawa24/internal/platform/storage"
)

func TestOfficialDocumentRejectsFailedPersistentStorage(t *testing.T) {
	t.Setenv("UPLOAD_DIR", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("<Error><Code>AccessDenied</Code></Error>"))
	}))
	defer server.Close()
	client, err := storage.New(context.Background(), config.Storage{
		Endpoint: server.URL, Region: "test", Bucket: "documents",
		AccessKeyID: "test", SecretAccessKey: "test", UsePathStyle: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	previous := uploadStorageClient
	uploadStorageClient = client
	t.Cleanup(func() { uploadStorageClient = previous })
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	file, err := writer.CreateFormFile("file", "license.pdf")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = file.Write([]byte("%PDF-1.4\n%%EOF"))
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/documents/upload", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	meta, err := saveUploadedFileFull(req, "file", "documents")
	if err == nil || !strings.Contains(err.Error(), "persist official document") || meta.URL != "" {
		t.Fatalf("failed durable upload reported success: URL=%q error=%v", meta.URL, err)
	}
}
