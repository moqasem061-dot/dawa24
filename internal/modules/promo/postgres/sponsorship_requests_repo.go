package postgres

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/storage"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// ExpireSponsorshipRequests marks requests past their expiry as expired and purges any associated offer media.
func (r *Repository) ExpireSponsorshipRequests(ctx context.Context) (int64, error) {
	var (
		n         int64
		mediaURLs []string
	)
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		// Collect offer IDs from expiring requests
		rows, err := tx.Query(txCtx, `
			SELECT item_id
			FROM promo.sponsorship_requests
			WHERE status = 'active' AND expires_at < now() AND item_type = 'offer';
		`)
		if err != nil {
			return err
		}
		var offerIDs []int64
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err == nil && id > 0 {
				offerIDs = append(offerIDs, id)
			}
		}
		rows.Close()

		if len(offerIDs) > 0 {
			imgRows, err := tx.Query(txCtx, `SELECT image FROM promo.offers WHERE id = ANY($1) AND image <> '';`, offerIDs)
			if err == nil {
				for imgRows.Next() {
					var img string
					if err := imgRows.Scan(&img); err == nil && strings.TrimSpace(img) != "" {
						mediaURLs = append(mediaURLs, strings.TrimSpace(img))
					}
				}
				imgRows.Close()
			}
			_, _ = tx.Exec(txCtx, `UPDATE promo.offers SET image = '', is_active = false, updated_at = now() WHERE id = ANY($1);`, offerIDs)
		}

		tag, err := tx.Exec(txCtx, `
			UPDATE promo.sponsorship_requests
			SET status = 'expired', updated_at = now()
			WHERE status = 'active' AND expires_at < now();
		`)
		if err != nil {
			return err
		}
		n = tag.RowsAffected()

		_, err = tx.Exec(txCtx, `
			UPDATE promo.offer_sponsorships
			SET status = 'expired'
			WHERE status = 'active' AND expires_at < now();
		`)
		if err != nil {
			return err
		}
		return nil
	})
	if err == nil {
		for _, u := range mediaURLs {
			_ = storage.DeleteUploadedMedia(ctx, u, nil)
		}
	}
	return n, err
}

const sponsorshipRequestColumns = `
SELECT sr.id, sr.public_id, sr.organization_id, sr.purchase_id, sr.package_id,
       sr.item_type, sr.item_id, sr.credits_used, sr.admin_status, COALESCE(sr.admin_notes, ''),
       sr.reviewed_by, sr.reviewed_at, sr.starts_at, sr.expires_at, sr.status,
       sr.created_at, sr.updated_at,
       op.tier_level, op.name, op.credits, op.duration_days
FROM promo.sponsorship_requests sr
LEFT JOIN promo.offer_packages op ON op.id = sr.package_id`

func scanSponsorshipRequest(row pgx.Row, sr *promo.SponsorshipRequest) error {
	var (
		pkgTier    *int
		pkgName    i18n.Text
		pkgCredits *int
		pkgDur     *int
	)
	err := row.Scan(
		&sr.ID, &sr.PublicID, &sr.OrganizationID, &sr.PurchaseID, &sr.PackageID,
		&sr.ItemType, &sr.ItemID, &sr.CreditsUsed, &sr.AdminStatus, &sr.AdminNotes,
		&sr.ReviewedBy, &sr.ReviewedAt, &sr.StartsAt, &sr.ExpiresAt, &sr.Status,
		&sr.CreatedAt, &sr.UpdatedAt,
		&pkgTier, &pkgName, &pkgCredits, &pkgDur,
	)
	if err != nil {
		return err
	}
	if pkgTier != nil {
		sr.Package = &promo.OfferPackage{
			ID:           sr.PackageID,
			TierLevel:    *pkgTier,
			Name:         pkgName,
			Credits:      derefInt(pkgCredits),
			DurationDays: derefInt(pkgDur),
		}
	}
	return nil
}

func derefInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

// ListAllSponsorshipRequestsWithTotal returns all requests for admin moderation with total count.
func (r *Repository) ListAllSponsorshipRequestsWithTotal(ctx context.Context, limit, offset int) ([]*promo.SponsorshipRequest, int, error) {
	return r.listSponsorshipRequestsWithTotal(database.AsSystem(ctx), ``, int64(0), limit, offset)
}

// ListSponsorshipRequestsByOrgWithTotal returns paginated requests for an organization with total count.
func (r *Repository) ListSponsorshipRequestsByOrgWithTotal(ctx context.Context, orgID int64, limit, offset int) ([]*promo.SponsorshipRequest, int, error) {
	return r.listSponsorshipRequestsWithTotal(ctx, `WHERE sr.organization_id = $1`, orgID, limit, offset)
}

func (r *Repository) listSponsorshipRequestsWithTotal(ctx context.Context, where string, orgID int64, limit, offset int) ([]*promo.SponsorshipRequest, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 25
	}
	if offset < 0 {
		offset = 0
	}
	var list []*promo.SponsorshipRequest
	var total int

	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		countQuery := `SELECT count(*) FROM promo.sponsorship_requests sr ` + where + `;`
		var err error
		if orgID > 0 {
			err = tx.QueryRow(txCtx, countQuery, orgID).Scan(&total)
		} else {
			err = tx.QueryRow(txCtx, countQuery).Scan(&total)
		}
		if err != nil {
			return err
		}

		query := sponsorshipRequestColumns + ` ` + where + ` ORDER BY sr.created_at DESC, sr.id DESC LIMIT $` + whereParam(orgID) + ` OFFSET $` + whereParam2(orgID) + `;`
		var rows pgx.Rows
		if orgID > 0 {
			rows, err = tx.Query(txCtx, query, orgID, limit, offset)
		} else {
			rows, err = tx.Query(txCtx, query, limit, offset)
		}
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var sr promo.SponsorshipRequest
			if err := scanSponsorshipRequest(rows, &sr); err != nil {
				return err
			}
			list = append(list, &sr)
		}
		return rows.Err()
	})
	return list, total, err
}
