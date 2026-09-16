package postgres

import (
	"context"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// The moderator hierarchy: who reports to whom, and who may see whose uploads.
//
// Every query here is the authority for an access decision, not a convenience
// for a sidebar. ModeratorSubordinateIDs in particular is what
// /admin/team/temparte-warehouses scopes its listing by, and what each of its
// per-file actions re-checks — a permission says a moderator may manage their
// team's warehouses, and only this says whose those are.

// ListModerators returns the platform's moderators with their parent, for the
// super admin's assignment screen.
func (r *Repository) ListModerators(ctx context.Context) ([]*identity.Moderator, error) {
	var out []*identity.Moderator
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, `
			SELECT u.id, u.name, u.email, u.role, u.moderator_parent_id,
			       p.name, COALESCE(kids.n, 0)
			FROM identity.users u
			LEFT JOIN identity.users p ON p.id = u.moderator_parent_id AND p.deleted_at IS NULL
			LEFT JOIN LATERAL (
			    SELECT count(*) AS n FROM identity.users c
			    WHERE c.moderator_parent_id = u.id AND c.deleted_at IS NULL
			) kids ON TRUE
			WHERE u.deleted_at IS NULL AND u.role = $1
			ORDER BY (u.moderator_parent_id IS NOT NULL), u.id;`, identity.RoleModerator)
		if err != nil {
			return fmt.Errorf("identity postgres: list moderators: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var m identity.Moderator
			var parentName *i18n.Text
			if err := rows.Scan(&m.UserID, &m.Name, &m.Email, &m.Role,
				&m.ParentID, &parentName, &m.SubordinateCount); err != nil {
				return err
			}
			if parentName != nil {
				m.ParentName = *parentName
			}
			out = append(out, &m)
		}
		return rows.Err()
	})
	return out, err
}

// ModeratorSubordinateIDs lists the moderators reporting directly to one user.
//
// Direct reports only, and that is the whole of the hierarchy: migration 168
// explains why the tree is one level deep. A recursive walk would need a cycle
// guard, and a cycle in an access-control graph is a request that never
// returns.
func (r *Repository) ModeratorSubordinateIDs(ctx context.Context, parentID int64) ([]int64, error) {
	if parentID <= 0 {
		return nil, nil
	}
	var ids []int64
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, `
			SELECT id FROM identity.users
			WHERE moderator_parent_id = $1 AND deleted_at IS NULL
			ORDER BY id;`, parentID)
		if err != nil {
			return fmt.Errorf("identity postgres: list subordinates: %w", err)
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
	return ids, err
}

// ModeratorParentID returns whose team a moderator belongs to, or nil.
func (r *Repository) ModeratorParentID(ctx context.Context, userID int64) (*int64, error) {
	var parent *int64
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		err := tx.QueryRow(txCtx, `
			SELECT moderator_parent_id FROM identity.users
			WHERE id = $1 AND deleted_at IS NULL;`, userID).Scan(&parent)
		if database.IsNotFound(err) {
			return apperr.NotFound("user")
		}
		return err
	})
	return parent, err
}

// SetModeratorParent assigns a moderator to a parent, or promotes them to
// top-level with a nil parent.
//
// Three things are refused, and each of them is a way the tree could stop being
// a tree:
//
//   - parenting a user to themselves (the database also refuses it);
//   - parenting someone who already has moderators under them, which would make
//     the tree two levels deep in a way nothing below is written to walk;
//   - parenting to a user who is not a moderator, which would put warehouses
//     under someone with no screen to see them on.
func (r *Repository) SetModeratorParent(ctx context.Context, userID int64, parentID *int64, actorID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var before *int64
		var role string
		err := tx.QueryRow(txCtx, `
			SELECT moderator_parent_id, role FROM identity.users
			WHERE id = $1 AND deleted_at IS NULL;`, userID).Scan(&before, &role)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("user")
			}
			return fmt.Errorf("identity postgres: read moderator parent: %w", err)
		}
		if role != identity.RoleModerator {
			return apperr.Validation("identity.moderator.not_moderator",
				"لا يمكن تعيين مشرف رئيسي لمستخدم ليس دوره «مشرف».", nil)
		}

		if parentID != nil {
			if *parentID == userID {
				return apperr.Validation("identity.moderator.self_parent",
					"لا يمكن أن يتبع المشرف نفسه.", nil)
			}
			var parentRole string
			err := tx.QueryRow(txCtx, `
				SELECT role FROM identity.users
				WHERE id = $1 AND deleted_at IS NULL;`, *parentID).Scan(&parentRole)
			if err != nil {
				if database.IsNotFound(err) {
					return apperr.NotFound("user")
				}
				return fmt.Errorf("identity postgres: read parent role: %w", err)
			}
			if parentRole != identity.RoleModerator {
				return apperr.Validation("identity.moderator.parent_not_moderator",
					"المشرف الرئيسي يجب أن يكون مستخدماً دوره «مشرف».", nil)
			}

			var parentOfParent *int64
			if err := tx.QueryRow(txCtx, `
				SELECT moderator_parent_id FROM identity.users WHERE id = $1;`,
				*parentID).Scan(&parentOfParent); err != nil {
				return fmt.Errorf("identity postgres: read parent chain: %w", err)
			}
			if parentOfParent != nil {
				return apperr.Validation("identity.moderator.depth",
					"المشرف المختار يتبع مشرفاً آخر بالفعل. التسلسل مستوى واحد فقط: اختر مشرفاً رئيسياً مستقلاً.", nil)
			}

			var children int
			if err := tx.QueryRow(txCtx, `
				SELECT count(*) FROM identity.users
				WHERE moderator_parent_id = $1 AND deleted_at IS NULL;`,
				userID).Scan(&children); err != nil {
				return fmt.Errorf("identity postgres: count subordinates: %w", err)
			}
			if children > 0 {
				return apperr.Validation("identity.moderator.has_children",
					"هذا المشرف يدير مشرفين آخرين، فلا يمكن أن يتبع مشرفاً. انقل من يتبعونه أولاً.", nil)
			}
		}

		if _, err := tx.Exec(txCtx, `
			UPDATE identity.users SET moderator_parent_id = $1, updated_at = now()
			WHERE id = $2 AND deleted_at IS NULL;`, parentID, userID); err != nil {
			return fmt.Errorf("identity postgres: set moderator parent: %w", err)
		}

		return database.WriteAudit(txCtx, tx, database.AuditEntry{
			ActorUserID: actorID,
			Action:      "identity.moderator.parent_assigned",
			EntityType:  "identity.user",
			EntityID:    strconv.FormatInt(userID, 10),
			Before:      map[string]any{"moderator_parent_id": before},
			After:       map[string]any{"moderator_parent_id": parentID},
		})
	})
}
