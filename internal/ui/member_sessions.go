package ui

import (
	"context"
)

// revokeMemberSessions signs a removed or deactivated member out of the
// organization. Their sessions are ended now, and a later sign-in is checked
// against the membership (Batch 3, S08).
func (h *UIHandler) revokeMemberSessions(ctx context.Context, userID, orgID int64) {
	if h.idSvc == nil || userID <= 0 || orgID <= 0 {
		return
	}
	if err := h.idSvc.RevokeUserOrgSessions(ctx, userID, orgID); err != nil {
		h.log.ErrorContext(ctx, "revoke member sessions", "error", err, "user_id", userID, "org_id", orgID)
	}
}
