package catalog

import "context"

// ListMatchDecisionsFiltered returns paginated decision memories using rich filters.
func (s *Service) ListMatchDecisionsFiltered(ctx context.Context, f DecisionMemoryFilter) ([]*MatchDecisionView, int, error) {
	return s.repo.ListMatchDecisionsFiltered(ctx, f)
}

// ListMatchDecisionsForOrgWithPlatform returns decisions visible to an organization (own + platform-inherited).
func (s *Service) ListMatchDecisionsForOrgWithPlatform(ctx context.Context, orgID int64, search string, limit, offset int) ([]*MatchDecisionView, int, error) {
	return s.repo.ListMatchDecisionsForOrgWithPlatform(ctx, orgID, search, limit, offset)
}

// GetDecisionMemoryPreference returns whether the organization has platform memory enabled.
func (s *Service) GetDecisionMemoryPreference(ctx context.Context, orgID int64) (bool, error) {
	return s.repo.GetDecisionMemoryPreference(ctx, orgID)
}

// SetDecisionMemoryPreference updates the organization's preference for platform decision memory.
func (s *Service) SetDecisionMemoryPreference(ctx context.Context, orgID int64, usePlatform bool, updatedBy int64) error {
	return s.repo.SetDecisionMemoryPreference(ctx, orgID, usePlatform, updatedBy)
}

// PromoteMatchDecision elevates an organization's decision to platform scope.
func (s *Service) PromoteMatchDecision(ctx context.Context, id int64, adminUserID int64) error {
	return s.repo.PromoteMatchDecision(ctx, id, adminUserID)
}

// DemoteMatchDecision reverts a decision back to organization scope.
func (s *Service) DemoteMatchDecision(ctx context.Context, id int64) error {
	return s.repo.DemoteMatchDecision(ctx, id)
}

// BulkPromoteMatchDecisions promotes multiple decisions to platform scope.
func (s *Service) BulkPromoteMatchDecisions(ctx context.Context, ids []int64, adminUserID int64) (int64, error) {
	return s.repo.BulkPromoteMatchDecisions(ctx, ids, adminUserID)
}

// BulkDeleteMatchDecisions deletes multiple decisions from the cache.
func (s *Service) BulkDeleteMatchDecisions(ctx context.Context, ids []int64) (int64, error) {
	return s.repo.BulkDeleteMatchDecisions(ctx, ids)
}

// RelinkMatchDecision updates the matched catalog product for a decision as admin.
func (s *Service) RelinkMatchDecision(ctx context.Context, id int64, productID *int64, adminUserID int64) error {
	return s.repo.RelinkMatchDecision(ctx, id, productID, adminUserID)
}

// RelinkMatchDecisionForOrg updates a decision for an organization.
func (s *Service) RelinkMatchDecisionForOrg(ctx context.Context, orgID int64, id int64, productID *int64, userID int64) error {
	return s.repo.RelinkMatchDecisionForOrg(ctx, orgID, id, productID, userID)
}
