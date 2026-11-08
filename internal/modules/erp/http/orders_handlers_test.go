package http

import (
	"context"
	"encoding/json"
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

// orderHandlerRepo is an in-memory slice of the order repository. Only the
// methods the authentication path and the order handlers call are implemented;
// the embedded nil interface panics for anything else, which would be a bug.
type orderHandlerRepo struct {
	erp.Repository
	keys       map[string]erp.KeyAuthRow
	sales      []erp.SalesRow
	purchase   []erp.PurchaseRow
	lastFilter erp.OrderFilter
	imports    []erp.OrderImport
}

func (r *orderHandlerRepo) FindKeyByPrefixAsSystem(_ context.Context, prefix string) (erp.KeyAuthRow, bool, error) {
	row, ok := r.keys[prefix]
	return row, ok, nil
}
func (r *orderHandlerRepo) OrgStatusType(context.Context, int64) (string, string, error) {
	return "approved", "vendor", nil
}
func (r *orderHandlerRepo) TouchLastUsed(context.Context, int64, int64, string) error { return nil }

func (r *orderHandlerRepo) ListSalesShipmentsAsSystem(_ context.Context, _, _ int64, f erp.OrderFilter) ([]erp.SalesRow, error) {
	r.lastFilter = f
	var out []erp.SalesRow
	for _, row := range r.sales {
		if row.ShipmentID > f.After && len(out) < f.Limit {
			out = append(out, row)
		}
	}
	return out, nil
}

func (r *orderHandlerRepo) GetSalesShipmentAsSystem(_ context.Context, _ int64, orderNumber string) (erp.SalesRow, error) {
	for _, row := range r.sales {
		if row.OrderNumber == orderNumber {
			return row, nil
		}
	}
	return erp.SalesRow{}, erp.ErrNotFound
}

func (r *orderHandlerRepo) ListPurchaseOrders(_ context.Context, _, _ int64, f erp.OrderFilter) ([]erp.PurchaseRow, error) {
	r.lastFilter = f
	return r.purchase, nil
}

func (r *orderHandlerRepo) GetPurchaseOrder(_ context.Context, _ int64, orderNumber string) (erp.PurchaseRow, error) {
	for _, row := range r.purchase {
		if row.OrderNumber == orderNumber {
			return row, nil
		}
	}
	return erp.PurchaseRow{}, erp.ErrNotFound
}

func (r *orderHandlerRepo) ListPurchaseShipmentsAsSystem(context.Context, int64, []int64) ([]erp.PurchaseShipment, error) {
	return nil, nil
}

func (r *orderHandlerRepo) UpsertOrderImport(_ context.Context, imp erp.OrderImport, _ erp.AuditRecord) (erp.AckOutcome, error) {
	r.imports = append(r.imports, imp)
	return erp.AckOutcome{Result: "acknowledged"}, nil
}

type orderFixture struct {
	t      *testing.T
	repo   *orderHandlerRepo
	routes http.Handler
	key    string
}

func newOrderFixture(t *testing.T) *orderFixture {
	t.Helper()
	pepper := []byte("order-handler-pepper-32-bytes-x")
	full, prefix, hash, err := erp.GenerateKey("test", pepper)
	if err != nil {
		t.Fatal(err)
	}
	repo := &orderHandlerRepo{keys: map[string]erp.KeyAuthRow{prefix: {
		APIKey: erp.APIKey{ID: 1, OrganizationID: 7, IntegrationID: 3, KeyPrefix: prefix, Status: "active",
			Scopes: []string{erp.ScopeOrdersRead, erp.ScopeOrdersAck}, WarehouseID: 40, BranchID: 41, RateLimitPerMinute: 600, MaxBatchItems: 50000},
		KeyHash: hash, IntegrationStatus: "active", Provider: provider.DefaultProvider,
	}}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := erp.NewService(repo, erp.Ports{}, erp.Config{Pepper: pepper, Tag: "test", DefaultRate: 600, MaxBatchItems: 50000, Provider: provider.DefaultProvider}, logger)
	return &orderFixture{t: t, repo: repo, routes: NewHandler(svc, erp.Config{}, nil, logger).Routes(), key: full}
}

func (f *orderFixture) do(method, path, body string) *httptest.ResponseRecorder {
	f.t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+f.key)
	w := httptest.NewRecorder()
	f.routes.ServeHTTP(w, req)
	return w
}

func decodeEnvelope(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var env map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("response is not JSON: %v (%s)", err, w.Body.String())
	}
	return env
}

