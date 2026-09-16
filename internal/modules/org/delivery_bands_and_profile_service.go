package org

import (
	"context"
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// SaveSocialMedia updates an organization's social media links.
func (s *Service) SaveSocialMedia(ctx context.Context, orgID int64, links []*SocialMedia) error {
	return s.repo.SaveSocialMedia(ctx, orgID, links)
}

// UpdateOrganization updates organization details.
func (s *Service) UpdateOrganization(ctx context.Context, o *Organization) error {
	if err := o.Validate(); err != nil {
		return err
	}
	defer forgetOrganization(ctx, o.ID)
	return s.repo.UpdateOrganization(ctx, o)
}

// DeleteOrganization deactivates an organization.
func (s *Service) DeleteOrganization(ctx context.Context, id int64) error {
	defer forgetOrganization(ctx, id)
	return s.repo.DeleteOrganization(ctx, id)
}

// UpdateMemberRole changes a member role.

func (s *Service) UpdateMemberRole(ctx context.Context, orgID, userID int64, role string) error {
	return s.repo.UpdateMemberRole(ctx, orgID, userID, role)
}

// GetDeliveryBands retrieves delivery bands for distance pricing.
func (s *Service) GetDeliveryBands(ctx context.Context, orgID int64) ([]*DeliveryBand, error) {
	return s.repo.GetDeliveryBands(ctx, orgID)
}

// SaveDeliveryBands updates the delivery bands for an organization.
//
// It validates first. A table with two tiers both covering seven kilometres has
// no correct answer for a seven-kilometre delivery, and storing it means the
// cart, the checkout and the invoice can each pick a different one. See
// ValidateDeliveryBands.
func (s *Service) SaveDeliveryBands(ctx context.Context, orgID int64, bands []*DeliveryBand) error {
	if err := ValidateDeliveryBands(bands); err != nil {
		return err
	}
	return s.repo.SaveDeliveryBands(ctx, orgID, bands)
}

// CalculateDeliveryFee prices one distance against a vendor's declared bands.
//
// Kept as the narrow "just the money" form for callers that have a distance
// already; everything about how the answer is reached lives in QuoteDelivery,
// so this can no longer drift from what the cart shows. The bool reports
// whether a band actually decided the fee.
func (s *Service) CalculateDeliveryFee(
	ctx context.Context, vendorOrgID int64, distanceMeters int,
) (money.Amount, bool, error) {
	bands, err := s.repo.GetDeliveryBands(ctx, vendorOrgID)
	if err != nil {
		return money.Zero, false, err
	}
	q := QuoteDelivery(bands, distanceMeters)
	return q.Fee, q.Band != nil, nil
}

// GetReviewCriteria returns review criteria for a given context.
func (s *Service) GetReviewCriteria(ctx context.Context, contextType string) ([]*ReviewCriterion, error) {
	return s.repo.GetReviewCriteria(ctx, contextType)
}

// AddReviewWithRatings adds a multi-criteria review with verified rating weights.
func (s *Service) AddReviewWithRatings(ctx context.Context, rev *Review, ratings []ReviewRating) error {
	if rev.Rating < 1 || rev.Rating > 5 {
		return apperr.Validation("review.rating_invalid", "Rating must be between 1 and 5.", nil)
	}
	return s.repo.AddReviewWithRatings(ctx, rev, ratings)
}

// SubmitReview records a review and its individual criteria ratings.
func (s *Service) SubmitReview(ctx context.Context, rev *Review) error {
	if rev.Rating < 1 {
		rev.Rating = 5
	}
	if rev.Rating > 5 {
		rev.Rating = 5
	}
	if err := s.repo.AddReview(ctx, rev); err != nil {
		return err
	}
	s.log.InfoContext(ctx, "review submitted", "org_id", rev.OrganizationID, "user_id", rev.UserID, "rating", rev.Rating)
	return nil
}

// ListReviewsForVendor returns all reviews received by a vendor.
func (s *Service) ListReviewsForVendor(ctx context.Context, vendorOrgID int64, limit, offset int) ([]*Review, error) {
	return s.repo.ListReviewsForVendor(ctx, vendorOrgID, limit, offset)
}

// GetReviewByOrderAndVendor gets a review for a specific vendor on an order.
func (s *Service) GetReviewByOrderAndVendor(ctx context.Context, orderID, vendorOrgID int64) (*Review, error) {
	return s.repo.GetReviewByOrderAndVendor(ctx, orderID, vendorOrgID)
}

// ListReviewsForOrder lists all reviews submitted for an order.
func (s *Service) ListReviewsForOrder(ctx context.Context, orderID int64) ([]*Review, error) {
	return s.repo.ListReviewsForOrder(ctx, orderID)
}

// ReplyToReview posts a vendor reply to a pharmacy review.
func (s *Service) ReplyToReview(ctx context.Context, reviewID, orgID int64, response string, responderID int64) error {
	if strings.TrimSpace(response) == "" {
		return apperr.Validation("review.response_empty", "Reply message cannot be empty.", nil)
	}
	return s.repo.ReplyToReview(ctx, reviewID, orgID, response, responderID)
}

// HasDeliveredOrderFromVendor verifies that a customer pharmacy has received at least one delivered order from a vendor.
func (s *Service) HasDeliveredOrderFromVendor(ctx context.Context, customerOrgID, vendorOrgID int64) (bool, error) {
	return s.repo.HasDeliveredOrderFromVendor(ctx, customerOrgID, vendorOrgID)
}

// CreateInstitutionalWork creates a new institutional structure category.
func (s *Service) CreateInstitutionalWork(ctx context.Context, iw *InstitutionalWork) error {
	if err := s.repo.CreateInstitutionalWork(ctx, iw); err != nil {
		return err
	}
	s.log.InfoContext(ctx, "institutional work created", "id", iw.ID, "title_ar", iw.Title.Get("ar"))
	return nil
}

// GetInstitutionalWork returns an institutional work by its ID.
func (s *Service) GetInstitutionalWork(ctx context.Context, id int64) (*InstitutionalWork, error) {
	return s.repo.GetInstitutionalWorkByID(ctx, id)
}

// UpdateInstitutionalWork updates an existing institutional category.
func (s *Service) UpdateInstitutionalWork(ctx context.Context, iw *InstitutionalWork) error {
	return s.repo.UpdateInstitutionalWork(ctx, iw)
}

// DeleteInstitutionalWork soft-deletes an institutional category.
func (s *Service) DeleteInstitutionalWork(ctx context.Context, id int64) error {
	return s.repo.DeleteInstitutionalWork(ctx, id)
}

// ToggleInstitutionalWorkStatus toggles active/inactive state of an institutional category.
func (s *Service) ToggleInstitutionalWorkStatus(ctx context.Context, id int64) error {
	return s.repo.ToggleInstitutionalWorkStatus(ctx, id)
}

// ListInstitutionalWorks returns the full hierarchical tree of institutional categories.
func (s *Service) ListInstitutionalWorks(ctx context.Context, onlyActive bool) ([]*InstitutionalWork, error) {
	return s.repo.ListInstitutionalWorks(ctx, onlyActive)
}

// ListAllFlatInstitutionalWorks returns all institutional categories in a flat list with hierarchy level.
func (s *Service) ListAllFlatInstitutionalWorks(ctx context.Context, onlyActive bool) ([]*InstitutionalWork, error) {
	return s.repo.ListAllFlatInstitutionalWorks(ctx, onlyActive)
}

// CanConnectInstitutionalWorks checks if entity fromID is permitted to connect to entity toID.
func (s *Service) CanConnectInstitutionalWorks(ctx context.Context, fromID, toID int64) (bool, error) {
	return s.repo.CanConnectInstitutionalWorks(ctx, fromID, toID)
}

// AssignBranchInstitutionalWorks assigns institutional categories to a branch.
func (s *Service) AssignBranchInstitutionalWorks(ctx context.Context, branchID int64, workIDs []int64) error {
	defer forgetBranch(ctx, branchID)
	return s.repo.AssignBranchInstitutionalWorks(ctx, branchID, workIDs)
}

// GetBranchInstitutionalWorks returns all institutional categories assigned to a branch.
func (s *Service) GetBranchInstitutionalWorks(ctx context.Context, branchID int64) ([]*InstitutionalWork, error) {
	return s.repo.GetBranchInstitutionalWorks(ctx, branchID)
}

// GetConnectedInstitutionalWorkIDs returns connected target institutional work IDs for given source work IDs.
func (s *Service) GetConnectedInstitutionalWorkIDs(ctx context.Context, fromWorkIDs []int64) ([]int64, error) {
	return s.repo.GetConnectedInstitutionalWorkIDs(ctx, fromWorkIDs)
}

// AllowedWorkIDs returns the institutional work ids a user may see products for.
// Implements the two Laravel institutional filter modes documented in institutional_work_filter.md.
func (s *Service) AllowedWorkIDs(ctx context.Context, userID int64, mode InstitutionalFilterMode) ([]int64, error) {
	if userID <= 0 {
		return []int64{}, nil
	}
	userWorks, err := s.repo.GetUserInstitutionalWorkIDs(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(userWorks) == 0 {
		return []int64{}, nil
	}

	switch mode {
	case FilterSimple:
		return userWorks, nil
	case FilterWithConnections:
		return s.repo.GetConnectedInstitutionalWorkIDs(ctx, userWorks)
	default:
		return userWorks, nil
	}
}

// AssignEmployeeInstitutionalWork assigns an institutional work group to a user.
func (s *Service) AssignEmployeeInstitutionalWork(ctx context.Context, orgID, userID, workID int64) error {
	if userID <= 0 || workID <= 0 {
		return apperr.Validation("institutional.invalid_params", "User ID and Work ID are required.", nil)
	}
	if err := s.repo.AssignEmployeeInstitutionalWork(ctx, orgID, userID, workID); err != nil {
		return err
	}
	s.log.InfoContext(ctx, "assigned employee institutional work", "org_id", orgID, "user_id", userID, "work_id", workID)
	return nil
}

// RemoveEmployeeInstitutionalWork removes an institutional work assignment from a user.
func (s *Service) RemoveEmployeeInstitutionalWork(ctx context.Context, orgID, userID, workID int64) error {
	if userID <= 0 || workID <= 0 {
		return apperr.Validation("institutional.invalid_params", "User ID and Work ID are required.", nil)
	}
	if err := s.repo.RemoveEmployeeInstitutionalWork(ctx, orgID, userID, workID); err != nil {
		return err
	}
	s.log.InfoContext(ctx, "removed employee institutional work", "org_id", orgID, "user_id", userID, "work_id", workID)
	return nil
}

// ListEmployeeInstitutionalWorks lists all institutional work assignments for a user.
func (s *Service) ListEmployeeInstitutionalWorks(ctx context.Context, userID int64) ([]*EmployeeInstitutionalWork, error) {
	return s.repo.ListEmployeeInstitutionalWorks(ctx, userID)
}

// ListOrgEmployeeInstitutionalWorks lists all employee institutional work assignments for an organization.
func (s *Service) ListOrgEmployeeInstitutionalWorks(ctx context.Context, orgID int64) ([]*EmployeeInstitutionalWork, error) {
	return s.repo.ListOrgEmployeeInstitutionalWorks(ctx, orgID)
}

// CreateUserOrgLink allows a customer user or vendor to link an organization number.
func (s *Service) CreateUserOrgLink(ctx context.Context, userID int64, customerOrgID *int64, vendorOrgID int64, orgNumber string, initialStatus UserOrganizationStatus) (*UserOrganization, error) {
	if userID <= 0 || vendorOrgID <= 0 {
		return nil, apperr.Validation("user_org.invalid_params", "User ID and Vendor Organization ID are required.", nil)
	}
	orgNumber = strings.TrimSpace(orgNumber)
	if orgNumber == "" {
		return nil, apperr.Validation("user_org.number_required", "Organization Number is required.", nil)
	}
	if initialStatus == "" {
		initialStatus = UserOrgStatusPending
	}
	uo := &UserOrganization{
		UserID:             userID,
		CustomerOrgID:      customerOrgID,
		VendorOrgID:        vendorOrgID,
		OrganizationNumber: orgNumber,
		Status:             initialStatus,
	}
	if err := s.repo.CreateUserOrganization(ctx, uo); err != nil {
		return nil, err
	}
	s.log.InfoContext(ctx, "created user organization link", "user_id", userID, "vendor_org_id", vendorOrgID, "org_number", orgNumber, "status", initialStatus)
	return uo, nil
}

// UpdateUserOrgLink updates the organization number or notes.
func (s *Service) UpdateUserOrgLink(ctx context.Context, id int64, orgNumber, notes string) error {
	if id <= 0 {
		return apperr.Validation("user_org.invalid_id", "Link ID is required.", nil)
	}
	return s.repo.UpdateUserOrganization(ctx, id, strings.TrimSpace(orgNumber), "", strings.TrimSpace(notes))
}

// ApproveUserOrgLink approves a customer link by the vendor or admin.
func (s *Service) ApproveUserOrgLink(ctx context.Context, id int64) error {
	if id <= 0 {
		return apperr.Validation("user_org.invalid_id", "Link ID is required.", nil)
	}
	return s.repo.UpdateUserOrganization(ctx, id, "", UserOrgStatusApproved, "")
}

// RejectUserOrgLink rejects a customer link by the vendor or admin with optional notes.
func (s *Service) RejectUserOrgLink(ctx context.Context, id int64, notes string) error {
	if id <= 0 {
		return apperr.Validation("user_org.invalid_id", "Link ID is required.", nil)
	}
	return s.repo.UpdateUserOrganization(ctx, id, "", UserOrgStatusRejected, strings.TrimSpace(notes))
}

// DeleteUserOrgLink removes a link.
func (s *Service) DeleteUserOrgLink(ctx context.Context, id int64) error {
	if id <= 0 {
		return apperr.Validation("user_org.invalid_id", "Link ID is required.", nil)
	}
	return s.repo.DeleteUserOrganization(ctx, id)
}

// ListUserOrganizationsByUser returns all links for a customer user.
func (s *Service) ListUserOrganizationsByUser(ctx context.Context, userID int64) ([]*UserOrganization, error) {
	if userID <= 0 {
		return []*UserOrganization{}, nil
	}
	return s.repo.ListUserOrganizationsByUser(ctx, userID)
}

// ListUserOrganizationsByVendor returns all customer links for a vendor.
func (s *Service) ListUserOrganizationsByVendor(ctx context.Context, vendorOrgID int64, statusFilter string) ([]*UserOrganization, error) {
	if vendorOrgID <= 0 {
		return []*UserOrganization{}, nil
	}
	return s.repo.ListUserOrganizationsByVendor(ctx, vendorOrgID, statusFilter)
}

// ListUserOrganizationsByVendorWithTotal returns paginated customer links for a vendor with total count.
func (s *Service) ListUserOrganizationsByVendorWithTotal(ctx context.Context, vendorOrgID int64, statusFilter string, limit, offset int) ([]*UserOrganization, int, error) {
	if vendorOrgID <= 0 {
		return []*UserOrganization{}, 0, nil
	}
	return s.repo.ListUserOrganizationsByVendorWithTotal(ctx, vendorOrgID, statusFilter, limit, offset)
}

// ListAllUserOrganizations returns all links across the platform for admin.
func (s *Service) ListAllUserOrganizations(ctx context.Context, statusFilter string) ([]*UserOrganization, error) {
	return s.repo.ListAllUserOrganizations(ctx, statusFilter)
}

// ListAllUserOrganizationsWithTotal returns paginated links across the platform for admin with total count.
func (s *Service) ListAllUserOrganizationsWithTotal(ctx context.Context, statusFilter string, limit, offset int) ([]*UserOrganization, int, error) {
	return s.repo.ListAllUserOrganizationsWithTotal(ctx, statusFilter, limit, offset)
}
