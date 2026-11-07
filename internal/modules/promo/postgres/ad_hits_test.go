package postgres

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// S23: repeats of one ad from one address inside the window are counted once,
// a different address is counted, and a repeat after the window is counted.
func TestAdHitsFoldRepeatsFromOneAddress(t *testing.T) {
	db := getTestDB(t)
	resetFixtures(t, db)
	repo := NewRepository(db)
	ctx := context.Background()
	sys := database.AsSystem(ctx)

	var adID int64
	err := db.InTx(sys, func(txCtx context.Context, tx pgx.Tx) error {
		_, _ = tx.Exec(txCtx, `DELETE FROM promo.ads WHERE title = 'Batch5 Hit Test'`)
		return tx.QueryRow(txCtx, `
			INSERT INTO promo.ads (organization_id, title, image_url, media_url, target_url, position, is_active, starts_at, expires_at)
			VALUES ($1, 'Batch5 Hit Test', 'https://cdn.example.com/ad.jpg', 'https://cdn.example.com/ad.jpg', 'https://dawa24.test/sale', 'top_banner', true, now() - interval '1 hour', now() + interval '1 day')
			RETURNING id;`, testOrgID).Scan(&adID)
	})
	if err != nil {
		t.Fatalf("insert ad: %v", err)
	}
	counts := func() (adClicks, adImpressions, clicks, impressions int) {
		t.Helper()
		err := db.InReadTx(sys, func(txCtx context.Context, tx pgx.Tx) error {
			if err := tx.QueryRow(txCtx, `SELECT clicks, impressions FROM promo.ads WHERE id = $1`, adID).Scan(&clicks, &impressions); err != nil {
				return err
			}
			if err := tx.QueryRow(txCtx, `SELECT count(*) FROM promo.ad_clicks WHERE ad_id = $1`, adID).Scan(&adClicks); err != nil {
				return err
			}
			return tx.QueryRow(txCtx, `SELECT count(*) FROM promo.ad_impressions WHERE ad_id = $1`, adID).Scan(&adImpressions)
		})
		if err != nil {
			t.Fatalf("read counts: %v", err)
		}
		return
	}

	for i := 0; i < 3; i++ {
		if err := repo.RecordAdClick(ctx, adID, nil, "203.0.113.7", "Batch5Agent"); err != nil {
			t.Fatalf("click %d: %v", i+1, err)
		}
		if err := repo.RecordAdImpression(ctx, adID, nil, "203.0.113.7", "Batch5Agent"); err != nil {
			t.Fatalf("impression %d: %v", i+1, err)
		}
	}
	if err := repo.RecordAdClick(ctx, adID, nil, "203.0.113.8", "Batch5Agent"); err != nil {
		t.Fatalf("click from second address: %v", err)
	}
	if err := repo.RecordAdImpression(ctx, adID, nil, "203.0.113.8", "Batch5Agent"); err != nil {
		t.Fatalf("impression from second address: %v", err)
	}

	adClicks, adImpressions, clicks, impressions := counts()
	if adClicks != 2 || clicks != 2 || adImpressions != 2 || impressions != 2 {
		t.Fatalf("inside the window: ad_clicks=%d clicks=%d ad_impressions=%d impressions=%d, want 2 each", adClicks, clicks, adImpressions, impressions)
	}

	// Age the first address's rows past the window: its next hit counts again.
	err = db.InTx(sys, func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `UPDATE promo.ad_clicks SET created_at = now() - interval '2 minutes' WHERE ad_id = $1 AND ip_address = '203.0.113.7'`, adID)
		if err != nil {
			return err
		}
		_, err = tx.Exec(txCtx, `UPDATE promo.ad_impressions SET created_at = now() - interval '2 minutes' WHERE ad_id = $1 AND ip_address = '203.0.113.7'`, adID)
		return err
	})
	if err != nil {
		t.Fatalf("age rows: %v", err)
	}
	if err := repo.RecordAdClick(ctx, adID, nil, "203.0.113.7", "Batch5Agent"); err != nil {
		t.Fatalf("click after window: %v", err)
	}
	if err := repo.RecordAdImpression(ctx, adID, nil, "203.0.113.7", "Batch5Agent"); err != nil {
		t.Fatalf("impression after window: %v", err)
	}
	adClicks, adImpressions, clicks, impressions = counts()
	if adClicks != 3 || clicks != 3 || adImpressions != 3 || impressions != 3 {
		t.Fatalf("after the window: ad_clicks=%d clicks=%d ad_impressions=%d impressions=%d, want 3 each", adClicks, clicks, adImpressions, impressions)
	}
}
