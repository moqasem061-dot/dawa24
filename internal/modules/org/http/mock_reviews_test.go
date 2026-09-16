package http_test

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/modules/org"
)

func (r stubRepo) ListReviewsForVendor(ctx context.Context, vendorOrgID int64, limit, offset int) ([]*org.Review, error) {
	r.fail("ListReviewsForVendor")
	return nil, nil
}

func (r stubRepo) GetReviewByOrderAndVendor(ctx context.Context, orderID, vendorOrgID int64) (*org.Review, error) {
	r.fail("GetReviewByOrderAndVendor")
	return nil, nil
}

func (r stubRepo) ListReviewsForOrder(ctx context.Context, orderID int64) ([]*org.Review, error) {
	r.fail("ListReviewsForOrder")
	return nil, nil
}

func (r stubRepo) HasDeliveredOrderFromVendor(ctx context.Context, customerOrgID, vendorOrgID int64) (bool, error) {
	r.fail("HasDeliveredOrderFromVendor")
	return false, nil
}

func (r stubRepo) ListAdminReviewsWithTotal(_ context.Context, _ org.AdminReviewFilter) ([]*org.Review, int, error) {
	r.fail("ListAdminReviewsWithTotal")
	return nil, 0, nil
}

func (r stubRepo) GetAdminReviewStats(_ context.Context) (*org.AdminReviewStats, error) {
	r.fail("GetAdminReviewStats")
	return nil, nil
}

func (r stubRepo) UpdateReviewStatus(_ context.Context, _ int64, _ bool) error {
	r.fail("UpdateReviewStatus")
	return nil
}

func (r stubRepo) SoftDeleteReview(_ context.Context, _ int64) error {
	r.fail("SoftDeleteReview")
	return nil
}

func (happyRepo) ListReviewsForVendor(ctx context.Context, vendorOrgID int64, limit, offset int) ([]*org.Review, error) {
	return nil, nil
}

func (happyRepo) GetReviewByOrderAndVendor(ctx context.Context, orderID, vendorOrgID int64) (*org.Review, error) {
	return nil, nil
}

func (happyRepo) ListReviewsForOrder(ctx context.Context, orderID int64) ([]*org.Review, error) {
	return nil, nil
}

func (happyRepo) HasDeliveredOrderFromVendor(ctx context.Context, customerOrgID, vendorOrgID int64) (bool, error) {
	return true, nil
}

func (happyRepo) ListAdminReviewsWithTotal(_ context.Context, _ org.AdminReviewFilter) ([]*org.Review, int, error) {
	return nil, 0, nil
}

func (happyRepo) GetAdminReviewStats(_ context.Context) (*org.AdminReviewStats, error) {
	return &org.AdminReviewStats{}, nil
}

func (happyRepo) UpdateReviewStatus(_ context.Context, _ int64, _ bool) error {
	return nil
}

func (happyRepo) SoftDeleteReview(_ context.Context, _ int64) error {
	return nil
}
