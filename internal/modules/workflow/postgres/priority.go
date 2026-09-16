package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// ListPriorityRequestsByUser lists past priority engine runs for a user/organization.
func (r *Repository) ListPriorityRequestsByUser(ctx context.Context, userID int64, limit, offset int) ([]*workflow.PurchasePriorityRequest, error) {
	var list []*workflow.PurchasePriorityRequest

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, user_id, organization_id, request_number, status,
			       priority_highest_discount, priority_lowest_price, priority_fastest_delivery,
			       priority_preferred_suppliers_only, budget_constraint, parameters, recommendations,
			       processed_at, created_at, updated_at
			FROM workflow.purchase_priority_engines
			WHERE user_id = $1
			ORDER BY created_at DESC
			LIMIT $2 OFFSET $3;
		`
		rows, err := tx.Query(txCtx, query, userID, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var req workflow.PurchasePriorityRequest
			var paramsJSON, recomJSON []byte
			var budgetStr *string
			err := rows.Scan(
				&req.ID, &req.PublicID, &req.UserID, &req.OrganizationID, &req.RequestNumber, &req.Status,
				&req.PriorityHighestDiscount, &req.PriorityLowestPrice, &req.PriorityFastestDelivery,
				&req.PriorityPreferredSuppliersOnly, &budgetStr, &paramsJSON, &recomJSON,
				&req.ProcessedAt, &req.CreatedAt, &req.UpdatedAt,
			)
			if err != nil {
				return err
			}
			if budgetStr != nil {
				req.BudgetConstraint, _ = money.Parse(*budgetStr)
			}
			_ = json.Unmarshal(paramsJSON, &req.Parameters)
			_ = json.Unmarshal(recomJSON, &req.Recommendations)
			list = append(list, &req)
		}
		return rows.Err()
	})

	if err != nil {
		return nil, fmt.Errorf("list priority requests: %w", err)
	}
	return list, nil
}

// UpdatePriorityRequestStatus marks the engine execution status with ranking results and recommendations.
func (r *Repository) UpdatePriorityRequestStatus(ctx context.Context, id int64, status string, notes string, processedBy *int64, results map[string]any) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		now := time.Now().UTC()
		var recomJSON, rankJSON []byte

		if recs, ok := results["recommendations"]; ok {
			recomJSON, _ = json.Marshal(recs)
		} else {
			recomJSON = []byte("{}")
		}

		if ranks, ok := results["ranked_products"]; ok {
			rankJSON, _ = json.Marshal(ranks)
		} else {
			rankJSON = []byte("[]")
		}

		query := `
			UPDATE workflow.purchase_priority_engines
			SET status = $2, notes = COALESCE(NULLIF($3, ''), notes),
			    processed_at = $4, processed_by = $5, recommendations = $6,
			    ranking_results = $7, updated_at = $4
			WHERE id = $1;
		`
		tag, err := tx.Exec(txCtx, query, id, status, notes, now, processedBy, recomJSON, rankJSON)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("priority_request")
		}
		return nil
	})
}

// GetCandidateProducts queries catalog.product_index with institutional Simple mode filtering.
func (r *Repository) GetCandidateProducts(ctx context.Context, userID int64, authorizedWorkIDs []int64, preferredSupplierIDs []int64, budget *money.Amount, limit int) ([]workflow.CandidateProduct, error) {
	var candidates []workflow.CandidateProduct

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT product_id, name_ar, price, discount, sku,
			       organization_id, branch_id, stock_quantity, organization_name,
			       institutional_work_ids
			FROM catalog.product_index
			WHERE status = 'active'
			  AND stock_quantity > 0
			  AND ($1::BIGINT[] IS NULL OR cardinality($1::BIGINT[]) = 0 OR institutional_work_ids IS NULL OR cardinality(institutional_work_ids) = 0 OR institutional_work_ids && $1)
			  AND ($2::NUMERIC IS NULL OR price <= $2 OR (price - discount) <= $2)
			  AND ($3::BIGINT[] IS NULL OR cardinality($3::BIGINT[]) = 0 OR organization_id = ANY($3))
			ORDER BY product_id ASC
			LIMIT $4;
		`

		var budgetVal *string
		if budget != nil && budget.IsPositive() {
			s := budget.String()
			budgetVal = &s
		}

		rows, err := tx.Query(txCtx, query, authorizedWorkIDs, budgetVal, preferredSupplierIDs, limit)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var cp workflow.CandidateProduct
			var priceStr, discStr string
			err := rows.Scan(
				&cp.ProductID, &cp.ProductName, &priceStr, &discStr, &cp.ProductSKU,
				&cp.OrganizationID, &cp.BranchID, &cp.StockQuantity, &cp.OrganizationName,
				&cp.InstitutionalWorkIDs,
			)
			if err != nil {
				return err
			}
			cp.ProductPrice, _ = money.Parse(priceStr)
			cp.ProductPriceDiscount, _ = money.Parse(discStr)
			cp.EstimatedDelivery = 1 // default 1 day
			candidates = append(candidates, cp)
		}
		return rows.Err()
	})

	if err != nil {
		return nil, fmt.Errorf("get candidate products: %w", err)
	}
	return candidates, nil
}
