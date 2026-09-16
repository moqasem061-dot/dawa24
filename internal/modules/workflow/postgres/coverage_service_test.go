package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

func TestCoverageService_ServesPoint_CityMatch(t *testing.T) {
	db := getTestDB(t)
	resetFixtures(t, db)

	ctx := database.AsSystem(context.Background())
	svc := workflow.NewCoverageService(db)

	var cityID int64
	err := db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `SELECT id FROM platform_admin.cities LIMIT 1`).Scan(&cityID)
	})
	if err != nil {
		t.Fatalf("query test city: %v", err)
	}

	otherCityID := cityID + 9999

	cov := &workflow.WeeklyCoverage{
		OrganizationID: testOrgID,
		BranchID:       testBranchID,
		CityID:         &cityID,
		DayOfWeek:      1, // Monday
		DistanceMeters: 15000,
		IsActive:       true,
		Address:        "City route",
	}
	repo := NewRepository(db)
	if err := repo.SaveWeeklyCoverage(ctx, cov); err != nil {
		t.Fatalf("save weekly coverage: %v", err)
	}

	t.Run("matches exact city on matching day", func(t *testing.T) {
		served, _, err := svc.ServesPoint(ctx, testOrgID, time.Monday, workflow.Coord{CityID: &cityID})
		if err != nil {
			t.Fatalf("ServesPoint: %v", err)
		}
		if !served {
			t.Errorf("expected served = true for city match")
		}
	})

	t.Run("fails on different weekday", func(t *testing.T) {
		served, _, err := svc.ServesPoint(ctx, testOrgID, time.Tuesday, workflow.Coord{CityID: &cityID})
		if err != nil {
			t.Fatalf("ServesPoint: %v", err)
		}
		if served {
			t.Errorf("expected served = false on wrong weekday")
		}
	})

	t.Run("fails for different city", func(t *testing.T) {
		served, _, err := svc.ServesPoint(ctx, testOrgID, time.Monday, workflow.Coord{CityID: &otherCityID})
		if err != nil {
			t.Fatalf("ServesPoint: %v", err)
		}
		if served {
			t.Errorf("expected served = false for different city")
		}
	})
}

func TestCoverageService_ServesPoint_RadiusMatch(t *testing.T) {
	db := getTestDB(t)
	resetFixtures(t, db)

	ctx := database.AsSystem(context.Background())
	svc := workflow.NewCoverageService(db)

	lat := 30.0444
	lon := 31.2357
	cov := &workflow.WeeklyCoverage{
		OrganizationID: testOrgID,
		BranchID:       testBranchID,
		Latitude:       &lat,
		Longitude:      &lon,
		DayOfWeek:      3, // Wednesday
		DistanceMeters: 10000,
		IsActive:       true,
		Address:        "Cairo center radius",
	}
	repo := NewRepository(db)
	if err := repo.SaveWeeklyCoverage(ctx, cov); err != nil {
		t.Fatalf("save weekly coverage: %v", err)
	}

	t.Run("within radius on correct day", func(t *testing.T) {
		// ~1 km away
		target := workflow.Coord{Lat: 30.0500, Lon: 31.2400}
		served, meters, err := svc.ServesPoint(ctx, testOrgID, time.Wednesday, target)
		if err != nil {
			t.Fatalf("ServesPoint: %v", err)
		}
		if !served {
			t.Errorf("expected served = true within radius")
		}
		if meters <= 0 || meters > 10000 {
			t.Errorf("expected actual meters between 0 and 10000, got %d", meters)
		}
	})

	t.Run("outside radius", func(t *testing.T) {
		// ~60 km away
		target := workflow.Coord{Lat: 30.5000, Lon: 31.8000}
		served, _, err := svc.ServesPoint(ctx, testOrgID, time.Wednesday, target)
		if err != nil {
			t.Fatalf("ServesPoint: %v", err)
		}
		if served {
			t.Errorf("expected served = false outside radius")
		}
	})

	t.Run("inactive coverage row fails closed", func(t *testing.T) {
		if err := repo.ToggleWeeklyCoverage(ctx, cov.ID, false); err != nil {
			t.Fatalf("toggle coverage to inactive: %v", err)
		}
		target := workflow.Coord{Lat: 30.0500, Lon: 31.2400}
		served, _, err := svc.ServesPoint(ctx, testOrgID, time.Wednesday, target)
		if err != nil {
			t.Fatalf("ServesPoint: %v", err)
		}
		if served {
			t.Errorf("expected served = false for inactive coverage")
		}
	})
}

func TestCoverageService_ServesPoint_RespectsVendorBranch(t *testing.T) {
	db := getTestDB(t)
	resetFixtures(t, db)

	ctx := database.AsSystem(context.Background())
	svc := workflow.NewCoverageService(db)
	repo := NewRepository(db)
	var cityID int64
	if err := db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `SELECT id FROM platform_admin.cities LIMIT 1`).Scan(&cityID)
	}); err != nil {
		t.Fatalf("query test city: %v", err)
	}

	coverage := &workflow.WeeklyCoverage{
		OrganizationID: testOrgID,
		BranchID:       testBranchID,
		CityID:         &cityID,
		DayOfWeek:      1,
		DistanceMeters: 15000,
		IsActive:       true,
	}
	if err := repo.SaveWeeklyCoverage(ctx, coverage); err != nil {
		t.Fatalf("save branch coverage: %v", err)
	}

	tests := []struct {
		name     string
		branchID int64
		want     bool
	}{
		{name: "covered vendor branch", branchID: testBranchID, want: true},
		{name: "different vendor branch", branchID: testBranchID + 1, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			served, _, err := svc.ServesPoint(ctx, testOrgID, time.Monday,
				workflow.Coord{CityID: &cityID}, tt.branchID)
			if err != nil {
				t.Fatalf("ServesPoint: %v", err)
			}
			if served != tt.want {
				t.Fatalf("served = %v, want %v", served, tt.want)
			}
		})
	}
}
