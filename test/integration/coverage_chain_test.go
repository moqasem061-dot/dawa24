package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
	promoPostgres "github.com/muhiya/dawa24-store/internal/modules/promo/postgres"
	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	workflowPostgres "github.com/muhiya/dawa24-store/internal/modules/workflow/postgres"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// TestCoverageChain_VisibilityRule verifies that vendor weekly coverage coordinates
// and active status directly govern offer visibility for pharmacies (Plan V5 Phase 0 Task 0.1 & T9).
func TestCoverageChain_VisibilityRule(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		return
	}
	ctx := context.Background()

	promoRepo := promoPostgres.NewRepository(db)
	wfRepo := workflowPostgres.NewRepository(db)

	// Clean up any test fixtures after run
	var orgID, branchID, offerID, covID, workID int64
	defer func() {
		_ = db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
			_, _ = tx.Exec(txCtx, "DELETE FROM promo.offers WHERE id = $1", offerID)
			_, _ = tx.Exec(txCtx, "DELETE FROM workflow.weekly_coverages WHERE id = $1", covID)
			_, _ = tx.Exec(txCtx, "DELETE FROM org.branches WHERE id = $1", branchID)
			_, _ = tx.Exec(txCtx, "DELETE FROM org.organizations WHERE id = $1", orgID)
			_, _ = tx.Exec(txCtx, "DELETE FROM org.institutional_works WHERE id = $1", workID)
			return nil
		})
	}()

	// 1. Setup: Create Vendor Org & Branch at Cairo (30.0444, 31.2357)
	err := db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		err := tx.QueryRow(txCtx, `
			INSERT INTO org.organizations (
				legal_name, trade_name, tax_number, commercial_register,
				type, status, created_at, updated_at
			) VALUES (
				'Coverage Test Vendor Co', '{"ar":"مورد اختبار التغطية","en":"Coverage Test Vendor"}',
				'TX-99887766', 'CR-99887766', 'vendor', 'approved', now(), now()
			) RETURNING id;
		`).Scan(&orgID)
		if err != nil {
			return err
		}

		lat, lng := 30.0444, 31.2357
		err = tx.QueryRow(txCtx, `
			INSERT INTO org.branches (
				organization_id, name, code, address,
				latitude, longitude, created_at, updated_at
			) VALUES (
				$1, '{"ar":"فرع القاهرة المركزي","en":"Cairo Central Branch"}',
				'CAI-01', '123 Tahrir St, Cairo', $2, $3, now(), now()
			) RETURNING id;
		`, orgID, lat, lng).Scan(&branchID)
		if err != nil {
			return err
		}
		// The branch holds an institutional work the buyer is connected to;
		// the offer rule requires it as the catalogue does.
		if err := tx.QueryRow(txCtx, `INSERT INTO org.institutional_works (title) VALUES ('{"ar":"صيدليات","en":"Pharmacies"}') RETURNING id`).Scan(&workID); err != nil {
			return err
		}
		_, err = tx.Exec(txCtx, `INSERT INTO org.branch_institutional_works (branch_id, work_category, institutional_work_id) VALUES ($1, 'pharmacy', $2)`, branchID, workID)
		return err
	})
	allowedWorks := []int64{workID}
	if err != nil {
		t.Fatalf("failed creating vendor org & branch: %v", err)
	}

	// 2. Vendor creates weekly coverage for Sunday (day 0), radius 25km (25,000m)
	covLat, covLng := 30.0444, 31.2357
	err = db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `
			INSERT INTO workflow.weekly_coverages (
				organization_id, branch_id, day_of_week, coverage_from, coverage_to,
				address, latitude, longitude, distance_meters, is_active, created_at, updated_at
			) VALUES (
				$1, $2, 0, '08:00', '18:00',
				'Cairo Metropolitan Area', $3, $4, 25000, true, now(), now()
			) RETURNING id;
		`, orgID, branchID, covLat, covLng).Scan(&covID)
	})
	if err != nil {
		t.Fatalf("failed creating weekly coverage: %v", err)
	}

	// 3. Vendor creates approved active offer
	startsAt := time.Now().Add(-1 * time.Hour)
	expiresAt := time.Now().Add(24 * time.Hour)
	err = db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `
			INSERT INTO promo.offers (
				organization_id, branch_id, title, description,
				discount_type, discount_value, min_order_amount, admin_status,
				starts_at, expires_at, is_active, created_at, updated_at
			) VALUES (
				$1, $2, '{"en":"Special Sunday Discount"}'::jsonb, '{"en":"10% off for in-range pharmacies"}'::jsonb,
				'percentage', 10.0, 50000, 'approved',
				$3, $4, true, now(), now()
			) RETURNING id;
		`, orgID, branchID, startsAt, expiresAt).Scan(&offerID)
	})
	if err != nil {
		t.Fatalf("failed creating offer: %v", err)
	}

	// Offers are listed for a buying branch from the coverage sets workflow
	// resolves for it, exactly as the offers board does.
	coverage := workflow.NewCoverageService(db)
	visibleTo := func(lat, lng float64) []*promo.BuyerOffer {
		t.Helper()
		rows, err := coverage.VendorBranchesServing(ctx, time.Sunday, workflow.Coord{Lat: lat, Lon: lng})
		if err != nil {
			t.Fatalf("VendorBranchesServing: %v", err)
		}
		var sets promo.SupplierCoverage
		for _, row := range rows {
			sets.OrgIDs = append(sets.OrgIDs, row.OrganizationID)
			if row.BranchID > 0 {
				sets.BranchIDs = append(sets.BranchIDs, row.BranchID)
			} else {
				sets.OrgWideIDs = append(sets.OrgWideIDs, row.OrganizationID)
			}
		}
		offers, _, err := promoRepo.ListBuyerOffers(ctx, promo.BuyerOfferQuery{
			Buying:   true,
			Branch:   promo.BuyerBranch{Lat: lat, Lon: lng, HasCoords: true, Weekday: time.Sunday, AllowedWorkIDs: allowedWorks},
			Coverage: sets,
			Limit:    50,
		})
		if err != nil {
			t.Fatalf("ListBuyerOffers: %v", err)
		}
		return offers
	}
	listed := func(offers []*promo.BuyerOffer) bool {
		for _, o := range offers {
			if o.ID == offerID {
				return true
			}
		}
		return false
	}

	// 4. Pharmacy A in Cairo (~1km away) sees the offer on Sunday.
	if !listed(visibleTo(30.0500, 31.2400)) {
		t.Errorf("expected offer %d to be visible to Cairo pharmacy (~1km away)", offerID)
	}

	// 5. Pharmacy B in Alexandria (~180km away, radius 25km) does not.
	if listed(visibleTo(31.2001, 29.9187)) {
		t.Errorf("offer %d should NOT be visible to Alexandria pharmacy", offerID)
	}

	// 6. Vendor toggles coverage to inactive: Cairo no longer sees it.
	if err := wfRepo.ToggleWeeklyCoverage(ctx, covID, false); err != nil {
		t.Fatalf("ToggleWeeklyCoverage failed: %v", err)
	}
	if listed(visibleTo(30.0500, 31.2400)) {
		t.Errorf("offer %d should NOT be visible after coverage is disabled", offerID)
	}
}