func testSale(id int64, number, erpCode string) erp.SalesRow {
	created := time.Date(2026, 11, 7, 8, 2, 11, 0, time.UTC)
	total, _ := money.Parse("1118.80")
	line, _ := money.Parse("1093.80")
	unit, _ := money.Parse("54.69")
	zero, _ := money.Parse("0.00")
	return erp.SalesRow{
		OrderID: id + 500, ShipmentID: id, OrderNumber: number, ShipmentNumber: number + "-1", Status: "pending",
		ImportState: "new", PaymentMethod: "cash_on_delivery", PaymentStatus: "unpaid", CreatedAt: created, UpdatedAt: created,
		Total: total, Subtotal: line, ShippingFee: zero,
		Customer: erp.CustomerInfo{ErpCode: erpCode, Mapped: erpCode != "", Name: map[string]string{"ar": "صيدلية الشفاء", "en": "Al Shifa Pharmacy"}, BranchName: map[string]string{"en": "Main branch"}},
		Lines: []erp.OrderLine{{LineID: 88231, VariantPublicID: "3b9c1a4e-6f02-4d58-8c7e-12a4d9e05b33", ExternalProductCode: "100245",
			Name: map[string]string{"en": "Panadol Extra"}, Barcode: "6221024000123", Quantity: 20, UnitPrice: unit, DiscountAmount: zero, TotalPrice: line}},
	}
}

