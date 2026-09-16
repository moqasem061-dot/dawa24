package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// ListPurchaseRequestsByVendor lists incoming purchase requests directed to a vendor.
func (r *Repository) ListPurchaseRequestsByVendor(ctx context.Context, vendorOrgID int64, status string, limit, offset int) ([]*commerce.PurchaseRequest, error) {
	reqs, _, err := r.ListPurchaseRequestsByVendorWithTotal(ctx, vendorOrgID, status, limit, offset)
	return reqs, err
}

// ListPurchaseRequestsByVendorWithTotal lists incoming purchase requests directed to a vendor with total count.
func (r *Repository) ListPurchaseRequestsByVendorWithTotal(ctx context.Context, vendorOrgID int64, status string, limit, offset int) ([]*commerce.PurchaseRequest, int, error) {
	var results []*commerce.PurchaseRequest
	var total int

	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		countQuery := `
			SELECT count(*)
			FROM commerce.purchase_requests pr
			WHERE pr.vendor_org_id = $1
			  AND ($2 = '' OR $2 = 'all' OR pr.status = $2);
		`
		if err := tx.QueryRow(txCtx, countQuery, vendorOrgID, status).Scan(&total); err != nil {
			return fmt.Errorf("count vendor purchase requests: %w", err)
		}

		query := `
			SELECT pr.id, pr.public_id, pr.request_number, pr.customer_id, pr.organization_id, pr.branch_id,
			       pr.vendor_org_id, pr.vendor_branch_id, pr.status, pr.total_items, pr.estimated_total,
			       pr.buyer_notes, pr.vendor_notes, pr.created_at, pr.updated_at, pr.responded_at, pr.responded_by,
			       COALESCE(vo.name->>'ar', vo.name->>'en', '') as vendor_name, COALESCE(co.name->>'ar', co.name->>'en', '') as customer_name
			FROM commerce.purchase_requests pr
			LEFT JOIN org.organizations vo ON vo.id = pr.vendor_org_id
			LEFT JOIN org.organizations co ON co.id = pr.organization_id
			WHERE pr.vendor_org_id = $1
			  AND ($2 = '' OR $2 = 'all' OR pr.status = $2)
			ORDER BY pr.created_at DESC, pr.id DESC
			LIMIT $3 OFFSET $4;
		`

		rows, err := tx.Query(txCtx, query, vendorOrgID, status, limit, offset)
		if err != nil {
			return fmt.Errorf("list vendor purchase requests: %w", err)
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
				return fmt.Errorf("scan vendor purchase request: %w", err)
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
		return nil, 0, err
	}
	return results, total, nil
}
