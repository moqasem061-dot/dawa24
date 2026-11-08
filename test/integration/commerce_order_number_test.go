package integration_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	commercepg "github.com/moqasem061-dot/dawa24/internal/modules/commerce/postgres"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// openProbe answers every availability question as yes for the one buyer
// branch and the one supplier of the fixture. The order number test is about
// numbering, not stock, so the probe only has to let the checkout through.
type openProbe struct{ buyer, supplier, branch, variant int64 }

func (p openProbe) Variant(_ context.Context, id int64) (commerce.VariantAvailability, error) {
	return commerce.VariantAvailability{ID: id, OrganizationID: p.supplier, VendorBranchID: p.branch, StockQty: 100000, MinOrderQty: 1, Active: true}, nil
}
func (p openProbe) Vendor(_ context.Context, id int64) (commerce.VendorAvailability, error) {
	return commerce.VendorAvailability{ID: id, IsVendor: true, Approved: true}, nil
}
func (p openProbe) CustomerBranch(_ context.Context, id int64) (commerce.BranchAvailability, error) {
	lat, lon := 30.04, 31.23
	return commerce.BranchAvailability{ID: id, OrganizationID: p.buyer, InstitutionalWorks: []string{"pharmacy"}, Latitude: &lat, Longitude: &lon}, nil
}
func (p openProbe) VendorCovers(context.Context, int64, int64, float64, float64, time.Weekday, *int64) (bool, error) {
	return true, nil
}
func (p openProbe) VendorInstitutionalConnection(context.Context, int64, int64, int64) (bool, error) {
	return true, nil
}
func (p openProbe) VariantsByIDs(ctx context.Context, ids []int64) (map[int64]commerce.VariantAvailability, error) {
	out := map[int64]commerce.VariantAvailability{}
	for _, id := range ids {
		v, _ := p.Variant(ctx, id)
		out[id] = v
	}
	return out, nil
}
func (p openProbe) VendorsByIDs(ctx context.Context, ids []int64) (map[int64]commerce.VendorAvailability, error) {
	out := map[int64]commerce.VendorAvailability{}
	for _, id := range ids {
		v, _ := p.Vendor(ctx, id)
		out[id] = v
	}
	return out, nil
}
func (p openProbe) VendorInstitutionalConnections(_ context.Context, _ int64, lines []commerce.AvailabilityLine) (map[int64]bool, error) {
	out := map[int64]bool{}
	for _, l := range lines {
		out[l.VariantID] = true
	}
	return out, nil
}

// TestOrderNumberSequenceConcurrent runs 200 checkouts through two commerce
// services at once. Every order number must be distinct, and every number must
// come from commerce.order_number_seq, so a second server process draws from the
// same sequence and cannot print a duplicate.
func TestOrderNumberSequenceConcurrent(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		return
	}
	ctx := context.Background()
	base := time.Now().UnixNano() / 1000 % 1000000000 * 10
	buyer, buyerUser, supplier := base, base+1, base+2
	var branch, product, variant int64
	err := db.InTx(database.AsSystem(ctx), func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO org.organizations (id, name, type, status) VALUES ($1, '{"en":"Seq buyer"}', 'pharmacy', 'approved'), ($2, '{"en":"Seq supplier"}', 'vendor', 'approved')`, buyer, supplier); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO identity.users (id, email, password_hash) VALUES ($1, $2, 'x')`, buyerUser, fmt.Sprintf("seq-%d@example.test", base)); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO org.branches (organization_id, name) VALUES ($1, '{"en":"Seq branch"}') RETURNING id`, buyer).Scan(&branch); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO catalog.products (organization_id, name) VALUES ($1, '{"en":"Seq product"}') RETURNING id`, supplier).Scan(&product); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `INSERT INTO catalog.product_variants (organization_id, product_id, name, sku, price, status) VALUES ($1, $2, '{"en":"Seq variant"}', $3, 10.00, 'active') RETURNING id`, supplier, product, fmt.Sprintf("SEQ-%d", base)).Scan(&variant)
	})
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	cleanupERPOrgs(t, db, ctx, buyer, supplier)

	before := sequenceValue(ctx, t, db)
	probe := openProbe{buyer: buyer, supplier: supplier, branch: branch, variant: variant}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	services := make([]*commerce.Service, 2)
	for i := range services {
		svc := commerce.NewService(commercepg.NewRepository(db), log)
		svc.SetAvailabilityProbe(probe)
		svc.SetAPIQuoteResolver(func(_ context.Context, in commerce.CheckoutInput) (commerce.CheckoutInput, error) { return in, nil })
		services[i] = svc
	}

	const orders = 200
	numbers := make(chan string, orders)
	errs := make(chan error, orders)
	var wg sync.WaitGroup
	for i := 0; i < orders; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			order, err := services[i%2].CheckoutAPI(ctx, commerce.CheckoutInput{
				CustomerID: buyerUser, CustomerOrgID: buyer, CustomerOrgType: "pharmacy", BranchID: &branch,
				PaymentMethod: commerce.DefaultPaymentMethod,
				Items: []commerce.CheckoutLineItem{{
					VendorOrgID: supplier, ProductID: &product, ProductVariantID: &variant,
					ProductName: i18n.Text{"en": "Seq product"}, Quantity: 1, UnitPrice: money.MustParse("10.00"),
				}},
			})
			if err != nil {
				errs <- err
				return
			}
			numbers <- order.OrderNumber
		}(i)
	}
	wg.Wait()
	close(numbers)
	close(errs)
	for err := range errs {
		t.Fatalf("checkout: %v", err)
	}
	seen := map[string]bool{}
	var list []string
	for n := range numbers {
		if seen[n] {
			t.Fatalf("duplicate order number %s", n)
		}
		seen[n] = true
		list = append(list, n)
	}
	if len(list) != orders {
		t.Fatalf("got %d orders, want %d", len(list), orders)
	}
	sort.Strings(list)

	// The numbers must come from the database sequence, not a process counter:
	// the sequence advances by one for every order (the first draw from a fresh
	// sequence leaves last_value at its start, so the delta is orders-1 there).
	after := sequenceValue(ctx, t, db)
	if after-before < orders-1 {
		t.Fatalf("sequence advanced by %d, want at least %d: numbers did not come from commerce.order_number_seq", after-before, orders-1)
	}
}

// sequenceValue reads commerce.order_number_seq.last_value.
func sequenceValue(ctx context.Context, t *testing.T, db *database.DB) int64 {
	t.Helper()
	var last int64
	if err := db.InReadTx(database.AsSystem(ctx), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT last_value FROM commerce.order_number_seq`).Scan(&last)
	}); err != nil {
		t.Fatalf("order_number_seq: %v", err)
	}
	return last
}
