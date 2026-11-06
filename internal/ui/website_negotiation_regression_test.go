package ui

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

type negotiationOrderRepo struct {
	commerce.Repository
	order *commerce.Order
}

func (r *negotiationOrderRepo) CreateOrder(_ context.Context, order *commerce.Order, _ []*commerce.OrderShipment, _ []*commerce.OrderLine) error {
	order.ID = 123
	r.order = order
	return nil
}

type negotiationProbe struct{}

func (negotiationProbe) Variant(context.Context, int64) (commerce.VariantAvailability, error) {
	return commerce.VariantAvailability{}, nil
}
func (negotiationProbe) Vendor(context.Context, int64) (commerce.VendorAvailability, error) {
	return commerce.VendorAvailability{}, nil
}
func (negotiationProbe) CustomerBranch(context.Context, int64) (commerce.BranchAvailability, error) {
	city := int64(1)
	return commerce.BranchAvailability{ID: 3, OrganizationID: 99, CityID: &city, InstitutionalWorks: []string{"retail"}}, nil
}
func (negotiationProbe) VendorCovers(context.Context, int64, int64, float64, float64, time.Weekday, *int64) (bool, error) {
	return true, nil
}
func (negotiationProbe) VendorInstitutionalConnection(context.Context, int64, int64, int64) (bool, error) {
	return true, nil
}
func (negotiationProbe) VariantsByIDs(context.Context, []int64) (map[int64]commerce.VariantAvailability, error) {
	return map[int64]commerce.VariantAvailability{10: {ID: 10, OrganizationID: 7, Active: true, StockQty: 10, IsNegotiable: true, Price: money.MustParse("100.00"), EffectivePrice: money.MustParse("100.00")}}, nil
}
func (negotiationProbe) VendorsByIDs(context.Context, []int64) (map[int64]commerce.VendorAvailability, error) {
	return map[int64]commerce.VendorAvailability{7: {ID: 7, IsVendor: true, Approved: true}}, nil
}
func (negotiationProbe) VendorInstitutionalConnections(context.Context, int64, []commerce.AvailabilityLine) (map[int64]bool, error) {
	return map[int64]bool{10: true}, nil
}

func TestWebsiteNegotiationStillCreatesProposedOrder(t *testing.T) {
	repo := &negotiationOrderRepo{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := commerce.NewService(repo, logger)
	svc.SetAvailabilityProbe(negotiationProbe{})
	handler := &UIHandler{commSvc: svc, log: logger}
	form := url.Values{"variant_id": {"10"}, "vendor_org_id": {"7"}, "branch_id": {"3"}, "qty": {"2"}, "proposed_price": {"60.00"}}
	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/customer/orders/negotiate", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request = request.WithContext(authctx.WithActor(request.Context(), authctx.Actor{UserID: 1, OrganizationID: 99, OrgType: "pharmacy"}))
	response := httptest.NewRecorder()
	handler.CustomerNegotiateOrderSubmit(response, request)
	if response.Code != http.StatusSeeOther || repo.order == nil || !repo.order.IsNegotiation || repo.order.TotalAmount != money.MustParse("120.00") {
		t.Fatalf("status=%d location=%q order=%+v", response.Code, response.Header().Get("Location"), repo.order)
	}
}
