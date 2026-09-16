package postgres

import (
	"context"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// ListAccountDeletionRequests lists all deletion requests with user info.
func (r *Repository) ListAccountDeletionRequests(ctx context.Context, status string) ([]*identity.AccountDeletionRequest, error) {
	list, _, err := r.ListAccountDeletionRequestsWithTotal(ctx, status, 100, 0)
	return list, err
}

// ListAccountDeletionRequestsWithTotal lists paginated deletion requests with total count.
func (r *Repository) ListAccountDeletionRequestsWithTotal(ctx context.Context, status string, limit, offset int) ([]*identity.AccountDeletionRequest, int, error) {
	var list []*identity.AccountDeletionRequest
	var total int
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var statusPtr *string
		if status != "" {
			statusPtr = &status
		}
		countQ := `SELECT count(*) FROM identity.account_deletion_requests WHERE ($1::text IS NULL OR status = $1);`
		if err := tx.QueryRow(txCtx, countQ, statusPtr).Scan(&total); err != nil {
			return err
		}

		const query = `
			SELECT r.id, r.user_id, COALESCE(u.name->>'ar', u.name->>'en', ''), COALESCE(u.email, ''), COALESCE(u.role, ''),
			       r.organization_id, COALESCE(o.name->>'ar', o.name->>'en', ''), r.reason, r.status,
			       r.admin_notes, r.reviewed_by, COALESCE(r.reviewed_at, r.resolved_at) AS reviewed_at, r.created_at, r.updated_at
			FROM identity.account_deletion_requests r
			JOIN identity.users u ON u.id = r.user_id
			LEFT JOIN org.organizations o ON o.id = r.organization_id
			WHERE ($1::text IS NULL OR r.status = $1)
			ORDER BY r.created_at DESC, r.id DESC
			LIMIT $2 OFFSET $3;
		`
		rows, err := tx.Query(txCtx, query, statusPtr, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var item identity.AccountDeletionRequest
			if err := rows.Scan(
				&item.ID, &item.UserID, &item.UserName, &item.UserEmail, &item.UserRole,
				&item.OrganizationID, &item.OrganizationName, &item.Reason, &item.Status,
				&item.AdminNotes, &item.ReviewedBy, &item.ReviewedAt, &item.CreatedAt, &item.UpdatedAt,
			); err != nil {
				return err
			}
			list = append(list, &item)
		}
		return rows.Err()
	})
	return list, total, err
}

// ReviewAccountDeletionRequest approves or rejects an account deletion request.
func (r *Repository) ReviewAccountDeletionRequest(ctx context.Context, requestID, reviewerID int64, approve bool, adminNotes string) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var userID int64
		var currentStatus string
		if err := tx.QueryRow(txCtx,
			`SELECT user_id, status FROM identity.account_deletion_requests WHERE id = $1;`, requestID,
		).Scan(&userID, &currentStatus); err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("account_deletion_request")
			}
			return err
		}

		newStatus := "rejected"
		if approve {
			newStatus = "approved"
			_, err := tx.Exec(txCtx,
				`UPDATE identity.users SET status = 'deleted', deleted_at = now(), updated_at = now() WHERE id = $1;`, userID)
			if err != nil {
				return err
			}

			_, err = tx.Exec(txCtx,
				`UPDATE identity.user_sessions SET is_active = false, logged_out_at = now() WHERE user_id = $1 AND is_active = true;`, userID)
			if err != nil {
				return err
			}
			if err := rbac.BumpVersion(txCtx, tx, rbac.PlatformVersionKey); err != nil {
				return err
			}
		}

		_, err := tx.Exec(txCtx, `
			UPDATE identity.account_deletion_requests
			SET status = $1, admin_notes = $2, reviewed_by = $3, reviewed_at = now(), resolved_at = now(), updated_at = now()
			WHERE id = $4;
		`, newStatus, adminNotes, reviewerID, requestID)
		if err != nil {
			return err
		}

		auditAction := "user.deletion.reject"
		if approve {
			auditAction = "user.deletion.approve"
		}
		_ = database.WriteAudit(txCtx, tx, database.AuditEntry{
			ActorUserID: reviewerID,
			Action:      auditAction,
			EntityType:  "account_deletion_request",
			EntityID:    strconv.FormatInt(requestID, 10),
			Before:      map[string]any{"status": currentStatus},
			After:       map[string]any{"status": newStatus, "user_id": userID, "notes": adminNotes},
		})

		return nil
	})
}

// CreateAccountDeletionRequest submits a new deletion request.
func (r *Repository) CreateAccountDeletionRequest(ctx context.Context, req *identity.AccountDeletionRequest) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			INSERT INTO identity.account_deletion_requests (
				user_id, organization_id, reason, status, admin_notes, requested_at, created_at, updated_at
			) VALUES ($1, $2, $3, 'pending', '', now(), now(), now())
			RETURNING id, created_at, updated_at;
		`
		err := tx.QueryRow(txCtx, query, req.UserID, req.OrganizationID, req.Reason).Scan(
			&req.ID, &req.CreatedAt, &req.UpdatedAt,
		)
		if err != nil {
			if database.IsUniqueViolation(err) {
				return apperr.Conflict("identity.deletion.pending", "يوجد طلب حذف حساب قيد المراجعة بالفعل.")
			}
			return err
		}
		return nil
	})
}

// GetPendingAccountDeletionRequest returns any open deletion request for the given user.
func (r *Repository) GetPendingAccountDeletionRequest(ctx context.Context, userID int64) (*identity.AccountDeletionRequest, error) {
	var item identity.AccountDeletionRequest
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT id, user_id, organization_id, reason, status, admin_notes, created_at, updated_at
			FROM identity.account_deletion_requests
			WHERE user_id = $1 AND status IN ('pending', 'under_review')
			ORDER BY created_at DESC
			LIMIT 1;
		`
		return tx.QueryRow(txCtx, query, userID).Scan(
			&item.ID, &item.UserID, &item.OrganizationID, &item.Reason, &item.Status,
			&item.AdminNotes, &item.CreatedAt, &item.UpdatedAt,
		)
	})
	if err != nil {
		if database.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

// CancelAccountDeletionRequest cancels an active pending deletion request by the user.
func (r *Repository) CancelAccountDeletionRequest(ctx context.Context, userID, requestID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(txCtx, `
			UPDATE identity.account_deletion_requests
			SET status = 'rejected', admin_notes = 'تم إلغاء الطلب من قبل المستخدم', updated_at = now()
			WHERE id = $1 AND user_id = $2 AND status IN ('pending', 'under_review');
		`, requestID, userID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("account_deletion_request")
		}
		return nil
	})
}
