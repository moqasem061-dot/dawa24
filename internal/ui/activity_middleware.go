package ui

import (
	"context"
	"net/http"

	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

func (h *UIHandler) ActivityMiddleware(next http.Handler) http.Handler {
	var record func(context.Context, database.AuditEntry) error
	if h.idSvc != nil {
		record = h.idSvc.RecordAudit
	}
	return authctx.ActivityAudit(record, h.trustedProxyHops, h.log)(next)
}
