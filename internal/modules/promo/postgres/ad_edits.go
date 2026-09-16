package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// SubmitAdEditRequest stores proposed changes in pending_changes without interrupting live display.
func (r *Repository) SubmitAdEditRequest(ctx context.Context, id int64, changes *promo.AdPendingChanges) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		b, err := json.Marshal(changes)
		if err != nil {
			return err
		}
		tag, err := tx.Exec(txCtx, `
			UPDATE promo.ads
			SET pending_changes = $1, updated_at = now()
			WHERE id = $2;
		`, b, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("ad")
		}
		return nil
	})
}

// ApproveAdEditRequest merges pending_changes into the main columns and clears pending_changes.
func (r *Repository) ApproveAdEditRequest(ctx context.Context, id int64, reviewerID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var pendingJSON []byte
		err := tx.QueryRow(txCtx, `SELECT pending_changes FROM promo.ads WHERE id = $1;`, id).Scan(&pendingJSON)
		if err != nil {
			return err
		}
		if len(pendingJSON) == 0 || string(pendingJSON) == "null" {
			return fmt.Errorf("no pending edit request for ad %d", id)
		}

		var pc promo.AdPendingChanges
		if err := json.Unmarshal(pendingJSON, &pc); err != nil {
			return err
		}

		title := pc.TitleAr
		if title == "" {
			title = pc.TitleEn
		}

		mediaType := string(pc.MediaType)
		if mediaType == "" {
			mediaType = string(promo.MediaImage)
		}

		clickTarget := string(pc.ClickTargetType)
		if clickTarget == "" {
			clickTarget = string(promo.ClickTargetProduct)
		}

		query := `
			UPDATE promo.ads SET
				title = COALESCE(NULLIF($2, ''), title),
				title_ar = COALESCE(NULLIF($3, ''), title_ar),
				title_en = COALESCE(NULLIF($4, ''), title_en),
				ad_text_ar = COALESCE(NULLIF($5, ''), ad_text_ar),
				ad_text_en = COALESCE(NULLIF($6, ''), ad_text_en),
				media_type = $7,
				media_url = COALESCE(NULLIF($8, ''), media_url),
				thumbnail_url = COALESCE(NULLIF($9, ''), thumbnail_url),
				position = COALESCE(NULLIF($10, ''), position),
				target_url = COALESCE(NULLIF($11, ''), target_url),
				click_target_type = $12,
				click_target_id = COALESCE($13, click_target_id),
				pending_changes = NULL,
				reviewed_by = $14,
				reviewed_at = now(),
				updated_at = now()
			WHERE id = $1;
		`
		tag, err := tx.Exec(txCtx, query,
			id, title, pc.TitleAr, pc.TitleEn, pc.AdTextAr, pc.AdTextEn,
			mediaType, pc.MediaURL, pc.ThumbnailURL, pc.Position, pc.TargetURL,
			clickTarget, pc.ClickTargetID, reviewerID,
		)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("ad")
		}
		return nil
	})
}

// RejectAdEditRequest discards pending_changes and records admin notes.
func (r *Repository) RejectAdEditRequest(ctx context.Context, id int64, reviewerID int64, notes string) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(txCtx, `
			UPDATE promo.ads
			SET pending_changes = NULL, admin_notes = $1, reviewed_by = $2, reviewed_at = now(), updated_at = now()
			WHERE id = $3;
		`, notes, reviewerID, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("ad")
		}
		return nil
	})
}

// RecordAdImpression logs an impression and increments the counter.
func (r *Repository) RecordAdImpression(ctx context.Context, adID int64, userID *int64, ip, ua string) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(txCtx, `UPDATE promo.ads SET impressions = impressions + 1 WHERE id = $1;`, adID); err != nil {
			return err
		}
		_, err := tx.Exec(txCtx, `INSERT INTO promo.ad_impressions (ad_id, user_id, ip_address, user_agent) VALUES ($1, $2, $3, $4);`, adID, userID, ip, ua)
		return err
	})
}
