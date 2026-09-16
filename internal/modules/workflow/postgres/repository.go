package postgres

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Repository implements workflow.Repository using PostgreSQL.
type Repository struct {
	db *database.DB
}

// NewRepository creates a workflow PostgreSQL repository.
func NewRepository(db *database.DB) *Repository {
	return &Repository{db: db}
}

// CreatePriorityRequest writes a purchasing priority task.
func (r *Repository) CreatePriorityRequest(ctx context.Context, req *workflow.PurchasePriorityRequest) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		paramsJSON, _ := json.Marshal(req.Parameters)
		recomJSON, _ := json.Marshal(req.Recommendations)

		query := `
			INSERT INTO workflow.purchase_priority_engines (
				user_id, organization_id, request_number, status, priority_highest_discount,
				priority_lowest_price, priority_fastest_delivery, priority_preferred_suppliers_only,
				budget_constraint, parameters, recommendations
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			RETURNING id, public_id, created_at, updated_at;
		`
		return tx.QueryRow(txCtx, query,
			req.UserID, req.OrganizationID, req.RequestNumber, req.Status, req.PriorityHighestDiscount,
			req.PriorityLowestPrice, req.PriorityFastestDelivery, req.PriorityPreferredSuppliersOnly,
			req.BudgetConstraint, paramsJSON, recomJSON,
		).Scan(&req.ID, &req.PublicID, &req.CreatedAt, &req.UpdatedAt)
	})
}

// GetPriorityRequestByID retrieves a priority request.
func (r *Repository) GetPriorityRequestByID(ctx context.Context, id int64) (*workflow.PurchasePriorityRequest, error) {
	var req workflow.PurchasePriorityRequest
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, user_id, organization_id, request_number, status,
			       priority_highest_discount, priority_lowest_price, priority_fastest_delivery,
			       priority_preferred_suppliers_only, budget_constraint, parameters,
			       recommendations, created_at, updated_at
			FROM workflow.purchase_priority_engines
			WHERE id = $1;
		`
		var paramsJSON, recomJSON []byte
		err := tx.QueryRow(txCtx, query, id).Scan(
			&req.ID, &req.PublicID, &req.UserID, &req.OrganizationID, &req.RequestNumber, &req.Status,
			&req.PriorityHighestDiscount, &req.PriorityLowestPrice, &req.PriorityFastestDelivery,
			&req.PriorityPreferredSuppliersOnly, &req.BudgetConstraint, &paramsJSON,
			&recomJSON, &req.CreatedAt, &req.UpdatedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("priority_request")
			}
			return err
		}

		if len(paramsJSON) > 0 {
			_ = json.Unmarshal(paramsJSON, &req.Parameters)
		}
		if len(recomJSON) > 0 {
			_ = json.Unmarshal(recomJSON, &req.Recommendations)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &req, nil
}

// CreateIssue records a new issue report ticket.
func (r *Repository) CreateIssue(ctx context.Context, i *workflow.ReportIssue) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO workflow.report_issues (
				reported_by, organization_id, order_id, issue_type, description, status, priority
			) VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING id, public_id, created_at, updated_at;
		`
		return tx.QueryRow(txCtx, query,
			i.ReportedBy, i.OrganizationID, i.OrderID, i.IssueType, i.Description, i.Status, i.Priority,
		).Scan(&i.ID, &i.PublicID, &i.CreatedAt, &i.UpdatedAt)
	})
}

// GetIssueByID retrieves a specific issue report ticket.
func (r *Repository) GetIssueByID(ctx context.Context, id int64) (*workflow.ReportIssue, error) {
	var i workflow.ReportIssue
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, reported_by, organization_id, order_id, issue_type,
			       description, status, priority, response_notes, created_at, updated_at
			FROM workflow.report_issues
			WHERE id = $1;
		`
		return tx.QueryRow(txCtx, query, id).Scan(
			&i.ID, &i.PublicID, &i.ReportedBy, &i.OrganizationID, &i.OrderID,
			&i.IssueType, &i.Description, &i.Status, &i.Priority, &i.ResponseNotes,
			&i.CreatedAt, &i.UpdatedAt,
		)
	})
	if err != nil {
		if database.IsNotFound(err) {
			return nil, apperr.NotFound("issue")
		}
		return nil, err
	}
	return &i, nil
}

// ListIssues retrieves paginated tickets.
func (r *Repository) ListIssues(ctx context.Context, limit, offset int) ([]*workflow.ReportIssue, error) {
	var list []*workflow.ReportIssue
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, reported_by, organization_id, order_id, issue_type,
			       description, status, priority, response_notes, created_at, updated_at
			FROM workflow.report_issues
			ORDER BY created_at DESC
			LIMIT $1 OFFSET $2;
		`
		if limit <= 0 || limit > 100 {
			limit = 20
		}
		rows, err := tx.Query(txCtx, query, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var i workflow.ReportIssue
			if err := rows.Scan(
				&i.ID, &i.PublicID, &i.ReportedBy, &i.OrganizationID, &i.OrderID,
				&i.IssueType, &i.Description, &i.Status, &i.Priority, &i.ResponseNotes,
				&i.CreatedAt, &i.UpdatedAt,
			); err != nil {
				return err
			}
			list = append(list, &i)
		}
		return rows.Err()
	})
	return list, err
}

