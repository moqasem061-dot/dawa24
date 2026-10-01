package matching

import (
	"context"
)

// ListPolicies lists all registered match policies.
func (s *serviceImpl) ListPolicies(ctx context.Context) ([]PolicyRecord, error) {
	if s.repo == nil {
		return nil, ErrNotImplemented
	}
	return s.repo.ListPolicies(ctx)
}

// ListAssignments lists all policy assignments across workflows and cohorts.
func (s *serviceImpl) ListAssignments(ctx context.Context) ([]PolicyAssignment, error) {
	if s.repo == nil {
		return nil, ErrNotImplemented
	}
	return s.repo.ListAssignments(ctx)
}

// UpdateAssignment updates an active/shadow policy assignment with audit logging (US13 T152).
func (s *serviceImpl) UpdateAssignment(ctx context.Context, workflow, cohort, activeVersion string, shadowVersion *string, changedBy, reason string) error {
	if s.repo == nil {
		return ErrNotImplemented
	}
	return s.repo.UpdateAssignment(ctx, workflow, cohort, activeVersion, shadowVersion, changedBy, reason)
}

// RollbackAssignment rolls back an assignment to the previously active version (US13 T152).
func (s *serviceImpl) RollbackAssignment(ctx context.Context, workflow, cohort, changedBy string) error {
	if s.repo == nil {
		return ErrNotImplemented
	}
	return s.repo.RollbackAssignment(ctx, workflow, cohort, changedBy)
}
