package ui

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	"github.com/moqasem061-dot/dawa24/internal/modules/org"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
)

type assistantPriceCatalog struct {
	catalog.Repository
	calls int
	err   error
}

func (r *assistantPriceCatalog) GetVariantByID(context.Context, int64) (*catalog.ProductVariant, error) {
	r.calls++
	if r.calls > 1 {
		return nil, r.err
	}
	return &catalog.ProductVariant{ID: 10, ProductID: 20, OrganizationID: 30, Status: catalog.StatusActive}, nil
}

type assistantPriceBranches struct{ org.Repository }

func (r assistantPriceBranches) ListBranchesByOrg(context.Context, int64) ([]*org.Branch, error) {
	return []*org.Branch{{ID: 40, OrganizationID: 50, Status: "active"}}, nil
}
func (r assistantPriceBranches) GetBranchByID(context.Context, int64) (*org.Branch, error) {
	return &org.Branch{ID: 40, OrganizationID: 50, Status: "active"}, nil
}

type assistantPriceAvailability struct{ commerce.AvailabilityProbe }

func (p assistantPriceAvailability) VariantsByIDs(context.Context, []int64) (map[int64]commerce.VariantAvailability, error) {
	return map[int64]commerce.VariantAvailability{10: {ID: 10, OrganizationID: 30, StockQty: 10, Active: true}}, nil
}
func (p assistantPriceAvailability) VendorsByIDs(context.Context, []int64) (map[int64]commerce.VendorAvailability, error) {
	return map[int64]commerce.VendorAvailability{30: {ID: 30, IsVendor: true, Approved: true}}, nil
}
func (p assistantPriceAvailability) CustomerBranch(context.Context, int64) (commerce.BranchAvailability, error) {
	cityID := int64(1)
	return commerce.BranchAvailability{ID: 40, OrganizationID: 50, CityID: &cityID, InstitutionalWorks: []string{"1"}}, nil
}
func (p assistantPriceAvailability) VendorInstitutionalConnections(context.Context, int64, []commerce.AvailabilityLine) (map[int64]bool, error) {
	return map[int64]bool{10: true}, nil
}
func (p assistantPriceAvailability) VendorCovers(context.Context, int64, int64, float64, float64, time.Weekday, *int64) (bool, error) {
	return true, nil
}

func TestAssistantCartAddRefusesFailedPriceLookup(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	priceErr := errors.New("price database unavailable")
	catRepo := &assistantPriceCatalog{err: priceErr}
	comm := commerce.NewService(nil, logger)
	comm.SetAvailabilityProbe(assistantPriceAvailability{})
	h := &UIHandler{log: logger, catSvc: catalog.NewService(catRepo, logger), commSvc: comm, orgSvc: org.NewService(assistantPriceBranches{}, logger)}
	plan, err := h.planCartAdd(context.Background(), authctx.Actor{UserID: 1, OrganizationID: 50, IsOwner: true}, 10, 1)
	if plan != nil || !errors.Is(err, priceErr) {
		t.Fatalf("price failure produced a cart plan: plan=%v error=%v", plan, err)
	}
}
