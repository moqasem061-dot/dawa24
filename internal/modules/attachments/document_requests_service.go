package attachments

import (
	"context"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// VerifyDocumentWithType allows platform admins to assign category and approve or reject submitted documents.
func (s *Service) VerifyDocumentWithType(ctx context.Context, actor authctx.Actor, id int64, docType DocumentType, status DocumentStatus, notes string) error {
	if !actor.IsPlatformAdmin() {
		return apperr.Forbidden("document.admin_required", i18n.TDefault("w4_mod.s_237_237"))
	}

	var reviewerID *int64
	if actor.UserID > 0 {
		v := actor.UserID
		reviewerID = &v
	}

	var err error
	if docType != "" {
		err = s.repo.UpdateTypeAndStatus(ctx, id, docType, status, notes, reviewerID)
	} else {
		err = s.repo.UpdateStatus(ctx, id, status, notes, reviewerID)
	}
	if err != nil {
		return err
	}

	if status == StatusVerified {
		if doc, err := s.repo.GetByID(ctx, id); err == nil && doc != nil && doc.OrganizationID != nil {
			actualType := docType
			if actualType == "" {
				actualType = doc.DocumentType
			}
			_ = s.repo.FulfillRequestByDoc(ctx, *doc.OrganizationID, actualType, doc.ID)
		}
	}
	return nil
}

// CreateDocumentRequest issues an official document request from platform admin to an organization.
func (s *Service) CreateDocumentRequest(ctx context.Context, actor authctx.Actor, orgID int64, docType DocumentType, title, description string, deadlineDays int) (*DocumentRequest, error) {
	if !actor.IsPlatformAdmin() {
		return nil, apperr.Forbidden("document.admin_required", i18n.TDefault("w4_mod.s_238_238"))
	}
	if orgID <= 0 {
		return nil, apperr.Validation("org_id.required", i18n.TDefault("w4_mod.s_239_239"), nil)
	}
	if strings.TrimSpace(title) == "" {
		return nil, apperr.Validation("title.required", i18n.TDefault("w4_mod.s_240_240"), nil)
	}
	if deadlineDays <= 0 {
		deadlineDays = 30
	}

	var reqBy *int64
	if actor.UserID > 0 {
		v := actor.UserID
		reqBy = &v
	}

	req := &DocumentRequest{
		OrganizationID: orgID,
		RequestedBy:    reqBy,
		DocumentType:   docType,
		Title:          strings.TrimSpace(title),
		Description:    strings.TrimSpace(description),
		DeadlineAt:     time.Now().Add(time.Duration(deadlineDays) * 24 * time.Hour),
		Status:         DocReqPending,
	}

	return s.repo.CreateDocumentRequest(ctx, req)
}

// ListDocumentRequests returns administrative document requests for a specific org or across the platform.
func (s *Service) ListDocumentRequests(ctx context.Context, actor authctx.Actor, orgID *int64) ([]*DocumentRequest, error) {
	if orgID != nil && *orgID > 0 {
		if !actor.IsPlatformAdmin() && actor.OrgID != *orgID {
			return nil, apperr.Forbidden("document.access_denied", i18n.TDefault("w4_mod.s_241_241"))
		}
		return s.repo.ListRequestsByOrg(ctx, *orgID)
	}

	if !actor.IsPlatformAdmin() {
		return nil, apperr.Forbidden("document.admin_required", i18n.TDefault("w4_mod.s_242_242"))
	}
	return s.repo.ListAllRequests(ctx)
}

// CancelDocumentRequest cancels an administrative document request.
func (s *Service) CancelDocumentRequest(ctx context.Context, actor authctx.Actor, reqID int64) error {
	if !actor.IsPlatformAdmin() {
		return apperr.Forbidden("document.admin_required", i18n.TDefault("w4_mod.s_242_242"))
	}
	return s.repo.UpdateRequestStatus(ctx, reqID, DocReqCancelled, nil)
}

// SubmitDocumentForRequest marks a document request as submitted with the newly uploaded document ID.
func (s *Service) SubmitDocumentForRequest(ctx context.Context, reqID int64, docID int64) error {
	return s.repo.UpdateRequestStatus(ctx, reqID, DocReqSubmitted, &docID)
}

// Delete soft-deletes a document record.
func (s *Service) Delete(ctx context.Context, actor authctx.Actor, id int64) error {
	doc, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if !actor.IsPlatformAdmin() {
		if doc.UserID != nil && *doc.UserID != actor.UserID && (doc.OrganizationID == nil || *doc.OrganizationID != actor.OrgID) {
			return apperr.Forbidden("document.access_denied", i18n.TDefault("w4_mod.s_243_243"))
		}
	}

	return s.repo.SoftDelete(ctx, id)
}

// ListByOrganization returns all documents belonging to an organization.
func (s *Service) ListByOrganization(ctx context.Context, orgID int64) ([]*Document, error) {
	return s.repo.ListByOrganization(ctx, orgID)
}

// ListByUser returns all documents uploaded by a user.
func (s *Service) ListByUser(ctx context.Context, userID int64) ([]*Document, error) {
	return s.repo.ListByUser(ctx, userID)
}

// ListAll returns documents matching administrative search criteria.
func (s *Service) ListAll(ctx context.Context, filter DocumentFilter) ([]*Document, int, error) {
	return s.repo.ListAll(ctx, filter)
}

// presignTitle derives the human-readable title stored alongside the document
// row; platform_admin.documents.title is NOT NULL with no default.
func presignTitle(req PresignRequest) string {
	title := strings.TrimSpace(req.OriginalName)
	if title == "" {
		title = string(req.DocumentType)
	}
	return title
}
