package postgres

import (
	"context"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// AdminListUsersWithTotal returns a paginated slice of users matching the filter along with the total matching count.
func (r *Repository) AdminListUsersWithTotal(ctx context.Context, filter identity.AdminUserFilter, limit, offset int) ([]*identity.User, int, error) {
	var list []*identity.User
	var total int

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		where := []string{"deleted_at IS NULL"}
		var args []any

		if filter.Role != "" {
			rLower := strings.ToLower(filter.Role)
			switch rLower {
			case "customer", "pharmacy", "pharmacies":
				where = append(where, "role IN ('customer', 'pharmacy', 'individual', 'pharmacist', 'buyer', 'pharmacist_assistant')")
			case "vendor", "supplier", "suppliers", "vendors":
				where = append(where, "role IN ('vendor', 'supplier', 'warehouse_keeper', 'sales_rep', 'driver')")
			case "staff", "admin", "admins", "super_admin":
				where = append(where, "role IN ('super_admin', 'admin', 'staff', 'support', 'developer', 'finance', 'auditor', 'employer')")
			case "new":
				where = append(where, "created_at >= NOW() - INTERVAL '30 days'")
			default:
				args = append(args, filter.Role)
				where = append(where, "role = $"+strconv.Itoa(len(args)))
			}
		}

		if filter.Status != "" {
			args = append(args, filter.Status)
			where = append(where, "status = $"+strconv.Itoa(len(args)))
		}

		if filter.Type != "" {
			tLower := strings.ToLower(filter.Type)
			switch tLower {
			case "customer", "pharmacy":
				where = append(where, "role IN ('customer', 'pharmacy', 'individual', 'pharmacist', 'buyer', 'pharmacist_assistant')")
			case "vendor", "supplier":
				where = append(where, "role IN ('vendor', 'supplier', 'warehouse_keeper', 'sales_rep', 'driver')")
			case "staff", "admin":
				where = append(where, "role IN ('super_admin', 'admin', 'staff', 'support', 'developer', 'finance', 'auditor', 'employer')")
			case "new":
				where = append(where, "created_at >= NOW() - INTERVAL '30 days'")
			}
		}

		if s := strings.TrimSpace(filter.Search); s != "" {
			args = append(args, "%"+s+"%")
			p := "$" + strconv.Itoa(len(args))
			where = append(where, "(email ILIKE "+p+" OR phone ILIKE "+p+" OR name->>'ar' ILIKE "+p+" OR name->>'en' ILIKE "+p+")")
		}

		if filter.OrgID > 0 {
			args = append(args, filter.OrgID)
			where = append(where, "id IN (SELECT user_id FROM org.members WHERE organization_id = $"+strconv.Itoa(len(args))+")")
		}

		if filter.MFA != nil {
			if *filter.MFA {
				where = append(where, "EXISTS (SELECT 1 FROM identity.user_mfa mfa WHERE mfa.user_id = identity.users.id AND mfa.enabled = true)")
			} else {
				where = append(where, "NOT EXISTS (SELECT 1 FROM identity.user_mfa mfa WHERE mfa.user_id = identity.users.id AND mfa.enabled = true)")
			}
		}

		if filter.LastLoginFrom != nil {
			args = append(args, *filter.LastLoginFrom)
			where = append(where, "EXISTS (SELECT 1 FROM identity.user_security sec WHERE sec.user_id = identity.users.id AND sec.last_login_at >= $"+strconv.Itoa(len(args))+")")
		}

		if filter.LastLoginTo != nil {
			args = append(args, *filter.LastLoginTo)
			where = append(where, "EXISTS (SELECT 1 FROM identity.user_security sec WHERE sec.user_id = identity.users.id AND sec.last_login_at <= $"+strconv.Itoa(len(args))+")")
		}

		clause := strings.Join(where, " AND ")

		// 1. COUNT over the SAME joins and WHERE clause
		countSQL := "SELECT count(*) FROM identity.users WHERE " + clause + ";"
		if err := tx.QueryRow(txCtx, countSQL, args...).Scan(&total); err != nil {
			return err
		}

		if limit <= 0 || limit > 100 {
			limit = 25
		}
		if offset < 0 {
			offset = 0
		}

		args = append(args, limit, offset)
		limParam := "$" + strconv.Itoa(len(args)-1)
		offParam := "$" + strconv.Itoa(len(args))

		querySQL := `
			SELECT id, public_id, email, name, role, status, language, timezone,
			       phone, COALESCE(avatar_url, ''), email_verified_at, phone_verified_at, created_at, updated_at
			FROM identity.users
			WHERE ` + clause + `
			ORDER BY created_at DESC, id DESC
			LIMIT ` + limParam + ` OFFSET ` + offParam + `;
		`

		rows, err := tx.Query(txCtx, querySQL, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var u identity.User
			var statusStr, langStr string
			if err := rows.Scan(
				&u.ID, &u.PublicID, &u.Email, &u.Name, &u.Role, &statusStr, &langStr,
				&u.Timezone, &u.Phone, &u.AvatarURL, &u.EmailVerifiedAt, &u.PhoneVerifiedAt,
				&u.CreatedAt, &u.UpdatedAt,
			); err != nil {
				return err
			}
			u.Status = identity.UserStatus(statusStr)
			u.Language = i18n.Lang(langStr)
			list = append(list, &u)
		}
		return rows.Err()
	})
	return list, total, err
}

// AdminUserStats returns aggregated user metrics across all roles and statuses.
func (r *Repository) AdminUserStats(ctx context.Context) (identity.AdminUserStatsResult, error) {
	var stats identity.AdminUserStatsResult
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT
				COUNT(*) AS total,
				COUNT(*) FILTER (WHERE role IN ('super_admin', 'admin', 'staff', 'support', 'developer', 'finance', 'auditor', 'employer')) AS staff,
				COUNT(*) FILTER (WHERE role IN ('vendor', 'supplier', 'warehouse_keeper', 'sales_rep', 'driver')) AS vendor,
				COUNT(*) FILTER (WHERE role NOT IN ('super_admin', 'admin', 'staff', 'support', 'developer', 'finance', 'auditor', 'employer', 'vendor', 'supplier', 'warehouse_keeper', 'sales_rep', 'driver')) AS customer,
				COUNT(*) FILTER (WHERE status = 'active') AS active,
				COUNT(*) FILTER (WHERE status = 'suspended') AS suspended
			FROM identity.users
			WHERE deleted_at IS NULL;
		`
		return tx.QueryRow(txCtx, query).Scan(
			&stats.Total,
			&stats.Staff,
			&stats.Vendor,
			&stats.Customer,
			&stats.Active,
			&stats.Suspended,
		)
	})
	return stats, err
}
