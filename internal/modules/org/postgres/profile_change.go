package postgres

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Storing and deciding proposed changes to a company's profile.

const profileChangeColumns = `
	r.id, r.public_id, r.organization_id, r.requested_by, r.section,
	r.proposed, r.previous, r.status, r.admin_notes,
	r.reviewed_by, r.reviewed_at, r.created_at, r.updated_at`

func scanProfileChange(row pgx.Row) (*org.ProfileChangeRequest, error) {
	var r org.ProfileChangeRequest
	err := row.Scan(
		&r.ID, &r.PublicID, &r.OrganizationID, &r.RequestedBy, &r.Section,
		&r.Proposed, &r.Previous, &r.Status, &r.AdminNotes,
		&r.ReviewedBy, &r.ReviewedAt, &r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// CreateProfileChangeRequest opens one request for one section.
//
// The partial unique index allows one pending request per section per company,
// so a second submission conflicts rather than queueing a second answer to the
// same question. That is reported as a conflict the person can act on, not a
// 500.
func (r *Repository) CreateProfileChangeRequest(
	ctx context.Context, req *org.ProfileChangeRequest,
) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			INSERT INTO org.profile_change_requests
				(organization_id, requested_by, section, proposed, previous)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING id, public_id, status, created_at, updated_at;`
		err := tx.QueryRow(txCtx, query,
			req.OrganizationID, req.RequestedBy, string(req.Section), req.Proposed, req.Previous,
		).Scan(&req.ID, &req.PublicID, &req.Status, &req.CreatedAt, &req.UpdatedAt)
		if err != nil {
			if database.IsUniqueViolation(err) {
				return apperr.Conflict("org.profile.change_pending",
					"A change to this section is already awaiting review.")
			}
			return fmt.Errorf("org postgres: create profile change request: %w", err)
		}
		return nil
	})
}

// PendingProfileChanges returns one company's open requests, keyed by section,
// so the page can show each section's own pending state beside its form.
func (r *Repository) PendingProfileChanges(
	ctx context.Context, orgID int64,
) (map[org.ProfileSection]*org.ProfileChangeRequest, error) {
	out := make(map[org.ProfileSection]*org.ProfileChangeRequest)
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `SELECT ` + profileChangeColumns + `
			FROM org.profile_change_requests r
			WHERE r.organization_id = $1 AND r.status = 'pending';`
		rows, err := tx.Query(txCtx, query, orgID)
		if err != nil {
			return fmt.Errorf("org postgres: pending profile changes: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			req, scanErr := scanProfileChange(rows)
			if scanErr != nil {
				return scanErr
			}
			out[req.Section] = req
		}
		return rows.Err()
	})
	return out, err
}

// GetProfileChangeRequest reads one request by id, for the admin screen.
func (r *Repository) GetProfileChangeRequest(
	ctx context.Context, id int64,
) (*org.ProfileChangeRequest, error) {
	var req *org.ProfileChangeRequest
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `SELECT ` + profileChangeColumns + `
			FROM org.profile_change_requests r WHERE r.id = $1;`
		var scanErr error
		req, scanErr = scanProfileChange(tx.QueryRow(txCtx, query, id))
		return scanErr
	})
	if err != nil {
		if database.IsNotFound(err) {
			return nil, apperr.NotFound("profile_change_request")
		}
		return nil, err
	}
	return req, nil
}

// ProfileChangeRequestCounts aggregates counts of change requests by status.
func (r *Repository) ProfileChangeRequestCounts(ctx context.Context) (org.ProfileChangeCounts, error) {
	var counts org.ProfileChangeCounts
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT status, COUNT(*)
			FROM org.profile_change_requests
			GROUP BY status;`
		rows, err := tx.Query(txCtx, query)
		if err != nil {
			return fmt.Errorf("org postgres: count profile change requests: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var st string
			var count int
			if err := rows.Scan(&st, &count); err != nil {
				return err
			}
			switch st {
			case string(org.ChangePending):
				counts.Pending = count
			case string(org.ChangeApproved):
				counts.Approved = count
			case string(org.ChangeRejected):
				counts.Rejected = count
			case string(org.ChangeWithdrawn):
				counts.Withdrawn = count
			}
			counts.All += count
		}
		return rows.Err()
	})
	return counts, err
}

