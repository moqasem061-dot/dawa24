package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

func writeAudit(ctx context.Context, tx pgx.Tx, orgID int64, a erp.AuditRecord) error {
	return database.WriteAudit(ctx, tx, database.AuditEntry{
		OrganizationID: &orgID, ActorUserID: a.ActorUserID, ActorKind: a.ActorKind, ActorKeyID: a.ActorKeyID,
		Action: a.Action, EntityType: a.EntityType, EntityID: a.EntityID, Before: a.Before, After: a.After,
	})
}
