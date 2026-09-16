package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// ListAdminReviewsWithTotal returns paginated reviews across the platform matching filters.
func (r *Repository) ListAdminReviewsWithTotal(ctx context.Context, filter org.AdminReviewFilter) ([]*org.Review, int, error) {
	var list []*org.Review
	var total int

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var whereParts []string
		var args []any
		argIdx := 1

		whereParts = append(whereParts, "r.deleted_at IS NULL")

		if filter.VendorOrgID != nil && *filter.VendorOrgID > 0 {
			whereParts = append(whereParts, fmt.Sprintf("r.organization_id = $%d", argIdx))
			args = append(args, *filter.VendorOrgID)
			argIdx++
		}

		if filter.ReviewerOrgID != nil && *filter.ReviewerOrgID > 0 {
			whereParts = append(whereParts, fmt.Sprintf("COALESCE(r.reviewer_org_id, (SELECT organization_id FROM org.members WHERE user_id = r.user_id LIMIT 1)) = $%d", argIdx))
			args = append(args, *filter.ReviewerOrgID)
			argIdx++
		}

		if filter.Rating != nil && *filter.Rating > 0 {
			whereParts = append(whereParts, fmt.Sprintf("r.rating = $%d", argIdx))
			args = append(args, *filter.Rating)
			argIdx++
		}

		switch filter.Status {
		case "approved":
			whereParts = append(whereParts, "r.is_approved = true")
		case "pending":
			whereParts = append(whereParts, "r.is_approved = false")
		}

		if filter.Search != "" {
			pattern := "%" + strings.TrimSpace(filter.Search) + "%"
			whereParts = append(whereParts, fmt.Sprintf(
				"(r.review_text ILIKE $%d OR ord.order_number ILIKE $%d OR COALESCE(o.trade_name->>'ar', o.name->>'ar', '') ILIKE $%d OR COALESCE(rev_o.trade_name->>'ar', rev_o.name->>'ar', '') ILIKE $%d)",
				argIdx, argIdx, argIdx, argIdx,
			))
			args = append(args, pattern)
			argIdx++
		}

		whereClause := strings.Join(whereParts, " AND ")

		countQuery := fmt.Sprintf(`
			SELECT COUNT(*)
			FROM org.organization_reviews r
			LEFT JOIN org.organizations o ON o.id = r.organization_id
			LEFT JOIN org.organizations rev_o ON rev_o.id = COALESCE(r.reviewer_org_id, (SELECT organization_id FROM org.members WHERE user_id = r.user_id LIMIT 1))
			LEFT JOIN commerce.orders ord ON ord.id = r.order_id
			WHERE %s;
		`, whereClause)

		if err := tx.QueryRow(txCtx, countQuery, args...).Scan(&total); err != nil {
			return err
		}

		limit := filter.Limit
		if limit <= 0 || limit > 100 {
			limit = 25
		}
		offset := filter.Offset
		if offset < 0 {
			offset = 0
		}

		queryArgs := append(args, limit, offset)
		limitArgIdx := argIdx
		offsetArgIdx := argIdx + 1

		dataQuery := fmt.Sprintf(`
			SELECT r.id, r.public_id, r.organization_id, r.user_id, r.order_id, r.shipment_id, r.reviewer_org_id,
			       r.rating, r.review_text, r.response, r.response_at, r.responded_by,
			       r.is_approved, r.created_at, r.updated_at,
			       COALESCE(NULLIF(o.trade_name->>'ar', ''), NULLIF(o.trade_name->>'en', ''), NULLIF(o.name->>'ar', ''), NULLIF(o.name->>'en', ''), 'مورد معتمد') AS vendor_org_name,
			       COALESCE(NULLIF(rev_o.trade_name->>'ar', ''), NULLIF(rev_o.trade_name->>'en', ''), NULLIF(rev_o.name->>'ar', ''), NULLIF(rev_o.name->>'en', ''), 'صيدلية معتمدة') AS reviewer_org_name,
			       COALESCE(ord.order_number, '') AS order_number
			FROM org.organization_reviews r
			LEFT JOIN org.organizations o ON o.id = r.organization_id
			LEFT JOIN org.organizations rev_o ON rev_o.id = COALESCE(r.reviewer_org_id, (SELECT organization_id FROM org.members WHERE user_id = r.user_id LIMIT 1))
			LEFT JOIN commerce.orders ord ON ord.id = r.order_id
			WHERE %s
			ORDER BY r.created_at DESC, r.id DESC
			LIMIT $%d OFFSET $%d;
		`, whereClause, limitArgIdx, offsetArgIdx)

		rows, err := tx.Query(txCtx, dataQuery, queryArgs...)
		if err != nil {
			return err
		}
		defer rows.Close()

		var reviewIDs []int64
		for rows.Next() {
			var rev org.Review
			var revText, response, orderNum *string
			var vendorName, reviewerName string
			if err := rows.Scan(
				&rev.ID, &rev.PublicID, &rev.OrganizationID, &rev.UserID, &rev.OrderID, &rev.ShipmentID, &rev.ReviewerOrgID,
				&rev.Rating, &revText, &response, &rev.ResponseAt, &rev.RespondedBy,
				&rev.IsApproved, &rev.CreatedAt, &rev.UpdatedAt,
				&vendorName, &reviewerName, &orderNum,
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
			rev.VendorOrgName = vendorName
			rev.ReviewerOrgName = reviewerName
			list = append(list, &rev)
			reviewIDs = append(reviewIDs, rev.ID)
		}
		if rows.Err() != nil {
			return rows.Err()
		}

		if len(reviewIDs) > 0 {
			ratQuery := `SELECT review_id, criterion, score FROM org.review_ratings WHERE review_id = ANY($1);`
			ratRows, ratErr := tx.Query(txCtx, ratQuery, reviewIDs)
			if ratErr == nil {
				defer ratRows.Close()
				ratMap := make(map[int64][]org.ReviewRating)
				for ratRows.Next() {
					var rr org.ReviewRating
					if err := ratRows.Scan(&rr.ReviewID, &rr.Criterion, &rr.Score); err == nil {
						ratMap[rr.ReviewID] = append(ratMap[rr.ReviewID], rr)
					}
				}
				for _, rev := range list {
					if ratings, ok := ratMap[rev.ID]; ok {
						rev.Ratings = ratings
						for _, r := range ratings {
							switch r.Criterion {
							case "rep":
								rev.ScoreRep = r.Score
							case "quality":
								rev.ScoreQuality = r.Score
							case "speed":
								rev.ScoreSpeed = r.Score
							}
						}
					}
				}
			}
		}

		return nil
	})

	return list, total, err
}

