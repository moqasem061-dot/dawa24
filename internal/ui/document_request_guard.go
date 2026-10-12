package ui

import (
	"context"
	"strconv"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/modules/attachments"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
)

func (h *UIHandler) validateDocumentRequest(ctx context.Context, actor authctx.Actor, docType attachments.DocumentType, requestID string) error {
	if strings.TrimSpace(requestID) == "" {
		return nil
	}
	id, err := strconv.ParseInt(requestID, 10, 64)
	if err != nil || id <= 0 {
		return apperr.NotFound("document request")
	}
	orgID := actor.OrganizationID
	if orgID <= 0 {
		orgID = actor.OrgID
	}
	requests, err := h.attSvc.ListDocumentRequests(database.AsSystem(ctx), actor, &orgID)
	if err != nil {
		return err
	}
	for _, request := range requests {
		if request.ID == id && request.OrganizationID == orgID && request.DocumentType == docType &&
			(request.Status == attachments.DocReqPending || request.Status == attachments.DocReqSubmitted) {
			return nil
		}
	}
	return apperr.NotFound("document request")
}
