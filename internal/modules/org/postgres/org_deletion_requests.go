package postgres

import (
	"context"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// CreateOrgDeletionRequest submits a new organization deletion request.
func (r *Repository) CreateOrgDeletionRequest(ctx context.Context, req *org.OrganizationDeletionRequest) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			INSERT INTO org.organization_deletion_requests (
				organization_id, requested_by, reason, status, admin_notes
			) VALUES ($1, $2, $3, 'pending', '')
			RETURNING id, public_id, status, created_at, updated_at;
		`
		err := tx.QueryRow(txCtx, query, req.OrganizationID, req.RequestedBy, req.Reason).
			Scan(&req.ID, &req.PublicID, &req.Status, &req.CreatedAt, &req.UpdatedAt)
		if err != nil {
			if database.IsUniqueViolation(err) {
				return apperr.Conflict("org.deletion.pending", "يوجد طلب حذف قيد المراجعة بالفعل لهذه المنشأة.")
			}
			return fmt.Errorf("create org deletion request: %w", err)
		}
		return nil
	})
}

// GetPendingOrgDeletionRequest returns the open pending deletion request for the given organization, if any.
func (r *Repository) GetPendingOrgDeletionRequest(ctx context.Context, orgID int64) (*org.OrganizationDeletionRequest, error) {
	var item org.OrganizationDeletionRequest
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT r.id, r.public_id, r.organization_id, r.requested_by, r.reason, r.status,
			       r.admin_notes, r.reviewed_by, r.reviewed_at, r.created_at, r.updated_at,
			       COALESCE(o.trade_name->>'ar', o.name->>'ar', o.legal_name, ''),
			       COALESCE(o.type, ''), COALESCE(o.status, ''),
			       COALESCE(o.commercial_register, ''), COALESCE(o.tax_number, ''),
			       COALESCE(u.name->>'ar', u.name->>'en', ''), COALESCE(u.email, ''), COALESCE(u.phone, '')
			FROM org.organization_deletion_requests r
			JOIN org.organizations o ON o.id = r.organization_id
			JOIN identity.users u ON u.id = r.requested_by
			WHERE r.organization_id = $1 AND r.status = 'pending'
			LIMIT 1;
		`
		return tx.QueryRow(txCtx, query, orgID).Scan(
			&item.ID, &item.PublicID, &item.OrganizationID, &item.RequestedBy, &item.Reason, &item.Status,
			&item.AdminNotes, &item.ReviewedBy, &item.ReviewedAt, &item.CreatedAt, &item.UpdatedAt,
			&item.OrgName, &item.OrgType, &item.OrgStatus,
			&item.CommercialReg, &item.TaxNumber,
			&item.RequesterName, &item.RequesterEmail, &item.RequesterPhone,
		)
	})
	if err != nil {
		if database.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("get pending org deletion request: %w", err)
	}
	return &item, nil
}