// GetAdminReviewStats returns platform-wide review KPIs.
func (r *Repository) GetAdminReviewStats(ctx context.Context) (*org.AdminReviewStats, error) {
	var stats org.AdminReviewStats

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT
				COUNT(*),
				COUNT(*) FILTER (WHERE is_approved = true),
				COUNT(*) FILTER (WHERE is_approved = false),
				COALESCE(AVG(rating), 0)
			FROM org.organization_reviews
			WHERE deleted_at IS NULL;
		`
		if err := tx.QueryRow(txCtx, query).Scan(
			&stats.TotalReviews,
			&stats.ApprovedCount,
			&stats.PendingCount,
			&stats.AverageRating,
		); err != nil {
			return err
		}

		criteriaQuery := `
			SELECT
				COALESCE(AVG(score) FILTER (WHERE criterion = 'rep'), 0),
				COALESCE(AVG(score) FILTER (WHERE criterion = 'quality'), 0),
				COALESCE(AVG(score) FILTER (WHERE criterion = 'speed'), 0)
			FROM org.review_ratings rr
			JOIN org.organization_reviews r ON r.id = rr.review_id
			WHERE r.deleted_at IS NULL;
		`
		return tx.QueryRow(txCtx, criteriaQuery).Scan(
			&stats.AvgScoreRep,
			&stats.AvgScoreQual,
			&stats.AvgScoreSpeed,
		)
	})

	return &stats, err
}

// UpdateReviewStatus modifies review approval/moderation status.
func (r *Repository) UpdateReviewStatus(ctx context.Context, reviewID int64, isApproved bool) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `UPDATE org.organization_reviews SET is_approved = $1, updated_at = now() WHERE id = $2 AND deleted_at IS NULL;`
		_, err := tx.Exec(txCtx, query, isApproved, reviewID)
		return err
	})
}

// SoftDeleteReview marks a review as deleted.
func (r *Repository) SoftDeleteReview(ctx context.Context, reviewID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `UPDATE org.organization_reviews SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL;`
		_, err := tx.Exec(txCtx, query, reviewID)
		return err
	})
}
