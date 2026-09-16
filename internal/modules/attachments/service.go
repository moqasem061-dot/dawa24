package attachments

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/storage"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// Service provides high-level business operations for attachments and storage presigning.
type Service struct {
	repo    Repository
	storage *storage.Client
	log     *slog.Logger
}

// NewService creates a new attachments Service.
func NewService(repo Repository, storage *storage.Client, log *slog.Logger) *Service {
	return &Service{
		repo:    repo,
		storage: storage,
		log:     log,
	}
}

// MissingRequiredDocuments returns missing document indicator if the
// organization has no verified copy of any document, or nil when the org can trade.
// An organization requires at least one active/approved document to trade.
func (s *Service) MissingRequiredDocuments(ctx context.Context, orgID int64, orgType string) ([]DocumentType, error) {
	if orgID <= 0 {
		return nil, apperr.Unauthorized()
	}

	docs, err := s.repo.ListByOrganization(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("attachments.MissingRequiredDocuments: %w", err)
	}

	verifiedCount := 0
	for _, d := range docs {
		if d != nil && d.Status == StatusVerified && d.DeletedAt == nil {
			verifiedCount++
		}
	}

	if verifiedCount == 0 {
		return []DocumentType{DocCommercialRegister}, nil
	}
	return nil, nil
}

// RegisterUpload records a document that was already uploaded through the
// local /uploads flow (Rebuild V2 §4.2) — the organization-owned files screen
// uses this instead of presigned storage when object storage is absent.
// Unlike PresignUpload it needs no storage client: the uploader hands over
// the final public URL.
func (s *Service) RegisterUpload(ctx context.Context, actor authctx.Actor, docType DocumentType, url, originalName string) (*Document, error) {
	orgID := actor.OrganizationID
	if orgID <= 0 {
		orgID = actor.OrgID
	}
	if orgID <= 0 && !actor.IsPlatformAdmin() {
		return nil, apperr.Unauthorized()
	}

	validMimes, ok := allowedMIMEs[docType]
	if !ok {
		return nil, apperr.Validation("document.type_invalid", i18n.T("ar", "err.doc_type_invalid"), map[string]string{"document_type": i18n.T("ar", "err.doc_type_invalid")})
	}

	ext := strings.ToLower(filepath.Ext(originalName))
	mime := "application/octet-stream"
	switch ext {
	case "", ".pdf":
		mime = "application/pdf"
	case ".png":
		mime = "image/png"
	case ".jpg", ".jpeg":
		mime = "image/jpeg"
	case ".webp":
		mime = "image/webp"
	}
	mimeMatched := false
	for _, m := range validMimes {
		if m == mime {
			mimeMatched = true
			break
		}
	}
	if !mimeMatched {
		return nil, apperr.Validation("document.mime_unsupported", i18n.T("ar", "err.doc_mime_unsupported"), map[string]string{"mime_type": i18n.T("ar", "err.doc_mime_unsupported")})
	}

	url = strings.TrimSpace(url)
	if url == "" {
		return nil, apperr.Validation("document.file_required", i18n.T("ar", "err.doc_file_required"), nil)
	}

	var orgIDPtr *int64
	if orgID > 0 {
		orgIDPtr = &orgID
	}

	var userIDPtr *int64
	if actor.UserID > 0 {
		v := actor.UserID
		userIDPtr = &v
	}

	title := strings.TrimSpace(originalName)
	if title == "" {
		title = string(docType)
	}

	doc := &Document{
		OrganizationID: orgIDPtr,
		UserID:         userIDPtr,
		DocumentType:   docType,
		FileURL:        url,
		Title:          title,
		StorageKey:     url,
		OriginalName:   strings.TrimSpace(originalName),
		MimeType:       mime,
		Status:         StatusPending,
		Meta: map[string]interface{}{
			"uploader_user_id": actor.UserID,
		},
	}

	created, err := s.repo.Create(ctx, doc)
	if err != nil {
		return nil, fmt.Errorf("attachments.RegisterUpload: %w", err)
	}
	return created, nil
}

// PresignUpload validates file parameters, reserves a pending document row, and returns a secure presigned PUT URL.
func (s *Service) PresignUpload(ctx context.Context, actor authctx.Actor, req PresignRequest) (*PresignResult, error) {
	if err := ValidatePresignRequest(req); err != nil {
		return nil, err
	}

	var orgID *int64
	if req.OrganizationID != nil && *req.OrganizationID > 0 {
		// Verify actor belongs to this org or is platform admin
		if !actor.IsPlatformAdmin() && actor.OrgID != *req.OrganizationID {
			return nil, apperr.Forbidden("document.unauthorized_org", i18n.T("ar", "err.doc_unauthorized_org"))
		}
		orgID = req.OrganizationID
	} else if actor.OrgID > 0 {
		v := actor.OrgID
		orgID = &v
	}

	var userID *int64
	if actor.UserID > 0 {
		v := actor.UserID
		userID = &v
	}

	if orgID == nil && userID == nil {
		return nil, apperr.Unauthorized()
	}

	storageKey := GenerateStorageKey(req.DocumentType, orgID, userID, req.OriginalName)

	doc := &Document{
		OrganizationID: orgID,
		UserID:         userID,
		DocumentType:   req.DocumentType,
		FileURL:        storageKey,
		Title:          presignTitle(req),
		StorageKey:     storageKey,
		OriginalName:   req.OriginalName,
		MimeType:       req.MimeType,
		SizeBytes:      req.SizeBytes,
		Status:         StatusPending,
		Meta: map[string]interface{}{
			"uploader_user_id": actor.UserID,
		},
	}

	created, err := s.repo.Create(ctx, doc)
	if err != nil {
		return nil, fmt.Errorf("attachments.PresignUpload: %w", err)
	}

	uploadURL := ""
	expiresAt := time.Now().Add(15 * time.Minute)
	if s.storage != nil {
		url, err := s.storage.PresignPut(ctx, storageKey, req.MimeType, 15*time.Minute)
		if err != nil {
			s.log.Error("failed to generate presigned PUT url", "error", err, "key", storageKey)
			_ = s.repo.HardDelete(ctx, created.ID)
			return nil, fmt.Errorf("storage presign error: %w", err)
		}
		uploadURL = url
	} else {
		// Local mock fallback for development without external S3
		uploadURL = fmt.Sprintf("/api/v1/attachments/%d/upload-mock", created.ID)
	}

	return &PresignResult{
		DocumentID: created.ID,
		PublicID:   created.PublicID,
		UploadURL:  uploadURL,
		ExpiresAt:  expiresAt,
		StorageKey: storageKey,
	}, nil
}

