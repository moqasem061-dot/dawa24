package http_test

import (
	"context"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

type apiHappyProbe struct{}

func (apiHappyProbe) Variant(context.Context, int64) (commerce.VariantAvailability, error) {
	return commerce.VariantAvailability{}, nil
}
func (apiHappyProbe) Vendor(context.Context, int64) (commerce.VendorAvailability, error) {
	return commerce.VendorAvailability{}, nil
}
func (apiHappyProbe) CustomerBranch(context.Context, int64) (commerce.BranchAvailability, error) {
	city := int64(1)
	return commerce.BranchAvailability{ID: 3, OrganizationID: 1, CityID: &city, InstitutionalWorks: []string{"retail"}}, nil
}
func (apiHappyProbe) VendorCovers(context.Context, int64, int64, float64, float64, time.Weekday, *int64) (bool, error) {
	return true, nil
}
func (apiHappyProbe) VendorInstitutionalConnection(context.Context, int64, int64, int64) (bool, error) {
	return true, nil
}
func (apiHappyProbe) VariantsByIDs(context.Context, []int64) (map[int64]commerce.VariantAvailability, error) {
	return map[int64]commerce.VariantAvailability{1: {ID: 1, OrganizationID: 2, Active: true, StockQty: 10, Price: money.MustParse("50.00"), EffectivePrice: money.MustParse("50.00")}}, nil
}
func (apiHappyProbe) VendorsByIDs(context.Context, []int64) (map[int64]commerce.VendorAvailability, error) {
	return map[int64]commerce.VendorAvailability{2: {ID: 2, IsVendor: true, Approved: true}}, nil
}
func (apiHappyProbe) VendorInstitutionalConnections(context.Context, int64, []commerce.AvailabilityLine) (map[int64]bool, error) {
	return map[int64]bool{1: true}, nil
}

type pendingHappyRepo struct{ happyRepo }

func (pendingHappyRepo) GetOrderByID(_ context.Context, id int64) (*commerce.Order, error) {
	return &commerce.Order{ID: id, CustomerID: 1, Status: commerce.StatusPending, PaymentMethod: "cod"}, nil
}
