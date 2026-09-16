package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// RankedSponsorshipsForProducts returns the active, approved sponsorships for
// the given product IDs, ranked by package tier level (highest first). Ties at
// the same tier are returned in arbitrary order — the caller shuffles them.
func (r *Repository) RankedSponsorshipsForProducts(ctx context.Context, productIDs []int64) ([]*promo.RankedSponsorship, error) {
	return r.rankedSponsorships(ctx, promo.SponsorItemProduct, productIDs)
}

// RankedSponsorshipsForOffers returns the active, approved sponsorships for
// the given offer IDs.
func (r *Repository) RankedSponsorshipsForOffers(ctx context.Context, offerIDs []int64) ([]*promo.RankedSponsorship, error) {
	return r.rankedSponsorships(ctx, promo.SponsorItemOffer, offerIDs)
}

// ListActiveRankedSponsorships returns all active, approved sponsorships for an item type,
// ordered by package tier level (highest first), breaking ties at the same tier randomly.
func (r *Repository) ListActiveRankedSponsorships(ctx context.Context, itemType promo.SponsorshipItemType) ([]*promo.RankedSponsorship, error) {
	var list []*promo.RankedSponsorship
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT os.item_id, os.organization_id, os.package_id, op.tier_level, os.expires_at, os.item_type
			FROM (
				SELECT COALESCE(item_id, offer_id) AS item_id, organization_id, package_id, expires_at, item_type
				FROM promo.offer_sponsorships
				WHERE status = 'active'
				  AND admin_status = 'approved'
				  AND expires_at > now()
				UNION
				SELECT item_id, organization_id, package_id, expires_at, item_type
				FROM promo.sponsorship_requests
				WHERE status = 'active'
				  AND admin_status = 'approved'
				  AND expires_at > now()
			) os
			JOIN promo.offer_packages op ON op.id = os.package_id
			WHERE ($1 = '' OR os.item_type = $1 OR os.item_type = 'product' OR os.item_type = 'variant')
			ORDER BY op.tier_level DESC, random();
		`
		rows, err := tx.Query(txCtx, query, string(itemType))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var rs promo.RankedSponsorship
			var it string
			if err := rows.Scan(&rs.ItemID, &rs.OrganizationID, &rs.PackageID, &rs.TierLevel, &rs.ExpiresAt, &it); err != nil {
				return err
			}
			rs.ItemType = promo.SponsorshipItemType(it)
			list = append(list, &rs)
		}
		return rows.Err()
	})
	return list, err
}

func (r *Repository) rankedSponsorships(ctx context.Context, itemType promo.SponsorshipItemType, itemIDs []int64) ([]*promo.RankedSponsorship, error) {
	if len(itemIDs) == 0 {
		return nil, nil
	}
	var list []*promo.RankedSponsorship
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT os.item_id, os.organization_id, os.package_id, op.tier_level, os.expires_at, os.item_type
			FROM (
				SELECT COALESCE(item_id, offer_id) AS item_id, organization_id, package_id, expires_at, item_type
				FROM promo.offer_sponsorships
				WHERE status = 'active'
				  AND admin_status = 'approved'
				  AND expires_at > now()
				UNION
				SELECT item_id, organization_id, package_id, expires_at, item_type
				FROM promo.sponsorship_requests
				WHERE status = 'active'
				  AND admin_status = 'approved'
				  AND expires_at > now()
			) os
			JOIN promo.offer_packages op ON op.id = os.package_id
			WHERE os.item_id = ANY($1)
			  AND COALESCE(os.item_type, 'offer') = $2
			ORDER BY op.tier_level DESC, os.expires_at DESC;
		`
		// Product and offer ids are separate sequences: without the type an
		// offer whose id equals a sponsored product's id ranked as sponsored.
		rows, err := tx.Query(txCtx, query, itemIDs, string(itemType))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var rs promo.RankedSponsorship
			var it string
			if err := rows.Scan(&rs.ItemID, &rs.OrganizationID, &rs.PackageID, &rs.TierLevel, &rs.ExpiresAt, &it); err != nil {
				return err
			}
			rs.ItemType = promo.SponsorshipItemType(it)
			list = append(list, &rs)
		}
		return rows.Err()
	})
	return list, err
}

// IsSponsored returns the highest-tier active sponsorship for a single item, or nil.
func (r *Repository) IsSponsored(ctx context.Context, itemType promo.SponsorshipItemType, itemID int64) (*promo.RankedSponsorship, error) {
	list, err := r.rankedSponsorships(ctx, itemType, []int64{itemID})
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, nil
	}
	return list[0], nil
}
