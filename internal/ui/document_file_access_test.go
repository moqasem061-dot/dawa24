package ui

import (
	"context"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/attachments"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
)

func TestDocumentDownloadConfinesMetadataAndStaffPermissions(t *testing.T) {
	root := t.TempDir()
	uploads := filepath.Join(root, "uploads")
	if err := os.Mkdir(uploads, 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("UPLOAD_DIR", uploads)
	secret := filepath.Join(root, "secret.pdf")
	if err := os.WriteFile(secret, []byte("private-server-secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(uploads, "documents"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(uploads, "documents", "valid.pdf"), []byte("valid-upload"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(uploads, "orgs", "10", "uploads"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(uploads, "orgs", "10", "uploads", "scoped.pdf"), []byte("scoped-upload"), 0600); err != nil {
		t.Fatal(err)
	}
	orgID := int64(10)
	owner := authctx.Actor{UserID: 1, OrgID: orgID}
	for _, tc := range []struct {
		name, key string
		actor     authctx.Actor
		status    int
	}{
		{"valid local document", "documents/valid.pdf", owner, http.StatusOK},
		{"valid scoped document", "orgs/10/uploads/scoped.pdf", owner, http.StatusOK},
		{"scoped key cannot alias legacy file", "orgs/10/uploads/valid.pdf", owner, http.StatusNotFound},
		{"foreign organization key", "orgs/11/uploads/valid.pdf", owner, http.StatusNotFound},
		{"parent traversal", "../secret.pdf", owner, http.StatusNotFound},
		{"absolute file", secret, owner, http.StatusNotFound},
		{"staff without grant", "documents/valid.pdf", authctx.Actor{UserID: 2, IsStaff: true}, http.StatusForbidden},
		{"staff with grant", "documents/valid.pdf", authctx.Actor{UserID: 2, IsStaff: true, Permissions: []string{"hr.document.view"}}, http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newMockAttachmentsRepo()
			repo.docs[1] = &attachments.Document{ID: 1, OrganizationID: &orgID, StorageKey: tc.key, OriginalName: filepath.Base(tc.key)}
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			h := &UIHandler{attSvc: attachments.NewService(repo, nil, logger), log: logger}
			request := httptest.NewRequestWithContext(authctx.WithActor(context.Background(), tc.actor), http.MethodGet, "/documents/download?id=1", nil)
			response := httptest.NewRecorder()
			h.DocumentDownloadHandler(response, request)
			if response.Code != tc.status {
				t.Fatalf("status %d, want %d", response.Code, tc.status)
			}
			if tc.status != http.StatusOK && response.Body.String() == "private-server-secret" {
				t.Fatal("server file disclosed")
			}
		})
	}
}

func TestDocumentStorageKeysRemainInsideNamespace(t *testing.T) {
	orgID := int64(10)
	userID := int64(12)
	document := &attachments.Document{OrganizationID: &orgID, UserID: &userID}
	for _, key := range []string{"../secret.pdf", "orgs/11/documents/private.pdf", "uploads/orgs/11/private.pdf", "uploads/uploads/orgs/11/private.pdf", "orgs/10/../11/private.pdf", "users/13/private.pdf", "C:\\secret.pdf"} {
		if documentStorageKeyAllowed(document, key) {
			t.Errorf("unsafe key accepted: %q", key)
		}
	}
	for _, key := range []string{"documents/legacy.pdf", "orgs/10/documents/current.pdf", "users/12/personal.pdf"} {
		if !documentStorageKeyAllowed(document, key) {
			t.Errorf("valid key refused: %q", key)
		}
	}
}

func TestDocumentUploadRejectsEscapingSymlink(t *testing.T) {
	root := t.TempDir()
	uploads := filepath.Join(root, "uploads")
	if err := os.Mkdir(uploads, 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("UPLOAD_DIR", uploads)
	secret := filepath.Join(root, "secret.pdf")
	if err := os.WriteFile(secret, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(uploads, "alias.pdf")
	if err := os.Symlink(secret, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	file, err := openDocumentUpload(link)
	if err == nil {
		_ = file.Close()
		t.Fatal("opened a symlink outside upload roots")
	}
}

func TestDocumentActiveContentSandboxAndEncodedFilename(t *testing.T) {
	uploads := t.TempDir()
	t.Setenv("UPLOAD_DIR", uploads)
	if err := os.WriteFile(filepath.Join(uploads, "logo.svg"), []byte("<svg/>"), 0600); err != nil {
		t.Fatal(err)
	}
	orgID := int64(10)
	filename := "شعار\".svg"
	repo := newMockAttachmentsRepo()
	repo.docs[1] = &attachments.Document{ID: 1, OrganizationID: &orgID, FileURL: "/uploads/logo.svg", OriginalName: filename}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := &UIHandler{attSvc: attachments.NewService(repo, nil, logger), log: logger}
	request := httptest.NewRequestWithContext(authctx.WithActor(context.Background(), authctx.Actor{UserID: 1, OrgID: orgID}), http.MethodGet, "/documents/download?id=1", nil)
	response := httptest.NewRecorder()
	h.DocumentDownloadHandler(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("download status %d", response.Code)
	}
	if response.Header().Get("Content-Security-Policy") != "default-src 'none'; sandbox" {
		t.Fatal("active document content was not sandboxed")
	}
	disposition, parameters, err := mime.ParseMediaType(response.Header().Get("Content-Disposition"))
	if err != nil || disposition != "attachment" || parameters["filename"] != filename {
		t.Fatalf("filename header was corrupted: %v, %v", parameters, err)
	}
}
