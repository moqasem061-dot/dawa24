package org

import "context"

// ListAdminReviewsWithTotal retrieves reviews matching filters with total count for administrators.
func (s *Service) ListAdminReviewsWithTotal(ctx context.Context, filter AdminReviewFilter) ([]*Review, int, error) {
	return s.repo.ListAdminReviewsWithTotal(ctx, filter)
}

// GetAdminReviewStats computes platform-wide review KPIs.
func (s *Service) GetAdminReviewStats(ctx context.Context) (*AdminReviewStats, error) {
	return s.repo.GetAdminReviewStats(ctx)
}

// UpdateReviewStatus updates the approval status of a review.
func (s *Service) UpdateReviewStatus(ctx context.Context, reviewID int64, isApproved bool) error {
	return s.repo.UpdateReviewStatus(ctx, reviewID, isApproved)
}

// SoftDeleteReview marks a review as deleted.
func (s *Service) SoftDeleteReview(ctx context.Context, reviewID int64) error {
	return s.repo.SoftDeleteReview(ctx, reviewID)
}
