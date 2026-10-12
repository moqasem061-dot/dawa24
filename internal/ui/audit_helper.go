package ui

import (
	"context"

	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// recordAudit emits an audit log entry via idSvc if available.
func (h *UIHandler) recordAudit(ctx context.Context, e database.AuditEntry) {
	if h.idSvc != nil {
		if err := h.idSvc.RecordAudit(ctx, e); err != nil {
			h.log.ErrorContext(ctx, "record activity audit", "action", e.Action, "entity", e.EntityID, "error", err)
		}
	}
}
