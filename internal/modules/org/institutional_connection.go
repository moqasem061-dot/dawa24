package org

import (
	"context"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/platform/reqcache"
)

// The institutional-work connection rule (العمل المؤسسي), in one place.
//
// A pharmacy branch may buy from a supplier branch only when the works the
// buyer's branch holds are connected — in org.institutional_work_connections —
// to a work the supplier's branch holds. It is a directed graph between
// branches, not a property of the product, and it is the rule the catalogue
// listings and the ordinary purchase path have always applied through
// commerce.CheckAvailability.
//
// It lives here because it was previously written out twice and the two copies
// did not agree. The composition root's availability probe implemented the rule
// above; smart ordering implemented a different one entirely — it intersected
// catalog.products.institutional_work_ids with the buyer organisation's
// EMPLOYEE work assignments, which is neither the same set nor the same
// question. The consequence was the defect this file exists to remove: a smart
// order's review screen showed lines as orderable that checkout then refused,
// so the buyer learned about the restriction at the last click, on the whole
// order at once, with no line named.
//
// Two callers, one rule: cmd/server's availability probe resolves a variant to
// its branch and calls this; the smart-order pipeline already knows the branch
// and calls it directly.

// InstitutionalConnection asks whether one buyer branch may buy from one
// supplier.
//
// VendorBranchID is the supplier branch the offer sits on. When it is nil the
// offer names no branch, and every branch of the supplier is considered — which
// is what a variant with no branch means everywhere else on the platform.
type InstitutionalConnection struct {
	BuyerBranchID  int64
	VendorOrgID    int64
	VendorBranchID *int64
}

// ConnectedWorkIDsForBranch returns the institutional work IDs that are connected
// to the given buyer branch's institutional works.
func (s *Service) ConnectedWorkIDsForBranch(ctx context.Context, branchID int64) ([]int64, error) {
	if branchID <= 0 {
		return nil, nil
	}
	buyerWorkIDs, err := s.branchWorkIDs(ctx, branchID)
	if err != nil {
		return nil, err
	}
	if len(buyerWorkIDs) == 0 {
		return nil, nil
	}
	return s.repo.GetConnectedInstitutionalWorkIDs(ctx, buyerWorkIDs)
}

// BranchesInstitutionallyConnected reports whether the buyer's branch holds an
// institutional work connected to one the supplier's branch holds.
//
// It fails closed. A buyer branch with no institutional works at all, a
// supplier with none, or no connection between them all return false — which is
// exactly what commerce.CheckAvailability does with the same facts, and the
// reason the two agree now.
func (s *Service) BranchesInstitutionallyConnected(ctx context.Context, c InstitutionalConnection) (bool, error) {
	if c.BuyerBranchID <= 0 || c.VendorOrgID <= 0 {
		return false, nil
	}

	allowed, err := s.ConnectedWorkIDsForBranch(ctx, c.BuyerBranchID)
	if err != nil {
		return false, err
	}
	if len(allowed) == 0 {
		return false, nil
	}

	vendorBranchIDs, err := s.vendorBranchIDs(ctx, c)
	if err != nil {
		return false, err
	}
	if len(vendorBranchIDs) == 0 {
		return false, nil
	}

	return s.repo.AnyBranchHasInstitutionalWork(ctx, vendorBranchIDs, allowed)
}

// vendorBranchIDs is the supplier branches an offer may be satisfied from.
//
// One branch when the offer names it; otherwise every branch the supplier has
// that is not inactive.
func (s *Service) vendorBranchIDs(ctx context.Context, c InstitutionalConnection) ([]int64, error) {
	if c.VendorBranchID != nil && *c.VendorBranchID > 0 {
		return []int64{*c.VendorBranchID}, nil
	}
	branches, err := s.repo.ListBranchesByOrg(ctx, c.VendorOrgID)
	if err != nil {
		return nil, err
	}
	out := make([]int64, 0, len(branches))
	for _, b := range branches {
		if b != nil && b.Status != "inactive" {
			out = append(out, b.ID)
		}
	}
	return out, nil
}

// branchWorkIDs reads a branch's institutional works from the join table, and
// falls back to the ids carried on the branch row itself.
//
// The fallback is not cosmetic: branches created before the join table existed
// still carry their works as strings in branches.institutional_works, and a
// rule that read only one of the two sources would refuse them.
func (s *Service) branchWorkIDs(ctx context.Context, branchID int64) ([]int64, error) {
	works, err := reqcache.Get(ctx, branchWorksKey(branchID), func() ([]*InstitutionalWork, error) {
		return s.repo.GetBranchInstitutionalWorks(ctx, branchID)
	})
	if err != nil {
		return nil, err
	}
	out := make([]int64, 0, len(works))
	for _, w := range works {
		if w != nil && w.ID > 0 {
			out = append(out, w.ID)
		}
	}
	if len(out) > 0 {
		return out, nil
	}

	branch, err := s.repo.GetBranchByID(ctx, branchID)
	if err != nil || branch == nil {
		// A branch that cannot be read is not a branch with no works; the
		// caller's own error handling decides what to do about the read.
		return nil, err
	}
	for _, raw := range branch.InstitutionalWorks {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if id, convErr := strconv.ParseInt(raw, 10, 64); convErr == nil && id > 0 {
			out = append(out, id)
			continue
		}
		// Fallback: resolve slug
		iw, err := s.repo.GetInstitutionalWorkBySlug(ctx, raw)
		if err == nil && iw != nil && iw.ID > 0 {
			out = append(out, iw.ID)
		} else if s.log != nil {
			s.log.WarnContext(ctx, "branch institutional work unresolved", "branch_id", branchID, "raw", raw)
		}
	}
	return out, nil
}

// BranchHasInstitutionalWorks reports whether a branch is allowed to trade at
// all — commerce refuses a purchase outright when the delivery branch carries
// no institutional work, and the smart order has to say the same thing at the
// review step rather than at checkout.
func (s *Service) BranchHasInstitutionalWorks(ctx context.Context, branchID int64) (bool, error) {
	ids, err := s.branchWorkIDs(ctx, branchID)
	if err != nil {
		return false, err
	}
	return len(ids) > 0, nil
}

// ListBranchesWithoutInstitutionalWorks returns branches that have zero resolvable institutional works.
func (s *Service) ListBranchesWithoutInstitutionalWorks(ctx context.Context) ([]*BranchWithoutWorks, error) {
	return s.repo.ListBranchesWithoutInstitutionalWorks(ctx)
}

// GetReachableBuyerWorksForBranch returns buyer institutional works that can reach this branch.
func (s *Service) GetReachableBuyerWorksForBranch(ctx context.Context, branchID int64) ([]*InstitutionalWork, error) {
	return s.repo.GetReachableBuyerWorksForBranch(ctx, branchID)
}
