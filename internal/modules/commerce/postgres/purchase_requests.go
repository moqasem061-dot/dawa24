package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// CreatePurchaseRequest inserts a new multi-line purchase request within a transaction (Plan V5 Phase 3 Â§3.1).
func (r *Repository) CreatePurchaseRequest(ctx context.Context, pr *commerce.PurchaseRequest, lines []*commerce.PurchaseRequestLine) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		pr.CreatedAt = time.Now().UTC()
		pr.UpdatedAt = pr.CreatedAt
		if pr.Status == "" {
			pr.Status = commerce.PurchaseRequestPending
		}
		pr.TotalItems = len(lines)

		var estimatedTotal money.Amount
		for _, l := range lines {
			if l.TargetPrice.IsPositive() && l.Quantity > 0 {
				lineTotal, _ := l.TargetPrice.MulInt(int64(l.Quantity))
				estimatedTotal, _ = estimatedTotal.Add(lineTotal)
			}
		}
		pr.EstimatedTotal = estimatedTotal

		query := `
			INSERT INTO commerce.purchase_requests (
				request_number, customer_id, organization_id, branch_id,
				vendor_org_id, vendor_branch_id, status, total_items, estimated_total,
				buyer_notes, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
			RETURNING id, public_id;
		`

		var reqID int64
		var estTotalStr *string
		if pr.EstimatedTotal.IsPositive() {
			s := pr.EstimatedTotal.String()
			estTotalStr = &s
		}

		err := tx.QueryRow(txCtx, query,
			pr.RequestNumber, pr.CustomerID, pr.OrganizationID, pr.BranchID,
			pr.VendorOrgID, pr.VendorBranchID, string(pr.Status), pr.TotalItems, estTotalStr,
			pr.BuyerNotes, pr.CreatedAt, pr.UpdatedAt,
		).Scan(&reqID, &pr.PublicID)
		if err != nil {
			return fmt.Errorf("insert purchase request: %w", err)
		}
		pr.ID = reqID

		lineQuery := `
			INSERT INTO commerce.purchase_request_lines (
				request_id, product_id, product_name, product_sku, quantity,
				target_price, target_discount, status, notes, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			RETURNING id;
		`

		for _, line := range lines {
			line.RequestID = reqID
			line.CreatedAt = pr.CreatedAt
			line.UpdatedAt = pr.CreatedAt
			if line.Status == "" {
				line.Status = "pending"
			}
			var targetPriceStr *string
			if line.TargetPrice.IsPositive() {
				s := line.TargetPrice.String()
				targetPriceStr = &s
			}
			err = tx.QueryRow(txCtx, lineQuery,
				reqID, line.ProductID, line.ProductName, line.ProductSKU, line.Quantity,
				targetPriceStr, line.TargetDiscount, line.Status, line.Notes, line.CreatedAt, line.UpdatedAt,
			).Scan(&line.ID)
			if err != nil {
				return fmt.Errorf("insert purchase request line: %w", err)
			}
		}

		return nil
	})
}

