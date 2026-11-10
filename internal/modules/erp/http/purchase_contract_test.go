package http

import (
	"net/http"
	"reflect"
	"sort"
	"testing"
)

// pinKeys fails unless the object has exactly the wanted keys (the contract's
// field list, order ignored).
func pinKeys(t *testing.T, label string, got map[string]any, want ...string) {
	t.Helper()
	have := make([]string, 0, len(got))
	for k := range got {
		have = append(have, k)
	}
	sort.Strings(have)
	sort.Strings(want)
	if !reflect.DeepEqual(have, want) {
		t.Fatalf("%s keys = %v, want %v", label, have, want)
	}
}

// TestPurchaseContractKeySets pins the exact keys of the S7 replies against the
// contract (openapi.yaml: OfferList and OrgasoftOrder).
func TestPurchaseContractKeySets(t *testing.T) {
	f := newPurchaseFixture(t)

	offersEnv := decodeEnvelope(t, f.do("GET", "/offers?barcode=6221024000123", ""))
	pinKeys(t, "offers envelope", offersEnv, "status", "requestId", "data")
	offer := offersEnv["data"].([]any)[0].(map[string]any)
	pinKeys(t, "offer", offer, "variantId", "supplier", "name", "barcode", "publicPrice", "discountPercent", "unitPrice", "inStock")
	pinKeys(t, "offer.supplier", offer["supplier"].(map[string]any), "name", "linked")

	w := f.do("POST", "/orders/purchase", purchaseBodyOK)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	// The created order is the partner's order shape: a bare object with
	// exactly these ten fields, and items with exactly five.
	order := decodeEnvelope(t, w)
	pinKeys(t, "purchase order", order, "orderId", "status", "total", "pharmacyId", "pharmacyName",
		"pharmacyCode", "pharmacyNumber", "pharmacyCity", "pharmacyAddress", "items")
	item := order["items"].([]any)[0].(map[string]any)
	pinKeys(t, "item", item, "code", "name", "qty", "price", "discount")
}