// ConfirmUpload verifies with MinIO/S3 that the object actually exists, records real file size, and marks status.
func (s *Service) ConfirmUpload(ctx context.Context, actor authctx.Actor, id int64) (*Document, error) {
	doc, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// Verify actor ownership
	if !actor.IsPlatformAdmin() {
		if doc.UserID != nil && *doc.UserID != actor.UserID && (doc.OrganizationID == nil || *doc.OrganizationID != actor.OrgID) {
			return nil, apperr.Forbidden("document.access_denied", i18n.TDefault("w4_mod.s_233_233"))
		}
	}

	if s.storage != nil {
		size, _, err := s.storage.HeadObject(ctx, doc.FileURL)
		if err != nil {
			s.log.Warn("confirm upload failed: object not found in storage", "id", id, "key", doc.FileURL, "error", err)
			_ = s.repo.HardDelete(ctx, id)
			return nil, apperr.Validation("document.not_in_storage", i18n.TDefault("w4_mod.s_234_234"), map[string]string{"upload": i18n.TDefault("w4_mod.s_235_235")})
		}
		doc.SizeBytes = size
	}

	return doc, nil
}

// GetDocumentByID retrieves a document ensuring the actor has access permissions.
func (s *Service) GetDocumentByID(ctx context.Context, actor authctx.Actor, id int64) (*Document, error) {
	doc, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if !actor.IsPlatformAdmin() {
		orgID := actor.OrganizationID
		if orgID <= 0 {
			orgID = actor.OrgID
		}
		hasAccess := false
		if doc.OrganizationID != nil && orgID > 0 && *doc.OrganizationID == orgID {
			hasAccess = true
		}
		if doc.UserID != nil && actor.UserID > 0 && *doc.UserID == actor.UserID {
			hasAccess = true
		}
		if !hasAccess {
			return nil, apperr.Forbidden("document.access_denied", i18n.TDefault("w4_mod.s_236_236"))
		}
	}
	return doc, nil
}

// GetByIDAdmin fetches a document by ID directly for platform administrative tasks.
func (s *Service) GetByIDAdmin(ctx context.Context, id int64) (*Document, error) {
	return s.repo.GetByID(ctx, id)
}

// GetDownloadURL generates a secure URL for viewing or downloading the document.
func (s *Service) GetDownloadURL(ctx context.Context, actor authctx.Actor, id int64) (string, error) {
	doc, err := s.GetDocumentByID(ctx, actor, id)
	if err != nil {
		if actor.IsPlatformAdmin() {
			doc, err = s.repo.GetByID(ctx, id)
		}
		if err != nil || doc == nil {
			return "", err
		}
	}

	fileURL := strings.TrimSpace(doc.FileURL)
	if fileURL == "" {
		return "", apperr.NotFound("document.file_empty")
	}

	// If it's a local /uploads/ path for a private document, route through the authenticated viewer.
	if strings.HasPrefix(fileURL, "/uploads/") {
		return fmt.Sprintf("/documents/%d/view", doc.ID), nil
	}

	// If it's already an external HTTP(S) URL or viewer route, return it directly
	if strings.HasPrefix(fileURL, "http://") || strings.HasPrefix(fileURL, "https://") || strings.HasPrefix(fileURL, "/documents/") {
		return fileURL, nil
	}

	if s.storage != nil {
		presigned, presignErr := s.storage.PresignGet(ctx, fileURL, 60*time.Minute)
		if presignErr == nil && presigned != "" {
			return presigned, nil
		}
		if s.log != nil {
			s.log.WarnContext(ctx, "storage presign get failed", "id", id, "key", fileURL, "error", presignErr)
		}
	}

	return fileURL, nil
}

// VerifyDocument allows platform admins to approve or reject submitted certificates and licenses.
func (s *Service) VerifyDocument(ctx context.Context, actor authctx.Actor, id int64, status DocumentStatus, notes string) error {
	if !actor.IsPlatformAdmin() {
		return apperr.Forbidden("document.admin_required", i18n.TDefault("w4_mod.s_237_237"))
	}

	var reviewerID *int64
	if actor.UserID > 0 {
		v := actor.UserID
		reviewerID = &v
	}

	if err := s.repo.UpdateStatus(ctx, id, status, notes, reviewerID); err != nil {
		return err
	}

	if status == StatusVerified {
		if doc, err := s.repo.GetByID(ctx, id); err == nil && doc != nil && doc.OrganizationID != nil {
			_ = s.repo.FulfillRequestByDoc(ctx, *doc.OrganizationID, doc.DocumentType, doc.ID)
		}
	}
	return nil
}
