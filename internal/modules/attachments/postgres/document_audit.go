package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/moqasem061-dot/dawa24/internal/modules/attachments"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
)

func documentChangeAudit(ctx context.Context, tx pgx.Tx, id int64) (database.AuditEntry, error) {
	entry := database.AuditEntry{Action: "document.updated", EntityType: "document", EntityID: fmt.Sprint(id)}
	var docType, status, notes string
	err := tx.QueryRow(ctx, `SELECT organization_id, document_type, status, review_notes FROM platform_admin.documents WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, id).Scan(&entry.OrganizationID, &docType, &status, &notes)
	if database.IsNotFound(err) {
		return entry, apperr.NotFound("document")
	}
	entry.Before = map[string]any{"document_type": docType, "status": status, "review_notes": notes}
	return entry, err
}

func documentUploaderID(doc *attachments.Document) int64 {
	if doc.UserID != nil {
		return *doc.UserID
	}
	return 0
}
