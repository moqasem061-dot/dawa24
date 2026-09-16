package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// ListReviewsForVendor returns reviews received by a vendor, with criteria ratings and order information.
func (r *Repository) ListReviewsForVendor(ctx context.Context, vendorOrgID int64, limit, offset int) ([]*org.Review, error) {
	return r.ListReviewsByOrg(ctx, vendorOrgID, limit, offset)
}

// GetReviewByOrderAndVendor returns the review for a specific vendor on an order, if any.
func (r *Repository) GetReviewByOrderAndVendor(ctx context.Context, orderID, vendorOrgID int64) (*org.Review, error) {
	var rev org.Review
	var revText, response, orderNum *string
	var orgName string

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT r.id, r.public_id, r.organization_id, r.user_id, r.order_id, r.shipment_id, r.reviewer_org_id,
			       r.rating, r.review_text, r.response, r.response_at, r.responded_by,
			       r.is_approved, r.created_at, r.updated_at,
			       COALESCE(NULLIF(o.trade_name->>'ar', ''), NULLIF(o.trade_name->>'en', ''), NULLIF(o.name->>'ar', ''), NULLIF(o.name->>'en', ''), 'صيدلية معتمدة') AS reviewer_org_name,
			       COALESCE(ord.order_number, '') AS order_number
			FROM org.organization_reviews r
			LEFT JOIN org.organizations o ON o.id = COALESCE(r.reviewer_org_id, (SELECT organization_id FROM org.members WHERE user_id = r.user_id LIMIT 1))
			LEFT JOIN commerce.orders ord ON ord.id = r.order_id
			WHERE r.order_id = $1 AND r.organization_id = $2 AND r.deleted_at IS NULL
			LIMIT 1;
		`
		err := tx.QueryRow(txCtx, query, orderID, vendorOrgID).Scan(
			&rev.ID, &rev.PublicID, &rev.OrganizationID, &rev.UserID, &rev.OrderID, &rev.ShipmentID, &rev.ReviewerOrgID,
			&rev.Rating, &revText, &response, &rev.ResponseAt, &rev.RespondedBy,
			&rev.IsApproved, &rev.CreatedAt, &rev.UpdatedAt, &orgName, &orderNum,
		)
		if err != nil {
			return err
		}
		if revText != nil {
			rev.ReviewText = *revText
		}
		if response != nil {
			rev.Response = *response
		}
		if orderNum != nil {
			rev.OrderNumber = *orderNum
		}
		rev.ReviewerOrgName = orgName

		// Load criteria ratings
		ratRows, ratErr := tx.Query(txCtx, `SELECT criterion, score FROM org.review_ratings WHERE review_id = $1;`, rev.ID)
		if ratErr == nil {
			defer ratRows.Close()
			for ratRows.Next() {
				var rr org.ReviewRating
				rr.ReviewID = rev.ID
				if err := ratRows.Scan(&rr.Criterion, &rr.Score); err == nil {
					rev.Ratings = append(rev.Ratings, rr)
					switch rr.Criterion {
					case "rep":
						rev.ScoreRep = rr.Score
					case "quality":
						rev.ScoreQuality = rr.Score
					case "speed":
						rev.ScoreSpeed = rr.Score
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &rev, nil
}

// ListReviewsForOrder returns all vendor reviews submitted for a given master order.
func (r *Repository) ListReviewsForOrder(ctx context.Context, orderID int64) ([]*org.Review, error) {
	var list []*org.Review
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT r.id, r.public_id, r.organization_id, r.user_id, r.order_id, r.shipment_id, r.reviewer_org_id,
			       r.rating, r.review_text, r.response, r.response_at, r.responded_by,
			       r.is_approved, r.created_at, r.updated_at,
			       COALESCE(NULLIF(o.trade_name->>'ar', ''), NULLIF(o.trade_name->>'en', ''), NULLIF(o.name->>'ar', ''), NULLIF(o.name->>'en', ''), 'صيدلية معتمدة') AS reviewer_org_name,
			       COALESCE(ord.order_number, '') AS order_number
			FROM org.organization_reviews r
			LEFT JOIN org.organizations o ON o.id = COALESCE(r.reviewer_org_id, (SELECT organization_id FROM org.members WHERE user_id = r.user_id LIMIT 1))
			LEFT JOIN commerce.orders ord ON ord.id = r.order_id
			WHERE r.order_id = $1 AND r.deleted_at IS NULL;
		`
		rows, err := tx.Query(txCtx, query, orderID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var rev org.Review
			var revText, response, orderNum *string
			var orgName string
			if err := rows.Scan(
				&rev.ID, &rev.PublicID, &rev.OrganizationID, &rev.UserID, &rev.OrderID, &rev.ShipmentID, &rev.ReviewerOrgID,
				&rev.Rating, &revText, &response, &rev.ResponseAt, &rev.RespondedBy,
				&rev.IsApproved, &rev.CreatedAt, &rev.UpdatedAt, &orgName, &orderNum,
			); err != nil {
				return err
			}
			if revText != nil {
				rev.ReviewText = *revText
			}
			if response != nil {
				rev.Response = *response
			}
			if orderNum != nil {
				rev.OrderNumber = *orderNum
			}
			rev.ReviewerOrgName = orgName
			list = append(list, &rev)
		}
		return rows.Err()
	})
	return list, err
}

// HasDeliveredOrderFromVendor reports whether a customer organization has any delivered or completed orders from a vendor.
func (r *Repository) HasDeliveredOrderFromVendor(ctx context.Context, customerOrgID, vendorOrgID int64) (bool, error) {
	var count int
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT COUNT(1)
			FROM commerce.orders o
			JOIN commerce.order_shipments s ON s.order_id = o.id
			WHERE o.customer_id = $1 AND s.organization_id = $2
			  AND s.status IN ('delivered', 'completed');
		`
		return tx.QueryRow(txCtx, query, customerOrgID, vendorOrgID).Scan(&count)
	})
	return count > 0, err
}
