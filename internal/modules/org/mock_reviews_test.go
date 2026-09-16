package org

import "context"

func (m *mockOrgRepo) ListReviewsForVendor(_ context.Context, vendorOrgID int64, _, _ int) ([]*Review, error) {
	return m.reviews[vendorOrgID], nil
}

func (m *mockOrgRepo) GetReviewByOrderAndVendor(_ context.Context, _, vendorOrgID int64) (*Review, error) {
	revs := m.reviews[vendorOrgID]
	if len(revs) > 0 {
		return revs[0], nil
	}
	return nil, nil
}

func (m *mockOrgRepo) ListReviewsForOrder(_ context.Context, _ int64) ([]*Review, error) {
	return nil, nil
}

func (m *mockOrgRepo) HasDeliveredOrderFromVendor(_ context.Context, _, _ int64) (bool, error) {
	return true, nil
}

func (m *mockOrgRepo) ListAdminReviewsWithTotal(_ context.Context, _ AdminReviewFilter) ([]*Review, int, error) {
	return nil, 0, nil
}

func (m *mockOrgRepo) GetAdminReviewStats(_ context.Context) (*AdminReviewStats, error) {
	return &AdminReviewStats{}, nil
}

func (m *mockOrgRepo) UpdateReviewStatus(_ context.Context, _ int64, _ bool) error {
	return nil
}

func (m *mockOrgRepo) SoftDeleteReview(_ context.Context, _ int64) error {
	return nil
}
