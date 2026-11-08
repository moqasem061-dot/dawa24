package http

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	"github.com/moqasem061-dot/dawa24/internal/modules/erp/provider"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// purchaseHandlerRepo adds the S7 purchase reads and the idempotency record to
// the order fake. Offers and variants are seeded by the tests.
type purchaseHandlerRepo struct {
	*orderHandlerRepo
	offers   []erp.Offer
	variants map[string]erp.VariantOfferRow
	idem     map[string]erp.IdemState
	stored   map[string][]byte
}

func (r *purchaseHandlerRepo) ListOffersAsSystem(context.Context, int64, string, int) ([]erp.Offer, error) {
	return r.offers, nil
}

func (r *purchaseHandlerRepo) VariantOfferAsSystem(_ context.Context, publicID string) (erp.VariantOfferRow, error) {
	row, ok := r.variants[publicID]
	if !ok {
		return erp.VariantOfferRow{}, erp.ErrNotFound
	}
	return row, nil
}

func (r *purchaseHandlerRepo) BeginIdempotency(_ context.Context, _, _ int64, ref, sha string) (erp.IdemState, error) {
	if state, ok := r.idem[ref]; ok {
		return state, nil
	}
	r.idem[ref] = erp.IdemState{RequestSHA: sha, InProgress: true}
	return erp.IdemState{Created: true, RequestSHA: sha}, nil
}

func (r *purchaseHandlerRepo) FinishIdempotency(_ context.Context, _, _ int64, ref string, _ int64, _ string, response []byte, _ erp.AuditRecord) error {
	state := r.idem[ref]
	state.InProgress, state.Replay, state.Response = false, true, response
	r.idem[ref] = state
	return nil
}

func (r *purchaseHandlerRepo) DeleteIdempotency(_ context.Context, _, _ int64, ref string) error {
	delete(r.idem, ref)
	return nil
}

func (r *purchaseHandlerRepo) FindOrderByNoteMarker(context.Context, int64, string) (int64, string, bool, error) {
	return 0, "", false, nil
}

func (r *purchaseHandlerRepo) SupplierLinked(context.Context, int64, int64) (bool, error) {
	return true, nil
}

func (r *purchaseHandlerRepo) ActiveMember(context.Context, int64, int64) (bool, error) {
	return true, nil
}

func (r *purchaseHandlerRepo) GetPurchaseOrder(_ context.Context, _ int64, orderNumber string) (erp.PurchaseRow, error) {
	created := time.Date(2026, 11, 7, 9, 40, 5, 0, time.UTC)
	total := money.MustParse("626.00")
	line := erp.OrderLine{LineID: 90412, VariantPublicID: "3b9c1a4e-6f02-4d58-8c7e-12a4d9e05b33", Name: map[string]string{"en": "Panadol Extra 24 tablets"},
		Barcode: "6221024000123", Quantity: 12, UnitPrice: money.MustParse("50.40"), DiscountAmount: money.MustParse("3.80"), TotalPrice: money.MustParse("601.00")}
	return erp.PurchaseRow{OrderID: 77, OrderNumber: orderNumber, Status: "pending", ImportState: "new", CreatedAt: created, UpdatedAt: created, Total: total,
		Shipments: []erp.PurchaseShipment{{OrderID: 77, ShipmentNumber: orderNumber + "-1", Status: "pending", SupplierName: map[string]string{"en": "Al Nour Pharma"},
			Subtotal: money.MustParse("601.00"), ShippingFee: money.MustParse("25.00"), Lines: []erp.OrderLine{line}}}}, nil
}

func (r *purchaseHandlerRepo) ListPurchaseShipmentsAsSystem(_ context.Context, _ int64, orderIDs []int64) ([]erp.PurchaseShipment, error) {
	row, _ := r.GetPurchaseOrder(context.Background(), 0, "90077")
	return row.Shipments, nil
}

// stubPlacer answers every order with order 77 and counts the calls.
type stubPlacer struct{ calls int }

func (p *stubPlacer) Place(context.Context, erp.PurchaseRequest) (erp.PlacedOrder, error) {
	p.calls++
	return erp.PlacedOrder{OrderID: 77, OrderNumber: "90077", Total: money.MustParse("626.00")}, nil
}

// purchaseFixture is a buyer key with erp.po.create and a buying user.
type purchaseFixture struct {
	t      *testing.T
	repo   *purchaseHandlerRepo
	placer *stubPlacer
	routes http.Handler
	key    string
}

