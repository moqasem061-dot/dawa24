package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

const adColumns = `
	id, public_id, organization_id, title, title_ar, title_en, ad_text_ar, ad_text_en,
	image_url, media_type, media_url, thumbnail_url, target_url,
	click_target_type, click_target_id, position, is_active, admin_status, admin_notes,
	reviewed_by, reviewed_at, ad_plan_id, duration_days, starts_at, expires_at,
	impressions, clicks, created_at, updated_at, pending_changes`

// CreateAd inserts a new advertisement.
func (r *Repository) CreateAd(ctx context.Context, a *promo.Ad) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO promo.ads (
				organization_id, title, title_ar, title_en, ad_text_ar, ad_text_en,
				image_url, media_type, media_url, thumbnail_url, target_url,
				click_target_type, click_target_id, position, is_active, admin_status,
				ad_plan_id, duration_days, starts_at, expires_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)
			RETURNING id, public_id, created_at, updated_at;
		`
		if a.AdminStatus == "" {
			a.AdminStatus = promo.AdminPending
		}
		if a.MediaType == "" {
			a.MediaType = promo.MediaImage
		}
		if a.ClickTargetType == "" {
			a.ClickTargetType = promo.ClickTargetVendor
		}
		if a.Position == "" {
			a.Position = "home_banner"
		}
		if a.DurationDays <= 0 {
			a.DurationDays = 30
		}
		if a.ExpiresAt.IsZero() {
			a.ExpiresAt = time.Now().UTC().Add(time.Duration(a.DurationDays) * 24 * time.Hour)
		}
		if a.StartsAt.IsZero() {
			a.StartsAt = time.Now().UTC()
		}
		imageURL := a.ImageURL
		if imageURL == "" && a.MediaURL != "" {
			imageURL = a.MediaURL
		}
		err := tx.QueryRow(txCtx, query,
			a.OrganizationID, a.Title, a.TitleAr, a.TitleEn, a.AdTextAr, a.AdTextEn,
			imageURL, string(a.MediaType), a.MediaURL, a.ThumbnailURL, a.TargetURL,
			string(a.ClickTargetType), a.ClickTargetID, a.Position, a.IsActive, string(a.AdminStatus),
			a.AdPlanID, a.DurationDays, a.StartsAt, a.ExpiresAt,
		).Scan(&a.ID, &a.PublicID, &a.CreatedAt, &a.UpdatedAt)
		if err != nil && strings.Contains(err.Error(), "ads_click_target_type_check") {
			// Compatibility fallback for unmigrated databases where 'product' is not yet in the check constraint:
			// Fallback to 'vendor_page' in click_target_type column while preserving the exact target_url
			return tx.QueryRow(txCtx, query,
				a.OrganizationID, a.Title, a.TitleAr, a.TitleEn, a.AdTextAr, a.AdTextEn,
				imageURL, string(a.MediaType), a.MediaURL, a.ThumbnailURL, a.TargetURL,
				string(promo.ClickTargetVendor), a.ClickTargetID, a.Position, a.IsActive, string(a.AdminStatus),
				a.AdPlanID, a.DurationDays, a.StartsAt, a.ExpiresAt,
			).Scan(&a.ID, &a.PublicID, &a.CreatedAt, &a.UpdatedAt)
		}
		return err
	})
}

// UpdateAd updates an existing advertisement.
func (r *Repository) UpdateAd(ctx context.Context, a *promo.Ad) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE promo.ads SET
				title = $2, title_ar = $3, title_en = $4, ad_text_ar = $5, ad_text_en = $6,
				image_url = $7, media_type = $8, media_url = $9, thumbnail_url = $10, target_url = $11,
				click_target_type = $12, click_target_id = $13, position = $14, is_active = $15,
				duration_days = $16, starts_at = $17, expires_at = $18, updated_at = now()
			WHERE id = $1;
		`
		tag, err := tx.Exec(txCtx, query,
			a.ID, a.Title, a.TitleAr, a.TitleEn, a.AdTextAr, a.AdTextEn,
			a.ImageURL, string(a.MediaType), a.MediaURL, a.ThumbnailURL, a.TargetURL,
			string(a.ClickTargetType), a.ClickTargetID, a.Position, a.IsActive,
			a.DurationDays, a.StartsAt, a.ExpiresAt,
		)
		if err != nil && strings.Contains(err.Error(), "ads_click_target_type_check") {
			tag, err = tx.Exec(txCtx, query,
				a.ID, a.Title, a.TitleAr, a.TitleEn, a.AdTextAr, a.AdTextEn,
				a.ImageURL, string(a.MediaType), a.MediaURL, a.ThumbnailURL, a.TargetURL,
				string(promo.ClickTargetVendor), a.ClickTargetID, a.Position, a.IsActive,
				a.DurationDays, a.StartsAt, a.ExpiresAt,
			)
		}
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("ad")
		}
		return nil
	})
}

