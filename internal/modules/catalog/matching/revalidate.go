package matching

import (
	"context"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// Revalidate checks if a decision made under an older revision needs revalidation.
func (s *serviceImpl) Revalidate(ctx context.Context, sc Scope, ref RowRef, d matchflow.RowDecision) (matchflow.RowDecision, error) {
	if d.ProductID == nil || s.repo == nil {
		return d, nil
	}
	catRev, err := s.repo.CurrentRevision(ctx, "master")
	if err != nil || d.Versions.CatalogRevision == catRev {
		return d, nil
	}
	if s.indexCache == nil {
		return d, nil
	}
	idx, err := s.indexCache.Get(ctx, "master", catRev)
	if err != nil || idx == nil {
		// The catalogue cannot be read to check against. The association was
		// valid when it was made, and refusing every commit while the catalogue
		// is briefly unavailable is worse than committing what was checked.
		return d, nil
	}

	// The catalogue moved on since this row was decided. What matters is
	// whether THIS product did: still there, still active, still the same
	// strength, form and name it had when the row was matched to it.
	p, ok := idx.Lookup(*d.ProductID)
	changed := !ok || p == nil || (d.Versions.Target != "" && ProductFactFingerprint(p) != d.Versions.Target)
	if changed {
		if !hasStringFlag(d.Flags, "catalog_changed") {
			d.Flags = append(d.Flags, "catalog_changed")
		}
		d.State = matchflow.StateNeedsReview
		d.Reasons = append([]matchflow.Reason{{Code: string(productmatch.ReasonCatalogChanged)}}, d.Reasons...)
		return d, nil
	}
	d.Versions.CatalogRevision = catRev
	return d, nil
}

func hasStringFlag(flags []string, want string) bool {
	for _, f := range flags {
		if f == want {
			return true
		}
	}
	return false
}