func newPurchaseFixture(t *testing.T) *purchaseFixture {
	t.Helper()
	pepper := []byte("purchase-handler-pepper-32-bytes")
	full, prefix, hash, err := erp.GenerateKey("test", pepper)
	if err != nil {
		t.Fatal(err)
	}
	user := int64(5)
	base := &orderHandlerRepo{keys: map[string]erp.KeyAuthRow{prefix: {
		APIKey: erp.APIKey{ID: 1, OrganizationID: 7, IntegrationID: 3, KeyPrefix: prefix, Status: "active",
			Scopes: []string{erp.ScopePurchaseCreate}, WarehouseID: 40, BranchID: 41, BuyingUserID: &user, RateLimitPerMinute: 600, MaxBatchItems: 50000},
		KeyHash: hash, IntegrationStatus: "active", Provider: provider.DefaultProvider,
	}}}
	repo := &purchaseHandlerRepo{orderHandlerRepo: base, idem: map[string]erp.IdemState{}, variants: map[string]erp.VariantOfferRow{
		"3b9c1a4e-6f02-4d58-8c7e-12a4d9e05b33": {ID: 11, OrganizationID: 20, PublicID: "3b9c1a4e-6f02-4d58-8c7e-12a4d9e05b33", PublicPrice: money.MustParse("62.50"), DiscountPercent: money.MustParse("12.50"), Active: true, InStock: true, SupplierName: map[string]string{"en": "Al Nour Pharma"}},
	}}
	repo.offers = []erp.Offer{{VariantID: "3b9c1a4e-6f02-4d58-8c7e-12a4d9e05b33", SupplierName: map[string]string{"en": "Al Nour Pharma"}, SupplierLinked: true,
		Name: map[string]string{"en": "Panadol Extra 24 tablets"}, Barcode: "6221024000123", PublicPrice: money.MustParse("62.50"), DiscountPercent: money.MustParse("12.50"),
		UnitPrice: money.MustParse("54.69"), InStock: true}}
	placer := &stubPlacer{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := erp.NewService(repo, erp.Ports{Placer: placer}, erp.Config{Pepper: pepper, Tag: "test", DefaultRate: 600, MaxBatchItems: 50000, Provider: provider.DefaultProvider}, logger)
	return &purchaseFixture{t: t, repo: repo, placer: placer, routes: NewHandler(svc, erp.Config{}, nil, logger).Routes(), key: full}
}

func (f *purchaseFixture) do(method, path, body string) *httptest.ResponseRecorder {
	f.t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+f.key)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	f.routes.ServeHTTP(w, req)
	return w
}

// errorCode returns the stable code of an error envelope, or "" for success.
func errorCode(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	e, _ := decodeEnvelope(t, w)["error"].(map[string]any)
	code, _ := e["code"].(string)
	return code
}

const purchaseBodyOK = `{"externalReference":"ORGA-PO-2026-000451","notes":"weekly","lines":[{"variantId":"3b9c1a4e-6f02-4d58-8c7e-12a4d9e05b33","quantity":12,"unitPrice":"54.69"}]}`

func TestPurchaseOffersHandler(t *testing.T) {
	f := newPurchaseFixture(t)
	w := f.do("GET", "/offers?barcode=6221024000123", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	data := decodeEnvelope(t, w)["data"].([]any)
	if len(data) != 1 {
		t.Fatalf("offers = %d, want 1", len(data))
	}
}

func TestPurchaseOrderHandlerCreatesThenReplays(t *testing.T) {
	f := newPurchaseFixture(t)
	first := f.do("POST", "/orders/purchase", purchaseBodyOK)
	if first.Code != http.StatusCreated {
		t.Fatalf("first status = %d, body %s", first.Code, first.Body.String())
	}
	second := f.do("POST", "/orders/purchase", purchaseBodyOK)
	if second.Code != http.StatusOK {
		t.Fatalf("replay status = %d, body %s", second.Code, second.Body.String())
	}
	if f.placer.calls != 1 {
		t.Fatalf("placer calls = %d, want 1", f.placer.calls)
	}
}

func TestPurchaseOrderHandlerRejectsBadLines(t *testing.T) {
	f := newPurchaseFixture(t)
	cases := map[string]string{
		"fractional quantity": `{"externalReference":"BAD-1","lines":[{"variantId":"3b9c1a4e-6f02-4d58-8c7e-12a4d9e05b33","quantity":1.5}]}`,
		"string quantity":     `{"externalReference":"BAD-2","lines":[{"variantId":"3b9c1a4e-6f02-4d58-8c7e-12a4d9e05b33","quantity":"12"}]}`,
		"bad json":            `{"externalReference":`,
	}
	want := map[string]string{"fractional quantity": "ERP_PO_QUANTITY_INVALID", "string quantity": "ERP_PO_QUANTITY_INVALID", "bad json": "ERP_BAD_JSON"}
	for name, body := range cases {
		w := f.do("POST", "/orders/purchase", body)
		if got := errorCode(t, w); got != want[name] {
			t.Fatalf("%s: code = %q, want %q (status %d)", name, got, want[name], w.Code)
		}
	}
	if f.placer.calls != 0 {
		t.Fatalf("placer called %d times for bad input", f.placer.calls)
	}
}

func TestPurchaseOrderHandlerNumericUnitPrice(t *testing.T) {
	f := newPurchaseFixture(t)
	body := `{"externalReference":"NUM-1","lines":[{"variantId":"3b9c1a4e-6f02-4d58-8c7e-12a4d9e05b33","quantity":12,"unitPrice":54.69}]}`
	if w := f.do("POST", "/orders/purchase", body); w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
}
