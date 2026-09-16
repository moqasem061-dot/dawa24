package postgres

import (
	"context"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// TestCoverageWindowRoundTrip covers the two bugs that made the vendor coverage
// screen unusable, and through it the whole offer marketplace: ListOffersVisibleTo
// INNER JOINs workflow.weekly_coverages, so a vendor who cannot save a coverage
// row makes every customer offer listing return nothing.
//
//   - WRITE: coverage_from/coverage_to are Postgres TIME columns. The handler
//     passed a Go "" for a blank form field, and Postgres rejects that with
//     `invalid input syntax for type time: ""`. The write path now casts through
//     NULLIF($n,”)::time and the domain sends nil for a blank field.
//
//   - READ: every SELECT scanned the TIME columns straight into a Go string.
//     pgx maps TIME (OID 1083) to pgtype.Time, never to string, so every row
//     failed to scan and the page rendered its error state. Reads now go through
//     to_char(..., 'HH24:MI') via the shared coverageColumns constant.
//
// The pre-existing "Save and List Weekly Coverage" case in repository_test.go
// only ever used a filled-in window, which is why neither bug was caught.
func TestCoverageWindowRoundTrip(t *testing.T) {
	db := getTestDB(t)
	resetFixtures(t, db)

	repo := NewRepository(db)
	ctx := database.AsSystem(context.Background())

	t.Run("blank window is stored as NULL and read back as nil", func(t *testing.T) {
		cov := &workflow.WeeklyCoverage{
			OrganizationID: testOrgID,
			BranchID:       testBranchID,
			DayOfWeek:      2,
			CoverageFrom:   workflow.TimeOfDay(""), // what a blank form field produces
			CoverageTo:     workflow.TimeOfDay(""),
			Address:        "No window route",
			DistanceMeters: 7000,
			IsActive:       true,
		}

		// Bug A: this INSERT used to fail outright.
		if err := repo.SaveWeeklyCoverage(ctx, cov); err != nil {
			t.Fatalf("SaveWeeklyCoverage with a blank window failed: %v", err)
		}

		// Bug B: this SELECT used to fail to scan.
		got, err := repo.GetWeeklyCoverageByID(ctx, cov.ID)
		if err != nil {
			t.Fatalf("GetWeeklyCoverageByID failed: %v", err)
		}
		if got.CoverageFrom != nil {
			t.Errorf("CoverageFrom = %q, want nil for a blank window", *got.CoverageFrom)
		}
		if got.CoverageTo != nil {
			t.Errorf("CoverageTo = %q, want nil for a blank window", *got.CoverageTo)
		}
	})

	t.Run("a filled window survives the round trip as HH:MM", func(t *testing.T) {
		cov := &workflow.WeeklyCoverage{
			OrganizationID: testOrgID,
			BranchID:       testBranchID,
			DayOfWeek:      3,
			CoverageFrom:   workflow.TimeOfDay("09:00"),
			CoverageTo:     workflow.TimeOfDay("17:30"),
			Address:        "Windowed route",
			DistanceMeters: 8000,
			IsActive:       true,
		}
		if err := repo.SaveWeeklyCoverage(ctx, cov); err != nil {
			t.Fatalf("SaveWeeklyCoverage failed: %v", err)
		}

		got, err := repo.GetWeeklyCoverageByID(ctx, cov.ID)
		if err != nil {
			t.Fatalf("GetWeeklyCoverageByID failed: %v", err)
		}
		if got.CoverageFrom == nil || *got.CoverageFrom != "09:00" {
			t.Errorf("CoverageFrom = %v, want 09:00", deref(got.CoverageFrom))
		}
		// to_char must not widen HH:MM into HH:MM:SS — the UI compares and
		// re-submits this value.
		if got.CoverageTo == nil || *got.CoverageTo != "17:30" {
			t.Errorf("CoverageTo = %v, want 17:30", deref(got.CoverageTo))
		}
	})

	t.Run("the organization listing reads the window", func(t *testing.T) {
		// This is the exact query behind /vendor/coverage, which is the screen
		// that reported "تعذر تحميل جدول التغطية".
		list, err := repo.ListCoverageForOrganization(ctx, testOrgID)
		if err != nil {
			t.Fatalf("ListCoverageForOrganization failed: %v", err)
		}
		if len(list) == 0 {
			t.Fatal("expected the rows saved above, got none")
		}

		var sawBlank, sawFilled bool
		for _, v := range list {
			switch v.DayOfWeek {
			case 2:
				sawBlank = v.CoverageFrom == nil && v.CoverageTo == nil
			case 3:
				sawFilled = v.CoverageFrom != nil && *v.CoverageFrom == "09:00" &&
					v.CoverageTo != nil && *v.CoverageTo == "17:30"
			}
			if v.BranchName == "" {
				t.Errorf("coverage %d has no branch name; the join is not populating it", v.ID)
			}
		}
		if !sawBlank {
			t.Error("the blank-window row did not come back with nil bounds")
		}
		if !sawFilled {
			t.Error("the filled-window row did not come back as 09:00-17:30")
		}
	})

	t.Run("updating clears a window back to NULL", func(t *testing.T) {
		cov := &workflow.WeeklyCoverage{
			OrganizationID: testOrgID,
			BranchID:       testBranchID,
			DayOfWeek:      4,
			CoverageFrom:   workflow.TimeOfDay("10:00"),
			CoverageTo:     workflow.TimeOfDay("12:00"),
			Address:        "To be cleared",
			DistanceMeters: 3000,
			IsActive:       true,
		}
		if err := repo.SaveWeeklyCoverage(ctx, cov); err != nil {
			t.Fatalf("SaveWeeklyCoverage failed: %v", err)
		}

		cov.CoverageFrom = workflow.TimeOfDay("")
		cov.CoverageTo = workflow.TimeOfDay("")
		if err := repo.UpdateWeeklyCoverage(ctx, cov); err != nil {
			t.Fatalf("UpdateWeeklyCoverage clearing the window failed: %v", err)
		}

		got, err := repo.GetWeeklyCoverageByID(ctx, cov.ID)
		if err != nil {
			t.Fatalf("GetWeeklyCoverageByID failed: %v", err)
		}
		if got.CoverageFrom != nil || got.CoverageTo != nil {
			t.Errorf("window = %v-%v, want both nil after clearing",
				deref(got.CoverageFrom), deref(got.CoverageTo))
		}
	})
}

func deref(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}
