package erp

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// AuditTxHook returns the writer of one audit record for organization orgID. It
// has the shape of commerce.ShipmentTxHook, so a decider can run it inside the
// transaction that changes the order: the audit row and the change commit or
// roll back together (trap 15, research D).
func AuditTxHook(orgID int64, a AuditRecord) func(context.Context, pgx.Tx) error {
	return func(ctx context.Context, tx pgx.Tx) error {
		return database.WriteAudit(ctx, tx, database.AuditEntry{
			OrganizationID: &orgID, ActorUserID: a.ActorUserID, ActorKind: a.ActorKind, ActorKeyID: a.ActorKeyID,
			Action: a.Action, EntityType: a.EntityType, EntityID: a.EntityID, Before: a.Before, After: a.After,
		})
	}
}