// GetPurchaseRequestByID loads a purchase request with all its lines.
func (r *Repository) GetPurchaseRequestByID(ctx context.Context, id int64) (*commerce.PurchaseRequest, error) {
	var pr commerce.PurchaseRequest
	var estTotalStr *string
	var statusStr string

	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT pr.id, pr.public_id, pr.request_number, pr.customer_id, pr.organization_id, pr.branch_id,
			       pr.vendor_org_id, pr.vendor_branch_id, pr.status, pr.total_items, pr.estimated_total,
			       pr.buyer_notes, pr.vendor_notes, pr.created_at, pr.updated_at, pr.responded_at, pr.responded_by,
			       COALESCE(vo.name->>'ar', vo.name->>'en', '') as vendor_name, COALESCE(co.name->>'ar', co.name->>'en', '') as customer_name
			FROM commerce.purchase_requests pr
			LEFT JOIN org.organizations vo ON vo.id = pr.vendor_org_id
			LEFT JOIN org.organizations co ON co.id = pr.organization_id
			WHERE pr.id = $1;
		`

		err := tx.QueryRow(txCtx, query, id).Scan(
			&pr.ID, &pr.PublicID, &pr.RequestNumber, &pr.CustomerID, &pr.OrganizationID, &pr.BranchID,
			&pr.VendorOrgID, &pr.VendorBranchID, &statusStr, &pr.TotalItems, &estTotalStr,
			&pr.BuyerNotes, &pr.VendorNotes, &pr.CreatedAt, &pr.UpdatedAt, &pr.RespondedAt, &pr.RespondedBy,
			&pr.VendorName, &pr.CustomerName,
		)
		if err != nil {
			if err == pgx.ErrNoRows {
				return apperr.NotFound("purchase_request")
			}
			return fmt.Errorf("query purchase request: %w", err)
		}
		pr.Status = commerce.PurchaseRequestStatus(statusStr)
		if estTotalStr != nil {
			pr.EstimatedTotal, _ = money.Parse(*estTotalStr)
		}

		lines, err := r.listPurchaseRequestLinesTx(txCtx, tx, pr.ID)
		if err != nil {
			return err
		}
		pr.Lines = lines
		return nil
	})

	if err != nil {
		return nil, err
	}
	return &pr, nil
}

// GetPurchaseRequestByNumber loads a purchase request by human-readable request number.
func (r *Repository) GetPurchaseRequestByNumber(ctx context.Context, number string) (*commerce.PurchaseRequest, error) {
	var id int64
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `SELECT pr.id FROM commerce.purchase_requests pr WHERE pr.request_number = $1;`
		err := tx.QueryRow(txCtx, query, number).Scan(&id)
		if err != nil {
			if err == pgx.ErrNoRows {
				return apperr.NotFound("purchase_request")
			}
			return fmt.Errorf("query purchase request by number: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return r.GetPurchaseRequestByID(ctx, id)
}

// ListPurchaseRequestsByCustomer lists purchase requests placed by a customer.
func (r *Repository) ListPurchaseRequestsByCustomer(ctx context.Context, customerID int64, orgID *int64, status string, limit, offset int) ([]*commerce.PurchaseRequest, error) {
	var results []*commerce.PurchaseRequest

	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT pr.id, pr.public_id, pr.request_number, pr.customer_id, pr.organization_id, pr.branch_id,
			       pr.vendor_org_id, pr.vendor_branch_id, pr.status, pr.total_items, pr.estimated_total,
			       pr.buyer_notes, pr.vendor_notes, pr.created_at, pr.updated_at, pr.responded_at, pr.responded_by,
			       COALESCE(vo.name->>'ar', vo.name->>'en', '') as vendor_name, COALESCE(co.name->>'ar', co.name->>'en', '') as customer_name
			FROM commerce.purchase_requests pr
			LEFT JOIN org.organizations vo ON vo.id = pr.vendor_org_id
			LEFT JOIN org.organizations co ON co.id = pr.organization_id
			WHERE (pr.customer_id = $1 OR ($2::BIGINT IS NOT NULL AND pr.organization_id = $2))
			  AND ($3 = '' OR $3 = 'all' OR pr.status = $3)
			ORDER BY pr.created_at DESC
			LIMIT $4 OFFSET $5;
		`

		rows, err := tx.Query(txCtx, query, customerID, orgID, status, limit, offset)
		if err != nil {
			return fmt.Errorf("list customer purchase requests: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var pr commerce.PurchaseRequest
			var estTotalStr *string
			var statusStr string

			err := rows.Scan(
				&pr.ID, &pr.PublicID, &pr.RequestNumber, &pr.CustomerID, &pr.OrganizationID, &pr.BranchID,
				&pr.VendorOrgID, &pr.VendorBranchID, &statusStr, &pr.TotalItems, &estTotalStr,
				&pr.BuyerNotes, &pr.VendorNotes, &pr.CreatedAt, &pr.UpdatedAt, &pr.RespondedAt, &pr.RespondedBy,
				&pr.VendorName, &pr.CustomerName,
			)
			if err != nil {
				return fmt.Errorf("scan purchase request: %w", err)
			}
			pr.Status = commerce.PurchaseRequestStatus(statusStr)
			if estTotalStr != nil {
				pr.EstimatedTotal, _ = money.Parse(*estTotalStr)
			}
			results = append(results, &pr)
		}
		return rows.Err()
	})

	if err != nil {
		return nil, err
	}
	return results, nil
}