// ListProfileChangeRequests returns one page of the admin review queue.
func (r *Repository) ListProfileChangeRequests(
	ctx context.Context, status string, limit, offset int,
) ([]*org.ProfileChangeRequest, int, error) {
	return r.ListProfileChangeRequestsWithFilter(ctx, org.ProfileChangeFilter{Status: status}, limit, offset)
}

// ListProfileChangeRequestsWithFilter returns one page of the admin review queue matching the filter.
func (r *Repository) ListProfileChangeRequestsWithFilter(
	ctx context.Context, filter org.ProfileChangeFilter, limit, offset int,
) ([]*org.ProfileChangeRequest, int, error) {
	var (
		list  []*org.ProfileChangeRequest
		total int
	)
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var (
			whereClauses []string
			args         []any
			argIdx       = 1
		)

		if filter.Status != "" {
			whereClauses = append(whereClauses, fmt.Sprintf("r.status = $%d", argIdx))
			args = append(args, filter.Status)
			argIdx++
		}
		if filter.Section != "" {
			whereClauses = append(whereClauses, fmt.Sprintf("r.section = $%d", argIdx))
			args = append(args, filter.Section)
			argIdx++
		}
		if filter.OrganizationID > 0 {
			whereClauses = append(whereClauses, fmt.Sprintf("r.organization_id = $%d", argIdx))
			args = append(args, filter.OrganizationID)
			argIdx++
		}
		if filter.DateFrom != nil {
			whereClauses = append(whereClauses, fmt.Sprintf("r.created_at >= $%d", argIdx))
			args = append(args, *filter.DateFrom)
			argIdx++
		}
		if filter.DateTo != nil {
			whereClauses = append(whereClauses, fmt.Sprintf("r.created_at <= $%d", argIdx))
			args = append(args, *filter.DateTo)
			argIdx++
		}
		if filter.Search != "" {
			pattern := "%" + filter.Search + "%"
			whereClauses = append(whereClauses, fmt.Sprintf(
				"(o.legal_name ILIKE $%d OR o.trade_name->>'ar' ILIKE $%d OR o.trade_name->>'en' ILIKE $%d OR o.name->>'ar' ILIKE $%d)",
				argIdx, argIdx, argIdx, argIdx,
			))
			args = append(args, pattern)
			argIdx++
		}

		whereSQL := ""
		if len(whereClauses) > 0 {
			whereSQL = "WHERE " + strings.Join(whereClauses, " AND ")
		}

		countQuery := fmt.Sprintf(`
			SELECT COUNT(*)
			FROM org.profile_change_requests r
			JOIN org.organizations o ON o.id = r.organization_id
			%s;`, whereSQL)
		if err := tx.QueryRow(txCtx, countQuery, args...).Scan(&total); err != nil {
			return fmt.Errorf("org postgres: count profile change requests: %w", err)
		}

		query := fmt.Sprintf(`SELECT %s,
			       COALESCE(NULLIF(o.trade_name->>'ar', ''), NULLIF(o.legal_name, ''), NULLIF(o.name->>'ar', ''), '') AS org_name,
			       COALESCE(NULLIF(u.name->>'ar', ''), NULLIF(u.name->>'en', ''), u.email, '') AS requester_name,
			       COALESCE(NULLIF(rev.name->>'ar', ''), NULLIF(rev.name->>'en', ''), rev.email, '') AS reviewer_name
			FROM org.profile_change_requests r
			JOIN org.organizations o ON o.id = r.organization_id
			LEFT JOIN identity.users u ON u.id = r.requested_by
			LEFT JOIN identity.users rev ON rev.id = r.reviewed_by
			%s
			ORDER BY (r.status = 'pending') DESC, r.created_at DESC
			LIMIT $%d OFFSET $%d;`, profileChangeColumns, whereSQL, argIdx, argIdx+1)

		argsWithPagination := append(args, limit, offset)
		rows, err := tx.Query(txCtx, query, argsWithPagination...)
		if err != nil {
			return fmt.Errorf("org postgres: list profile change requests: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var req org.ProfileChangeRequest
			if err := rows.Scan(
				&req.ID, &req.PublicID, &req.OrganizationID, &req.RequestedBy, &req.Section,
				&req.Proposed, &req.Previous, &req.Status, &req.AdminNotes,
				&req.ReviewedBy, &req.ReviewedAt, &req.CreatedAt, &req.UpdatedAt,
				&req.OrganizationName, &req.RequesterName, &req.ReviewerName,
			); err != nil {
				return err
			}
			list = append(list, &req)
		}
		return rows.Err()
	})
	return list, total, err
}