// GetOrgDeletionRequest retrieves a single deletion request by its ID.
func (r *Repository) GetOrgDeletionRequest(ctx context.Context, id int64) (*org.OrganizationDeletionRequest, error) {
	var item org.OrganizationDeletionRequest
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT r.id, r.public_id, r.organization_id, r.requested_by, r.reason, r.status,
			       r.admin_notes, r.reviewed_by, r.reviewed_at, r.created_at, r.updated_at,
			       COALESCE(o.trade_name->>'ar', o.name->>'ar', o.legal_name, ''),
			       COALESCE(o.type, ''), COALESCE(o.status, ''),
			       COALESCE(o.commercial_register, ''), COALESCE(o.tax_number, ''),
			       COALESCE(u.name->>'ar', u.name->>'en', ''), COALESCE(u.email, ''), COALESCE(u.phone, ''),
			       COALESCE(rev.name->>'ar', rev.name->>'en', '')
			FROM org.organization_deletion_requests r
			JOIN org.organizations o ON o.id = r.organization_id
			JOIN identity.users u ON u.id = r.requested_by
			LEFT JOIN identity.users rev ON rev.id = r.reviewed_by
			WHERE r.id = $1;
		`
		return tx.QueryRow(txCtx, query, id).Scan(
			&item.ID, &item.PublicID, &item.OrganizationID, &item.RequestedBy, &item.Reason, &item.Status,
			&item.AdminNotes, &item.ReviewedBy, &item.ReviewedAt, &item.CreatedAt, &item.UpdatedAt,
			&item.OrgName, &item.OrgType, &item.OrgStatus,
			&item.CommercialReg, &item.TaxNumber,
			&item.RequesterName, &item.RequesterEmail, &item.RequesterPhone,
			&item.ReviewerName,
		)
	})
	if err != nil {
		if database.IsNotFound(err) {
			return nil, apperr.NotFound("org.deletion_request")
		}
		return nil, fmt.Errorf("get org deletion request: %w", err)
	}
	return &item, nil
}

// CancelOrgDeletionRequest marks a pending deletion request as cancelled by the owner.
func (r *Repository) CancelOrgDeletionRequest(ctx context.Context, orgID, requestID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			UPDATE org.organization_deletion_requests
			SET status = 'cancelled', updated_at = now()
			WHERE id = $1 AND organization_id = $2 AND status = 'pending';
		`
		tag, err := tx.Exec(txCtx, query, requestID, orgID)
		if err != nil {
			return fmt.Errorf("cancel org deletion request: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("org.deletion_request.pending")
		}
		return nil
	})
}