// CountPurchaseRequestsByCustomer returns status counts for a customer (Plan V5 Â§3.1).
func (r *Repository) CountPurchaseRequestsByCustomer(ctx context.Context, customerID int64, orgID *int64) (map[string]int, error) {
	counts := make(map[string]int)

	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT status, COUNT(*)
			FROM commerce.purchase_requests
			WHERE (customer_id = $1 OR ($2::BIGINT IS NOT NULL AND organization_id = $2))
			GROUP BY status;
		`

		rows, err := tx.Query(txCtx, query, customerID, orgID)
		if err != nil {
			return fmt.Errorf("count purchase requests: %w", err)
		}
		defer rows.Close()

		total := 0
		for rows.Next() {
			var status string
			var count int
			if err := rows.Scan(&status, &count); err != nil {
				return err
			}
			counts[status] = count
			total += count
		}
		counts["all"] = total
		return rows.Err()
	})

	if err != nil {
		return nil, err
	}
	return counts, nil
}

// UpdatePurchaseRequestStatus records vendor status updates and notes.
func (r *Repository) UpdatePurchaseRequestStatus(ctx context.Context, id int64, status commerce.PurchaseRequestStatus, vendorNotes string, responderID *int64) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		now := time.Now().UTC()
		query := `
			UPDATE commerce.purchase_requests
			SET status = $2, vendor_notes = COALESCE(NULLIF($3, ''), vendor_notes),
			    responded_at = $4, responded_by = $5, updated_at = $4
			WHERE id = $1;
		`
		tag, err := tx.Exec(txCtx, query, id, string(status), vendorNotes, now, responderID)
		if err != nil {
			return fmt.Errorf("update purchase request status: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("purchase_request")
		}
		return nil
	})
}

// UpdatePurchaseRequestLineOffer updates vendor counter-offered price or discount for a line item.
func (r *Repository) UpdatePurchaseRequestLineOffer(ctx context.Context, lineID int64, price money.Amount, discount float64, status string) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		now := time.Now().UTC()
		var priceStr *string
		if price.IsPositive() {
			s := price.String()
			priceStr = &s
		}

		actor, ok := authctx.From(ctx)
		if ok && !actor.IsStaff && !actor.Can("commerce.admin") && actor.OrganizationID > 0 {
			query := `
				UPDATE commerce.purchase_request_lines
				SET offered_price = $2, offered_discount = $3, status = $4, updated_at = $5
				WHERE id = $1 AND request_id IN (
					SELECT id FROM commerce.purchase_requests WHERE vendor_org_id = $6
				);
			`
			tag, err := tx.Exec(txCtx, query, lineID, priceStr, discount, status, now, actor.OrganizationID)
			if err != nil {
				return fmt.Errorf("update purchase request line offer: %w", err)
			}
			if tag.RowsAffected() == 0 {
				return apperr.NotFound("purchase_request_line")
			}
			return nil
		}

		query := `
			UPDATE commerce.purchase_request_lines
			SET offered_price = $2, offered_discount = $3, status = $4, updated_at = $5
			WHERE id = $1;
		`
		tag, err := tx.Exec(txCtx, query, lineID, priceStr, discount, status, now)
		if err != nil {
			return fmt.Errorf("update purchase request line offer: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("purchase_request_line")
		}
		return nil
	})
}

func (r *Repository) listPurchaseRequestLinesTx(ctx context.Context, tx pgx.Tx, requestID int64) ([]*commerce.PurchaseRequestLine, error) {
	query := `
		SELECT id, request_id, product_id, product_name, product_sku, quantity,
		       target_price, target_discount, offered_price, offered_discount, status, notes,
		       created_at, updated_at
		FROM commerce.purchase_request_lines
		WHERE request_id = $1
		ORDER BY id ASC;
	`

	rows, err := tx.Query(ctx, query, requestID)
	if err != nil {
		return nil, fmt.Errorf("list purchase request lines: %w", err)
	}
	defer rows.Close()

	var lines []*commerce.PurchaseRequestLine
	for rows.Next() {
		var l commerce.PurchaseRequestLine
		var tpStr, opStr *string

		err := rows.Scan(
			&l.ID, &l.RequestID, &l.ProductID, &l.ProductName, &l.ProductSKU, &l.Quantity,
			&tpStr, &l.TargetDiscount, &opStr, &l.OfferedDiscount, &l.Status, &l.Notes,
			&l.CreatedAt, &l.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan line: %w", err)
		}
		if tpStr != nil {
			l.TargetPrice, _ = money.Parse(*tpStr)
		}
		if opStr != nil {
			l.OfferedPrice, _ = money.Parse(*opStr)
		}
		lines = append(lines, &l)
	}
	return lines, rows.Err()
}
