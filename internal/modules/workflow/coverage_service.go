package workflow

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// Coord represents geographic latitude and longitude, with optional CityID.
type Coord struct {
	Lat    float64 `json:"lat"`
	Lon    float64 `json:"lon"`
	CityID *int64  `json:"city_id,omitempty"`
}

// CoverageService evaluates spatial reachability between suppliers and pharmacies.
type CoverageService struct {
	db *database.DB
}

// NewCoverageService constructs a new CoverageService.
func NewCoverageService(db *database.DB) *CoverageService {
	return &CoverageService{db: db}
}

// ServesPoint checks whether an organization covers the given location coordinates or city on a specified weekday.
// Per decision Q1 (see Master Plan), time-of-day windows are not enforced for order gating; coverage is evaluated on weekday and spatial/city reachability.
// Returns (serves bool, distanceMeters int, err error).
func (cs *CoverageService) ServesPoint(ctx context.Context, orgID int64, day time.Weekday, target Coord, branchID ...int64) (bool, int, error) {
	dayInt := int(day) // 0 = Sunday, 1 = Monday ...
	vendorBranchID := int64(0)
	if len(branchID) > 0 {
		vendorBranchID = branchID[0]
	}
	var distanceMeters int
	var actualMeters *int

	var targetCityID int64
	if target.CityID != nil && *target.CityID > 0 {
		targetCityID = *target.CityID
	}
	hasCoords := target.Lat != 0 || target.Lon != 0

	err := cs.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		// Evaluates coverage for the specified weekday (or all-week coverage).
		// A branch is covered if the vendor explicitly covers the branch's city,
		// or if the branch's GPS coordinates fall within the vendor's delivery radius.
		query := `
			SELECT COALESCE(NULLIF(wc.distance_meters, 0), NULLIF(c.coverage_radius_meters, 0), 15000) AS allowed_radius,
			       CASE 
			           WHEN COALESCE(wc.latitude, c.latitude, b.latitude) IS NOT NULL AND $5::boolean = true THEN
			               platform.distance_meters(
			                   COALESCE(wc.latitude, c.latitude, b.latitude)::numeric,
			                   COALESCE(wc.longitude, c.longitude, b.longitude)::numeric,
			                   $2::numeric,
			                   $3::numeric
			               )::integer
			           ELSE 0
			       END AS actual_meters
			FROM workflow.weekly_coverages wc
			LEFT JOIN platform_admin.cities c ON c.id = wc.city_id
			LEFT JOIN org.branches b ON b.id = wc.branch_id
			WHERE wc.organization_id = $1::bigint
			  AND ($7::bigint = 0 OR wc.branch_id IS NULL OR wc.branch_id = $7::bigint)
			  AND (wc.day_of_week = $4::integer OR wc.day_of_week IS NULL)
			  AND wc.is_active = true
			  -- A deleted or deactivated supplier branch ships nothing.
			  AND (wc.branch_id IS NULL OR (b.deleted_at IS NULL AND b.status <> 'inactive'))
			  AND (
			      -- 1. Direct city match: if vendor covers this city, all branches in this city are covered
			      ($6::bigint > 0 AND wc.city_id = $6::bigint)
			      OR
			      -- 2. Spatial distance match: branch coordinates fall within coverage radius
			      (
			          $5::boolean = true
			          AND COALESCE(wc.latitude, c.latitude, b.latitude) IS NOT NULL
			          AND COALESCE(wc.longitude, c.longitude, b.longitude) IS NOT NULL
			          AND platform.distance_meters(
			              COALESCE(wc.latitude, c.latitude, b.latitude)::numeric,
			              COALESCE(wc.longitude, c.longitude, b.longitude)::numeric,
			              $2::numeric,
			              $3::numeric
			          )::integer <= COALESCE(NULLIF(wc.distance_meters, 0), NULLIF(c.coverage_radius_meters, 0), 15000)
			      )
			  )
			ORDER BY (wc.day_of_week = $4::integer) DESC, actual_meters ASC
			LIMIT 1;
		`
		err := tx.QueryRow(txCtx, query, orgID, target.Lat, target.Lon, dayInt, hasCoords, targetCityID, vendorBranchID).Scan(&distanceMeters, &actualMeters)
		if err == nil {
			return nil
		}
		if !database.IsNotFound(err) {
			return err
		}

		// If vendor has no active coverage for today's weekday, fail closed.
		return pgx.ErrNoRows
	})

	if err != nil {
		if database.IsNotFound(err) {
			return false, 0, nil
		}
		return false, 0, fmt.Errorf("coverage.ServesPoint: %w", err)
	}

	retMeters := 0
	if actualMeters != nil {
		retMeters = *actualMeters
	}
	return true, retMeters, nil
}
