package postgres

import (
	"context"
	"math"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// AddReview adds a review for an organization with individual criteria ratings.
func (r *Repository) AddReview(ctx context.Context, rev *org.Review) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if len(rev.Ratings) > 0 {
			total := 0
			for _, rating := range rev.Ratings {
				total += rating.Score
			}
			rev.Rating = int(math.Round(float64(total) / float64(len(rev.Ratings))))
		}
		if rev.Rating < 1 {
			rev.Rating = 5
		}
		if rev.Context == "" {
			rev.Context = "supplier"
		}
		query := `
			INSERT INTO org.organization_reviews (
				organization_id, user_id, order_id, shipment_id, reviewer_org_id,
				rating, review_text, is_approved, is_public, status, context
			) VALUES (
				$1, $2, $3, $4, $5,
				$6, $7, $8, true, 'approved', $9
			)
			RETURNING id, public_id, created_at, updated_at;
		`
		err := tx.QueryRow(txCtx, query,
			rev.OrganizationID, rev.UserID, rev.OrderID, rev.ShipmentID, rev.ReviewerOrgID,
			rev.Rating, rev.ReviewText, rev.IsApproved, rev.Context,
		).Scan(&rev.ID, &rev.PublicID, &rev.CreatedAt, &rev.UpdatedAt)
		if err != nil {
			return err
		}

		for _, rr := range rev.Ratings {
			crit := rr.Criterion
			if crit == "" {
				continue
			}
			qRating := `
				INSERT INTO org.review_ratings (review_id, criterion, score)
				VALUES ($1, $2, $3)
				ON CONFLICT (review_id, criterion) DO UPDATE SET score = EXCLUDED.score;
			`
			if _, err := tx.Exec(txCtx, qRating, rev.ID, crit, rr.Score); err != nil {
				return err
			}
		}
		return nil
	})
}

// ListReviewsByOrg returns approved reviews for an organization, joining reviewer organization name and ratings.
func (r *Repository) ListReviewsByOrg(ctx context.Context, orgID int64, limit, offset int) ([]*org.Review, error) {
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
			WHERE r.organization_id = $1 AND r.is_approved = true AND r.deleted_at IS NULL
			ORDER BY r.created_at DESC
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

		var reviewIDs []int64
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
	return list, err
}

// ToggleFollower toggles follower status for a user and organization.
func (r *Repository) ToggleFollower(ctx context.Context, orgID, userID int64) (bool, error) {
	var following bool
	err := r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		queryCheck := `DELETE FROM org.organization_followers WHERE organization_id = $1 AND user_id = $2;`
		tag, err := tx.Exec(txCtx, queryCheck, orgID, userID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() > 0 {
			following = false
			return nil
		}
		queryInsert := `INSERT INTO org.organization_followers (organization_id, user_id) VALUES ($1, $2);`
		_, err = tx.Exec(txCtx, queryInsert, orgID, userID)
		following = (err == nil)
		return err
	})
	return following, err
}

// IsFollowing checks if a user follows an organization.
func (r *Repository) IsFollowing(ctx context.Context, orgID, userID int64) (bool, error) {
	var exists bool
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `SELECT EXISTS(SELECT 1 FROM org.organization_followers WHERE organization_id = $1 AND user_id = $2);`
		return tx.QueryRow(txCtx, query, orgID, userID).Scan(&exists)
	})
	return exists, err
}

// ListFollowedOrgs returns all organizations followed by a user.
func (r *Repository) ListFollowedOrgs(ctx context.Context, userID int64) ([]*org.Organization, error) {
	var orgs []*org.Organization
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT o.id, o.public_id, o.legal_name, o.trade_name, o.tax_number, o.commercial_register,
			       o.type, o.status, o.credit_limit, o.payment_terms_days, o.created_at, o.updated_at
			FROM org.organizations o
			JOIN org.organization_followers f ON o.id = f.organization_id
			WHERE f.user_id = $1
			ORDER BY f.created_at DESC;
		`
		rows, err := tx.Query(txCtx, query, userID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var o org.Organization
			var typeStr, statusStr string
			if err := rows.Scan(
				&o.ID, &o.PublicID, &o.LegalName, &o.TradeName, &o.TaxNumber, &o.CommercialRegister,
				&typeStr, &statusStr, &o.CreditLimit, &o.PaymentTermsDays, &o.CreatedAt, &o.UpdatedAt,
			); err != nil {
				return err
			}
			o.Type = org.OrganizationType(typeStr)
			o.Status = org.OrganizationStatus(statusStr)
			orgs = append(orgs, &o)
		}
		return rows.Err()
	})
	return orgs, err
}

// CreatePolicy creates an organization policy.
func (r *Repository) CreatePolicy(ctx context.Context, p *org.Policy) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO org.organization_policies (organization_id, title, content, policy_type, is_active)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING id, public_id, created_at, updated_at;
		`
		if p.PolicyType == "" {
			p.PolicyType = org.PolicyTypeTerms
		}
		if p.PolicyType == org.PolicyTypeWarranty {
			subTx, subErr := tx.Begin(txCtx)
			if subErr == nil {
				err := subTx.QueryRow(txCtx, query, p.OrganizationID, p.Title, p.Content, p.PolicyType, p.IsActive).
					Scan(&p.ID, &p.PublicID, &p.CreatedAt, &p.UpdatedAt)
				if err != nil {
					_ = subTx.Rollback(txCtx)
					p.PolicyType = org.PolicyTypePrivacy
					return tx.QueryRow(txCtx, query, p.OrganizationID, p.Title, p.Content, p.PolicyType, p.IsActive).
						Scan(&p.ID, &p.PublicID, &p.CreatedAt, &p.UpdatedAt)
				}
				return subTx.Commit(txCtx)
			}
		}
		return tx.QueryRow(txCtx, query, p.OrganizationID, p.Title, p.Content, p.PolicyType, p.IsActive).
			Scan(&p.ID, &p.PublicID, &p.CreatedAt, &p.UpdatedAt)
	})
}