// ListIssuesByReporter retrieves tickets submitted by a specific user.
func (r *Repository) ListIssuesByReporter(ctx context.Context, userID int64, limit, offset int) ([]*workflow.ReportIssue, error) {
	var list []*workflow.ReportIssue
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, reported_by, organization_id, order_id, issue_type,
			       description, status, priority, response_notes, created_at, updated_at
			FROM workflow.report_issues
			WHERE reported_by = $1
			ORDER BY created_at DESC
			LIMIT $2 OFFSET $3;
		`
		if limit <= 0 || limit > 100 {
			limit = 20
		}
		if offset < 0 {
			offset = 0
		}
		rows, err := tx.Query(txCtx, query, userID, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var i workflow.ReportIssue
			if err := rows.Scan(
				&i.ID, &i.PublicID, &i.ReportedBy, &i.OrganizationID, &i.OrderID,
				&i.IssueType, &i.Description, &i.Status, &i.Priority, &i.ResponseNotes,
				&i.CreatedAt, &i.UpdatedAt,
			); err != nil {
				return err
			}
			list = append(list, &i)
		}
		return rows.Err()
	})
	return list, err
}

// ListIssuesDetailed returns filtered issues with joined user and organization data, plus total count.
func (r *Repository) ListIssuesDetailed(ctx context.Context, filter workflow.ReportIssueFilter) ([]*workflow.ReportIssueDetail, int, error) {
	var list []*workflow.ReportIssueDetail
	var total int

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT r.id, r.public_id, r.reported_by, r.organization_id, r.order_id, r.issue_type,
			       r.description, r.status, r.priority, r.response_notes, r.created_at, r.updated_at,
			       COALESCE(u.name->>'ar', u.name->>'en', ''),
			       COALESCE(u.email, ''),
			       COALESCE(u.phone, ''),
			       COALESCE(o.name->>'ar', o.name->>'en', ''),
			       COALESCE(o.type, ''),
			       COALESCE(ord.public_id::text, ''),
			       COUNT(*) OVER() AS full_count
			FROM workflow.report_issues r
			LEFT JOIN identity.users u ON u.id = r.reported_by
			LEFT JOIN org.organizations o ON o.id = r.organization_id
			LEFT JOIN commerce.orders ord ON ord.id = r.order_id
			WHERE ($1::text = '' OR r.status = $1)
			  AND ($2::text = '' OR r.issue_type = $2)
			  AND ($3::text = '' OR r.priority = $3)
			  AND (
				$4::text = ''
				OR r.description ILIKE '%' || $4 || '%'
				OR COALESCE(u.name->>'ar', u.name->>'en', '') ILIKE '%' || $4 || '%'
				OR COALESCE(u.phone, '') ILIKE '%' || $4 || '%'
				OR COALESCE(o.name->>'ar', o.name->>'en', '') ILIKE '%' || $4 || '%'
				OR CAST(r.id AS TEXT) = $4
			  )
			ORDER BY r.created_at DESC
			LIMIT $5 OFFSET $6;
		`
		limit := filter.Limit
		if limit <= 0 || limit > 100 {
			limit = 20
		}
		offset := filter.Offset
		if offset < 0 {
			offset = 0
		}

		rows, err := tx.Query(txCtx, query,
			filter.Status,
			filter.IssueType,
			filter.Priority,
			filter.Search,
			limit,
			offset,
		)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var d workflow.ReportIssueDetail
			var count int
			if err := rows.Scan(
				&d.ID, &d.PublicID, &d.ReportedBy, &d.OrganizationID, &d.OrderID,
				&d.IssueType, &d.Description, &d.Status, &d.Priority, &d.ResponseNotes,
				&d.CreatedAt, &d.UpdatedAt,
				&d.ReporterName, &d.ReporterEmail, &d.ReporterPhone,
				&d.OrgName, &d.OrgType, &d.OrderPublicID,
				&count,
			); err != nil {
				return err
			}
			total = count
			list = append(list, &d)
		}
		return rows.Err()
	})
	return list, total, err
}

// GetIssueStats returns count breakdown across all report issues.
func (r *Repository) GetIssueStats(ctx context.Context) (*workflow.ReportIssueStats, error) {
	var stats workflow.ReportIssueStats
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT
				COUNT(*)::int AS total,
				COUNT(*) FILTER (WHERE status = 'pending')::int AS pending,
				COUNT(*) FILTER (WHERE status = 'in_progress')::int AS in_progress,
				COUNT(*) FILTER (WHERE status = 'resolved')::int AS resolved
			FROM workflow.report_issues;
		`
		return tx.QueryRow(txCtx, query).Scan(&stats.Total, &stats.Pending, &stats.InProgress, &stats.Resolved)
	})
	if err != nil {
		return nil, err
	}
	return &stats, nil
}

// UpdateIssueStatus updates the ticket status and admin response notes.
func (r *Repository) UpdateIssueStatus(ctx context.Context, id int64, status, responseNotes string) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE workflow.report_issues
			SET status = $2, response_notes = $3, updated_at = now()
			WHERE id = $1;
		`
		tag, err := tx.Exec(txCtx, query, id, status, responseNotes)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("issue")
		}
		return nil
	})
}
