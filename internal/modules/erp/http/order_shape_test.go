package http

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// orgasoftSampleSale is the sample order from Orgasoft: three lines, a discount
// percentage on each, and a total of 1264.25. Subtotal is the price before
// discount (1640.00), as the database stores it, so the test fails if the total
// is ever taken from it again instead of from the line totals.
func orgasoftSampleSale() erp.SalesRow {
	m := money.MustParse
	return erp.SalesRow{
		OrderID: 500, ShipmentID: 21, BuyerOrgID: 1, OrderNumber: "21845", Status: "pending",
		Subtotal: m("1640.00"), ShippingFee: m("0.00"), Total: m("1264.25"),
		Customer: erp.CustomerInfo{Mapped: true, ErpCode: "263", Name: map[string]string{"en": "Direct"},
			Ordering: erp.ContactInfo{Phone: "01008042755", City: "Fayoum", Address: "الفيوم"}},
		Lines: []erp.OrderLine{
			{LineID: 1, ExternalProductCode: "100001", Name: map[string]string{"ar": "سبيركودا 20 قرص"}, Quantity: 1,
				UnitPrice: m("209.35"), ListPrice: m("265.00"), TotalPrice: m("209.35")},
			{LineID: 2, ExternalProductCode: "100002", Name: map[string]string{"ar": "فيتامين د 1000 وحدة"}, Quantity: 3,
				UnitPrice: m("300.30"), ListPrice: m("385.00"), TotalPrice: m("900.90")},
			{LineID: 3, ExternalProductCode: "100003", Name: map[string]string{"ar": "شراب الكحة"}, Quantity: 1,
				UnitPrice: m("154.00"), ListPrice: m("220.00"), TotalPrice: m("154.00")},
		},
	}
}

const orgasoftSampleBody = `[{"orderId":21845,"status":"PENDING","total":1264.25,"pharmacyId":1,"pharmacyName":"Direct",` +
	`"pharmacyCode":263,"pharmacyNumber":"01008042755","pharmacyCity":"Fayoum","pharmacyAddress":"الفيوم",` +
	`"items":[{"code":100001,"name":"سبيركودا 20 قرص","qty":1,"price":265.00,"discount":21.00},` +
	`{"code":100002,"name":"فيتامين د 1000 وحدة","qty":3,"price":385.00,"discount":22.00},` +
	`{"code":100003,"name":"شراب الكحة","qty":1,"price":220.00,"discount":30.00}]}]`

func TestOrgasoftSalesFeedMatchesSampleExactly(t *testing.T) {
	f := newOrderFixture(t)
	f.repo.sales = []erp.SalesRow{orgasoftSampleSale()}
	w := f.do("GET", "/orders/sales?state=all", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, w.Body.Bytes()); err != nil {
		t.Fatal(err)
	}
	if compact.String() != orgasoftSampleBody {
		t.Fatalf("body mismatch:\n got %s\nwant %s", compact.String(), orgasoftSampleBody)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q", ct)
	}
}

func TestOrgasoftSalesNextCursorHeader(t *testing.T) {
	f := newOrderFixture(t)
	f.repo.sales = []erp.SalesRow{testSale(10, "A-1", "C-1"), testSale(11, "A-2", "C-2"), testSale(12, "A-3", "C-3")}

	w := f.do("GET", "/orders/sales?state=all&limit=2", "")
	next := w.Header().Get("X-Next-Cursor")
	if next == "" {
		t.Fatal("X-Next-Cursor absent on a full page")
	}
	if want, err := DecodeCursor(next); err != nil || want != 11 {
		t.Fatalf("X-Next-Cursor %q decodes to %d, %v; want shipment 11", next, want, err)
	}
	var rows []any
	if err := json.Unmarshal(w.Body.Bytes(), &rows); err != nil || len(rows) != 2 {
		t.Fatalf("body = %s, want a bare array of two orders", w.Body.String())
	}

	w = f.do("GET", "/orders/sales?state=all&limit=10", "")
	if _, present := w.Header()["X-Next-Cursor"]; present {
		t.Fatalf("X-Next-Cursor present on the last page: %q", w.Header().Get("X-Next-Cursor"))
	}
}

