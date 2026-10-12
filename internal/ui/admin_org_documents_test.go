package ui

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/moqasem061-dot/dawa24/internal/modules/attachments"
	"github.com/moqasem061-dot/dawa24/internal/modules/org"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
)

type documentOrgRepo struct{ org.Repository }

func (documentOrgRepo) GetOrganizationByID(_ context.Context, id int64) (*org.Organization, error) {
	if id != 20 {
		return nil, apperr.NotFound("organization")
	}
	return &org.Organization{ID: 20, LegalName: "Target supplier", Type: org.TypeVendor}, nil
}

type adminDocumentRepo struct {
	*mockAttachmentsRepo
	created    *attachments.Document
	auditActor int64
	requests   []*attachments.DocumentRequest
}

func (r *adminDocumentRepo) ListRequestsByOrg(_ context.Context, id int64) ([]*attachments.DocumentRequest, error) {
	return r.requests, nil
}
func (r *adminDocumentRepo) Create(ctx context.Context, doc *attachments.Document) (*attachments.Document, error) {
	r.created = doc
	r.auditActor = database.AuditActorIDFrom(ctx)
	doc.ID = 100
	return doc, nil
}

func TestAdminOrganizationDocumentsPermissionsAndTarget(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := &adminDocumentRepo{mockAttachmentsRepo: newMockAttachmentsRepo()}
	h := &UIHandler{orgSvc: org.NewService(documentOrgRepo{}, logger), attSvc: attachments.NewService(repo, nil, logger), log: logger}
	router := chi.NewRouter()
	h.registerAdminOrgRoutes(router)
	for _, tc := range []struct {
		name   string
		actor  authctx.Actor
		status int
	}{
		{"no document permission", authctx.Actor{UserID: 7, IsStaff: true}, http.StatusSeeOther},
		{"tenant with permission", authctx.Actor{UserID: 7, OrganizationID: 20, Permissions: []string{"hr.document.view"}}, http.StatusSeeOther},
		{"staff viewer", authctx.Actor{UserID: 7, IsStaff: true, Permissions: []string{"hr.document.view"}}, http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(authctx.WithActor(context.Background(), tc.actor), http.MethodGet, "/admin/organizations/20/documents", nil)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != tc.status {
				t.Fatalf("status %d, want %d", response.Code, tc.status)
			}
			if tc.status == http.StatusOK && strings.Contains(response.Body.String(), `class="doc-upload-form"`) {
				t.Fatal("viewer offered upload controls")
			}
		})
	}
	t.Setenv("UPLOAD_DIR", t.TempDir())
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("document_type", "commercial_register"); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("organization_id", "99"); err != nil {
		t.Fatal(err)
	}
	file, err := writer.CreateFormFile("file", "license.png")
	if err != nil {
		t.Fatal(err)
	}
	png, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aL9sAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(png); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	actor := authctx.Actor{UserID: 7, IsStaff: true, OrganizationID: 99, OrgID: 99, Permissions: []string{"hr.document.update"}}
	ctx := authctx.WithActor(context.Background(), actor)
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/admin/organizations/20/documents/upload", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if repo.created == nil {
		t.Fatalf("upload failed: %d %s", response.Code, response.Header().Get("Location"))
	}
	if *repo.created.OrganizationID != 20 || *repo.created.UserID != 7 || repo.auditActor != 7 {
		t.Fatalf("wrong attribution: %+v", repo.created)
	}
	if !strings.HasPrefix(response.Header().Get("Location"), "/admin/organizations/20/documents?") {
		t.Fatal("wrong return page")
	}
	if authctx.FromContext(ctx).OrganizationID != 99 {
		t.Fatal("admin session changed")
	}
}

func TestDocumentRequestMustBelongToTargetAndType(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, tc := range []struct {
		name    string
		orgID   int64
		kind    attachments.DocumentType
		status  attachments.DocumentRequestStatus
		allowed bool
	}{
		{"same organization and type", 20, attachments.DocTaxCard, attachments.DocReqPending, true},
		{"other organization", 21, attachments.DocTaxCard, attachments.DocReqPending, false},
		{"other type", 20, attachments.DocPharmacyLicense, attachments.DocReqPending, false},
		{"cancelled request", 20, attachments.DocTaxCard, attachments.DocReqCancelled, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &adminDocumentRepo{mockAttachmentsRepo: newMockAttachmentsRepo(), requests: []*attachments.DocumentRequest{{ID: 1, OrganizationID: tc.orgID, DocumentType: tc.kind, Status: tc.status}}}
			h := &UIHandler{attSvc: attachments.NewService(repo, nil, logger), log: logger}
			actor := authctx.Actor{UserID: 7, OrganizationID: 20, OrgID: 20}
			err := h.validateDocumentRequest(context.Background(), actor, attachments.DocTaxCard, "1")
			if (err == nil) != tc.allowed {
				t.Fatalf("allowed=%t, error=%v", tc.allowed, err)
			}
		})
	}
}