// GetAdByID retrieves an ad by ID.
func (r *Repository) GetAdByID(ctx context.Context, id int64) (*promo.Ad, error) {
	var a promo.Ad
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		return scanAd(tx.QueryRow(txCtx, `SELECT `+adColumns+` FROM promo.ads WHERE id = $1;`, id), &a)
	})
	if err != nil {
		if database.IsNotFound(err) {
			return nil, apperr.NotFound("ad")
		}
		return nil, err
	}
	return &a, nil
}

// ListAdsByOrg returns ads for an organization.
func (r *Repository) ListAdsByOrg(ctx context.Context, orgID int64, limit, offset int) ([]*promo.Ad, error) {
	return r.listAds(ctx, `WHERE organization_id = $1`, orgID, limit, offset)
}

// ListAllAds returns all ads for admin moderation.
func (r *Repository) ListAllAds(ctx context.Context, limit, offset int) ([]*promo.Ad, error) {
	return r.listAds(database.AsSystem(ctx), ``, int64(0), limit, offset)
}

func (r *Repository) listAds(ctx context.Context, where string, orgID int64, limit, offset int) ([]*promo.Ad, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var list []*promo.Ad
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `SELECT ` + adColumns + ` FROM promo.ads ` + where + ` ORDER BY created_at DESC LIMIT $` + adLimitParam(orgID) + ` OFFSET $` + adOffsetParam(orgID) + `;`
		var rows pgx.Rows
		var err error
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
			var a promo.Ad
			if err := scanAd(rows, &a); err != nil {
				return err
			}
			list = append(list, &a)
		}
		return rows.Err()
	})
	return list, err
}

func adLimitParam(orgID int64) string {
	if orgID > 0 {
		return "2"
	}
	return "1"
}
func adOffsetParam(orgID int64) string {
	if orgID > 0 {
		return "3"
	}
	return "2"
}

// ListAdsByOrgWithTotal returns paginated ads for an organization with total count.
func (r *Repository) ListAdsByOrgWithTotal(ctx context.Context, orgID int64, limit, offset int) ([]*promo.Ad, int, error) {
	return r.listAdsWithTotal(ctx, `WHERE organization_id = $1`, orgID, limit, offset)
}

// ListAllAdsWithTotal returns all ads for admin moderation with total count.
func (r *Repository) ListAllAdsWithTotal(ctx context.Context, limit, offset int) ([]*promo.Ad, int, error) {
	return r.listAdsWithTotal(database.AsSystem(ctx), ``, int64(0), limit, offset)
}

func (r *Repository) listAdsWithTotal(ctx context.Context, where string, orgID int64, limit, offset int) ([]*promo.Ad, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 25
	}
	if offset < 0 {
		offset = 0
	}
	var list []*promo.Ad
	var total int

	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		countQuery := `SELECT count(*) FROM promo.ads ` + where + `;`
		var err error
		if orgID > 0 {
			err = tx.QueryRow(txCtx, countQuery, orgID).Scan(&total)
		} else {
			err = tx.QueryRow(txCtx, countQuery).Scan(&total)
		}
		if err != nil {
			return err
		}

		query := `SELECT ` + adColumns + ` FROM promo.ads ` + where + ` ORDER BY created_at DESC, id DESC LIMIT $` + adLimitParam(orgID) + ` OFFSET $` + adOffsetParam(orgID) + `;`
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
			var a promo.Ad
			if err := scanAd(rows, &a); err != nil {
				return err
			}
			list = append(list, &a)
		}
		return rows.Err()
	})
	return list, total, err
}