func TestOrgasoftSalesRefusesMissingScope(t *testing.T) {
	f := newOrderFixture(t)
	for prefix, row := range f.repo.keys {
		row.Scopes = []string{erp.ScopeOrdersAck}
		f.repo.keys[prefix] = row
	}
	w := f.do("GET", "/orders/sales?state=all", "")
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 (body %s)", w.Code, w.Body.String())
	}
}

func TestOrgasoftSalesRefusesPharmacyKey(t *testing.T) {
	f := newOrderFixture(t)
	f.repo.orgType = "pharmacy"
	f.repo.sales = []erp.SalesRow{orgasoftSampleSale()}
	w := f.do("GET", "/orders/sales?state=all", "")
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 (body %s)", w.Code, w.Body.String())
	}
}

func TestOrgasoftPurchaseFeedShape(t *testing.T) {
	f := newOrderFixture(t)
	f.repo.purchase = []erp.PurchaseRow{{
		OrderID: 900, OrderNumber: "21846", Status: "confirmed", Total: money.MustParse("265.00"), // header total is before discount
		BuyerName: map[string]string{"ar": "صيدلية الشفاء"},
		Ordering:  erp.ContactInfo{Phone: "01008042755", City: "Fayoum", Address: "الفيوم"},
	}}
	// The service attaches shipments from ListPurchaseShipmentsAsSystem.
	f.repo.purchaseShipments = []erp.PurchaseShipment{{OrderID: 900, Lines: []erp.OrderLine{{
		LineID: 1, ExternalProductCode: "100001", Name: map[string]string{"ar": "سبيركودا 20 قرص"}, Quantity: 1,
		UnitPrice: money.MustParse("209.35"), ListPrice: money.MustParse("265.00"), TotalPrice: money.MustParse("209.35"),
	}}}}
	w := f.do("GET", "/orders/purchase?state=all", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	var rows []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &rows); err != nil || len(rows) != 1 {
		t.Fatalf("body = %s", w.Body.String())
	}
	order := rows[0]
	if order["orderId"] != float64(21846) || order["status"] != "CONFIRMED" || order["pharmacyId"] != float64(7) {
		t.Fatalf("order = %+v", order)
	}
	if order["pharmacyCode"] != nil || order["pharmacyName"] != "صيدلية الشفاء" || order["pharmacyCity"] != "Fayoum" {
		t.Fatalf("purchase entry = %+v, want null pharmacyCode and the buyer's name and city", order)
	}
	item := order["items"].([]any)[0].(map[string]any)
	if item["code"] != float64(100001) || item["qty"] != float64(1) || item["discount"] != float64(21) {
		t.Fatalf("item = %+v", item)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte(`"total":209.35`)) {
		t.Fatalf("total is not the line total after discount: %s", w.Body.String())
	}
	if !bytes.Contains(w.Body.Bytes(), []byte(`"price":265.00`)) || !bytes.Contains(w.Body.Bytes(), []byte(`"discount":21.00`)) {
		t.Fatalf("money not written as exact decimals: %s", w.Body.String())
	}
}

func TestOrgasoftScalar(t *testing.T) {
	cases := map[string]string{
		"263":                 `263`,
		"007":                 `"007"`,
		"ORGA-100":            `"ORGA-100"`,
		"":                    `null`,
		"0":                   `0`,
		"123456789012345678":  `123456789012345678`,
		"1234567890123456789": `"1234567890123456789"`,
	}
	for in, want := range cases {
		if got := string(feedScalar(in)); got != want {
			t.Errorf("feedScalar(%q) = %s, want %s", in, got, want)
		}
	}
}
