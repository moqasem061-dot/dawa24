package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// CreateQuoteRequest persists a buyer price quote inquiry.
func (r *Repository) CreateQuoteRequest(ctx context.Context, q *commerce.QuoteRequest) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO commerce.quote_requests (
				organization_id, customer_org_id, product_id, product_name,
				requested_quantity, target_unit_price, status, buyer_notes
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			RETURNING id, public_id, created_at, updated_at;
		`
		return tx.QueryRow(txCtx, query,
			q.OrganizationID, q.CustomerOrgID, q.ProductID, q.ProductName,
			q.RequestedQuantity, q.TargetUnitPrice, string(q.Status), q.BuyerNotes,
		).Scan(&q.ID, &q.PublicID, &q.CreatedAt, &q.UpdatedAt)
	})
}

// GetQuoteRequestByID retrieves a quote by ID.
func (r *Repository) GetQuoteRequestByID(ctx context.Context, id int64) (*commerce.QuoteRequest, error) {
	var q commerce.QuoteRequest
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, organization_id, customer_org_id, product_id, product_name,
			       requested_quantity, target_unit_price, quote_unit_price, status,
			       buyer_notes, supplier_notes, valid_until, created_at, updated_at
			FROM commerce.quote_requests WHERE id = $1;
		`
		var statusStr string
		var buyerNotes, supplierNotes *string
		err := tx.QueryRow(txCtx, query, id).Scan(
			&q.ID, &q.PublicID, &q.OrganizationID, &q.CustomerOrgID, &q.ProductID, &q.ProductName,
			&q.RequestedQuantity, &q.TargetUnitPrice, &q.QuoteUnitPrice, &statusStr,
			&buyerNotes, &supplierNotes, &q.ValidUntil, &q.CreatedAt, &q.UpdatedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("quote_request")
			}
			return err
		}
		q.Status = commerce.QuoteStatus(statusStr)
		if buyerNotes != nil {
			q.BuyerNotes = *buyerNotes
		}
		if supplierNotes != nil {
			q.SupplierNotes = *supplierNotes
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &q, nil
}

// UpdateQuoteStatus modifies quote status, proposed unit price, and supplier notes.
func (r *Repository) UpdateQuoteStatus(
	ctx context.Context,
	id int64,
	status commerce.QuoteStatus,
	quotePrice money.Amount,
	supplierNotes string,
) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE commerce.quote_requests
			SET status = $1, quote_unit_price = $2, supplier_notes = $3, updated_at = now()
			WHERE id = $4;
		`
		tag, err := tx.Exec(txCtx, query, string(status), quotePrice, supplierNotes, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("quote_request")
		}
		return nil
	})
}

// ListQuoteRequestsByOrg returns quotes for buyer or vendor.
func (r *Repository) ListQuoteRequestsByOrg(
	ctx context.Context,
	orgID int64,
	isVendor bool,
	limit, offset int,
) ([]*commerce.QuoteRequest, error) {
	var list []*commerce.QuoteRequest
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		filterCol := "customer_org_id"
		if isVendor {
			filterCol = "organization_id"
		}
		query := `
			SELECT id, public_id, organization_id, customer_org_id, product_id, product_name,
			       requested_quantity, target_unit_price, quote_unit_price, status,
			       buyer_notes, supplier_notes, valid_until, created_at, updated_at
			FROM commerce.quote_requests
			WHERE ` + filterCol + ` = $1
			ORDER BY created_at DESC
			LIMIT $2 OFFSET $3;
		`
		if limit <= 0 || limit > 100 {
			limit = 20
		}
		rows, err := tx.Query(txCtx, query, orgID, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var q commerce.QuoteRequest
			var statusStr string
			var buyerNotes, supplierNotes *string
			if err := rows.Scan(
				&q.ID, &q.PublicID, &q.OrganizationID, &q.CustomerOrgID, &q.ProductID, &q.ProductName,
				&q.RequestedQuantity, &q.TargetUnitPrice, &q.QuoteUnitPrice, &statusStr,
				&buyerNotes, &supplierNotes, &q.ValidUntil, &q.CreatedAt, &q.UpdatedAt,
			); err != nil {
				return err
			}
			q.Status = commerce.QuoteStatus(statusStr)
			if buyerNotes != nil {
				q.BuyerNotes = *buyerNotes
			}
			if supplierNotes != nil {
				q.SupplierNotes = *supplierNotes
			}
			list = append(list, &q)
		}
		return rows.Err()
	})
	return list, err
}