// ListPoliciesByOrg lists policies for an organization.
func (r *Repository) ListPoliciesByOrg(ctx context.Context, orgID int64) ([]*org.Policy, error) {
	var list []*org.Policy
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `SELECT id, public_id, organization_id, title, content, policy_type, is_active, created_at, updated_at FROM org.organization_policies WHERE organization_id = $1 AND is_active = true;`
		rows, err := tx.Query(txCtx, query, orgID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var p org.Policy
			if err := rows.Scan(&p.ID, &p.PublicID, &p.OrganizationID, &p.Title, &p.Content, &p.PolicyType, &p.IsActive, &p.CreatedAt, &p.UpdatedAt); err != nil {
				return err
			}
			list = append(list, &p)
		}
		return rows.Err()
	})
	return list, err
}

// SavePolicies replaces the policies for an organization with the given set.
func (r *Repository) SavePolicies(ctx context.Context, orgID int64, policies []*org.Policy) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(txCtx, `DELETE FROM org.organization_policies WHERE organization_id = $1;`, orgID); err != nil {
			return err
		}
		for _, p := range policies {
			query := `
				INSERT INTO org.organization_policies (organization_id, title, content, policy_type, is_active)
				VALUES ($1, $2, $3, $4, $5)
				RETURNING id, public_id, created_at, updated_at;
			`
			if p.PolicyType == "" {
				p.PolicyType = org.PolicyTypeTerms
			}
			var err error
			if p.PolicyType == org.PolicyTypeWarranty {
				subTx, subErr := tx.Begin(txCtx)
				if subErr == nil {
					err = subTx.QueryRow(txCtx, query, orgID, p.Title, p.Content, p.PolicyType, p.IsActive).
						Scan(&p.ID, &p.PublicID, &p.CreatedAt, &p.UpdatedAt)
					if err != nil {
						_ = subTx.Rollback(txCtx)
						p.PolicyType = org.PolicyTypePrivacy
						err = tx.QueryRow(txCtx, query, orgID, p.Title, p.Content, p.PolicyType, p.IsActive).
							Scan(&p.ID, &p.PublicID, &p.CreatedAt, &p.UpdatedAt)
					} else {
						err = subTx.Commit(txCtx)
					}
				} else {
					err = tx.QueryRow(txCtx, query, orgID, p.Title, p.Content, p.PolicyType, p.IsActive).
						Scan(&p.ID, &p.PublicID, &p.CreatedAt, &p.UpdatedAt)
				}
			} else {
				err = tx.QueryRow(txCtx, query, orgID, p.Title, p.Content, p.PolicyType, p.IsActive).
					Scan(&p.ID, &p.PublicID, &p.CreatedAt, &p.UpdatedAt)
			}
			if err != nil {
				return err
			}
			p.OrganizationID = orgID
		}
		return nil
	})
}

// ListSocialMediaByOrg lists social media accounts for an organization.
func (r *Repository) ListSocialMediaByOrg(ctx context.Context, orgID int64) ([]*org.SocialMedia, error) {
	var list []*org.SocialMedia
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `SELECT id, public_id, organization_id, platform, url, created_at, updated_at FROM org.organization_social_media WHERE organization_id = $1 ORDER BY id ASC;`
		rows, err := tx.Query(txCtx, query, orgID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var s org.SocialMedia
			if err := rows.Scan(&s.ID, &s.PublicID, &s.OrganizationID, &s.Platform, &s.URL, &s.CreatedAt, &s.UpdatedAt); err != nil {
				return err
			}
			list = append(list, &s)
		}
		return rows.Err()
	})
	return list, err
}

// SaveSocialMedia replaces social media channels for an organization.
func (r *Repository) SaveSocialMedia(ctx context.Context, orgID int64, links []*org.SocialMedia) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(txCtx, `DELETE FROM org.organization_social_media WHERE organization_id = $1;`, orgID); err != nil {
			return err
		}
		for _, l := range links {
			if l.Platform == "" || l.URL == "" {
				continue
			}
			query := `
				INSERT INTO org.organization_social_media (organization_id, platform, url)
				VALUES ($1, $2, $3)
				RETURNING id, public_id, created_at, updated_at;
			`
			if err := tx.QueryRow(txCtx, query, orgID, l.Platform, l.URL).
				Scan(&l.ID, &l.PublicID, &l.CreatedAt, &l.UpdatedAt); err != nil {
				return err
			}
			l.OrganizationID = orgID
		}
		return nil
	})
}
