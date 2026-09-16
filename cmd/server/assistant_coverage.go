package main

import (
	"context"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// Satisfying the assistant's coverage question with the platform's own rule.
//
// The assistant declares assistant.CoverageProbe and imports nothing; this is
// the composition root's answer to it, and it is deliberately thin. It resolves
// a branch to a point exactly the way internal/ui/buying_coverage.go does —
// same fields, same "no location means nobody covers you" verdict — and then
// asks workflow.CoverageService, which is the one place the coverage predicate
// lives.
//
// Thin matters here. Every line of judgement added to this adapter is a line
// that can disagree with what checkout does, and an assistant that names a
// supplier checkout refuses is worse than an assistant that says nothing.
type assistantCoverageProbe struct {
	coverage *workflow.CoverageService
	workflow *workflow.Service
	orgs     *org.Service
}

var _ assistant.CoverageProbe = (*assistantCoverageProbe)(nil)

// VendorsServingBranch reports the suppliers covering one branch on one day.
func (p *assistantCoverageProbe) VendorsServingBranch(
	ctx context.Context, branchID int64, day time.Weekday,
) (assistant.CoverageAnswer, error) {
	if p == nil || p.coverage == nil || p.orgs == nil || branchID <= 0 {
		return assistant.CoverageAnswer{}, nil
	}

	// AsSystem: the branch is the caller's own — the tool resolved it from a
	// handle bound to this caller, or from their session — but reading it back
	// crosses into org's tables, which is exactly the case AsSystem is the
	// greppable marker for.
	branch, err := p.orgs.GetBranch(database.AsSystem(ctx), branchID)
	if err != nil || branch == nil {
		return assistant.CoverageAnswer{}, err
	}

	coord := workflow.Coord{CityID: branch.CityID}
	if branch.Latitude != nil {
		coord.Lat = *branch.Latitude
	}
	if branch.Longitude != nil {
		coord.Lon = *branch.Longitude
	}
	hasLocation := (branch.Latitude != nil && branch.Longitude != nil) ||
		(branch.CityID != nil && *branch.CityID > 0)

	answer := assistant.CoverageAnswer{BranchName: branchName(branch)}
	if !hasLocation {
		// Coverage was evaluated and admits nobody, which is what
		// CheckAvailability decides for the same branch (branch_no_location).
		// Reporting "could not evaluate" here would invite the model to answer
		// from something else.
		answer.Evaluated = true
		return answer, nil
	}

	vendorIDs, err := p.coverage.VendorsServing(ctx, day, coord)
	if err != nil {
		return assistant.CoverageAnswer{}, err
	}
	answer.Evaluated = true

	for _, id := range vendorIDs {
		row := assistant.CoverageVendorRow{ID: id}
		supplier, err := p.orgs.GetOrganization(database.AsSystem(ctx), id)
		if err == nil && supplier != nil {
			row.Name = organizationName(supplier)
		}
		if row.Name == "" {
			// A supplier whose name cannot be read is dropped rather than
			// reported as an id: an id is not an answer, and it is the one
			// thing the prompt rules forbid the model to repeat.
			continue
		}
		row.Windows, row.Area = p.windowsFor(ctx, id, day)
		answer.Vendors = append(answer.Vendors, row)
	}
	return answer, nil
}

// windowsFor reads the supplier's published delivery windows for one weekday.
//
// A supplier that covers a day without publishing a time returns no window,
// which the tool renders as "covers the day" rather than inventing hours.
func (p *assistantCoverageProbe) windowsFor(
	ctx context.Context, orgID int64, day time.Weekday,
) (windows []string, area string) {
	if p.workflow == nil {
		return nil, ""
	}
	views, err := p.workflow.ListCoverageForOrganization(database.AsSystem(ctx), orgID)
	if err != nil {
		return nil, ""
	}
	for _, v := range views {
		if v == nil || !v.IsActive || v.DayOfWeek != int(day) {
			continue
		}
		if area == "" {
			area = strings.TrimSpace(firstNonEmpty(v.CityNameAr, v.CityName, v.GovernorateNameAr, v.GovernorateName))
		}
		if v.CoverageFrom == nil || v.CoverageTo == nil {
			continue
		}
		from, to := trimClock(*v.CoverageFrom), trimClock(*v.CoverageTo)
		if from == "" || to == "" {
			continue
		}
		window := from + "-" + to
		if !contains(windows, window) {
			windows = append(windows, window)
		}
	}
	return windows, area
}

// trimClock renders a stored time as HH:MM. The column is a time without a
// zone and arrives as "09:00:00"; the seconds are noise in an answer.
func trimClock(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 5 {
		return s[:5]
	}
	return s
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// branchName and organizationName pick the Arabic label the answer will use.
//
// The prompt rules require the model to repeat a record's name verbatim so the
// reader can linkify it, which makes picking the right one here load-bearing
// rather than cosmetic.
func branchName(b *org.Branch) string {
	if b == nil {
		return ""
	}
	return strings.TrimSpace(b.Name.Get(i18n.AR))
}

func organizationName(o *org.Organization) string {
	if o == nil {
		return ""
	}
	if name := strings.TrimSpace(o.TradeName.Get(i18n.AR)); name != "" {
		return name
	}
	if name := strings.TrimSpace(o.Name.Get(i18n.AR)); name != "" {
		return name
	}
	return strings.TrimSpace(o.LegalName)
}
