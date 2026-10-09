package postgres_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// The real catalog integration test uses overlapping promotions to prove that
// aggregating before pagination neither duplicates offers nor loses the tier.
func seedBuyerSponsorships(t *testing.T, db *database.DB, orgID, productID, variantID int64) {
	t.Helper()
	var packages []int64
	err := db.InTx(database.AsSystem(context.Background()), func(ctx context.Context, tx pgx.Tx) error {
		for _, tc := range []struct {
			item    string
			target  int64
			tier    int
			expires string
		}{
			{"offer", variantID, 1, "1 day"}, {"offer", variantID, 3, "1 day"},
			{"product", productID, 2, "1 day"}, {"offer", variantID, 9, "-1 day"},
		} {
			var id int64
			if err := tx.QueryRow(ctx, `INSERT INTO promo.offer_packages(name, tier_level)
			VALUES ('{"ar":"اختبار","en":"Test"}', $1) RETURNING id`, tc.tier).Scan(&id); err != nil {
				return err
			}
			packages = append(packages, id)
			if _, err := tx.Exec(ctx, `INSERT INTO promo.offer_sponsorships
			(organization_id, package_id, item_type, item_id, status, admin_status, expires_at)
			VALUES ($1, $2, $3, $4, 'active', 'approved', now() + $5::interval)`, orgID, id, tc.item, tc.target, tc.expires); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.InTx(database.AsSystem(context.Background()), func(ctx context.Context, tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `DELETE FROM promo.offer_sponsorships WHERE package_id = ANY($1)`, packages); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `DELETE FROM promo.offer_packages WHERE id = ANY($1)`, packages)
			return err
		}); err != nil {
			t.Error(err)
		}
	})
}
