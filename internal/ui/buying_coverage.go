package ui

import (
	"context"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// The buying surfaces' view of coverage.
//
// Every listing that pages in SQL needs the covering suppliers as a set before
// it runs its query, because a predicate applied to the rows a page returns
// cannot correct the count that page was cut from. This is that resolution, in
// one place, so the catalogue and the supplier profile cannot disagree about
// who delivers.

// coveringVendorsFor resolves which suppliers can deliver to a buying branch
// today.
//
// The second return value distinguishes "coverage was evaluated" from "coverage
// does not apply here". A visitor with no branch selected is browsing rather
// than buying and still sees the catalogue; a pharmacy whose branch nobody
// reaches sees an empty one, which is the truthful answer and the one the pager
// must agree with.
//
// A branch with neither coordinates nor a city is not covered by anyone —
// commerce.CheckAvailability refuses it with branch_no_location, and a listing
// that showed those offers anyway would be offering rows checkout will refuse.
func (h *UIHandler) coveringVendorsFor(ctx context.Context, branchID int64) ([]int64, bool) {
	c := h.coveringVendorBranchesFor(ctx, branchID)
	return c.OrgIDs, c.Evaluated
}

// buyingCoverage is who delivers to one buying branch today, as sets.
type buyingCoverage struct {
	// OrgIDs reach the branch from at least one coverage row.
	OrgIDs []int64
	// BranchIDs are the supplier branches whose own rows reach it.
	BranchIDs []int64
	// OrgWideIDs reach it from a row bound to no branch, which covers every
	// branch of the supplier, as workflow.ServesPoint and so checkout treat it.
	OrgWideIDs []int64
	// Evaluated distinguishes "nobody reaches this branch" from "coverage does
	// not apply" (no branch: browsing, not buying).
	Evaluated bool
}

// coveringVendorBranchesFor resolves the supplier organizations and branches
// that cover a buying branch. A vendor can cover Cairo from one branch while
// its Aswan variant belongs to another branch, so an organization-only set is
// not sufficient for offer-level pagination.
func (h *UIHandler) coveringVendorBranchesFor(ctx context.Context, branchID int64) buyingCoverage {
	if branchID <= 0 {
		return buyingCoverage{}
	}
	if h.coverageSvc == nil || h.orgSvc == nil {
		return buyingCoverage{Evaluated: true}
	}

	branch, err := h.orgSvc.GetBranch(database.AsSystem(ctx), branchID)
	if err != nil || branch == nil {
		return buyingCoverage{Evaluated: true}
	}
	coord, hasLocation := branchCoord(branch)
	if !hasLocation {
		// Nothing can be evaluated against a branch with no location, and
		// showing everything would contradict the purchase rule. Coverage
		// applies and admits nobody.
		return buyingCoverage{Evaluated: true}
	}

	covered, err := h.coverageSvc.VendorBranchesServing(ctx, time.Now().Weekday(), coord)
	if err != nil {
		h.log.WarnContext(ctx, "could not resolve covering suppliers for a buying branch",
			"branch_id", branchID, "error", err)
		// Coverage is a purchase precondition. An outage must not turn into an
		// unfiltered catalogue; the shared availability check also fails closed.
		return buyingCoverage{Evaluated: true}
	}
	out := buyingCoverage{Evaluated: true}
	orgSet := make(map[int64]bool, len(covered))
	wideSet := make(map[int64]bool)
	for _, row := range covered {
		if row.OrganizationID <= 0 {
			continue
		}
		if !orgSet[row.OrganizationID] {
			orgSet[row.OrganizationID] = true
			out.OrgIDs = append(out.OrgIDs, row.OrganizationID)
		}
		if row.BranchID > 0 {
			out.BranchIDs = append(out.BranchIDs, row.BranchID)
		} else if !wideSet[row.OrganizationID] {
			wideSet[row.OrganizationID] = true
			out.OrgWideIDs = append(out.OrgWideIDs, row.OrganizationID)
		}
	}
	return out
}

// branchCoord is a branch's location for coverage, and whether it has one.
func branchCoord(branch *org.Branch) (workflow.Coord, bool) {
	coord := workflow.Coord{CityID: branch.CityID}
	if branch.Latitude != nil {
		coord.Lat = *branch.Latitude
	}
	if branch.Longitude != nil {
		coord.Lon = *branch.Longitude
	}
	has := (branch.Latitude != nil && branch.Longitude != nil) ||
		(branch.CityID != nil && *branch.CityID > 0)
	return coord, has
}
