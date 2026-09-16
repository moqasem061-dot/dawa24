package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// CreateRequest inserts a document/action request.
func (r *Repository) CreateRequest(ctx context.Context, req *workflow.Request) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			INSERT INTO workflow.requests (type, title, description, status, action_url, from_user_id, from_org_id, to_org_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			RETURNING id, public_id, created_at, updated_at;
		`
		return tx.QueryRow(txCtx, query,
			string(req.Type), req.Title, req.Description, string(req.Status), req.ActionURL,
			req.FromUserID, req.FromOrgID, req.ToOrgID,
		).Scan(&req.ID, &req.PublicID, &req.CreatedAt, &req.UpdatedAt)
	})
}

// GetRequestByID fetches one request.
func (r *Repository) GetRequestByID(ctx context.Context, id int64) (*workflow.Request, error) {
	var req workflow.Request
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT id, public_id, type, title, description, status, action_url,
			       from_user_id, from_org_id, to_org_id, created_at, updated_at
			FROM workflow.requests WHERE id = $1;
		`
		var typ, status string
		err := tx.QueryRow(txCtx, query, id).Scan(
			&req.ID, &req.PublicID, &typ, &req.Title, &req.Description, &status, &req.ActionURL,
			&req.FromUserID, &req.FromOrgID, &req.ToOrgID, &req.CreatedAt, &req.UpdatedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("request")
			}
			return err
		}
		req.Type = workflow.RequestType(typ)
		req.Status = workflow.RequestStatus(status)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &req, nil
}

// ListRequestsByOrg returns requests involving the org, newest first.
func (r *Repository) ListRequestsByOrg(ctx context.Context, orgID int64, status string, limit, offset int) ([]*workflow.Request, error) {
	reqs, _, err := r.ListRequestsByOrgWithTotal(ctx, orgID, status, limit, offset)
	return reqs, err
}

// ListRequestsByOrgWithTotal returns requests involving the org with total count, newest first.
func (r *Repository) ListRequestsByOrgWithTotal(ctx context.Context, orgID int64, status string, limit, offset int) ([]*workflow.Request, int, error) {
	var list []*workflow.Request
	var total int
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const countQuery = `
			SELECT count(*)
			FROM workflow.requests
			WHERE (from_org_id = $1 OR to_org_id = $1)
			  AND ($2 = '' OR status = $2);
		`
		if err := tx.QueryRow(txCtx, countQuery, orgID, status).Scan(&total); err != nil {
			return err
		}

		const query = `
			SELECT id, public_id, type, title, description, status, action_url,
			       from_user_id, from_org_id, to_org_id, created_at, updated_at
			FROM workflow.requests
			WHERE (from_org_id = $1 OR to_org_id = $1)
			  AND ($2 = '' OR status = $2)
			ORDER BY created_at DESC, id DESC
			LIMIT $3 OFFSET $4;
		`
		if limit <= 0 || limit > 100 {
			limit = 20
		}
		rows, err := tx.Query(txCtx, query, orgID, status, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var req workflow.Request
			var typ, st string
			if err := rows.Scan(
				&req.ID, &req.PublicID, &typ, &req.Title, &req.Description, &st, &req.ActionURL,
				&req.FromUserID, &req.FromOrgID, &req.ToOrgID, &req.CreatedAt, &req.UpdatedAt,
			); err != nil {
				return err
			}
			req.Type = workflow.RequestType(typ)
			req.Status = workflow.RequestStatus(st)
			list = append(list, &req)
		}
		return rows.Err()
	})
	return list, total, err
}

// UpdateRequestStatus changes a request's lifecycle state.
func (r *Repository) UpdateRequestStatus(ctx context.Context, id int64, status workflow.RequestStatus) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(txCtx, `UPDATE workflow.requests SET status = $1, updated_at = now() WHERE id = $2;`, string(status), id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("request")
		}
		return nil
	})
}