func TestOrdersSalesListShape(t *testing.T) {
	f := newOrderFixture(t)
	f.repo.sales = []erp.SalesRow{testSale(10, "10542", "C-00871")}
	w := f.do("GET", "/orders/sales?state=all", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	env := decodeEnvelope(t, w)
	data := env["data"].([]any)
	if len(data) != 1 {
		t.Fatalf("data = %d rows, want 1", len(data))
	}
	order := data[0].(map[string]any)
	customer := order["customer"].(map[string]any)
	if order["orderNumber"] != "10542" || customer["erpCode"] != "C-00871" || customer["mapped"] != true {
		t.Fatalf("order = %+v, want order 10542 with erp code C-00871", order)
	}
	line := order["lines"].([]any)[0].(map[string]any)
	if line["totalPrice"] != "1093.80" || line["unitPrice"] != "54.69" || line["externalProductCode"] != "100245" {
		t.Fatalf("line = %+v, want exact money and the external code", line)
	}
	page := env["page"].(map[string]any)
	if page["nextCursor"] != nil || page["limit"] != float64(100) {
		t.Fatalf("page = %+v, want null nextCursor and limit 100", page)
	}
}

func TestOrdersSalesUnmappedCustomerShape(t *testing.T) {
	f := newOrderFixture(t)
	row := testSale(11, "10543", "")
	row.Customer.UnmappedReason = "no_link"
	f.repo.sales = []erp.SalesRow{row}
	w := f.do("GET", "/orders/sales?state=all", "")
	customer := decodeEnvelope(t, w)["data"].([]any)[0].(map[string]any)["customer"].(map[string]any)
	if customer["erpCode"] != nil || customer["mapped"] != false || customer["unmappedReason"] != "no_link" {
		t.Fatalf("customer = %+v, want null erpCode, mapped false, no_link", customer)
	}
}

func TestOrdersSalesPagingCursorRoundTrip(t *testing.T) {
	f := newOrderFixture(t)
	f.repo.sales = []erp.SalesRow{testSale(10, "A-1", "C-1"), testSale(11, "A-2", "C-2"), testSale(12, "A-3", "C-3")}
	w := f.do("GET", "/orders/sales?state=all&limit=2", "")
	env := decodeEnvelope(t, w)
	next, _ := env["page"].(map[string]any)["nextCursor"].(string)
	if next == "" || len(env["data"].([]any)) != 2 {
		t.Fatalf("first page = %s, want two rows and a next cursor", w.Body.String())
	}
	want, err := DecodeCursor(next)
	if err != nil || want != 11 {
		t.Fatalf("next cursor decodes to %d, %v; want shipment 11", want, err)
	}
	w = f.do("GET", "/orders/sales?state=all&limit=2&cursor="+next, "")
	data := decodeEnvelope(t, w)["data"].([]any)
	if f.repo.lastFilter.After != 11 || len(data) != 1 {
		t.Fatalf("second page after %d with %d rows; want after 11 and one row", f.repo.lastFilter.After, len(data))
	}
}

func TestOrdersBadCursorIsRefusedBilingually(t *testing.T) {
	f := newOrderFixture(t)
	w := f.do("GET", "/orders/sales?cursor=not-a-cursor", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	msg := decodeEnvelope(t, w)["error"].(map[string]any)["message"].(map[string]any)
	if decodeEnvelope(t, w)["error"].(map[string]any)["code"] != "ERP_CURSOR_INVALID" || msg["ar"] == "" || msg["en"] == "" {
		t.Fatalf("error = %s, want ERP_CURSOR_INVALID with ar and en", w.Body.String())
	}
}

func TestOrdersPurchaseListAndGet(t *testing.T) {
	f := newOrderFixture(t)
	f.repo.purchase = []erp.PurchaseRow{{OrderID: 900, OrderNumber: "PO-1", Status: "pending", ImportState: "new", CreatedAt: time.Now(), UpdatedAt: time.Now()}}
	w := f.do("GET", "/orders/purchase?state=all", "")
	if w.Code != http.StatusOK || decodeEnvelope(t, w)["data"].([]any)[0].(map[string]any)["orderNumber"] != "PO-1" {
		t.Fatalf("purchase list = %d %s", w.Code, w.Body.String())
	}
	if w := f.do("GET", "/orders/purchase/PO-1", ""); w.Code != http.StatusOK {
		t.Fatalf("purchase get = %d", w.Code)
	}
	w = f.do("GET", "/orders/purchase/NOPE", "")
	if w.Code != http.StatusNotFound || decodeEnvelope(t, w)["error"].(map[string]any)["code"] != "ERP_ORDER_NOT_FOUND" {
		t.Fatalf("missing purchase = %d %s", w.Code, w.Body.String())
	}
}

func TestOrdersSalesAckResultsAndRefusal(t *testing.T) {
	f := newOrderFixture(t)
	f.repo.sales = []erp.SalesRow{testSale(10, "10542", "C-00871")}
	body := `{"orders":[{"orderNumber":"10542","erpReference":"ORGA-77123"}]}`
	w := f.do("POST", "/orders/sales/ack", body)
	if w.Code != http.StatusOK {
		t.Fatalf("ack status = %d, body %s", w.Code, w.Body.String())
	}
	result := decodeEnvelope(t, w)["data"].([]any)[0].(map[string]any)
	if result["orderNumber"] != "10542" || result["result"] != "acknowledged" {
		t.Fatalf("ack result = %+v, want acknowledged", result)
	}
	if len(f.repo.imports) != 1 || f.repo.imports[0].ErpReference != "ORGA-77123" || f.repo.imports[0].Direction != "sales" {
		t.Fatalf("stored imports = %+v", f.repo.imports)
	}

	w = f.do("POST", "/orders/sales/ack", "not json")
	env := decodeEnvelope(t, w)
	msg := env["error"].(map[string]any)["message"].(map[string]any)
	if w.Code != http.StatusUnprocessableEntity || env["error"].(map[string]any)["code"] != "ERP_VALIDATION" || msg["ar"] == "" || msg["en"] == "" {
		t.Fatalf("bad body = %d %s, want 422 ERP_VALIDATION with ar and en", w.Code, w.Body.String())
	}
}

// TestSalesReadMarksSensitiveRecord proves a sales read sets the flag the call
// recorder copies into erp.api_calls.sensitive_read, through the real
// middleware, and that a purchase read does not.
func TestOrdersSalesReadMarksSensitiveRecord(t *testing.T) {
	f := newOrderFixture(t)
	f.repo.sales = []erp.SalesRow{testSale(10, "10542", "C-00871")}
	f.repo.purchase = []erp.PurchaseRow{{OrderID: 900, OrderNumber: "PO-1", CreatedAt: time.Now(), UpdatedAt: time.Now()}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewHandler(erp.NewService(f.repo, erp.Ports{}, erp.Config{Pepper: []byte("order-handler-pepper-32-bytes-x"), Tag: "test", DefaultRate: 600, MaxBatchItems: 50000, Provider: provider.DefaultProvider}, logger), erp.Config{}, nil, logger)
	var records []erp.CallRecord
	layer := authLayer{authenticate: func(ctx context.Context, raw, _ string) (erp.Principal, error) {
		return erp.Principal{OrganizationID: 7, IntegrationID: 3, KeyID: 1, Provider: provider.DefaultProvider,
			Scopes: []string{erp.ScopeOrdersRead}, OrgType: "vendor", KeyPrefix: "testkey1"}, nil
	}, log: logger, record: func(rec erp.CallRecord) { records = append(records, rec) }}

	for _, tc := range []struct {
		path      string
		handler   http.HandlerFunc
		sensitive bool
	}{
		{"/orders/sales", h.listSales, true},
		{"/orders/purchase", h.listPurchase, false},
	} {
		req := httptest.NewRequest("GET", tc.path+"?state=all", nil)
		req.Header.Set("Authorization", "Bearer d24_test_abcdefgh_x")
		w := httptest.NewRecorder()
		layer.middleware(tc.handler).ServeHTTP(w, req)
		if len(records) == 0 {
			t.Fatalf("%s wrote no call record", tc.path)
		}
		if got := records[len(records)-1].SensitiveRead; got != tc.sensitive {
			t.Fatalf("%s SensitiveRead = %v, want %v", tc.path, got, tc.sensitive)
		}
	}
}