// DecideProfileChangeRequest records a decision and, when approving, applies
// the change in the same transaction.
//
// One transaction is the point: an approval that stamped the request and then
// failed to write the organization would leave a request marked approved and a
// company unchanged, which is indistinguishable from a change nobody made.
func (r *Repository) DecideProfileChangeRequest(
	ctx context.Context, id, reviewerID int64, approve bool, notes string,
	apply func(context.Context, pgx.Tx, *org.ProfileChangeRequest) error,
) (*org.ProfileChangeRequest, error) {
	var decided *org.ProfileChangeRequest
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		lockQuery := `SELECT ` + profileChangeColumns + `
			FROM org.profile_change_requests r WHERE r.id = $1 FOR UPDATE;`
		req, err := scanProfileChange(tx.QueryRow(txCtx, lockQuery, id))
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("profile_change_request")
			}
			return err
		}
		if req.Status != org.ChangePending {
			return apperr.Conflict("org.profile.change_decided",
				"This request has already been decided.")
		}

		status := org.ChangeRejected
		if approve {
			status = org.ChangeApproved
			if apply != nil {
				if err := apply(txCtx, tx, req); err != nil {
					return err
				}
			}
		}

		const update = `
			UPDATE org.profile_change_requests
			SET status = $2, admin_notes = $3, reviewed_by = $4, reviewed_at = now()
			WHERE id = $1;`
		if _, err := tx.Exec(txCtx, update, id, string(status), notes, reviewerID); err != nil {
			return fmt.Errorf("org postgres: decide profile change request: %w", err)
		}

		req.Status = status
		req.AdminNotes = notes
		req.ReviewedBy = &reviewerID
		decided = req

		auditAction := "org.change_request.reject"
		if approve {
			auditAction = "org.change_request.approve"
		}
		_ = database.WriteAudit(txCtx, tx, database.AuditEntry{
			OrganizationID: &req.OrganizationID,
			ActorUserID:    reviewerID,
			Action:         auditAction,
			EntityType:     "profile_change_request",
			EntityID:       strconv.FormatInt(id, 10),
			Before:         map[string]any{"status": string(org.ChangePending)},
			After:          map[string]any{"status": string(status), "notes": notes},
		})

		return nil
	})
	return decided, err
}

// WithdrawProfileChangeRequest lets a company take back its own request.
func (r *Repository) WithdrawProfileChangeRequest(ctx context.Context, orgID, id int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			UPDATE org.profile_change_requests
			SET status = 'withdrawn'
			WHERE id = $1 AND organization_id = $2 AND status = 'pending';`
		tag, err := tx.Exec(txCtx, query, id, orgID)
		if err != nil {
			return fmt.Errorf("org postgres: withdraw profile change request: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("profile_change_request")
		}
		return nil
	})
}