// ListOrgDeletionRequests lists deletion requests with filters, pagination, and enriched details.
func (r *Repository) ListOrgDeletionRequests(ctx context.Context, status string, limit, offset int) ([]*org.OrganizationDeletionRequest, int, error) {
	var list []*org.OrganizationDeletionRequest
	var total int
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var statusPtr *string
		if status != "" {
			statusPtr = &status
		}

		countQ := `SELECT count(*) FROM org.organization_deletion_requests WHERE ($1::text IS NULL OR status = $1);`
		if err := tx.QueryRow(txCtx, countQ, statusPtr).Scan(&total); err != nil {
			return err
		}

		const query = `
			SELECT r.id, r.public_id, r.organization_id, r.requested_by, r.reason, r.status,
			       r.admin_notes, r.reviewed_by, r.reviewed_at, r.created_at, r.updated_at,
			       COALESCE(o.trade_name->>'ar', o.name->>'ar', o.legal_name, ''),
			       COALESCE(o.type, ''), COALESCE(o.status, ''),
			       COALESCE(o.commercial_register, ''), COALESCE(o.tax_number, ''),
			       COALESCE(u.name->>'ar', u.name->>'en', ''), COALESCE(u.email, ''), COALESCE(u.phone, ''),
			       COALESCE(rev.name->>'ar', rev.name->>'en', ''),
			       COALESCE((SELECT count(*) FROM org.branches b WHERE b.organization_id = o.id AND b.deleted_at IS NULL), 0)
			FROM org.organization_deletion_requests r
			JOIN org.organizations o ON o.id = r.organization_id
			JOIN identity.users u ON u.id = r.requested_by
			LEFT JOIN identity.users rev ON rev.id = r.reviewed_by
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
			var item org.OrganizationDeletionRequest
			if err := rows.Scan(
				&item.ID, &item.PublicID, &item.OrganizationID, &item.RequestedBy, &item.Reason, &item.Status,
				&item.AdminNotes, &item.ReviewedBy, &item.ReviewedAt, &item.CreatedAt, &item.UpdatedAt,
				&item.OrgName, &item.OrgType, &item.OrgStatus,
				&item.CommercialReg, &item.TaxNumber,
				&item.RequesterName, &item.RequesterEmail, &item.RequesterPhone,
				&item.ReviewerName,
				&item.BranchesCount,
			); err != nil {
				return err
			}
			list = append(list, &item)
		}
		return rows.Err()
	})
	return list, total, err
}

// ReviewOrgDeletionRequest decides an organization deletion request.
// If approved: marks the organization deleted and suspended, deactivates active branches and memberships.
// If rejected: records the rejection reason.
func (r *Repository) ReviewOrgDeletionRequest(ctx context.Context, requestID, reviewerID int64, approve bool, adminNotes string) (*org.OrganizationDeletionRequest, error) {
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var orgID int64
		var currentStatus string
		if err := tx.QueryRow(txCtx,
			`SELECT organization_id, status FROM org.organization_deletion_requests WHERE id = $1 FOR UPDATE;`, requestID,
		).Scan(&orgID, &currentStatus); err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("org.deletion_request")
			}
			return err
		}

		if currentStatus != "pending" {
			return apperr.Conflict("org.deletion_request.not_pending", "هذا الطلب تمت معالجته مسبقاً.")
		}

		newStatus := "rejected"
		if approve {
			newStatus = "approved"

			// Soft delete organization with terminal status 'deleted'
			_, err := tx.Exec(txCtx,
				`UPDATE org.organizations
				 SET status = 'deleted', deleted_at = now(), updated_at = now()
				 WHERE id = $1;`, orgID)
			if err != nil {
				return fmt.Errorf("delete organization: %w", err)
			}

			// Soft delete branches
			_, err = tx.Exec(txCtx,
				`UPDATE org.branches
				 SET status = 'inactive', deleted_at = now(), updated_at = now()
				 WHERE organization_id = $1 AND deleted_at IS NULL;`, orgID)
			if err != nil {
				return fmt.Errorf("deactivate org branches: %w", err)
			}

			// Deactivate members
			_, err = tx.Exec(txCtx,
				`UPDATE org.members
				 SET is_active = false, updated_at = now()
				 WHERE organization_id = $1 AND is_active = true;`, orgID)
			if err != nil {
				return fmt.Errorf("deactivate org members: %w", err)
			}

			// Revoke sessions for users belonging to this organization (members and owner)
			_, err = tx.Exec(txCtx,
				`UPDATE identity.user_sessions
				 SET is_active = false, logged_out_at = now()
				 WHERE is_active = true AND user_id IN (
				     SELECT user_id FROM org.members WHERE organization_id = $1
				     UNION
				     SELECT owner_id FROM org.organizations WHERE id = $1
				 );`, orgID)
			if err != nil {
				return fmt.Errorf("revoke org user sessions: %w", err)
			}

			// Deactivate catalog variants
			_, err = tx.Exec(txCtx,
				`UPDATE catalog.product_variants
				 SET status = 'inactive', updated_at = now()
				 WHERE organization_id = $1 AND status = 'active';`, orgID)
			if err != nil {
				return fmt.Errorf("deactivate org catalog variants: %w", err)
			}

			// Deactivate promo offers
			_, err = tx.Exec(txCtx,
				`UPDATE promo.offers
				 SET is_active = false, deleted_at = now(), updated_at = now()
				 WHERE organization_id = $1 AND is_active = true;`, orgID)
			if err != nil {
				return fmt.Errorf("deactivate org promo offers: %w", err)
			}
		}

		_, err := tx.Exec(txCtx, `
			UPDATE org.organization_deletion_requests
			SET status = $1, admin_notes = $2, reviewed_by = $3, reviewed_at = now(), updated_at = now()
			WHERE id = $4;
		`, newStatus, adminNotes, reviewerID, requestID)
		if err != nil {
			return fmt.Errorf("update org deletion request: %w", err)
		}

		auditAction := "org.deletion.reject"
		if approve {
			auditAction = "org.deletion.approve"
		}
		_ = database.WriteAudit(txCtx, tx, database.AuditEntry{
			OrganizationID: &orgID,
			ActorUserID:    reviewerID,
			Action:         auditAction,
			EntityType:     "organization_deletion_request",
			EntityID:       strconv.FormatInt(requestID, 10),
			Before:         map[string]any{"status": currentStatus},
			After:          map[string]any{"status": newStatus, "notes": adminNotes},
		})

		return nil
	})
	if err != nil {
		return nil, err
	}

	return r.GetOrgDeletionRequest(ctx, requestID)
}
