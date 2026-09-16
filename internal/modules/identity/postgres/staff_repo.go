package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// ListStaffUserIDs returns the IDs of active platform staff — the audience for
// operational notifications such as new registrations awaiting review.
//
// Staff here means exactly what the login gate means: the user's platform role
// row carries is_staff (see loadPlatformSide in internal/platform/rbac). A
// hardcoded role-name list would silently miss a role an operator invents
// later, or keep notifying a role whose staff flag was revoked.
func (r *Repository) ListStaffUserIDs(ctx context.Context) ([]int64, error) {
	var ids []int64
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, `
			SELECT u.id
			FROM identity.users u
			LEFT JOIN identity.roles ro ON ro.key = u.role AND ro.deleted_at IS NULL
			WHERE u.deleted_at IS NULL
			  AND u.status = 'active'
			  AND (COALESCE(ro.is_staff, false) = true OR u.role IN ('admin', 'superadmin', 'platform_admin', 'support_agent', 'staff'))
			ORDER BY u.id ASC;
		`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				return err
			}
			ids = append(ids, id)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return ids, nil
}
