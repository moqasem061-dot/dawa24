package org

import "context"

// GetUserOrgLink returns one customer link by id, or nil when no such link
// exists. Callers authorize the link first with AuthorizeUserOrgLink.
func (s *Service) GetUserOrgLink(ctx context.Context, id int64) (*UserOrganization, error) {
	return s.repo.GetUserOrganizationByID(ctx, id)
}