// ListActiveAds returns approved, active display ads by screen position.
func (r *Repository) ListActiveAds(ctx context.Context, position string) ([]*promo.Ad, error) {
	var list []*promo.Ad
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT ` + adColumns + ` FROM promo.ads
			WHERE is_active = true AND admin_status = 'approved'
			  AND starts_at <= now() AND expires_at >= now()
			  AND ($1 = '' OR position = $1)
			ORDER BY created_at DESC;
		`
		rows, err := tx.Query(txCtx, query, position)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var a promo.Ad
			if err := scanAd(rows, &a); err != nil {
				return err
			}
			a.CTR = promo.ComputeCTR(a.Impressions, a.Clicks)
			list = append(list, &a)
		}
		return rows.Err()
	})
	return list, err
}

// UpdateAdAdminStatus sets the admin approval state.
func (r *Repository) UpdateAdAdminStatus(ctx context.Context, id int64, status promo.AdminStatus, notes string, reviewerID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var orgID *int64
		_ = tx.QueryRow(txCtx, `SELECT organization_id FROM promo.ads WHERE id = $1;`, id).Scan(&orgID)

		isActive := status == promo.AdminApproved
		tag, err := tx.Exec(txCtx, `
			UPDATE promo.ads
			SET admin_status = $1, admin_notes = $2, reviewed_by = $3, reviewed_at = now(),
			    is_active = $4,
			    starts_at = CASE WHEN $4 AND (expires_at <= now() OR starts_at < '2000-01-01'::timestamptz) THEN now() ELSE starts_at END,
			    expires_at = CASE WHEN $4 AND (expires_at <= now() OR expires_at < '2000-01-01'::timestamptz) THEN now() + (GREATEST(COALESCE(duration_days, 30), 1) || ' days')::interval ELSE expires_at END,
			    updated_at = now()
			WHERE id = $5;
		`, string(status), notes, reviewerID, isActive, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("ad")
		}

		auditAction := "promo.ad.reject"
		if status == promo.AdminApproved {
			auditAction = "promo.ad.approve"
		}
		_ = database.WriteAudit(txCtx, tx, database.AuditEntry{
			OrganizationID: orgID,
			ActorUserID:    reviewerID,
			Action:         auditAction,
			EntityType:     "ad",
			EntityID:       strconv.FormatInt(id, 10),
			After:          map[string]any{"status": string(status), "notes": notes},
		})

		return nil
	})
}

// AdminToggleAd toggles an ad's is_active flag directly as platform staff without tenant requirements.
func (r *Repository) AdminToggleAd(ctx context.Context, id int64) (*promo.Ad, error) {
	var a promo.Ad
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE promo.ads
			SET is_active = NOT is_active, updated_at = now()
			WHERE id = $1
			RETURNING ` + adColumns + `;
		`
		row := tx.QueryRow(txCtx, query, id)
		if err := scanAd(row, &a); err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("ad")
			}
			return err
		}
		a.CTR = promo.ComputeCTR(a.Impressions, a.Clicks)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func scanAd(row pgx.Row, a *promo.Ad) error {
	var (
		titleAr            sql.NullString
		titleEn            sql.NullString
		adTextAr           sql.NullString
		adTextEn           sql.NullString
		imageURL           sql.NullString
		mediaType          sql.NullString
		mediaURL           sql.NullString
		thumbURL           sql.NullString
		targetURL          sql.NullString
		clickTarget        sql.NullString
		position           sql.NullString
		adminStatus        sql.NullString
		adminNotes         sql.NullString
		pendingChangesJSON []byte
	)
	err := row.Scan(
		&a.ID, &a.PublicID, &a.OrganizationID, &a.Title, &titleAr, &titleEn, &adTextAr, &adTextEn,
		&imageURL, &mediaType, &mediaURL, &thumbURL, &targetURL,
		&clickTarget, &a.ClickTargetID, &position, &a.IsActive, &adminStatus, &adminNotes,
		&a.ReviewedBy, &a.ReviewedAt, &a.AdPlanID, &a.DurationDays, &a.StartsAt, &a.ExpiresAt,
		&a.Impressions, &a.Clicks, &a.CreatedAt, &a.UpdatedAt, &pendingChangesJSON,
	)
	if err != nil {
		return err
	}
	a.TitleAr = titleAr.String
	a.TitleEn = titleEn.String
	a.AdTextAr = adTextAr.String
	a.AdTextEn = adTextEn.String
	a.ImageURL = imageURL.String
	a.MediaURL = mediaURL.String
	a.ThumbnailURL = thumbURL.String
	a.TargetURL = targetURL.String
	a.Position = position.String
	a.AdminNotes = adminNotes.String

	if len(pendingChangesJSON) > 0 && string(pendingChangesJSON) != "null" {
		var pc promo.AdPendingChanges
		if json.Unmarshal(pendingChangesJSON, &pc) == nil {
			a.PendingChanges = &pc
		}
	}

	if mediaType.Valid && mediaType.String != "" {
		a.MediaType = promo.AdMediaType(mediaType.String)
	} else {
		a.MediaType = promo.MediaImage
	}
	if clickTarget.Valid && clickTarget.String != "" {
		a.ClickTargetType = promo.AdClickTarget(clickTarget.String)
	} else {
		a.ClickTargetType = promo.ClickTargetVendor
	}
	if adminStatus.Valid && adminStatus.String != "" {
		a.AdminStatus = promo.AdminStatus(adminStatus.String)
	} else {
		a.AdminStatus = promo.AdminPending
	}
	return nil
}
