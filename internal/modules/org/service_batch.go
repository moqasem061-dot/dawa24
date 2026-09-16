package org

import (
	"context"
)

// GetOrganizations resolves many organizations in one query and indexes them
// by ID. Callers that render supplier names for a whole page of offers use
// this instead of issuing one GetOrganization per variant.
func (s *Service) GetOrganizations(ctx context.Context, ids []int64) (map[int64]*Organization, error) {
	out := make(map[int64]*Organization, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	list, err := s.repo.GetOrganizationsByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, o := range list {
		if o != nil {
			out[o.ID] = o
		}
	}
	return out, nil
}

// GetBranches resolves many branches in one query and indexes them by ID.
func (s *Service) GetBranches(ctx context.Context, ids []int64) (map[int64]*Branch, error) {
	out := make(map[int64]*Branch, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	list, err := s.repo.GetBranchesByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, b := range list {
		if b != nil {
			out[b.ID] = b
		}
	}
	return out, nil
}

// uniqueInt64s returns ids without duplicates or non-positive values.
func uniqueInt64s(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
