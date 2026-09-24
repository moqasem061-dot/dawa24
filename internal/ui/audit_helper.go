package ui

import (
	"context"

	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// recordAudit emits an audit log entry via idSvc if available.
func (h *UIHandler) recordAudit(ctx context.Context, e database.AuditEntry) {
	if h.idSvc != nil {
		_ = h.idSvc.RecordAudit(ctx, e)
	}
}
