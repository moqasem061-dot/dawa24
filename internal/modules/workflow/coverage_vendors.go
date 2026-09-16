package workflow

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// VendorBranchCoverage identifies the supplier branch behind one coverage row.
// BranchID zero means the vendor coverage is organization-wide.
type VendorBranchCoverage struct {
	OrganizationID int64
	BranchID       int64
}

// Which suppliers can reach one point, as a set.
//
// ServesPoint answers the question one supplier at a time, which is the right
// shape for a purchase: there is one supplier and one branch and the answer
// gates one order. It is the wrong shape for a listing.
//
// A listing pages in SQL, so every predicate that decides whether a row belongs
// in the result has to be IN that SQL or the count is a lie. Coverage was the
// one predicate left in Go: the buying catalogue counted 1,695 offers for a
// Cairo branch and rendered two of them, because 1,553 of those offers came
// from a supplier that does not deliver to Cairo and were dropped after the
// page had already been cut. That is the same defect the offer-level pagination
// was built to remove, moved one layer down.
//
// Answering it as a set fixes that with one extra query per page: the covering
// suppliers are resolved once, and the listing filters on the resulting ids. It
// is the same predicate ServesPoint applies — the WHERE clause below is
// deliberately a copy of that one — so a row the listing shows cannot be a row
// checkout refuses for coverage.

// VendorsServing returns the organization ids that cover the given point on the
// given weekday.
//
// A supplier appears when it covers the target's city explicitly, or when the
// target's coordinates fall inside one of its delivery radii. Both arms are the
// ones ServesPoint uses; if that rule changes, this changes with it.
//
// The result is deliberately unbounded: a marketplace has tens of suppliers,
// not thousands, and a caller that filters on the set needs all of it.
func (cs *CoverageService) VendorsServing(ctx context.Context, day time.Weekday, target Coord) ([]int64, error) {
	rows, err := cs.VendorBranchesServing(ctx, day, target)
	if err != nil {
		return nil, err
	}
	seen := make(map[int64]bool, len(rows))
	result := make([]int64, 0, len(rows))
	for _, row := range rows {
		if row.OrganizationID > 0 && !seen[row.OrganizationID] {
			seen[row.OrganizationID] = true
			result = append(result, row.OrganizationID)
		}
	}
	return result, nil
}

// VendorBranchesServing returns the supplier branches that cover a point.
// It uses the same weekday, city, radius, and active-row predicate as
// ServesPoint, but retains branch identity for branch-pinned product variants.
func (cs *CoverageService) VendorBranchesServing(ctx context.Context, day time.Weekday, target Coord) ([]VendorBranchCoverage, error) {
	if cs == nil || cs.db == nil {
		// Fail closed. A caller that cannot prove coverage must show nothing
		// rather than everything, for the same reason CheckAvailability refuses
		// without a probe.
		return nil, nil
	}

	dayInt := int(day)
	var targetCityID int64
	if target.CityID != nil && *target.CityID > 0 {
		targetCityID = *target.CityID
	}
	hasCoords := target.Lat != 0 || target.Lon != 0

	var out []VendorBranchCoverage
	err := cs.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, `
			SELECT DISTINCT wc.organization_id, COALESCE(wc.branch_id, 0)
			FROM workflow.weekly_coverages wc
			LEFT JOIN platform_admin.cities c ON c.id = wc.city_id
			LEFT JOIN org.branches b ON b.id = wc.branch_id
			WHERE wc.is_active = true
			  AND (wc.branch_id IS NULL OR (b.deleted_at IS NULL AND b.status <> 'inactive'))
			  AND (wc.day_of_week = $3::integer OR wc.day_of_week IS NULL)
			  AND (
			      ($4::bigint > 0 AND wc.city_id = $4::bigint)
			      OR (
			          $5::boolean = true
			          AND COALESCE(wc.latitude, c.latitude, b.latitude) IS NOT NULL
			          AND COALESCE(wc.longitude, c.longitude, b.longitude) IS NOT NULL
			          AND platform.distance_meters(
			              COALESCE(wc.latitude, c.latitude, b.latitude)::numeric,
			              COALESCE(wc.longitude, c.longitude, b.longitude)::numeric,
			              $1::numeric,
			              $2::numeric
			          )::integer <= COALESCE(NULLIF(wc.distance_meters, 0), NULLIF(c.coverage_radius_meters, 0), 15000)
			      )
			  );`, target.Lat, target.Lon, dayInt, targetCityID, hasCoords)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var row VendorBranchCoverage
			if err := rows.Scan(&row.OrganizationID, &row.BranchID); err != nil {
				return err
			}
			if row.OrganizationID > 0 {
				out = append(out, row)
			}
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("coverage.VendorsServing: %w", err)
	}
	return out, nil
}
