package postgres

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// RecordAudit writes an audit log entry in its own transaction.
func (r *Repository) RecordAudit(ctx context.Context, e database.AuditEntry) error {
	return database.RecordAudit(ctx, r.db, e)
}
