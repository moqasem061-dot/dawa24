package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/storage"
)

// ListHighlightSections returns all active platform curated sections.
func (r *Repository) ListHighlightSections(ctx context.Context) ([]*promo.HighlightSection, error) {
	var list []*promo.HighlightSection
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `SELECT id, public_id, title, slug, display_order, is_active, owner_type, organization_id, created_at FROM promo.highlight_sections WHERE is_active = true AND owner_type = 'platform' ORDER BY display_order ASC;`
		rows, err := tx.Query(txCtx, query)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var h promo.HighlightSection
			if err := rows.Scan(&h.ID, &h.PublicID, &h.Title, &h.Slug, &h.DisplayOrder, &h.IsActive, &h.OwnerType, &h.OrganizationID, &h.CreatedAt); err != nil {
				return err
			}
			list = append(list, &h)
		}
		return rows.Err()
	})
	return list, err
}

// ExpirePromotionsAndCollectMedia marks expired ads, offers, and sponsorship requests as expired/inactive,
// clears their media URLs in the database, and returns all media URLs to be purged from storage.
func (r *Repository) ExpirePromotionsAndCollectMedia(ctx context.Context) ([]string, int64, error) {
	var (
		mediaURLs    []string
		totalExpired int64
	)

	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		// 1. Collect and expire Ads
		adRows, err := tx.Query(txCtx, `
			SELECT id, media_url, image_url, thumbnail_url, pending_changes
			FROM promo.ads
			WHERE expires_at < now()
			  AND (is_active = true OR media_url <> '' OR image_url <> '' OR thumbnail_url <> '' OR pending_changes IS NOT NULL);
		`)
		if err != nil {
			return fmt.Errorf("query expired ads: %w", err)
		}
		defer adRows.Close()

		var adIDs []int64
		for adRows.Next() {
			var (
				id                 int64
				mediaURL           string
				imageURL           string
				thumbnailURL       *string
				pendingChangesJSON []byte
			)
			if err := adRows.Scan(&id, &mediaURL, &imageURL, &thumbnailURL, &pendingChangesJSON); err != nil {
				return fmt.Errorf("scan expired ad: %w", err)
			}
			adIDs = append(adIDs, id)
			if m := strings.TrimSpace(mediaURL); m != "" {
				mediaURLs = append(mediaURLs, m)
			}
			if img := strings.TrimSpace(imageURL); img != "" {
				mediaURLs = append(mediaURLs, img)
			}
			if thumbnailURL != nil && strings.TrimSpace(*thumbnailURL) != "" {
				mediaURLs = append(mediaURLs, strings.TrimSpace(*thumbnailURL))
			}
			if len(pendingChangesJSON) > 0 && string(pendingChangesJSON) != "null" {
				var pc promo.AdPendingChanges
				if json.Unmarshal(pendingChangesJSON, &pc) == nil {
					if m := strings.TrimSpace(pc.MediaURL); m != "" {
						mediaURLs = append(mediaURLs, m)
					}
					if t := strings.TrimSpace(pc.ThumbnailURL); t != "" {
						mediaURLs = append(mediaURLs, t)
					}
				}
			}
		}
		if err := adRows.Err(); err != nil {
			return err
		}

		if len(adIDs) > 0 {
			tagAds, err := tx.Exec(txCtx, `
				UPDATE promo.ads
				SET is_active = false,
				    media_url = '',
				    image_url = '',
				    thumbnail_url = '',
				    pending_changes = NULL,
				    updated_at = now()
				WHERE id = ANY($1);
			`, adIDs)
			if err != nil {
				return fmt.Errorf("update expired ads: %w", err)
			}
			totalExpired += tagAds.RowsAffected()
		}

		// 2. Collect and expire Sponsorship Requests
		srRows, err := tx.Query(txCtx, `
			SELECT id, item_type, item_id
			FROM promo.sponsorship_requests
			WHERE status = 'active' AND expires_at < now();
		`)
		if err != nil {
			return fmt.Errorf("query expired sponsorship requests: %w", err)
		}
		defer srRows.Close()

		var (
			srIDs    []int64
			offerIDs []int64
		)
		for srRows.Next() {
			var (
				id       int64
				itemType string
				itemID   int64
			)
			if err := srRows.Scan(&id, &itemType, &itemID); err != nil {
				return fmt.Errorf("scan expired sponsorship request: %w", err)
			}
			srIDs = append(srIDs, id)
			if itemType == "offer" && itemID > 0 {
				offerIDs = append(offerIDs, itemID)
			}
		}
		if err := srRows.Err(); err != nil {
			return err
		}

		if len(srIDs) > 0 {
			tagSR, err := tx.Exec(txCtx, `
				UPDATE promo.sponsorship_requests
				SET status = 'expired', updated_at = now()
				WHERE id = ANY($1);
			`, srIDs)
			if err != nil {
				return fmt.Errorf("update expired sponsorship requests: %w", err)
			}
			totalExpired += tagSR.RowsAffected()
		}

		// 3. Expire offer sponsorships and purchases
		tagSponsors, err := tx.Exec(txCtx, `
			UPDATE promo.offer_sponsorships
			SET status = 'expired'
			WHERE status = 'active' AND expires_at < now();
		`)
		if err != nil {
			return fmt.Errorf("expire offer sponsorships: %w", err)
		}
		totalExpired += tagSponsors.RowsAffected()

		tagPurchases, err := tx.Exec(txCtx, `
			UPDATE promo.sponsorship_purchases
			SET status = 'expired', updated_at = now()
			WHERE status = 'active' AND expires_at < now();
		`)
		if err != nil {
			return fmt.Errorf("expire sponsorship purchases: %w", err)
		}
		totalExpired += tagPurchases.RowsAffected()

		// 4. Collect and expire Offers (both directly expired offers and offers from expired sponsorship requests)
		offerRows, err := tx.Query(txCtx, `
			SELECT id, image
			FROM promo.offers
			WHERE (is_active = true AND expires_at < now())
			   OR (id = ANY($1) AND image <> '');
		`, offerIDs)
		if err != nil {
			return fmt.Errorf("query expiring offers: %w", err)
		}
		defer offerRows.Close()

		var expiringOfferIDs []int64
		for offerRows.Next() {
			var (
				id    int64
				image string
			)
			if err := offerRows.Scan(&id, &image); err != nil {
				return fmt.Errorf("scan expiring offer: %w", err)
			}
			expiringOfferIDs = append(expiringOfferIDs, id)
			if img := strings.TrimSpace(image); img != "" {
				mediaURLs = append(mediaURLs, img)
			}
		}
		if err := offerRows.Err(); err != nil {
			return err
		}

		if len(expiringOfferIDs) > 0 {
			tagOffers, err := tx.Exec(txCtx, `
				UPDATE promo.offers
				SET is_active = false, image = '', updated_at = now()
				WHERE id = ANY($1);
			`, expiringOfferIDs)
			if err != nil {
				return fmt.Errorf("update expired offers: %w", err)
			}
			totalExpired += tagOffers.RowsAffected()
		}

		return nil
	})
	if err != nil {
		return nil, 0, err
	}

	// Deduplicate media URLs
	seen := make(map[string]struct{}, len(mediaURLs))
	deduped := make([]string, 0, len(mediaURLs))
	for _, u := range mediaURLs {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		if _, ok := seen[u]; !ok {
			seen[u] = struct{}{}
			deduped = append(deduped, u)
		}
	}

	return deduped, totalExpired, nil
}

// ExpirePromotions marks expired offers, sponsorships, and ads as inactive and purges their media.
func (r *Repository) ExpirePromotions(ctx context.Context) (int64, error) {
	urls, count, err := r.ExpirePromotionsAndCollectMedia(ctx)
	if err != nil {
		return 0, err
	}
	for _, u := range urls {
		_ = storage.DeleteUploadedMedia(ctx, u, nil)
	}
	return count, nil
}
